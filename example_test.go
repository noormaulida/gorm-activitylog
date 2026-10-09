package activitylog_test

import (
	"context"
	"fmt"

	activitylog "github.com/noormaulida/gorm-activitylog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type exampleArticle struct {
	ID    uint64
	Title string
}

func (*exampleArticle) ActivityLogOptions() activitylog.LogOptions {
	return activitylog.LogOptions{
		LogName:       "articles",
		SubjectType:   "articles",
		LogOnlyDirty:  true,
		LogAttributes: []string{"title"},
	}
}

func openExampleDB(name string) *gorm.DB {
	db, err := gorm.Open(
		sqlite.Open("file:"+name+"?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		panic(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic(err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := activitylog.Migrate(db); err != nil {
		panic(err)
	}
	if err := db.AutoMigrate(&exampleArticle{}); err != nil {
		panic(err)
	}
	if err := activitylog.Register(db); err != nil {
		panic(err)
	}
	return db
}

func ExampleRegister() {
	db := openExampleDB("example_register")

	fmt.Println(db.Migrator().HasTable(&activitylog.Activity{}))
	// Output: true
}

func ExampleNew() {
	db := openExampleDB("example_manual")

	err := activitylog.New(db).
		UseLog("auth").
		Event("login").
		CausedBy(uint64(42), "users").
		WithProperties(map[string]any{"ip_address": "127.0.0.1"}).
		Log("User logged in")
	if err != nil {
		panic(err)
	}

	activity, err := activitylog.Query(db).InLog("auth").First()
	if err != nil {
		panic(err)
	}
	fmt.Println(activity.Description, *activity.Event)
	// Output: User logged in login
}

func ExampleWithCauser() {
	db := openExampleDB("example_causer")
	ctx := activitylog.WithCauser(
		context.Background(),
		uint64(42),
		"users",
	)

	if err := db.WithContext(ctx).
		Create(&exampleArticle{Title: "GoDoc"}).Error; err != nil {
		panic(err)
	}

	activity, err := activitylog.Query(db).InLog("articles").First()
	if err != nil {
		panic(err)
	}
	causerID, _ := activity.CauserID.Uint64()
	fmt.Println(causerID, *activity.CauserType)
	// Output: 42 users
}

func ExampleQuery() {
	db := openExampleDB("example_query")
	for _, description := range []string{"First event", "Latest event"} {
		if err := activitylog.New(db).
			UseLog("system").
			Event("maintenance").
			Log(description); err != nil {
			panic(err)
		}
	}

	latest, err := activitylog.Query(db).
		InLog("system").
		ForEvent("maintenance").
		Latest().
		First()
	if err != nil {
		panic(err)
	}
	fmt.Println(latest.Description)
	// Output: Latest event
}
