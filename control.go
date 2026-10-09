package activitylog

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// WithoutLogging returns a context that suppresses automatic and manual logs.
func WithoutLogging(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggingDisabledContextKey, true)
}

func loggingDisabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	disabled, _ := ctx.Value(loggingDisabledContextKey).(bool)
	return disabled
}

// RunWithoutLogging executes callback with logging disabled only for its DB
// session. Other sessions and concurrent requests are unaffected.
func RunWithoutLogging(db *gorm.DB, callback func(tx *gorm.DB) error) error {
	if db == nil {
		return errors.New("activitylog: nil database")
	}
	if callback == nil {
		return errors.New("activitylog: nil callback")
	}

	ctx := context.Background()
	if db.Statement != nil && db.Statement.Context != nil {
		ctx = db.Statement.Context
	}

	return callback(db.WithContext(WithoutLogging(ctx)))
}
