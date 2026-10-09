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
	subjectID, err := NewMorphID(key)
	if err != nil {
		tx.AddError(err)
		return
	}

	options := loggable.ActivityLogOptions()
	subjectTypeValue := subjectType(loggable, options, tx.Statement.Schema)
	var causerID *MorphID
	var causerType *string
	var batchUUID *string

	if id, modelType, exists, err := causerFromContext(tx.Statement.Context); err != nil {
		tx.AddError(err)
		return
	} else if exists {
		causerID = &id
		causerType = &modelType
	}
	if value, exists := batchFromContext(tx.Statement.Context); exists {
		batchUUID = &value
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
		BatchUUID:   batchUUID,
		Properties:  datatypes.JSON(propsJSON),
	}

	result := tx.Session(&gorm.Session{NewDB: true, SkipHooks: true}).Create(&activity)
	if result.Error != nil {
		tx.AddError(result.Error)
	}
}
