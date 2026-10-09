package activitylog

import (
	"context"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Activity represents a row in Spatie's activity_log table.
type Activity struct {
	ID          uint64         `gorm:"primaryKey;autoIncrement"`
	LogName     *string        `gorm:"size:255;index"`
	Description string         `gorm:"type:text;not null"`
	SubjectType *string        `gorm:"size:255;index:subject"`
	Event       *string        `gorm:"size:255"`
	SubjectID   *MorphID       `gorm:"index:subject"`
	CauserType  *string        `gorm:"size:255;index:causer"`
	CauserID    *MorphID       `gorm:"index:causer"`
	Properties  datatypes.JSON `gorm:"type:json"`
	BatchUUID   *string        `gorm:"type:char(36);index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName keeps the model compatible with Spatie's default table name.
func (Activity) TableName() string {
	return "activity_log"
}

// ActivityProperties is the shape used by model lifecycle logs.
type ActivityProperties struct {
	Attributes map[string]any `json:"attributes,omitempty"`
	Old        map[string]any `json:"old,omitempty"`
}

// Loggable marks a model that should be audited by the GORM callbacks.
type Loggable interface {
	ActivityLogOptions() LogOptions
}

// LogOptions controls which model changes are recorded.
//
// Attribute names may be Go field names or database column names. SubjectType
// should be set to the Laravel morph class or alias when sharing a database.
type LogOptions struct {
	LogName          string
	SubjectType      string
	LogAttributes    []string
	IgnoreAttributes []string
	LogOnlyDirty     bool
}

type contextKey uint8

const (
	causerContextKey contextKey = iota
	batchContextKey
)

type causerContextValue struct {
	id        any
	modelType string
}

// WithCauser returns a context carrying the actor for automatic activity logs.
func WithCauser(ctx context.Context, id any, modelType string) context.Context {
	return context.WithValue(ctx, causerContextKey, causerContextValue{
		id:        id,
		modelType: modelType,
	})
}

func causerFromContext(ctx context.Context) (MorphID, string, bool, error) {
	if ctx == nil {
		return MorphID{}, "", false, nil
	}

	value, ok := ctx.Value(causerContextKey).(causerContextValue)
	if !ok {
		return MorphID{}, "", false, nil
	}

	id, err := NewMorphID(value.id)
	return id, value.modelType, true, err
}

// WithBatch returns a context that groups automatic activities under one UUID.
func WithBatch(ctx context.Context, batchUUID string) context.Context {
	return context.WithValue(ctx, batchContextKey, batchUUID)
}

func batchFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}

	value, ok := ctx.Value(batchContextKey).(string)
	return value, ok && value != ""
}

// Migrate creates or updates the activity_log table.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Activity{})
}
