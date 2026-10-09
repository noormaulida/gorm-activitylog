package activitylog

import (
	"reflect"
	"testing"

	"gorm.io/gorm"
)

type compositeModel struct {
	Left  uint `gorm:"primaryKey"`
	Right uint `gorm:"primaryKey"`
}

func TestDiffingGuardBranches(t *testing.T) {
	if _, ok := indirectValue(reflect.ValueOf((*int)(nil))); ok {
		t.Fatal("nil pointer must not resolve")
	}

	db := openTestDB(t)
	statement := &gorm.Statement{DB: db}
	if err := statement.Parse(&testUser{}); err != nil {
		t.Fatal(err)
	}
	nameField := statement.Schema.LookUpField("Name")
	if shouldLogField(nameField, LogOptions{LogAttributes: []string{"email"}}) {
		t.Fatal("field outside whitelist must not be logged")
	}

	txWithoutSchema := db.Session(&gorm.Session{NewDB: true})
	txWithoutSchema.Statement.Schema = nil
	if attributes := extractAttributes(txWithoutSchema, reflect.ValueOf(testUser{}), LogOptions{}); len(attributes) != 0 {
		t.Fatalf("expected no attributes without schema: %#v", attributes)
	}
	txWithoutSchema.Statement.Schema = statement.Schema
	if attributes := extractAttributes(txWithoutSchema, reflect.Value{}, LogOptions{}); len(attributes) != 0 {
		t.Fatalf("expected no attributes for invalid value: %#v", attributes)
	}

	channel := make(chan int)
	if !valuesEqual(channel, channel) {
		t.Fatal("fallback equality should compare identical channels")
	}
	if valuesEqual(make(chan int), make(chan int)) {
		t.Fatal("different channels must not compare equal")
	}
}

func TestPrimaryKeyAndSubjectTypeGuards(t *testing.T) {
	db := openTestDB(t)
	tx := db.Session(&gorm.Session{NewDB: true})
	tx.Statement.Schema = nil
	if _, _, ok := primaryKey(tx, reflect.ValueOf(testUser{ID: 1})); ok {
		t.Fatal("missing schema must not resolve primary key")
	}

	statement := &gorm.Statement{DB: db}
	if err := statement.Parse(&compositeModel{}); err != nil {
		t.Fatal(err)
	}
	tx.Statement.Schema = statement.Schema
	if _, _, ok := primaryKey(tx, reflect.ValueOf(compositeModel{Left: 1, Right: 2})); ok {
		t.Fatal("composite key must not resolve")
	}

	if err := statement.Parse(&testUser{}); err != nil {
		t.Fatal(err)
	}
	tx.Statement.Schema = statement.Schema
	if _, _, ok := primaryKey(tx, reflect.Value{}); ok {
		t.Fatal("invalid value must not resolve")
	}
	if _, _, ok := primaryKey(tx, reflect.ValueOf(testUser{})); ok {
		t.Fatal("zero key must not resolve")
	}

	if got := subjectType(&testUser{}, LogOptions{}, nil); got != "*activitylog.testUser" {
		t.Fatalf("unexpected fallback subject type: %q", got)
	}
	var nilLoggable Loggable
	if got := subjectType(nilLoggable, LogOptions{}, nil); got != "" {
		t.Fatalf("expected empty nil subject type, got %q", got)
	}
}
