package storage

import (
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// DB wraps a *sql.DB connection with royo-learn-specific lifecycle helpers.
type DB struct {
	DB   *sql.DB
	path string
	mu   sync.Mutex
}

// Open opens a SQLite database at path and applies the required pragmas.
// It returns a ready-to-use *DB connection.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("storage: open %q: %w", path, err)
	}

	// Apply pragmas.
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	for _, p := range pragmas {
		if _, execErr := conn.Exec(p); execErr != nil {
			conn.Close()
			return nil, fmt.Errorf("storage: %s: %w", p, execErr)
		}
	}

	return &DB{DB: conn, path: path}, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	if db.DB != nil {
		db.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	}
	return db.DB.Close()
}
