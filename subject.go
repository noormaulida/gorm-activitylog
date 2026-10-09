package activitylog

import (
	"context"
	"errors"
	"reflect"

	"gorm.io/gorm"
)

func resolveSubject(db *gorm.DB, model any) (MorphID, string, error) {
	if db == nil {
		return MorphID{}, "", errors.New("activitylog: nil database")
	}
	if model == nil {
		return MorphID{}, "", errors.New("activitylog: nil subject")
	}

	statement := &gorm.Statement{DB: db}
	if err := statement.Parse(model); err != nil {
		return MorphID{}, "", err
	}
	if len(statement.Schema.PrimaryFields) != 1 {
		return MorphID{}, "", errors.New("activitylog: subject requires one primary key")
	}

	value, ok := indirectValue(reflect.ValueOf(model))
	if !ok || value.Kind() != reflect.Struct {
		return MorphID{}, "", errors.New("activitylog: subject requires a model struct")
	}

	ctx := context.Background()
	if db.Statement != nil && db.Statement.Context != nil {
		ctx = db.Statement.Context
	}
	key, zero := statement.Schema.PrimaryFields[0].ValueOf(ctx, value)
	if zero {
		return MorphID{}, "", errors.New("activitylog: subject requires a non-zero primary key")
	}
	id, err := NewMorphID(key)
	if err != nil {
		return MorphID{}, "", err
	}

	modelType := statement.Schema.Table
	if loggable, ok := model.(Loggable); ok {
		modelType = subjectType(loggable, loggable.ActivityLogOptions(), statement.Schema)
	}

	return id, modelType, nil
}
