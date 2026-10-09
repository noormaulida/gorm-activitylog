package activitylog

import (
	"database/sql/driver"
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// MorphID stores either a numeric ID or a UUID/ULID string.
//
// Migrate uses Spatie's numeric morph schema by default. String IDs are
// intended for databases migrated with Laravel's uuidMorphs or ulidMorphs.
type MorphID struct {
	raw any
}

// NewMorphID validates and normalizes a supported polymorphic identifier.
func NewMorphID(value any) (MorphID, error) {
	normalized, err := normalizeMorphID(value)
	if err != nil {
		return MorphID{}, err
	}
	return MorphID{raw: normalized}, nil
}

func normalizeMorphID(value any) (any, error) {
	if value == nil {
		return nil, errors.New("activitylog: morph ID cannot be nil")
	}

	if valuer, ok := value.(driver.Valuer); ok {
		resolved, err := valuer.Value()
		if err != nil {
			return nil, fmt.Errorf("activitylog: resolve morph ID: %w", err)
		}
		return normalizeMorphID(resolved)
	}
	if marshaler, ok := value.(encoding.TextMarshaler); ok {
		text, err := marshaler.MarshalText()
		if err != nil {
			return nil, fmt.Errorf("activitylog: marshal morph ID: %w", err)
		}
		return normalizeMorphID(string(text))
	}
	if stringer, ok := value.(fmt.Stringer); ok {
		return normalizeMorphID(stringer.String())
	}

	reflected := reflect.ValueOf(value)
	for reflected.Kind() == reflect.Pointer {
		if reflected.IsNil() {
			return nil, errors.New("activitylog: morph ID cannot be nil")
		}
		reflected = reflected.Elem()
	}

	switch reflected.Kind() {
	case reflect.String:
		identifier := reflected.String()
		if identifier == "" {
			return nil, errors.New("activitylog: morph ID cannot be empty")
		}
		if numeric, err := strconv.ParseUint(identifier, 10, 64); err == nil {
			if numeric == 0 {
				return nil, errors.New("activitylog: morph ID cannot be zero")
			}
			return numeric, nil
		}
		if validUUID(identifier) || validULID(identifier) {
			return identifier, nil
		}
		return nil, errors.New("activitylog: string morph ID must be numeric, UUID, or ULID")
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		identifier := reflected.Uint()
		if identifier == 0 {
			return nil, errors.New("activitylog: morph ID cannot be zero")
		}
		return identifier, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		identifier := reflected.Int()
		if identifier <= 0 {
			return nil, errors.New("activitylog: morph ID must be positive")
		}
		return uint64(identifier), nil
	default:
		return nil, fmt.Errorf("activitylog: unsupported morph ID type %T", value)
	}
}

// Value implements driver.Valuer.
func (id MorphID) Value() (driver.Value, error) {
	switch value := id.raw.(type) {
	case string:
		return value, nil
	case uint64:
		// database/sql does not accept uint64 values with the high bit set.
		if value > uint64(^uint64(0)>>1) {
			return strconv.FormatUint(value, 10), nil
		}
		return int64(value), nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("activitylog: invalid normalized morph ID type %T", id.raw)
	}
}

// Scan implements sql.Scanner.
func (id *MorphID) Scan(value any) error {
	if value == nil {
		id.raw = nil
		return nil
	}

	switch value := value.(type) {
	case int64:
		if value <= 0 {
			return errors.New("activitylog: scanned morph ID must be positive")
		}
		id.raw = uint64(value)
		return nil
	case uint64:
		if value == 0 {
			return errors.New("activitylog: scanned morph ID cannot be zero")
		}
		id.raw = value
		return nil
	case []byte:
		return id.scanString(string(value))
	case string:
		return id.scanString(value)
	default:
		return fmt.Errorf("activitylog: cannot scan morph ID from %T", value)
	}
}

func (id *MorphID) scanString(value string) error {
	normalized, err := normalizeMorphID(value)
	if err != nil {
		return fmt.Errorf("activitylog: scan morph ID: %w", err)
	}
	id.raw = normalized
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}

	nonZero := false
	for index, character := range value {
		switch index {
		case 8, 13, 18, 23:
			if character != '-' {
				return false
			}
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
			return false
		}
		if character != '0' {
			nonZero = true
		}
	}
	return nonZero
}

func validULID(value string) bool {
	if len(value) != 26 {
		return false
	}

	value = strings.ToUpper(value)
	nonZero := false
	for index, character := range value {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", character) {
			return false
		}
		// The first ULID character is limited to 0-7 to fit 128 bits.
		if index == 0 && character > '7' {
			return false
		}
		if character != '0' {
			nonZero = true
		}
	}
	return nonZero
}

// String returns the identifier's database representation.
func (id MorphID) String() string {
	switch value := id.raw.(type) {
	case string:
		return value
	case uint64:
		return strconv.FormatUint(value, 10)
	default:
		return ""
	}
}

// Uint64 returns a numeric ID and whether this identifier is numeric.
func (id MorphID) Uint64() (uint64, bool) {
	value, ok := id.raw.(uint64)
	return value, ok
}

// GormDataType keeps AutoMigrate on Spatie's default numeric morph type.
func (MorphID) GormDataType() string {
	return "morph_id"
}

// GormDBDataType provides the numeric default used by Migrate.
func (MorphID) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Dialector.Name() {
	case "mysql":
		return "BIGINT UNSIGNED"
	case "postgres":
		return "BIGINT"
	default:
		return "INTEGER"
	}
}
