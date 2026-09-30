package activitylog

import (
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// extractPrimaryKeys safely retrieves the primary key values from the current model.
func extractPrimaryKeys(tx *gorm.DB, rv reflect.Value) []interface{} {
	var pks []interface{}
	for _, field := range tx.Statement.Schema.PrimaryFields {
		val, _ := field.ValueOf(tx.Statement.Context, rv)
		pks = append(pks, val)
	}
	return pks
}

// shouldLogField determines if a field should be tracked based on LogOptions (Whitelist/Blacklist).
func shouldLogField(fieldName string, opts LogOptions) bool {
	for _, ignored := range opts.IgnoreAttributes {
		if fieldName == ignored {
			return false
		}
	}

	if len(opts.LogAttributes) == 0 {
		return true
	}

	for _, allowed := range opts.LogAttributes {
		if fieldName == allowed {
			return true
		}
	}

	return false
}

// extractAttributes pulls the current struct values into a map, respecting LogOptions.
func extractAttributes(tx *gorm.DB, rv reflect.Value, opts LogOptions) map[string]interface{} {
	attrs := make(map[string]interface{})

	for _, field := range tx.Statement.Schema.Fields {
		if !field.IsNormal {
			continue
		}

		if shouldLogField(field.Name, opts) {
			val, _ := field.ValueOf(tx.Statement.Context, rv)
			attrs[field.DBName] = val
		}
	}

	return attrs
}