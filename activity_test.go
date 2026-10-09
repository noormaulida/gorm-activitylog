package activitylog

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestActivityContextAndMigrationGuards(t *testing.T) {
	if _, _, ok, err := causerFromContext(nil); ok || err != nil {
		t.Fatalf("unexpected nil causer result: ok=%v err=%v", ok, err)
	}
	wrongCauser := context.WithValue(context.Background(), causerContextKey, "wrong")
	if _, _, ok, err := causerFromContext(wrongCauser); ok || err != nil {
		t.Fatalf("unexpected wrong causer result: ok=%v err=%v", ok, err)
	}
	if batch, ok := batchFromContext(nil); ok || batch != "" {
		t.Fatalf("unexpected nil batch result: %q %v", batch, ok)
	}
	wrongBatch := context.WithValue(context.Background(), batchContextKey, 123)
	if batch, ok := batchFromContext(wrongBatch); ok || batch != "" {
		t.Fatalf("unexpected wrong batch result: %q %v", batch, ok)
	}

	db, err := gorm.Open(sqlite.Open("file:activity_migrate?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable(&Activity{}) {
		t.Fatal("activity table was not migrated")
	}
}
