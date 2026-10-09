package activitylog

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ActivityQuery provides fluent filters for reading the activity log.
type ActivityQuery struct {
	db  *gorm.DB
	err error
}

// Query starts an activity query while preserving the DB transaction and
// context supplied by the caller.
func Query(db *gorm.DB) *ActivityQuery {
	if db == nil {
		return &ActivityQuery{err: errors.New("activitylog: nil database")}
	}

	return &ActivityQuery{
		db: db.Session(&gorm.Session{NewDB: true}).Model(&Activity{}),
	}
}

// ForSubject filters activities for a persisted GORM model.
func (query *ActivityQuery) ForSubject(model any) *ActivityQuery {
	if query.err != nil {
		return query
	}

	id, modelType, err := resolveSubject(query.db, model)
	if err != nil {
		query.err = err
		return query
	}
	return query.ForSubjectID(id, modelType)
}

// ForSubjectID filters activities by a polymorphic subject ID and type.
func (query *ActivityQuery) ForSubjectID(id any, modelType string) *ActivityQuery {
	if query.err != nil {
		return query
	}
	identifier, err := NewMorphID(id)
	if err != nil {
		query.err = err
		return query
	}
	if modelType == "" {
		query.err = errors.New("activitylog: subject type cannot be empty")
		return query
	}

	query.db = query.db.Where("subject_id = ? AND subject_type = ?", identifier, modelType)
	return query
}

// ForSubjectType filters all activities for one polymorphic subject type.
func (query *ActivityQuery) ForSubjectType(modelType string) *ActivityQuery {
	if query.err == nil {
		query.db = query.db.Where("subject_type = ?", modelType)
	}
	return query
}

// CausedBy filters activities by a polymorphic causer ID and type.
func (query *ActivityQuery) CausedBy(id any, modelType string) *ActivityQuery {
	if query.err != nil {
		return query
	}
	identifier, err := NewMorphID(id)
	if err != nil {
		query.err = err
		return query
	}
	if modelType == "" {
		query.err = errors.New("activitylog: causer type cannot be empty")
		return query
	}

	query.db = query.db.Where("causer_id = ? AND causer_type = ?", identifier, modelType)
	return query
}

// InLog filters activities by log name.
func (query *ActivityQuery) InLog(name string) *ActivityQuery {
	if query.err == nil {
		query.db = query.db.Where("log_name = ?", name)
	}
	return query
}

// InBatch filters activities by batch UUID.
func (query *ActivityQuery) InBatch(batchUUID string) *ActivityQuery {
	if query.err == nil {
		query.db = query.db.Where("batch_uuid = ?", batchUUID)
	}
	return query
}

// ForEvent filters activities by lifecycle or custom event.
func (query *ActivityQuery) ForEvent(event string) *ActivityQuery {
	if query.err == nil {
		query.db = query.db.Where("event = ?", event)
	}
	return query
}

// Latest orders activities from newest to oldest.
func (query *ActivityQuery) Latest() *ActivityQuery {
	if query.err == nil {
		query.db = query.db.Order("activity_log.id DESC")
	}
	return query
}

// Oldest orders activities from oldest to newest.
func (query *ActivityQuery) Oldest() *ActivityQuery {
	if query.err == nil {
		query.db = query.db.Order("activity_log.id ASC")
	}
	return query
}

// Limit limits the number of returned activities.
func (query *ActivityQuery) Limit(limit int) *ActivityQuery {
	if query.err == nil {
		query.db = query.db.Limit(limit)
	}
	return query
}

// Offset skips a number of matching activities.
func (query *ActivityQuery) Offset(offset int) *ActivityQuery {
	if query.err == nil {
		query.db = query.db.Offset(offset)
	}
	return query
}

// Find executes the query and returns all matching activities.
func (query *ActivityQuery) Find() ([]Activity, error) {
	if query.err != nil {
		return nil, query.err
	}

	var activities []Activity
	if err := query.db.Find(&activities).Error; err != nil {
		return nil, err
	}
	return activities, nil
}

// First executes the query and returns the first matching activity.
func (query *ActivityQuery) First() (*Activity, error) {
	if query.err != nil {
		return nil, query.err
	}

	var activity Activity
	result := query.db.Take(&activity)
	if result.Error != nil {
		return nil, result.Error
	}
	return &activity, nil
}

// Count returns the number of matching activities.
func (query *ActivityQuery) Count() (int64, error) {
	if query.err != nil {
		return 0, query.err
	}

	var count int64
	if err := query.db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("activitylog: count activities: %w", err)
	}
	return count, nil
}
