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
	skipUpdateKey = "activitylog:skip_update"
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
	if tx.Error != nil || (tx.Statement != nil && tx.Statement.SkipHooks) {
		return
	}

	loggable, ok := statementLoggable(tx)
	if !ok {
		return
	}
	if !mutationLoggingEnabled(loggable.ActivityLogOptions()) {
		return
	}

	modelValue, ok := indirectValue(reflect.ValueOf(loggable))
	if !ok || modelValue.Kind() != reflect.Struct {
		return
	}
	primaryField, key, ok := primaryKey(tx, modelValue)
	if !ok || !instanceDestAuditable(tx) {
		tx.InstanceSet(skipUpdateKey, true)
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
	if tx.Error != nil || tx.RowsAffected == 0 || (tx.Statement != nil && tx.Statement.SkipHooks) {
		return
	}

	loggable, ok := statementLoggable(tx)
	if !ok {
		return
	}

	if _, skip := tx.InstanceGet(skipUpdateKey); skip {
		return
	}

	opts := loggable.ActivityLogOptions()
	if !mutationLoggingEnabled(opts) {
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
	event := changeEvent(tx, oldValue, reflect.ValueOf(newData))
	if !shouldLogEvent(opts, event) {
		return
	}
	props, ok := loggedChange(opts, event, oldAttrs, newAttrs, deletedAtColumn(tx))
	if !ok {
		return
	}

	saveActivity(tx, loggable, event, opts, props)
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
	if !softDelete(tx) {
		saveActivity(tx, loggable, EventDeleted, opts, props)
		return
	}

	modelValue, ok := indirectValue(reflect.ValueOf(loggable))
	if !ok || modelValue.Kind() != reflect.Struct {
		return
	}
	fresh, err := reloadModel(tx, modelValue)
	if err != nil {
		tx.AddError(err)
		return
	}
	newAttrs := extractAttributes(tx, fresh, opts)
	changed, _ := loggedChange(opts, EventDeleted, props.Old, newAttrs, deletedAtColumn(tx))
	saveActivity(tx, loggable, EventDeleted, opts, changed)
}

func mutationLoggingEnabled(opts LogOptions) bool {
	return shouldLogEvent(opts, EventUpdated) ||
		shouldLogEvent(opts, EventDeleted) ||
		shouldLogEvent(opts, EventRestored)
}

func instanceDestAuditable(tx *gorm.DB) bool {
	dest, ok := indirectValue(reflect.ValueOf(tx.Statement.Dest))
	if !ok {
		return false
	}
	if dest.Kind() == reflect.Map {
		return dest.Len() == 1
	}
	return dest.Kind() == reflect.Struct
}

func hasDeletedAt(tx *gorm.DB) bool {
	return tx.Statement != nil && tx.Statement.Schema != nil &&
		tx.Statement.Schema.LookUpField("DeletedAt") != nil
}

func deletedAtColumn(tx *gorm.DB) string {
	if !hasDeletedAt(tx) {
		return ""
	}
	return tx.Statement.Schema.LookUpField("DeletedAt").DBName
}

func deletedAtSet(tx *gorm.DB, value reflect.Value) bool {
	if !hasDeletedAt(tx) {
		return false
	}
	value, ok := indirectValue(value)
	if !ok || value.Kind() != reflect.Struct {
		return false
	}
	_, zero := tx.Statement.Schema.LookUpField("DeletedAt").ValueOf(tx.Statement.Context, value)
	return !zero
}

func softDelete(tx *gorm.DB) bool {
	return hasDeletedAt(tx) && !tx.Statement.Unscoped
}

func changeEvent(tx *gorm.DB, oldValue, newValue reflect.Value) string {
	if !hasDeletedAt(tx) {
		return EventUpdated
	}
	oldSet := deletedAtSet(tx, oldValue)
	newSet := deletedAtSet(tx, newValue)
	switch {
	case !oldSet && newSet:
		return EventDeleted
	case oldSet && !newSet:
		return EventRestored
	default:
		return EventUpdated
	}
}

func reloadModel(tx *gorm.DB, modelValue reflect.Value) (reflect.Value, error) {
	primaryField, key, ok := primaryKey(tx, modelValue)
	if !ok {
		return reflect.Value{}, ErrMissingOldState
	}
	fresh := reflect.New(modelValue.Type()).Interface()
	err := tx.Session(&gorm.Session{NewDB: true, SkipHooks: true}).
		Unscoped().
		Where(primaryField.DBName+" = ?", key).
		Take(fresh).Error
	if err != nil {
		return reflect.Value{}, err
	}
	return reflect.ValueOf(fresh), nil
}

func loggedChange(opts LogOptions, event string, oldAttrs, newAttrs map[string]any, deletedAtColumn string) (ActivityProperties, bool) {
	if opts.LogOnlyDirty {
		fullOld, fullNew := oldAttrs, newAttrs
		oldAttrs, newAttrs = dirtyAttributes(oldAttrs, newAttrs)
		if deletedAtColumn != "" && (event == EventDeleted || event == EventRestored) {
			if _, ok := fullNew[deletedAtColumn]; ok {
				oldAttrs[deletedAtColumn] = fullOld[deletedAtColumn]
				newAttrs[deletedAtColumn] = fullNew[deletedAtColumn]
			}
		}
		if len(newAttrs) == 0 {
			if event == EventUpdated {
				return ActivityProperties{}, false
			}
			oldAttrs, newAttrs = fullOld, fullNew
		}
	}

	return ActivityProperties{Attributes: newAttrs, Old: oldAttrs}, true
}
