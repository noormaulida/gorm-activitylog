package activitylog

import (
	"errors"
	"reflect"

	"gorm.io/gorm"
)

// ErrMissingOldState indicates that an update could not be audited safely.
var ErrMissingOldState = errors.New("activitylog: old model state was not captured")

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
	return registerCallbacks(
		func() error {
			return db.Callback().Create().
				After("gorm:after_create").
				Before("gorm:commit_or_rollback_transaction").
				Register("activitylog:after_create", afterCreateHook)
		},
		func() error {
			return db.Callback().Update().Before("gorm:update").
				Register("activitylog:before_update", beforeUpdateHook)
		},
		func() error {
			return db.Callback().Update().
				After("gorm:after_update").
				Before("gorm:commit_or_rollback_transaction").
				Register("activitylog:after_update", afterUpdateHook)
		},
		func() error {
			return db.Callback().Delete().Before("gorm:delete").
				Register("activitylog:before_delete", beforeDeleteHook)
		},
		func() error {
			return db.Callback().Delete().
				After("gorm:after_delete").
				Before("gorm:commit_or_rollback_transaction").
				Register("activitylog:after_delete", afterDeleteHook)
		},
	)
}

func registerCallbacks(callbacks ...func() error) error {
	for _, register := range callbacks {
		if err := register(); err != nil {
			return err
		}
	}
	return nil
}

// Register installs the package callbacks on a GORM connection.
func Register(db *gorm.DB) error {
	return db.Use(GORMPlugin{})
}

func statementLoggable(tx *gorm.DB) (Loggable, bool) {
	if tx.Statement == nil || tx.Statement.Schema == nil ||
		tx.Statement.Schema.Table == (Activity{}).TableName() ||
		loggingDisabled(tx.Statement.Context) {
		return nil, false
	}

	for _, candidate := range []any{tx.Statement.Model, tx.Statement.Dest} {
		if loggable, ok := candidate.(Loggable); ok {
			return loggable, true
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
	if !shouldLogEvent(opts, EventCreated) {
		return
	}
	props := ActivityProperties{
		Attributes: extractAttributes(tx, reflect.ValueOf(loggable), opts),
	}

	saveActivity(tx, loggable, EventCreated, opts, props)
}

func beforeUpdateHook(tx *gorm.DB) {
	if tx.Error != nil {
		return
	}

	loggable, ok := statementLoggable(tx)
	if !ok {
		return
	}
	if !shouldLogEvent(loggable.ActivityLogOptions(), EventUpdated) {
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

	opts := loggable.ActivityLogOptions()
	if !shouldLogEvent(opts, EventUpdated) {
		return
	}
	oldData, exists := tx.InstanceGet(oldDataKey)
	if !exists {
		tx.AddError(ErrMissingOldState)
		return
	}

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

	saveActivity(tx, loggable, EventUpdated, opts, props)
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
	if !shouldLogEvent(opts, EventDeleted) {
		return
	}
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
	if !shouldLogEvent(opts, EventDeleted) {
		return
	}
	saveActivity(tx, loggable, EventDeleted, opts, props)
}
