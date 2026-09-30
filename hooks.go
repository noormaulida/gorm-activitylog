package activitylog

import (
	"encoding/json"
	"reflect"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func afterCreateHook(tx *gorm.DB) {
	loggable, ok := tx.Statement.Model.(Loggable)
	if !ok {
		return
	}

	opts := loggable.ActivityLogOptions()
	rv := reflect.ValueOf(tx.Statement.Model)

	props := ActivityProperties{
		Attributes: extractAttributes(tx, rv, opts),
	}

	saveActivity(tx, loggable, "created", opts.LogName, props)
}

func beforeUpdateHook(tx *gorm.DB) {
	_, ok := tx.Statement.Model.(Loggable)
	if !ok {
		return
	}

	// 1. Get the struct type of the model being updated
	modelType := reflect.TypeOf(tx.Statement.Model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	// 2. Create a new instance of the model to hold the old data
	oldDataInstance := reflect.New(modelType).Interface()

	// 3. Extract primary keys to query the existing database record
	rv := reflect.ValueOf(tx.Statement.Model)
	pks := extractPrimaryKeys(tx, rv)

	if len(pks) == 0 {
		return // Cannot fetch old data without a primary key
	}

	// 4. Query the old data using a new session to avoid triggering hooks again
	err := tx.Session(&gorm.Session{NewDB: true}).
		First(oldDataInstance, pks...).Error

	if err == nil {
		// 5. Store the fetched old data in the GORM Context Instance
		tx.InstanceSet(oldDataKey, oldDataInstance)
	}
}

func afterUpdateHook(tx *gorm.DB) {
	loggable, ok := tx.Statement.Model.(Loggable)
	if !ok {
		return
	}

	opts := loggable.ActivityLogOptions()
	rvNew := reflect.ValueOf(tx.Statement.Model)
	
	newAttrs := extractAttributes(tx, rvNew, opts)
	oldAttrs := make(map[string]interface{})

	if oldDataInstance, exists := tx.InstanceGet(oldDataKey); exists {
		rvOld := reflect.ValueOf(oldDataInstance)
		oldAttrs = extractAttributes(tx, rvOld, opts)
	}

	if opts.LogOnlyDirty {
		dirtyNewAttrs := make(map[string]interface{})
		dirtyOldAttrs := make(map[string]interface{})
		isDirty := false

		for key, newVal := range newAttrs {
			oldVal := oldAttrs[key]
			
			if !reflect.DeepEqual(newVal, oldVal) {
				dirtyNewAttrs[key] = newVal
				dirtyOldAttrs[key] = oldVal
				isDirty = true
			}
		}

		if !isDirty {
			return
		}

		newAttrs = dirtyNewAttrs
		oldAttrs = dirtyOldAttrs
	}

	props := ActivityProperties{
		Attributes: newAttrs,
		Old:        oldAttrs,
	}

	saveActivity(tx, loggable, "updated", opts.LogName, props)
}

func afterDeleteHook(tx *gorm.DB) {
	loggable, ok := tx.Statement.Model.(Loggable)
	if !ok {
		return
	}

	opts := loggable.ActivityLogOptions()
	rv := reflect.ValueOf(tx.Statement.Model)

	props := ActivityProperties{
		Old: extractAttributes(tx, rv, opts),
	}

	saveActivity(tx, loggable, "deleted", opts.LogName, props)
}
