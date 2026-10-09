//go:build spatie

package activitylog

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type spatieArticle struct {
	ID        uint64
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

func (spatieArticle) TableName() string { return "spatie_articles" }

func (*spatieArticle) ActivityLogOptions() LogOptions {
	return LogOptions{
		LogName:          "go",
		SubjectType:      "articles",
		LogAttributes:    []string{"title", "deleted_at"},
		LogOnlyDirty:     true,
		IgnoreAttributes: []string{"updated_at", "created_at"},
	}
}

func openSpatieDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Register(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestGoWritesSpatieRows(t *testing.T) {
	db := openSpatieDB(t)
	if err := db.Exec("DELETE FROM activity_log WHERE log_name = ?", "go").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM spatie_articles WHERE title LIKE ?", "Go %").Error; err != nil {
		t.Fatal(err)
	}

	article := spatieArticle{Title: "Go Draft"}
	if err := db.Create(&article).Error; err != nil {
		t.Fatal(err)
	}
	article.Title = "Go Published"
	if err := db.Save(&article).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&article).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Unscoped().Model(&article).Update("deleted_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Unscoped().First(&article, article.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Unscoped().Delete(&article).Error; err != nil {
		t.Fatal(err)
	}
}

func TestGoReadsSpatieRows(t *testing.T) {
	db := openSpatieDB(t)

	rows, err := Query(db).InLog("laravel").Oldest().Find()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 {
		t.Fatalf("expected 5 Laravel activities, got %d", len(rows))
	}

	want := []string{EventCreated, EventUpdated, EventDeleted, EventRestored, EventDeleted}
	for i, event := range want {
		if rows[i].Event == nil || *rows[i].Event != event {
			t.Fatalf("row %d event = %v, want %s", i, rows[i].Event, event)
		}
		if rows[i].Description != event {
			t.Fatalf("row %d description = %q", i, rows[i].Description)
		}
		if rows[i].SubjectType == nil || *rows[i].SubjectType != `Compat\Article` {
			t.Fatalf("row %d subject type = %v", i, rows[i].SubjectType)
		}
		if rows[i].SubjectID == nil {
			t.Fatalf("row %d missing subject id", i)
		}
	}

	updated := decodeProps(t, rows[1])
	if updated["old"].(map[string]any)["title"] != "Laravel Draft" ||
		updated["attributes"].(map[string]any)["title"] != "Laravel Published" {
		t.Fatalf("updated properties = %#v", updated)
	}

	softDeleted := decodeProps(t, rows[2])
	if _, ok := softDeleted["attributes"]; ok {
		t.Fatalf("Spatie soft delete keeps only old: %#v", softDeleted)
	}
	old := softDeleted["old"].(map[string]any)
	if old["title"] != "Laravel Published" || old["deleted_at"] == nil {
		t.Fatalf("Spatie soft delete old = %#v", old)
	}

	restored := decodeProps(t, rows[3])
	if _, ok := restored["old"]; ok {
		t.Fatalf("Spatie restore keeps only attributes: %#v", restored)
	}
	attrs := restored["attributes"].(map[string]any)
	if attrs["title"] != "Laravel Published" || attrs["deleted_at"] != nil {
		t.Fatalf("Spatie restore attributes = %#v", attrs)
	}

	hard := decodeProps(t, rows[4])
	if _, ok := hard["attributes"]; ok || hard["old"].(map[string]any)["title"] != "Laravel Published" {
		t.Fatalf("Spatie hard delete = %#v", hard)
	}
}

func decodeProps(t *testing.T, row Activity) map[string]any {
	t.Helper()
	var props map[string]any
	if err := json.Unmarshal(row.Properties, &props); err != nil {
		t.Fatal(err)
	}
	return props
}
