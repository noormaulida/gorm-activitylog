package activitylog

import (
	"reflect"

	"gorm.io/gorm"
)

const (
	oldDataKey    = "activitylog:old_data"
	deleteDataKey = "activitylog:delete_data"
)

// GORMPlugin installs activity logging callbacks.
type GORMPlugin struct{}

// Name implements gorm.Plugin.
func (GORMPlugin) Name() string {
	return "gorm-activitylog"
}

// Initialize implements gorm.Plugin.
func (GORMPlugin) Initialize(db *gorm.DB) error {
	if err := db.Callback().Create().
		After("gorm:after_create").
		Before("gorm:commit_or_rollback_transaction").
		Register("activitylog:after_create", afterCreateHook); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").
		Register("activitylog:before_update", beforeUpdateHook); err != nil {
		return err
	}
	if err := db.Callback().Update().
		After("gorm:after_update").
		Before("gorm:commit_or_rollback_transaction").
		Register("activitylog:after_update", afterUpdateHook); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("gorm:delete").
		Register("activitylog:before_delete", beforeDeleteHook); err != nil {
		return err
	}
	return db.Callback().Delete().
		After("gorm:after_delete").
		Before("gorm:commit_or_rollback_transaction").
		Register("activitylog:after_delete", afterDeleteHook)
}

// Register installs the package callbacks on a GORM connection.
func Register(db *gorm.DB) error {
	return db.Use(GORMPlugin{})
}

func statementLoggable(tx *gorm.DB) (Loggable, bool) {
	if tx.Statement == nil || tx.Statement.Schema == nil ||
		tx.Statement.Schema.Table == (Activity{}).TableName() {
		return nil, false
	}

	for _, candidate := range []any{tx.Statement.Model, tx.Statement.Dest} {
		if loggable, ok := candidate.(Loggable); ok {
			return loggable, true
		}

		value := reflect.ValueOf(candidate)
		value, ok := indirectValue(value)
		if ok && value.Kind() == reflect.Struct && value.CanAddr() {
			if loggable, ok := value.Addr().Interface().(Loggable); ok {
				return loggable, true
			}
		}
	}

	return nil, false
}

func afterCreateHook(tx *gorm.DB) {
	if tx.Error != nil || tx.RowsAffected == 0 {
		return
	}

	loggable, ok := statementLoggable(tx)
	if !ok {
		return
	}

	opts := loggable.ActivityLogOptions()
	props := ActivityProperties{
		Attributes: extractAttributes(tx, reflect.ValueOf(loggable), opts),
	}

	saveActivity(tx, loggable, "created", opts.LogName, props)
}

func beforeUpdateHook(tx *gorm.DB) {
	if tx.Error != nil {
		return
	}

	loggable, ok := statementLoggable(tx)
	if !ok {
		return
	}

	modelValue, ok := indirectValue(reflect.ValueOf(loggable))
	if !ok || modelValue.Kind() != reflect.Struct {
		return
	}

	primaryField, key, ok := primaryKey(tx, modelValue)
	if !ok {
		return
	}

	oldData := reflect.New(modelValue.Type()).Interface()
	err := tx.Session(&gorm.Session{NewDB: true, SkipHooks: true}).
		Unscoped().
		Where(primaryField.DBName+" = ?", key).
		Take(oldData).Error
	if err != nil {
		tx.AddError(err)
		return
	}

	tx.InstanceSet(oldDataKey, oldData)
}

func afterUpdateHook(tx *gorm.DB) {
	if tx.Error != nil || tx.RowsAffected == 0 {
		return
	}

	loggable, ok := statementLoggable(tx)
	if !ok {
		return
	}

	oldData, exists := tx.InstanceGet(oldDataKey)
	if !exists {
		return
	}

	opts := loggable.ActivityLogOptions()
	oldAttrs := extractAttributes(tx, reflect.ValueOf(oldData), opts)
	oldValue, ok := indirectValue(reflect.ValueOf(oldData))
	if !ok || oldValue.Kind() != reflect.Struct {
		return
	}
	primaryField, key, ok := primaryKey(tx, oldValue)
	if !ok {
		return
	}
	newData := reflect.New(oldValue.Type()).Interface()
	err := tx.Session(&gorm.Session{NewDB: true, SkipHooks: true}).
		Unscoped().
		Where(primaryField.DBName+" = ?", key).
		Take(newData).Error
	if err != nil {
		tx.AddError(err)
		return
	}
	newAttrs := extractAttributes(tx, reflect.ValueOf(newData), opts)

	if opts.LogOnlyDirty {
		oldAttrs, newAttrs = dirtyAttributes(oldAttrs, newAttrs)
		if len(newAttrs) == 0 {
			return
		}
	}

	props := ActivityProperties{
		Attributes: newAttrs,
		Old:        oldAttrs,
	}

	saveActivity(tx, loggable, "updated", opts.LogName, props)
}

func beforeDeleteHook(tx *gorm.DB) {
	if tx.Error != nil {
		return
	}

	loggable, ok := statementLoggable(tx)
	if !ok {
		return
	}

	opts := loggable.ActivityLogOptions()
	tx.InstanceSet(deleteDataKey, ActivityProperties{
		Old: extractAttributes(tx, reflect.ValueOf(loggable), opts),
	})
}

func afterDeleteHook(tx *gorm.DB) {
	if tx.Error != nil || tx.RowsAffected == 0 {
		return
	}

	loggable, ok := statementLoggable(tx)
	if !ok {
		return
	}

	value, exists := tx.InstanceGet(deleteDataKey)
	if !exists {
		return
	}

	props, ok := value.(ActivityProperties)
	if !ok {
		return
	}
	opts := loggable.ActivityLogOptions()
	saveActivity(tx, loggable, "deleted", opts.LogName, props)
}
