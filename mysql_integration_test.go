package activitylog

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type mysqlArticle struct {
	ID        uint64 `gorm:"primaryKey"`
	Title     string
	Secret    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (mysqlArticle) TableName() string {
	return "integration_articles"
}

func (*mysqlArticle) ActivityLogOptions() LogOptions {
	return LogOptions{
		LogName:          "articles",
		SubjectType:      "App\\Models\\Article",
		IgnoreAttributes: []string{"secret", "updated_at"},
		LogOnlyDirty:     true,
	}
}

func TestSpatieMySQLSchemaCompatibility(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN is not configured")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"activity_log", "integration_articles"} {
		if err := db.Exec("DROP TABLE IF EXISTS " + table).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec("DROP TABLE IF EXISTS activity_log")
		db.Exec("DROP TABLE IF EXISTS integration_articles")
	})

	// Mirrors the current default Spatie Laravel Activitylog migration.
	const createActivityLog = `
CREATE TABLE activity_log (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    log_name VARCHAR(255) NULL,
    description TEXT NOT NULL,
    subject_type VARCHAR(255) NULL,
    event VARCHAR(255) NULL,
    subject_id BIGINT UNSIGNED NULL,
    causer_type VARCHAR(255) NULL,
    causer_id BIGINT UNSIGNED NULL,
    properties JSON NULL,
    batch_uuid CHAR(36) NULL,
    created_at TIMESTAMP NULL,
    updated_at TIMESTAMP NULL,
    INDEX subject (subject_type, subject_id),
    INDEX causer (causer_type, causer_id),
    INDEX activity_log_log_name_index (log_name),
    INDEX activity_log_batch_uuid_index (batch_uuid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	if err := db.Exec(createActivityLog).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&mysqlArticle{}); err != nil {
		t.Fatal(err)
	}
	if err := Register(db); err != nil {
		t.Fatal(err)
	}

	article := mysqlArticle{Title: "Before", Secret: "hidden"}
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
	if logs[0].SubjectType == nil || *logs[0].SubjectType != "App\\Models\\Article" {
		t.Fatalf("unexpected Laravel morph type: %v", logs[0].SubjectType)
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
}
