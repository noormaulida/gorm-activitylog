package activitylog

import (
	"testing"

	"gorm.io/gorm"
)

type invalidLoggableID struct {
	ID string `gorm:"primaryKey"`
}

func (*invalidLoggableID) ActivityLogOptions() LogOptions {
	return LogOptions{}
}

func TestSaveActivityRejectsMissingAndInvalidSubjectIDs(t *testing.T) {
	db := openTestDB(t)

	zero := &testUser{}
	zeroTX := db.Model(zero).Session(&gorm.Session{NewDB: false})
	if err := zeroTX.Statement.Parse(zero); err != nil {
		t.Fatal(err)
	}
	saveActivity(zeroTX, zero, EventCreated, zero.ActivityLogOptions(), ActivityProperties{})
	if zeroTX.Error != nil {
		t.Fatalf("zero ID should skip without error: %v", zeroTX.Error)
	}

	invalid := &invalidLoggableID{ID: "invalid"}
	invalidTX := db.Model(invalid).Session(&gorm.Session{NewDB: false})
	if err := invalidTX.Statement.Parse(invalid); err != nil {
		t.Fatal(err)
	}
	saveActivity(invalidTX, invalid, EventCreated, invalid.ActivityLogOptions(), ActivityProperties{})
	if invalidTX.Error == nil {
		t.Fatal("expected invalid subject ID error")
	}
}
