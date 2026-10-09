package activitylog

import "testing"

func TestActivityQueryValidationErrors(t *testing.T) {
	nilQuery := Query(nil)
	nilQuery.
		ForSubject(&testUser{ID: 1}).
		ForSubjectID(1, "users").
		ForSubjectType("users").
		CausedBy(1, "users").
		InLog("default").
		InBatch("batch").
		ForEvent(EventCreated).
		Latest().
		Oldest().
		Limit(1).
		Offset(1)
	if _, err := nilQuery.Find(); err == nil {
		t.Fatal("expected nil query find error")
	}
	if _, err := nilQuery.First(); err == nil {
		t.Fatal("expected nil query first error")
	}
	if _, err := nilQuery.Count(); err == nil {
		t.Fatal("expected nil query count error")
	}

	db := openTestDB(t)
	for name, query := range map[string]*ActivityQuery{
		"subject resolution": Query(db).ForSubject(&testUser{}),
		"subject id":         Query(db).ForSubjectID("invalid", "users"),
		"subject type":       Query(db).ForSubjectID(1, ""),
		"causer id":          Query(db).CausedBy("invalid", "users"),
		"causer type":        Query(db).CausedBy(1, ""),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := query.Find(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestActivityQuerySubjectTypeAndDatabaseErrors(t *testing.T) {
	db := openTestDB(t)
	if err := db.Create(&testUser{Name: "query"}).Error; err != nil {
		t.Fatal(err)
	}
	found, err := Query(db).ForSubjectType("users").Find()
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("expected one subject type match, got %d", len(found))
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Query(db).Find(); err == nil {
		t.Fatal("expected find database error")
	}
	if _, err := Query(db).First(); err == nil {
		t.Fatal("expected first database error")
	}
	if _, err := Query(db).Count(); err == nil {
		t.Fatal("expected count database error")
	}
}
