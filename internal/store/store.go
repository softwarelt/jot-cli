// Package store is the only package that knows SQL exists (DESIGN.md §2).
// Every other package talks to it through Note/TagCount and the methods
// below it — never through raw SQL.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Store wraps a single SQLite connection pool for one jot database file.
type Store struct {
	db *sql.DB
}

// Open opens (creating if necessary) the SQLite database at path, applies
// the pragmas required by NFR-3/NFR-4, and ensures the schema exists.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// A single connection avoids SQLITE_BUSY between our own goroutines and
	// lets busy_timeout do the work of waiting out other processes.
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA foreign_keys = ON;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}

	// The driver doesn't guarantee the mode a newly created file lands with;
	// enforce NFR-5 explicitly. A missing file at this point (e.g. an
	// in-memory DSN used by a test) is not an error.
	if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
		db.Close()
		return nil, fmt.Errorf("setting database file permissions: %w", err)
	}

	return &Store{db: db}, nil
}

// Close releases the underlying connection.
func (s *Store) Close() error {
	return s.db.Close()
}
