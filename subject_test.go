package activitylog

import "testing"

type invalidSchemaModel struct {
	Channel chan int
}

type invalidStringIDModel struct {
	ID string `gorm:"primaryKey"`
}

func TestResolveSubjectErrorsAndFallback(t *testing.T) {
	db := openTestDB(t)

	cases := []struct {
		name  string
		dbNil bool
		model any
	}{
		{name: "nil database", dbNil: true, model: &testUser{ID: 1}},
		{name: "nil model", model: nil},
		{name: "parse error", model: &invalidSchemaModel{}},
		{name: "composite key", model: &compositeModel{Left: 1, Right: 2}},
		{name: "non struct", model: &[]testUser{{ID: 1}}},
		{name: "zero key", model: &testUser{}},
		{name: "invalid string key", model: &invalidStringIDModel{ID: "invalid"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			targetDB := db
			if test.dbNil {
				targetDB = nil
			}
			if _, _, err := resolveSubject(targetDB, test.model); err == nil {
				t.Fatal("expected subject resolution error")
			}
		})
	}

	id, modelType, err := resolveSubject(db, &invalidStringIDModel{
		ID: "018f8f4e-735b-7c44-89b2-3f2fcf0d97a1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id.String() == "" || modelType != "invalid_string_id_models" {
		t.Fatalf("unexpected fallback subject: %q %q", id.String(), modelType)
	}
}
