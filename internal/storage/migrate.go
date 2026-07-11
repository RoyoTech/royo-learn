package storage

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrateError signals that a migration could not be applied.
type MigrateError struct {
	Version int
	Name    string
	Reason  string
}

func (e *MigrateError) Error() string {
	return fmt.Sprintf("storage: migration %d %q: %s", e.Version, e.Name, e.Reason)
}

// migration represents a single migration file ready to apply.
type migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

// Migrate applies pending SQL migration files from the embedded migrations
// directory. It is safe to call multiple times: applied migrations are skipped,
// and if a previously applied migration has been modified it returns an error.
//
// Concurrent calls are serialised via a mutex; only the first caller applies
// migrations while later callers receive an error.
func Migrate(db *DB) error {
	if db == nil || db.DB == nil {
		return errors.New("storage: nil database connection")
	}

	if !db.mu.TryLock() {
		return errors.New("storage: migration already in progress")
	}
	defer db.mu.Unlock()

	return migrateDB(db.DB)
}

func migrateDB(conn *sql.DB) error {
	files, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("storage: cannot load migrations: %w", err)
	}

	// Ensure the tracking table exists before we touch any application-level
	// migration — this is NOT part of 001_init.sql because the migration runner
	// itself needs it.
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name    TEXT NOT NULL,
		checksum TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("storage: create schema_migrations: %w", err)
	}

	for _, m := range files {
		applied, storedChecksum, err := isApplied(conn, m.Version)
		if err != nil {
			return &MigrateError{Version: m.Version, Name: m.Name, Reason: fmt.Sprintf("cannot check status: %v", err)}
		}

		if applied {
			if storedChecksum != m.Checksum {
				return &MigrateError{
					Version: m.Version,
					Name:    m.Name,
					Reason:  fmt.Sprintf("checksum mismatch: stored=%s current=%s", storedChecksum, m.Checksum),
				}
			}
			continue // already applied, skip
		}

		if err := applyMigration(conn, m); err != nil {
			return err
		}
	}

	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}

	var out []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		data, readErr := fs.ReadFile(migrationsFS, "migrations/"+e.Name())
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), readErr)
		}
		sqlStr := string(data)

		version, name, parseErr := parseVersion(e.Name())
		if parseErr != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), parseErr)
		}

		checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(sqlStr)))
		out = append(out, migration{
			Version:  version,
			Name:     name,
			SQL:      sqlStr,
			Checksum: checksum,
		})
	}

	// Apply in version order.
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// parseVersion extracts the integer prefix from "NNN_name.sql".
func parseVersion(filename string) (int, string, error) {
	idx := strings.Index(filename, "_")
	if idx < 1 {
		return 0, "", fmt.Errorf("expected NNN_name.sql, got %q", filename)
	}
	v := 0
	if _, err := fmt.Sscanf(filename[:idx], "%d", &v); err != nil {
		return 0, "", fmt.Errorf("invalid version prefix %q: %w", filename[:idx], err)
	}
	return v, strings.TrimSuffix(filename, ".sql"), nil
}

func isApplied(conn *sql.DB, version int) (bool, string, error) {
	var checksum string
	err := conn.QueryRow(`SELECT checksum FROM schema_migrations WHERE version = ?`, version).Scan(&checksum)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		// If the table doesn't exist yet, treat as not-applied.
		if strings.Contains(err.Error(), "no such table") {
			return false, "", nil
		}
		return false, "", err
	}
	return true, checksum, nil
}

func applyMigration(conn *sql.DB, m migration) error {
	tx, err := conn.Begin()
	if err != nil {
		return &MigrateError{Version: m.Version, Name: m.Name, Reason: fmt.Sprintf("begin tx: %v", err)}
	}
	defer func() {
		if tx != nil {
			tx.Rollback() //nolint: errcheck
		}
	}()

	if _, err := tx.Exec(m.SQL); err != nil {
		return &MigrateError{Version: m.Version, Name: m.Name, Reason: fmt.Sprintf("exec: %v", err)}
	}

	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, datetime('now'))`,
		m.Version, m.Name, m.Checksum,
	); err != nil {
		return &MigrateError{Version: m.Version, Name: m.Name, Reason: fmt.Sprintf("record: %v", err)}
	}

	if err := tx.Commit(); err != nil {
		return &MigrateError{Version: m.Version, Name: m.Name, Reason: fmt.Sprintf("commit: %v", err)}
	}
	tx = nil // prevent rollback in defer
	return nil
}

// MigrationPlan describes a single pending migration without applying it.
type MigrationPlan struct {
	Version  int
	Name     string
	Checksum string
	Status   string // "pending", "applied", or "modified"
}

// MigrateDryRun reports which migrations would be applied without executing
// any SQL beyond reading schema_migrations. It returns a plan of all known
// migration files and their status.
func MigrateDryRun(db *DB) ([]MigrationPlan, error) {
	if db == nil || db.DB == nil {
		return nil, errors.New("storage: nil database connection")
	}

	files, err := loadMigrations()
	if err != nil {
		return nil, fmt.Errorf("storage: cannot load migrations: %w", err)
	}

	var plan []MigrationPlan
	for _, m := range files {
		applied, storedChecksum, chkErr := isApplied(db.DB, m.Version)
		if chkErr != nil {
			return nil, chkErr
		}

		status := "pending"
		if applied {
			if storedChecksum != m.Checksum {
				status = "modified"
			} else {
				status = "applied"
			}
		}

		plan = append(plan, MigrationPlan{
			Version:  m.Version,
			Name:     m.Name,
			Checksum: m.Checksum,
			Status:   status,
		})
	}
	return plan, nil
}

// mu (sync.Mutex) is placed on the DB struct to protect concurrent Migrate callers.
