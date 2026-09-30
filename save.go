package activitylog

import (
	"encoding/json"
	"fmt"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func saveActivity(tx *gorm.DB, loggable Loggable, event string, logName string, props ActivityProperties) {
	if logName == "" {
		logName = "default"
	}

	subjectType := fmt.Sprintf("%T", loggable) // e.g., "*models.Article"
	
	var subjectID uint
	pks := extractPrimaryKeys(tx, reflect.ValueOf(loggable))
	if len(pks) > 0 {
		if id, ok := pks[0].(uint); ok {
			subjectID = id
		}
	}

	var causerID *uint
	var causerType *string

	if ctxID := tx.Statement.Context.Value(CauserIDKey); ctxID != nil {
		if id, ok := ctxID.(uint); ok {
			causerID = &id
		}
	}
	
	if ctxType := tx.Statement.Context.Value(CauserTypeKey); ctxType != nil {
		if ct, ok := ctxType.(string); ok {
			causerType = &ct
		}
	}

	propsJSON, err := json.Marshal(props)
	if err != nil {
		propsJSON = []byte("{}")
	}

	activity := Activity{
		LogName:     logName,
		Description: event,
		SubjectID:   subjectID,
		SubjectType: subjectType,
		CauserID:    causerID,
		CauserType:  causerType,
		Properties:  datatypes.JSON(propsJSON),
	}

	tx.Session(&gorm.Session{NewDB: true}).Create(&activity)
}