package activitylog

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestActivityLoggerGuardBranches(t *testing.T) {
	db := openTestDB(t)

	logger := New(db).UseLog("").InBatch("")
	if logger.logName != "default" || logger.batchUUID != nil {
		t.Fatalf("unexpected empty options: %#v", logger)
	}

	invalid := New(db).CausedBy("invalid", "User")
	if invalid.err == nil || invalid.Log("invalid") == nil {
		t.Fatal("expected invalid causer error")
	}
	if invalid.PerformedOn(&testUser{ID: 1}) != invalid {
		t.Fatal("logger chaining changed instance")
	}

	if err := New(nil).Log("nil database"); err == nil {
		t.Fatal("expected nil database error")
	}
	if New(nil).PerformedOn(&testUser{ID: 1}).subjectID != nil {
		t.Fatal("nil database must not resolve subject")
	}
	if New(db).PerformedOn(nil).subjectID != nil {
		t.Fatal("nil model must not resolve subject")
	}
	if New(db).PerformedOn(&testUser{}).err == nil {
		t.Fatal("expected zero subject ID error")
	}
	if err := New(&gorm.DB{}).
		WithProperties(map[string]any{"invalid": make(chan int)}).
		Log("marshal"); err == nil {
		t.Fatal("expected property marshal error")
	}
}

func TestActivityLoggerUsesContextDefaults(t *testing.T) {
	db := openTestDB(t)
	ctx := WithBatch(
		WithCauser(context.Background(), uint64(91), "Admin"),
		"018f8f4e-735b-7c44-89b2-3f2fcf0d97a1",
	)
	if err := New(db.WithContext(ctx)).Log("context defaults"); err != nil {
		t.Fatal(err)
	}

	var activity Activity
	if err := db.First(&activity).Error; err != nil {
		t.Fatal(err)
	}
	if numericMorphID(t, activity.CauserID) != 91 || activity.CauserType == nil || *activity.CauserType != "Admin" {
		t.Fatalf("unexpected causer: %#v", activity)
	}
	if activity.BatchUUID == nil || *activity.BatchUUID == "" {
		t.Fatalf("unexpected batch: %#v", activity.BatchUUID)
	}
}

func TestActivityLoggerReturnsContextAndDatabaseErrors(t *testing.T) {
	db := openTestDB(t)
	invalidContext := WithCauser(context.Background(), "invalid", "User")
	if err := New(db.WithContext(invalidContext)).Log("invalid context"); err == nil {
		t.Fatal("expected context causer error")
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := New(db).Log("closed"); err == nil {
		t.Fatal("expected closed database error")
	}

	sentinel := errors.New("sentinel")
	logger := New(db)
	logger.err = sentinel
	if !errors.Is(logger.Log("existing"), sentinel) {
		t.Fatal("expected existing logger error")
	}
}
