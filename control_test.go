package activitylog

import (
	"context"
	"testing"

	"gorm.io/gorm"
)

func TestLoggingControlGuards(t *testing.T) {
	if loggingDisabled(nil) {
		t.Fatal("nil context must not disable logging")
	}
	if loggingDisabled(context.Background()) {
		t.Fatal("plain context must not disable logging")
	}
	if err := RunWithoutLogging(nil, func(_ *gorm.DB) error { return nil }); err == nil {
		t.Fatal("expected nil database error")
	}

	db := openTestDB(t)
	if err := RunWithoutLogging(db, nil); err == nil {
		t.Fatal("expected nil callback error")
	}
}
