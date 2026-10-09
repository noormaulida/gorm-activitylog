// Package activitylog provides a GORM plugin to automatically track and log
// model activities such as create, update, and delete operations.
//
// Inspired by Spatie Laravel Activitylog, it records who made the changes
// (causer), which model was affected (subject), and what changed through
// old and new attribute values.
//
// It supports manual logging, configurable attribute filtering, and a schema
// designed to interoperate with Spatie Laravel Activitylog.
//
// Basic usage:
//
//	db.Use(activitylog.New())
//
// For configuration and advanced usage, see
// https://github.com/noormaulida/gorm-activitylog.
package activitylog
