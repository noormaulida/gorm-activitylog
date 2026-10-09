package activitylog

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testUser struct {
	ID        uint64
	Name      string
	Email     string
	Password  string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

func (*testUser) ActivityLogOptions() LogOptions {
	return LogOptions{
		LogName:          "users",
		SubjectType:      "App\\Models\\User",
		IgnoreAttributes: []string{"password", "updated_at", "deleted_at"},
		LogOnlyDirty:     true,
	}
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_busy_timeout=5000", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&Activity{}, &testUser{}); err != nil {
		t.Fatal(err)
	}
	if err := Register(db); err != nil {
		t.Fatal(err)
	}

	return db
}

func activities(t *testing.T, db *gorm.DB) []Activity {
	t.Helper()

	var result []Activity
	if err := db.Order("id").Find(&result).Error; err != nil {
		t.Fatal(err)
	}
	return result
}

func properties(t *testing.T, activity Activity) map[string]any {
	t.Helper()

	var result map[string]any
	if err := json.Unmarshal(activity.Properties, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCreateLogsFilteredAttributesAndCauser(t *testing.T) {
	db := openTestDB(t)
	ctx := WithCauser(context.Background(), 42, "App\\Models\\Admin")
	user := testUser{Name: "Noor", Email: "noor@example.com", Password: "secret"}

	if err := db.WithContext(ctx).Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	logs := activities(t, db)
	if len(logs) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(logs))
	}
	log := logs[0]
	if log.Event == nil || *log.Event != "created" {
		t.Fatalf("unexpected event: %v", log.Event)
	}
	if log.SubjectID == nil || *log.SubjectID != user.ID {
		t.Fatalf("unexpected subject ID: %v", log.SubjectID)
	}
	if log.SubjectType == nil || *log.SubjectType != "App\\Models\\User" {
		t.Fatalf("unexpected subject type: %v", log.SubjectType)
	}
	if log.CauserID == nil || *log.CauserID != 42 {
		t.Fatalf("unexpected causer ID: %v", log.CauserID)
	}

	attributes := properties(t, log)["attributes"].(map[string]any)
	if _, exists := attributes["password"]; exists {
		t.Fatal("password must not be logged")
	}
	if attributes["name"] != "Noor" {
		t.Fatalf("unexpected attributes: %#v", attributes)
	}
}

func TestUpdateLogsOnlyDirtyAttributes(t *testing.T) {
	db := openTestDB(t)
	user := testUser{Name: "Before", Email: "same@example.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	user.Name = "After"
	if err := db.Save(&user).Error; err != nil {
		t.Fatal(err)
	}

	logs := activities(t, db)
	if len(logs) != 2 {
		t.Fatalf("expected 2 activities, got %d", len(logs))
	}
	props := properties(t, logs[1])
	oldValues := props["old"].(map[string]any)
	newValues := props["attributes"].(map[string]any)
	if len(oldValues) != 1 || oldValues["name"] != "Before" {
		t.Fatalf("unexpected old values: %#v", oldValues)
	}
	if len(newValues) != 1 || newValues["name"] != "After" {
		t.Fatalf("unexpected new values: %#v", newValues)
	}
}

func TestPartialUpdateReloadsNewState(t *testing.T) {
	db := openTestDB(t)
	user := testUser{Name: "Before", Email: "same@example.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Model(&user).Update("name", "Partial").Error; err != nil {
		t.Fatal(err)
	}

	logs := activities(t, db)
	if len(logs) != 2 {
		t.Fatalf("expected 2 activities, got %d", len(logs))
	}
	props := properties(t, logs[1])
	oldValues := props["old"].(map[string]any)
	newValues := props["attributes"].(map[string]any)
	if oldValues["name"] != "Before" || newValues["name"] != "Partial" {
		t.Fatalf("unexpected partial update values: old=%#v new=%#v", oldValues, newValues)
	}
}

func TestNoOpUpdateDoesNotCreateActivity(t *testing.T) {
	db := openTestDB(t)
	user := testUser{Name: "Noor"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Save(&user).Error; err != nil {
		t.Fatal(err)
	}

	if count := len(activities(t, db)); count != 1 {
		t.Fatalf("expected only the create activity, got %d", count)
	}
}

func TestDeleteLogsPreDeleteValues(t *testing.T) {
	db := openTestDB(t)
	user := testUser{Name: "Deleted"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&user).Error; err != nil {
		t.Fatal(err)
	}

	logs := activities(t, db)
	if len(logs) != 2 {
		t.Fatalf("expected 2 activities, got %d", len(logs))
	}
	if logs[1].Event == nil || *logs[1].Event != "deleted" {
		t.Fatalf("unexpected delete event: %v", logs[1].Event)
	}
	oldValues := properties(t, logs[1])["old"].(map[string]any)
	if oldValues["name"] != "Deleted" {
		t.Fatalf("unexpected old values: %#v", oldValues)
	}
}

func TestManualLoggerStoresArbitraryProperties(t *testing.T) {
	db := openTestDB(t)
	user := testUser{Name: "Target"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	err := New(db).
		UseLog("auth").
		Event("login").
		CausedBy(7, "App\\Models\\User").
		InBatch("018f8f4e-735b-7c44-89b2-3f2fcf0d97a1").
		PerformedOn(&user).
		WithProperties(map[string]any{"ip_address": "127.0.0.1"}).
		Log("User logged in")
	if err != nil {
		t.Fatal(err)
	}

	logs := activities(t, db)
	manual := logs[len(logs)-1]
	props := properties(t, manual)
	if props["ip_address"] != "127.0.0.1" {
		t.Fatalf("unexpected manual properties: %#v", props)
	}
	if _, nested := props["attributes"]; nested {
		t.Fatalf("manual properties must not be nested: %#v", props)
	}
	if manual.BatchUUID == nil || *manual.BatchUUID != "018f8f4e-735b-7c44-89b2-3f2fcf0d97a1" {
		t.Fatalf("unexpected batch UUID: %v", manual.BatchUUID)
	}
}

func TestBatchContextGroupsAutomaticActivities(t *testing.T) {
	db := openTestDB(t)
	const batchUUID = "018f8f4e-735b-7c44-89b2-3f2fcf0d97a1"
	ctx := WithBatch(context.Background(), batchUUID)

	user := testUser{Name: "Before"}
	if err := db.WithContext(ctx).Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	user.Name = "After"
	if err := db.WithContext(ctx).Save(&user).Error; err != nil {
		t.Fatal(err)
	}

	logs := activities(t, db)
	if len(logs) != 2 {
		t.Fatalf("expected 2 activities, got %d", len(logs))
	}
	for _, log := range logs {
		if log.BatchUUID == nil || *log.BatchUUID != batchUUID {
			t.Fatalf("unexpected batch UUID: %v", log.BatchUUID)
		}
	}
}

func TestTransactionRollbackRemovesActivity(t *testing.T) {
	db := openTestDB(t)

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&testUser{Name: "Rolled back"}).Error; err != nil {
			return err
		}
		return fmt.Errorf("force rollback")
	})
	if err == nil {
		t.Fatal("expected transaction error")
	}

	if count := len(activities(t, db)); count != 0 {
		t.Fatalf("expected no activities after rollback, got %d", count)
	}
}

func TestActivityInsertFailureRollsBackModel(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&testUser{}); err != nil {
		t.Fatal(err)
	}
	if err := Register(db); err != nil {
		t.Fatal(err)
	}

	if err := db.Create(&testUser{Name: "Must roll back"}).Error; err == nil {
		t.Fatal("expected create to fail because activity_log does not exist")
	}

	var count int64
	if err := db.Unscoped().Model(&testUser{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected model insert to roll back, found %d rows", count)
	}
}

func TestConcurrentContextsDoNotLeakCausers(t *testing.T) {
	db := openTestDB(t)
	const workers = 8

	var wait sync.WaitGroup
	errors := make(chan error, workers)
	for i := 1; i <= workers; i++ {
		wait.Add(1)
		go func(id uint64) {
			defer wait.Done()
			ctx := WithCauser(context.Background(), id, "User")
			name := fmt.Sprintf("user-%d", id)
			errors <- db.WithContext(ctx).Create(&testUser{Name: name}).Error
		}(uint64(i))
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}

	logs := activities(t, db)
	if len(logs) != workers {
		t.Fatalf("expected %d activities, got %d", workers, len(logs))
	}
	for _, log := range logs {
		attrs := properties(t, log)["attributes"].(map[string]any)
		var expected uint64
		if _, err := fmt.Sscanf(attrs["name"].(string), "user-%d", &expected); err != nil {
			t.Fatal(err)
		}
		if log.CauserID == nil || *log.CauserID != expected {
			t.Fatalf("causer leaked for %q: got %v, want %d", attrs["name"], log.CauserID, expected)
		}
	}
}
