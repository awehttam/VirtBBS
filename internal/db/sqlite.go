// Package db opens the shared VirtBBS SQLite database with WAL journaling
// and a busy timeout so concurrent readers/writers share one pool safely.
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const (
	busyTimeoutMs = 10_000
	maxOpenConns  = 1
)

// Open opens (or creates) the SQLite database at path, enables WAL mode,
// sets a busy timeout, and configures the connection pool. Callers pass the
// returned *sql.DB to every store (users, messages, files, conferences,
// node) and Close it once at shutdown — individual stores do not own the
// handle.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)", path, busyTimeoutMs)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db %s: %w", path, err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping db %s: %w", path, err)
	}
	return db, nil
}

// OpenMemory opens an in-memory SQLite database with the same pragmas as Open.
// Useful in tests that need an isolated database.
func OpenMemory() (*sql.DB, error) {
	dsn := fmt.Sprintf("file::memory:?cache=shared&_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)", busyTimeoutMs)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
