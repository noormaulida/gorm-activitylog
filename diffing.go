package activitylog

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func indirectValue(value reflect.Value) (reflect.Value, bool) {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return reflect.Value{}, false
		}
		value = value.Elem()
	}

	return value, value.IsValid()
}

func optionContains(attributes []string, field *schema.Field) bool {
	for _, attribute := range attributes {
		if strings.EqualFold(attribute, field.Name) || attribute == field.DBName {
			return true
		}
	}

	return false
}

// shouldLogField applies the blacklist before the optional whitelist.
func shouldLogField(field *schema.Field, opts LogOptions) bool {
	if optionContains(opts.IgnoreAttributes, field) {
		return false
	}

	if len(opts.LogAttributes) == 0 {
		return true
	}

	return optionContains(opts.LogAttributes, field)
}

// extractAttributes reads normal database fields using their column names.
func extractAttributes(tx *gorm.DB, value reflect.Value, opts LogOptions) map[string]any {
	attributes := make(map[string]any)
	if tx.Statement.Schema == nil {
		return attributes
	}

	value, ok := indirectValue(value)
	if !ok || value.Kind() != reflect.Struct {
		return attributes
	}

	for _, field := range tx.Statement.Schema.Fields {
		if field.DBName == "" || !shouldLogField(field, opts) {
			continue
		}

		fieldValue, _ := field.ValueOf(tx.Statement.Context, value)
		attributes[field.DBName] = fieldValue
	}

	return attributes
}

func valuesEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr == nil && rightErr == nil {
		return bytes.Equal(leftJSON, rightJSON)
	}

	return reflect.DeepEqual(left, right)
}

func dirtyAttributes(oldAttributes, newAttributes map[string]any) (map[string]any, map[string]any) {
	oldDirty := make(map[string]any)
	newDirty := make(map[string]any)

	for name, newValue := range newAttributes {
		oldValue, exists := oldAttributes[name]
		if !exists || !valuesEqual(oldValue, newValue) {
			oldDirty[name] = oldValue
			newDirty[name] = newValue
		}
	}

	return oldDirty, newDirty
}

func primaryKey(tx *gorm.DB, value reflect.Value) (*schema.Field, any, bool) {
	if tx.Statement.Schema == nil || len(tx.Statement.Schema.PrimaryFields) != 1 {
		return nil, nil, false
	}

	value, ok := indirectValue(value)
	if !ok || value.Kind() != reflect.Struct {
		return nil, nil, false
	}

	field := tx.Statement.Schema.PrimaryFields[0]
	key, zero := field.ValueOf(tx.Statement.Context, value)
	if zero {
		return nil, nil, false
	}

	return field, key, true
}

func subjectType(model Loggable, opts LogOptions, parsedSchema *schema.Schema) string {
	if opts.SubjectType != "" {
		return opts.SubjectType
	}

	if parsedSchema != nil {
		return parsedSchema.Table
	}

	modelType := reflect.TypeOf(model)
	if modelType == nil {
		return ""
	}
	return modelType.String()
}
