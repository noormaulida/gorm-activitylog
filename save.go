package activitylog

import (
	"encoding/json"
	"reflect"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func saveActivity(tx *gorm.DB, loggable Loggable, event string, logName string, props ActivityProperties) {
	if logName == "" {
		logName = "default"
	}

	_, key, ok := primaryKey(tx, reflect.ValueOf(loggable))
	if !ok {
		return
	}
	subjectID, ok := numericID(key)
	if !ok {
		return
	}

	options := loggable.ActivityLogOptions()
	subjectTypeValue := subjectType(loggable, options, tx.Statement.Schema)
	var causerID *uint64
	var causerType *string

	if id, modelType, exists := causerFromContext(tx.Statement.Context); exists {
		causerID = &id
		causerType = &modelType
	}

	propsJSON, err := json.Marshal(props)
	if err != nil {
		tx.AddError(err)
		return
	}

	activity := Activity{
		LogName:     &logName,
		Description: event,
		Event:       &event,
		SubjectID:   &subjectID,
		SubjectType: &subjectTypeValue,
		CauserID:    causerID,
		CauserType:  causerType,
		Properties:  datatypes.JSON(propsJSON),
	}

	result := tx.Session(&gorm.Session{NewDB: true, SkipHooks: true}).Create(&activity)
	if result.Error != nil {
		tx.AddError(result.Error)
	}
}
