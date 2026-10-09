package activitylog

import (
	"testing"
	"time"
)

func TestPruneBeforeValidationAndDatabaseError(t *testing.T) {
	if _, err := PruneBefore(nil, time.Now()); err == nil {
		t.Fatal("expected nil database error")
	}

	db := openTestDB(t)
	if _, err := PruneBefore(db, time.Time{}); err == nil {
		t.Fatal("expected zero cutoff error")
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := PruneBefore(db, time.Now()); err == nil {
		t.Fatal("expected prune database error")
	}
}
