<p align="center">
  <img src="https://skillicons.dev/icons?i=go" alt="Go" width="80" />
</p>

<p align="center">
  <img src=".github/gorm-activitylog.svg" alt="gorm-activitylog" width="520" />
</p>

<p align="center">
  Automatic and manual activity logging for GORM, with an activity_log schema compatible with Spatie Laravel Activitylog.
</p>

<p align="center">
  <a href="https://github.com/noormaulida/gorm-activitylog/actions/workflows/go.yml">
    <img src="https://github.com/noormaulida/gorm-activitylog/actions/workflows/go.yml/badge.svg" alt="Go Tests" />
  </a>
  <a href="https://codecov.io/gh/noormaulida/gorm-activitylog">
    <img src="https://codecov.io/gh/noormaulida/gorm-activitylog/graph/badge.svg" alt="codecov" />
  </a>
  <a href="https://pkg.go.dev/github.com/noormaulida/gorm-activitylog">
    <img src="https://pkg.go.dev/badge/github.com/noormaulida/gorm-activitylog.svg" alt="Go Reference" />
  </a>
  <a href="https://github.com/noormaulida/gorm-activitylog/blob/master/LICENSE">
    <img src="https://img.shields.io/github/license/noormaulida/gorm-activitylog" alt="License" />
  </a>
</p>

## Installation

```bash
go get github.com/noormaulida/gorm-activitylog@latest
```

## Setup

Register the callbacks once after opening the GORM connection:

```go
db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
if err != nil {
    return err
}

if err := activitylog.Migrate(db); err != nil {
    return err
}
if err := activitylog.Register(db); err != nil {
    return err
}
```

`Migrate` is optional when the application already uses Spatie's migration.

## Automatic model logging

Implement `ActivityLogOptions` on each model that should be audited:

```go
type Article struct {
    ID        uint64
    Title     string
    Body      string
    Secret    string
    CreatedAt time.Time
    UpdatedAt time.Time
}

func (*Article) ActivityLogOptions() activitylog.LogOptions {
    return activitylog.LogOptions{
        LogName:          "articles",
        SubjectType:      "articles",
        IgnoreAttributes: []string{"secret", "updated_at"},
        LogOnlyDirty:     true,
    }
}
```

Normal GORM operations then create `created`, `updated`, and `deleted` activities:

```go
db.Create(&article)
db.Save(&article)
db.Delete(&article)
```

`LogAttributes` is an optional whitelist. `IgnoreAttributes` is always applied after it and therefore takes precedence. Both Go field names and database column names are accepted.

Set `SubjectType` to a stable alias such as `articles`. If omitted, the GORM
table name is used. When sharing a legacy Laravel database that stores PHP
class names, either use the exact existing value or configure a Laravel morph
map so both applications can use the same stable alias.

### Event controls and descriptions

By default, all three lifecycle events are logged. Use `LogEvents` to select a
subset and `DescriptionForEvent` to customize the human-readable description:

```go
func (*Article) ActivityLogOptions() activitylog.LogOptions {
    return activitylog.LogOptions{
        LogEvents: []string{
            activitylog.EventUpdated,
            activitylog.EventDeleted,
        },
        DescriptionForEvent: func(event string) string {
            if event == activitylog.EventUpdated {
                return "Article was published"
            }
            return event
        },
    }
}
```

This configuration does not log `created`. The `event` column remains
`updated`, while the `description` column contains `Article was published`.
An empty `LogEvents` list preserves the default of logging every event.

## Causer from request context

Put the authenticated user into a standard Go context and pass that context to GORM:

```go
ctx := activitylog.WithCauser(
    request.Context(),
    authenticatedUser.ID,
    "users",
)

if err := db.WithContext(ctx).Create(&article).Error; err != nil {
    return err
}
```

Gin middleware can attach the causer to the request context:

```go
func ActivityContext() gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := c.GetUint64("user_id")
        ctx := activitylog.WithCauser(
            c.Request.Context(),
            userID,
            "users",
        )
        c.Request = c.Request.WithContext(ctx)
        c.Next()
    }
}

// In a handler:
db.WithContext(c.Request.Context()).Create(&article)
```

Fiber v2 provides `UserContext` for the same purpose:

```go
func ActivityContext(c *fiber.Ctx) error {
    if userID, ok := c.Locals("user_id").(uint64); ok {
        ctx := activitylog.WithCauser(
            c.UserContext(),
            userID,
            "users",
        )
        c.SetUserContext(ctx)
    }
    return c.Next()
}

// In a handler:
db.WithContext(c.UserContext()).Create(&article)
```

Apply this middleware after authentication so `user_id` is already available.

## Manual logging

Manual properties are stored directly in the `properties` JSON object:

```go
err := activitylog.New(db).
    UseLog("auth").
    Event("login").
    CausedBy(user.ID, "users").
    WithProperties(map[string]any{
        "ip_address": request.RemoteAddr,
    }).
    Log("User logged in")
```

Use `PerformedOn(&article)` to attach a model subject. It reads the primary key and uses `SubjectType` from the model's `LogOptions`.

### UUID and ULID morph IDs

Model and causer IDs may be numeric values, canonical UUIDs, or ULIDs:

```go
activitylog.New(db).
    CausedBy("018f8f51-a3c1-7118-a408-3763ebd7167c", "users").
    PerformedOn(&document).
    Log("Document viewed")
```

Read an ID with `activity.SubjectID.String()`. For numeric schemas,
`activity.SubjectID.Uint64()` returns the numeric value and a boolean.

`activitylog.Migrate` intentionally creates Spatie's default numeric morph
columns. Applications using UUID or ULID IDs must create `activity_log` with
Laravel's `uuidMorphs` or `ulidMorphs` migration instead of calling `Migrate`.

## Batch logging

Group multiple automatic activities by carrying one UUID in their context:

```go
ctx := activitylog.WithBatch(
    request.Context(),
    "018f8f4e-735b-7c44-89b2-3f2fcf0d97a1",
)

tx := db.WithContext(ctx)
tx.Create(&article)
tx.Create(&comment)
```

The fluent logger supports the same column:

```go
activitylog.New(db).
    InBatch(batchUUID).
    Log("Import completed")
```

The package accepts UUID strings without generating or validating them, so the
application remains responsible for choosing its UUID version and generator.

## Querying activities

Use the fluent query helper to combine common activity filters:

```go
activities, err := activitylog.Query(db).
    ForSubject(&article).
    InLog("articles").
    ForEvent(activitylog.EventUpdated).
    Latest().
    Limit(20).
    Find()
```

Subject and causer filters support numeric, UUID, and ULID identifiers:

```go
count, err := activitylog.Query(db).
    CausedBy(user.ID, "users").
    InBatch(batchUUID).
    Count()

latest, err := activitylog.Query(db).
    ForSubjectID(articleID, "articles").
    Latest().
    First()
```

Available filters and modifiers include `ForSubject`, `ForSubjectID`,
`ForSubjectType`, `CausedBy`, `InLog`, `InBatch`, `ForEvent`, `Latest`,
`Oldest`, `Limit`, and `Offset`.

## Pruning old activities

Delete activities older than a retention duration:

```go
deleted, err := activitylog.Prune(db, 90*24*time.Hour)
if err != nil {
    return err
}
log.Printf("pruned %d activities", deleted)
```

Schedulers that already calculate a retention boundary can use an explicit
cutoff:

```go
deleted, err := activitylog.PruneBefore(db, cutoff)
```

`PruneBefore` deletes rows whose `created_at` is strictly earlier than the
cutoff. Both functions return the number of deleted rows and reject zero or
negative retention values, zero cutoffs, and nil database connections.

## Temporarily disabling logging

Suppress logging for one context while model operations continue normally:

```go
ctx := activitylog.WithoutLogging(request.Context())
db.WithContext(ctx).Create(&article)
```

For seeders, imports, or maintenance tasks, use a scoped callback:

```go
err := activitylog.RunWithoutLogging(db, func(tx *gorm.DB) error {
    if err := tx.CreateInBatches(records, 500).Error; err != nil {
        return err
    }
    return tx.Model(&Article{}).Update("indexed", true).Error
})
```

Suppression also applies to manual `activitylog.New(tx).Log(...)` calls made
with the scoped DB. It is context-local and does not affect concurrent
requests.

## Transaction behavior

Automatic activities are inserted through the same database transaction as the model operation. Rolling back the model change also rolls back its activity. Failure to serialize or insert an activity is returned as a GORM operation
error. Updates also fail with `activitylog.ErrMissingOldState` when the package
cannot safely capture the value that existed before the update.

## Soft delete and restore

A model with `gorm.DeletedAt` is soft-deleted by GORM. That writes a `deleted` activity:

- `properties.attributes` is the row after `deleted_at` is set.
- `properties.old` is the row before the soft delete.

Restoring with `Unscoped().Model(&row).Update("deleted_at", nil)` writes `restored`, with `attributes` after the restore and `old` while the row was deleted. `LogOnlyDirty` keeps `deleted_at` on those two events and drops unchanged fields. An empty `LogEvents` list includes `restored`. Omit `restored` from `LogEvents` to skip it.

`Unscoped().Delete` and deleting a model without `DeletedAt` are hard deletes. They store only `properties.old`.

Updating another column on a soft-deleted row through `Unscoped()` stays an `updated` activity.

## Supported updates

| Operation | Activity |
| --- | --- |
| `Create`, `Save` | Logged |
| `Updates` on a struct whose primary key is set | Logged |
| `Update` of one column on that same instance | Logged |
| `Model(&T{}).Where(...).Update` or `Updates` | Not logged; the update still succeeds |
| `Updates(map[string]any)` with more than one entry | Not logged; the update still succeeds |
| `UpdateColumn` / `UpdateColumns` | Not logged; the update still succeeds |
| `Table(...).Update` | Not logged; the update still succeeds |

`ErrMissingOldState` is returned only when an instance update should have been audited and the previous row could not be captured. Composite primary keys are not supported.

## Spatie compatibility

The two-way suite in `compat/laravel` pins `spatie/laravel-activitylog` 4.10.2. It runs that package's migrations, then checks that Laravel can read rows written by this package and that this package can read rows written by Spatie. CI runs it against MySQL 8.4.

| | This package | Spatie 4.10.2 |
| --- | --- | --- |
| Table and columns | `activity_log`, including `event` and `batch_uuid` | Same migrations |
| `created` | `attributes` | `attributes` |
| `updated` | `attributes` and `old` | `attributes` and `old` |
| Soft `deleted` | `attributes` (after) and `old` (before) | `old` only, snapshot after delete |
| `restored` | `attributes` (after) and `old` (before) | `attributes` only |
| Hard `deleted` | `old` only | `old` only |
| Subject type | Stable alias such as `articles` | PHP class name unless a morph map is set |

Schema compatibility is not the same as identical JSON. Compare an existing Laravel migration before running `Migrate`.

## Current limitations

- Automatic subjects require exactly one non-zero numeric, UUID, or ULID primary key.
- Composite primary keys are not currently supported.

## Running Tests

```bash
go test -v ./...
```

MySQL and PostgreSQL compatibility suites run when their DSNs are configured:

```bash
MYSQL_DSN='root:root@tcp(127.0.0.1:3306)/activitylog_test?parseTime=true' \
POSTGRES_DSN='host=127.0.0.1 user=postgres password=postgres dbname=activitylog_test sslmode=disable' \
go test -race ./...
```

CI runs both databases, enforces 100% statement coverage, and runs the Spatie 4.10.2 round trip.

## License

[MIT License](LICENSE)
