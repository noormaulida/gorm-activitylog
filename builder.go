package activitylog

import (
	"encoding/json"
	"errors"
	"reflect"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ActivityLogger provides a fluent interface for manual logging.
type ActivityLogger struct {
	db          *gorm.DB
	logName     string
	event       *string
	causerID    *MorphID
	causerType  *string
	subjectID   *MorphID
	subjectType *string
	batchUUID   *string
	properties  map[string]any
	err         error
}

// New creates a new instance of ActivityLogger.
func New(db *gorm.DB) *ActivityLogger {
	return &ActivityLogger{
		db:      db,
		logName: "default",
	}
}

// UseLog sets a custom log name.
func (l *ActivityLogger) UseLog(name string) *ActivityLogger {
	if name == "" {
		l.logName = "default"
	} else {
		l.logName = name
	}
	return l
}

// Event sets the optional event column for a manual activity.
func (l *ActivityLogger) Event(event string) *ActivityLogger {
	l.event = &event
	return l
}

// CausedBy manually sets the user who triggered the event.
func (l *ActivityLogger) CausedBy(id any, causerType string) *ActivityLogger {
	identifier, err := NewMorphID(id)
	if err != nil {
		l.err = err
		return l
	}
	l.causerID = &identifier
	l.causerType = &causerType
	return l
}

// InBatch groups this activity under the supplied batch UUID.
func (l *ActivityLogger) InBatch(batchUUID string) *ActivityLogger {
	if batchUUID == "" {
		l.batchUUID = nil
	} else {
		l.batchUUID = &batchUUID
	}
	return l
}

// WithProperties attaches arbitrary JSON properties to the activity.
func (l *ActivityLogger) WithProperties(props map[string]any) *ActivityLogger {
	l.properties = props
	return l
}

// PerformedOn sets the target model of the activity.
func (l *ActivityLogger) PerformedOn(model any) *ActivityLogger {
	if model == nil || l.db == nil || l.err != nil {
		return l
	}

	stmt := &gorm.Statement{DB: l.db}
	if err := stmt.Parse(model); err != nil {
		l.err = err
		return l
	}
	if len(stmt.Schema.PrimaryFields) != 1 {
		l.err = errors.New("activitylog: PerformedOn requires one primary key")
		return l
	}

	value, ok := indirectValue(reflect.ValueOf(model))
	if !ok || value.Kind() != reflect.Struct {
		l.err = errors.New("activitylog: PerformedOn requires a model struct")
		return l
	}

	key, zero := stmt.Schema.PrimaryFields[0].ValueOf(l.db.Statement.Context, value)
	if zero {
		l.err = errors.New("activitylog: PerformedOn requires a non-zero primary key")
		return l
	}
	id, err := NewMorphID(key)
	if err != nil {
		l.err = err
		return l
	}

	modelType := stmt.Schema.Table
	if loggable, ok := model.(Loggable); ok {
		modelType = subjectType(loggable, loggable.ActivityLogOptions(), stmt.Schema)
	}

	l.subjectID = &id
	l.subjectType = &modelType
	return l
}

// Log executes the insert query into the activity_log table.
func (l *ActivityLogger) Log(description string) error {
	if l.err != nil {
		return l.err
	}
	if l.db == nil {
		return errors.New("activitylog: nil database")
	}

	properties := l.properties
	if properties == nil {
		properties = map[string]any{}
	}
	propsJSON, err := json.Marshal(properties)
	if err != nil {
		return err
	}

	activity := Activity{
		LogName:     &l.logName,
		Description: description,
		Event:       l.event,
		Properties:  datatypes.JSON(propsJSON),
		CauserID:    l.causerID,
		CauserType:  l.causerType,
		SubjectID:   l.subjectID,
		SubjectType: l.subjectType,
		BatchUUID:   l.batchUUID,
	}

	if activity.CauserID == nil {
		if id, modelType, ok, err := causerFromContext(l.db.Statement.Context); err != nil {
			return err
		} else if ok {
			activity.CauserID = &id
			activity.CauserType = &modelType
		}
	}
	if activity.BatchUUID == nil {
		if batchUUID, ok := batchFromContext(l.db.Statement.Context); ok {
			activity.BatchUUID = &batchUUID
		}
	}

	return l.db.Session(&gorm.Session{SkipHooks: true}).Create(&activity).Error
}
