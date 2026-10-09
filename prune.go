package activitylog

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Prune deletes activities older than the supplied retention duration.
func Prune(db *gorm.DB, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		return 0, errors.New("activitylog: prune duration must be positive")
	}
	return PruneBefore(db, time.Now().Add(-olderThan))
}

// PruneBefore deletes activities created strictly before cutoff.
func PruneBefore(db *gorm.DB, cutoff time.Time) (int64, error) {
	if db == nil {
		return 0, errors.New("activitylog: nil database")
	}
	if cutoff.IsZero() {
		return 0, errors.New("activitylog: prune cutoff cannot be zero")
	}

	result := db.Session(&gorm.Session{SkipHooks: true}).
		Where("created_at < ?", cutoff).
		Delete(&Activity{})
	if result.Error != nil {
		return 0, fmt.Errorf("activitylog: prune activities: %w", result.Error)
	}
	return result.RowsAffected, nil
}
