package storage

import (
	"database/sql"
	"fmt"
	"strings"
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
	conn, err := sql.Open("sqlite", sqliteDSN(path))
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

func sqliteDSN(path string) string {
	const pragma = "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	if strings.HasSuffix(path, "?") || strings.HasSuffix(path, "&") {
		separator = ""
	}
	return path + separator + pragma
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	if db.DB == nil {
		return nil
	}
	if _, cerr := db.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); cerr != nil {
		// Non-fatal: proceed to close.
	}
	return db.DB.Close()
}
