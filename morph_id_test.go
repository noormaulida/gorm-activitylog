package activitylog

import (
	"database/sql/driver"
	"errors"
	"math"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

type testValuer struct {
	value driver.Value
	err   error
}

func (value testValuer) Value() (driver.Value, error) {
	return value.value, value.err
}

type testTextMarshaler struct {
	value string
	err   error
}

func (value testTextMarshaler) MarshalText() ([]byte, error) {
	return []byte(value.value), value.err
}

type testStringer string

func (value testStringer) String() string {
	return string(value)
}

type namedString string
type namedUint uint
type namedInt int

type namedDialector struct {
	name string
}

func (dialector namedDialector) Name() string { return dialector.name }
func (namedDialector) Initialize(*gorm.DB) error {
	return nil
}
func (namedDialector) Migrator(*gorm.DB) gorm.Migrator { return nil }
func (namedDialector) DataTypeOf(*schema.Field) string { return "" }
func (namedDialector) DefaultValueOf(*schema.Field) clause.Expression {
	return nil
}
func (namedDialector) BindVarTo(clause.Writer, *gorm.Statement, any) {}
func (namedDialector) QuoteTo(clause.Writer, string)                 {}
func (namedDialector) Explain(query string, _ ...any) string         { return query }

func TestNormalizeMorphIDInputs(t *testing.T) {
	validUUID := "018f8f4e-735b-7c44-89b2-3f2fcf0d97a1"
	validULID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	pointerUUID := namedString(validUUID)
	cases := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{name: "nil", input: nil, wantErr: true},
		{name: "valuer", input: testValuer{value: int64(9)}, want: "9"},
		{name: "valuer error", input: testValuer{err: errors.New("valuer")}, wantErr: true},
		{name: "text", input: testTextMarshaler{value: validULID}, want: validULID},
		{name: "text error", input: testTextMarshaler{err: errors.New("text")}, wantErr: true},
		{name: "stringer", input: testStringer(validUUID), want: validUUID},
		{name: "nil pointer", input: (*namedString)(nil), wantErr: true},
		{name: "pointer", input: &pointerUUID, want: validUUID},
		{name: "numeric string", input: namedString("12"), want: "12"},
		{name: "empty string", input: namedString(""), wantErr: true},
		{name: "zero string", input: namedString("0"), wantErr: true},
		{name: "invalid string", input: namedString("not-an-id"), wantErr: true},
		{name: "uuid", input: namedString(validUUID), want: validUUID},
		{name: "ulid", input: namedString(validULID), want: validULID},
		{name: "uint", input: namedUint(8), want: "8"},
		{name: "zero uint", input: namedUint(0), wantErr: true},
		{name: "int", input: namedInt(7), want: "7"},
		{name: "negative int", input: namedInt(-1), wantErr: true},
		{name: "unsupported", input: struct{}{}, wantErr: true},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			id, err := NewMorphID(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if id.String() != test.want {
				t.Fatalf("got %q, want %q", id.String(), test.want)
			}
		})
	}
}

func TestMorphIDValueAndScan(t *testing.T) {
	stringID := MorphID{raw: "018f8f4e-735b-7c44-89b2-3f2fcf0d97a1"}
	if value, err := stringID.Value(); err != nil || value != stringID.raw {
		t.Fatalf("unexpected string value: %v %v", value, err)
	}
	large := MorphID{raw: uint64(math.MaxInt64) + 1}
	if value, err := large.Value(); err != nil || value != "9223372036854775808" {
		t.Fatalf("unexpected large value: %v %v", value, err)
	}
	if value, err := (MorphID{}).Value(); err != nil || value != nil {
		t.Fatalf("unexpected nil value: %v %v", value, err)
	}
	if _, err := (MorphID{raw: struct{}{}}).Value(); err == nil {
		t.Fatal("expected invalid raw value error")
	}

	var id MorphID
	if err := id.Scan(nil); err != nil || id.raw != nil {
		t.Fatalf("unexpected nil scan: %#v %v", id, err)
	}
	for _, value := range []any{int64(4), uint64(5), []byte("6"), "7"} {
		if err := id.Scan(value); err != nil {
			t.Fatalf("scan %T: %v", value, err)
		}
	}
	for _, value := range []any{int64(0), uint64(0), "invalid", true} {
		if err := id.Scan(value); err == nil {
			t.Fatalf("expected scan error for %#v", value)
		}
	}
	if got := (MorphID{raw: struct{}{}}).String(); got != "" {
		t.Fatalf("unexpected invalid string: %q", got)
	}
}

func TestMorphIDFormatValidation(t *testing.T) {
	if validUUID("short") ||
		validUUID("018f8f4eX735b-7c44-89b2-3f2fcf0d97a1") ||
		validUUID("018f8f4e-735b-7c44-89b2-3f2fcf0d97ag") ||
		validUUID("00000000-0000-0000-0000-000000000000") {
		t.Fatal("invalid UUID accepted")
	}
	if !validUUID(strings.ToUpper("018f8f4e-735b-7c44-89b2-3f2fcf0d97a1")) {
		t.Fatal("uppercase UUID rejected")
	}

	if validULID("short") ||
		validULID("81ARZ3NDEKTSV4RRFFQ69G5FAV") ||
		validULID("01ARZ3NDEKTSV4RRFFQ69G5FAI") ||
		validULID("00000000000000000000000000") {
		t.Fatal("invalid ULID accepted")
	}
	if !validULID("01arz3ndektsv4rrffq69g5fav") {
		t.Fatal("lowercase ULID rejected")
	}
}

func TestMorphIDGORMTypes(t *testing.T) {
	id := MorphID{}
	for name, expected := range map[string]string{
		"mysql":    "BIGINT UNSIGNED",
		"postgres": "BIGINT",
		"sqlite":   "INTEGER",
	} {
		db := &gorm.DB{Config: &gorm.Config{Dialector: namedDialector{name: name}}}
		if got := id.GormDBDataType(db, nil); got != expected {
			t.Fatalf("%s: got %q, want %q", name, got, expected)
		}
	}
}
