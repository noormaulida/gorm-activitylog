package activitylog

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

type scalarLoggable int

func (*scalarLoggable) ActivityLogOptions() LogOptions { return LogOptions{} }

type disabledUpdateModel struct {
	ID uint64
}

func (*disabledUpdateModel) ActivityLogOptions() LogOptions {
	return LogOptions{LogEvents: []string{EventCreated}}
}

type plainModel struct {
	ID uint64
}

func hookTX(t *testing.T, db *gorm.DB, model any) *gorm.DB {
	t.Helper()
	tx := db.Session(&gorm.Session{NewDB: true})
	if err := tx.Statement.Parse(model); err != nil {
		t.Fatal(err)
	}
	tx.Statement.Model = model
	tx.Statement.Dest = model
	tx.RowsAffected = 1
	return tx
}

func hookTXWithInstance(t *testing.T, db *gorm.DB, model any, key string, value any) *gorm.DB {
	t.Helper()
	tx := hookTX(t, db, model).InstanceSet(key, value)
	if err := tx.Statement.Parse(model); err != nil {
		t.Fatal(err)
	}
	tx.Statement.Model = model
	tx.Statement.Dest = model
	tx.RowsAffected = 1
	return tx
}

func TestCallbackRegistrationAndStatementGuards(t *testing.T) {
	sentinel := errors.New("register")
	called := false
	if err := registerCallbacks(
		func() error { return sentinel },
		func() error {
			called = true
			return nil
		},
	); !errors.Is(err, sentinel) || called {
		t.Fatalf("unexpected callback registration result: %v called=%v", err, called)
	}
	if err := registerCallbacks(func() error { return nil }); err != nil {
		t.Fatal(err)
	}

	if _, ok := statementLoggable(&gorm.DB{}); ok {
		t.Fatal("statement-less DB must not be loggable")
	}
	db := openTestDB(t)
	noSchema := db.Session(&gorm.Session{NewDB: true})
	noSchema.Statement.Schema = nil
	if _, ok := statementLoggable(noSchema); ok {
		t.Fatal("schema-less statement must not be loggable")
	}
	if _, ok := statementLoggable(hookTX(t, db, &Activity{})); ok {
		t.Fatal("activity model must not log itself")
	}
	if _, ok := statementLoggable(hookTX(t, db, &plainModel{ID: 1})); ok {
		t.Fatal("plain model must not be loggable")
	}
}

func TestBeforeUpdateHookGuardBranches(t *testing.T) {
	db := openTestDB(t)

	withError := hookTX(t, db, &testUser{ID: 1})
	withError.Error = errors.New("existing")
	beforeUpdateHook(withError)

	beforeUpdateHook(hookTX(t, db, &plainModel{ID: 1}))
	beforeUpdateHook(hookTX(t, db, &disabledUpdateModel{ID: 1}))

	scalar := scalarLoggable(1)
	scalarTX := hookTX(t, db, &testUser{ID: 1})
	scalarTX.Statement.Model = &scalar
	scalarTX.Statement.Dest = &scalar
	beforeUpdateHook(scalarTX)

	zeroTX := hookTX(t, db, &testUser{})
	beforeUpdateHook(zeroTX)
	if _, exists := zeroTX.InstanceGet(oldDataKey); exists {
		t.Fatal("zero primary key must not capture old state")
	}

	missing := &testUser{ID: 999999, Name: "missing"}
	err := db.Save(missing).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected missing old row error, got %v", err)
	}
}

func TestAfterUpdateHookGuardBranches(t *testing.T) {
	db := openTestDB(t)

	withError := hookTX(t, db, &testUser{ID: 1})
	withError.Error = errors.New("existing")
	afterUpdateHook(withError)

	noRows := hookTX(t, db, &testUser{ID: 1})
	noRows.RowsAffected = 0
	afterUpdateHook(noRows)

	afterUpdateHook(hookTX(t, db, &plainModel{ID: 1}))

	disabled := hookTX(t, db, &disabledUpdateModel{ID: 1})
	afterUpdateHook(disabled)
	if disabled.Error != nil {
		t.Fatalf("disabled event must not require old state: %v", disabled.Error)
	}

	invalidModel := &testUser{ID: 1}
	invalidOld := hookTXWithInstance(t, db, invalidModel, oldDataKey, 7)
	afterUpdateHook(invalidOld)

	zeroModel := &testUser{ID: 1}
	zeroOld := hookTXWithInstance(t, db, zeroModel, oldDataKey, &testUser{})
	afterUpdateHook(zeroOld)

	missingModel := &testUser{ID: 1}
	missingNew := hookTXWithInstance(t, db, missingModel, oldDataKey, &testUser{ID: 999999})
	if _, exists := missingNew.InstanceGet(oldDataKey); !exists {
		t.Fatal("old state fixture was not stored")
	}
	if _, ok := statementLoggable(missingNew); !ok {
		t.Fatal("hook fixture is not loggable")
	}
	afterUpdateHook(missingNew)
	if !errors.Is(missingNew.Error, gorm.ErrRecordNotFound) {
		t.Fatalf("expected missing new state error, got %v", missingNew.Error)
	}
}

func TestDeleteHookGuardBranches(t *testing.T) {
	db := openTestDB(t)

	beforeError := hookTX(t, db, &testUser{ID: 1})
	beforeError.Error = errors.New("existing")
	beforeDeleteHook(beforeError)
	beforeDeleteHook(hookTX(t, db, &plainModel{ID: 1}))
	disabledBefore := hookTX(t, db, &disabledUpdateModel{ID: 1})
	beforeDeleteHook(disabledBefore)
	if _, exists := disabledBefore.InstanceGet(deleteDataKey); exists {
		t.Fatal("disabled delete event must not capture state")
	}

	afterError := hookTX(t, db, &testUser{ID: 1})
	afterError.Error = errors.New("existing")
	afterDeleteHook(afterError)
	noRows := hookTX(t, db, &testUser{ID: 1})
	noRows.RowsAffected = 0
	afterDeleteHook(noRows)
	afterDeleteHook(hookTX(t, db, &plainModel{ID: 1}))
	afterDeleteHook(hookTX(t, db, &testUser{ID: 1}))

	wrongModel := &testUser{ID: 1}
	wrongData := hookTXWithInstance(t, db, wrongModel, deleteDataKey, "wrong")
	afterDeleteHook(wrongData)

	disabledModel := &disabledUpdateModel{ID: 1}
	disabledAfter := hookTXWithInstance(t, db, disabledModel, deleteDataKey, ActivityProperties{})
	afterDeleteHook(disabledAfter)
}
