package activitylog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type postgresArticle struct {
	ID        uint64 `gorm:"primaryKey"`
	Title     string
	Secret    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (postgresArticle) TableName() string {
	return "integration_articles"
}

func (*postgresArticle) ActivityLogOptions() LogOptions {
	return LogOptions{
		LogName:          "articles",
		SubjectType:      "articles",
		IgnoreAttributes: []string{"secret", "updated_at"},
		LogOnlyDirty:     true,
	}
}

type postgresUUIDDocument struct {
	ID    string `gorm:"primaryKey;type:uuid"`
	Title string
}

func (postgresUUIDDocument) TableName() string {
	return "integration_documents"
}

func (*postgresUUIDDocument) ActivityLogOptions() LogOptions {
	return LogOptions{SubjectType: "documents"}
}

func TestSpatiePostgreSQLSchemaCompatibility(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN is not configured")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"activity_log", "integration_articles", "integration_documents"} {
		if err := db.Exec("DROP TABLE IF EXISTS " + table + " CASCADE").Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec("DROP TABLE IF EXISTS activity_log CASCADE")
		db.Exec("DROP TABLE IF EXISTS integration_articles CASCADE")
		db.Exec("DROP TABLE IF EXISTS integration_documents CASCADE")
	})

	// Mirrors Spatie's default migration as generated for PostgreSQL.
	const createNumericActivityLog = `
CREATE TABLE activity_log (
    id BIGSERIAL PRIMARY KEY,
    log_name VARCHAR(255) NULL,
    description TEXT NOT NULL,
    subject_type VARCHAR(255) NULL,
    event VARCHAR(255) NULL,
    subject_id BIGINT NULL,
    causer_type VARCHAR(255) NULL,
    causer_id BIGINT NULL,
    properties JSON NULL,
    batch_uuid UUID NULL,
    created_at TIMESTAMP(0) WITHOUT TIME ZONE NULL,
    updated_at TIMESTAMP(0) WITHOUT TIME ZONE NULL
)`
	if err := db.Exec(createNumericActivityLog).Error; err != nil {
		t.Fatal(err)
	}
	for _, index := range []string{
		"CREATE INDEX activity_log_subject_index ON activity_log (subject_type, subject_id)",
		"CREATE INDEX activity_log_causer_index ON activity_log (causer_type, causer_id)",
		"CREATE INDEX activity_log_log_name_index ON activity_log (log_name)",
		"CREATE INDEX activity_log_batch_uuid_index ON activity_log (batch_uuid)",
	} {
		if err := db.Exec(index).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AutoMigrate(&postgresArticle{}); err != nil {
		t.Fatal(err)
	}
	if err := Register(db); err != nil {
		t.Fatal(err)
	}

	article := postgresArticle{Title: "Before", Secret: "hidden"}
	if err := db.Create(&article).Error; err != nil {
		t.Fatal(err)
	}
	article.Title = "After"
	if err := db.Save(&article).Error; err != nil {
		t.Fatal(err)
	}

	var logs []Activity
	if err := db.Order("id").Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected create and update activities, got %d", len(logs))
	}
	var update ActivityProperties
	if err := json.Unmarshal(logs[1].Properties, &update); err != nil {
		t.Fatal(err)
	}
	if len(update.Old) != 1 || update.Old["title"] != "Before" {
		t.Fatalf("unexpected old properties: %#v", update.Old)
	}
	if len(update.Attributes) != 1 || update.Attributes["title"] != "After" {
		t.Fatalf("unexpected new properties: %#v", update.Attributes)
	}
	count, err := Query(db).ForSubject(&article).Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 queried activities, got %d", count)
	}

	beforeRollback := len(logs)
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&postgresArticle{Title: "Rolled back"}).Error; err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected forced rollback")
	}
	var afterRollback int64
	if err := db.Model(&Activity{}).Count(&afterRollback).Error; err != nil {
		t.Fatal(err)
	}
	if afterRollback != int64(beforeRollback) {
		t.Fatalf("rollback left an activity: got %d, want %d", afterRollback, beforeRollback)
	}

	if err := db.Model(&Activity{}).
		Where("id = ?", logs[0].ID).
		UpdateColumn("created_at", time.Now().Add(-48*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	pruned, err := Prune(db, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 1 {
		t.Fatalf("expected 1 pruned PostgreSQL activity, got %d", pruned)
	}

	if err := db.Exec("DROP TABLE activity_log CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	const createUUIDActivityLog = `
CREATE TABLE activity_log (
    id BIGSERIAL PRIMARY KEY,
    log_name VARCHAR(255) NULL,
    description TEXT NOT NULL,
    subject_type VARCHAR(255) NULL,
    event VARCHAR(255) NULL,
    subject_id UUID NULL,
    causer_type VARCHAR(255) NULL,
    causer_id UUID NULL,
    properties JSON NULL,
    batch_uuid UUID NULL,
    created_at TIMESTAMP(0) WITHOUT TIME ZONE NULL,
    updated_at TIMESTAMP(0) WITHOUT TIME ZONE NULL
)`
	if err := db.Exec(createUUIDActivityLog).Error; err != nil {
		t.Fatal(err)
	}
	for _, index := range []string{
		"CREATE INDEX activity_log_subject_index ON activity_log (subject_type, subject_id)",
		"CREATE INDEX activity_log_causer_index ON activity_log (causer_type, causer_id)",
	} {
		if err := db.Exec(index).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AutoMigrate(&postgresUUIDDocument{}); err != nil {
		t.Fatal(err)
	}

	const documentID = "018f8f4e-735b-7c44-89b2-3f2fcf0d97a1"
	const userID = "018f8f51-a3c1-7118-a408-3763ebd7167c"
	document := postgresUUIDDocument{ID: documentID, Title: "UUID"}
	ctx := WithCauser(context.Background(), userID, "users")
	if err := db.WithContext(ctx).Create(&document).Error; err != nil {
		t.Fatal(err)
	}

	var uuidLog Activity
	if err := db.First(&uuidLog).Error; err != nil {
		t.Fatal(err)
	}
	if uuidLog.SubjectID == nil || uuidLog.SubjectID.String() != documentID {
		t.Fatalf("unexpected UUID subject: %v", uuidLog.SubjectID)
	}
	if uuidLog.CauserID == nil || uuidLog.CauserID.String() != userID {
		t.Fatalf("unexpected UUID causer: %v", uuidLog.CauserID)
	}
}
