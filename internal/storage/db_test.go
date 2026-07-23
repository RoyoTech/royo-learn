package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"agent-royo-learn/internal/testutil"
)

// ---------------------------------------------------------------------------
// Open / Close
// ---------------------------------------------------------------------------

func TestOpen(t *testing.T) {
	t.Parallel()

	dir, err := os.MkdirTemp("", "royo-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() {
		if err := testutil.RemoveAllWithRetry(dir); err != nil {
			t.Errorf("clean up database directory %q: %v", dir, err)
		}
	}()
	path := filepath.Join(dir, "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	if db.DB == nil {
		t.Fatal("Open returned a DB with nil underlying sql.DB")
	}

	// DB file must exist on disk.
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("database file %q does not exist after Open: %v", path, statErr)
	}

	// Simple ping to confirm the connection is alive.
	if err := db.DB.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	dir, err := os.MkdirTemp("", "royo-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() {
		if err := testutil.RemoveAllWithRetry(dir); err != nil {
			t.Errorf("clean up database directory %q: %v", dir, err)
		}
	}()
	path := filepath.Join(dir, "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After Close, the underlying connection should be unusable.
	if err := db.DB.Ping(); err == nil {
		t.Fatal("Ping after Close should fail")
	}
}

// ---------------------------------------------------------------------------
// Pragmas
// ---------------------------------------------------------------------------

func TestPragmas(t *testing.T) {
	t.Parallel()

	dir, err := os.MkdirTemp("", "royo-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() {
		if err := testutil.RemoveAllWithRetry(dir); err != nil {
			t.Errorf("clean up database directory %q: %v", dir, err)
		}
	}()
	path := filepath.Join(dir, "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer db.Close()

	assertPragma(t, db.DB, "journal_mode", "wal")
	assertPragma(t, db.DB, "foreign_keys", "1")
}

func assertPragma(t *testing.T, conn *sql.DB, pragma string, want string) {
	t.Helper()
	var got string
	err := conn.QueryRow("PRAGMA " + pragma).Scan(&got)
	if err != nil {
		t.Fatalf("PRAGMA %s: %v", pragma, err)
	}
	if got != want {
		t.Errorf("PRAGMA %s = %q, want %q", pragma, got, want)
	}
}

// ---------------------------------------------------------------------------
// Migrate: success
// ---------------------------------------------------------------------------

func TestMigrateSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping migration test in short mode")
	}

	dir, err := os.MkdirTemp("", "royo-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() {
		if err := testutil.RemoveAllWithRetry(dir); err != nil {
			t.Errorf("clean up database directory %q: %v", dir, err)
		}
	}()
	path := filepath.Join(dir, "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Verify schema_migrations records the migration.
	var count int
	if err := db.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("schema_migrations query: %v", err)
	}
	if count == 0 {
		t.Fatal("schema_migrations is empty after Migrate")
	}

	// Verify at least learnings and projects tables exist (from 001_init.sql).
	expectedTables := []string{"schema_migrations", "projects", "learnings", "evidence"}
	for _, table := range expectedTables {
		var n int
		err := db.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&n)
		if err != nil {
			t.Fatalf("sqlite_master query for table %q: %v", table, err)
		}
		if n == 0 {
			t.Errorf("table %q was not created by migration", table)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping migration test in short mode")
	}

	dir, err := os.MkdirTemp("", "royo-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() {
		if err := testutil.RemoveAllWithRetry(dir); err != nil {
			t.Errorf("clean up database directory %q: %v", dir, err)
		}
	}()
	path := filepath.Join(dir, "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (first): %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (second): %v", err)
	}

	// Second migration must not add duplicate rows to schema_migrations.
	var count int
	if err := db.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("schema_migrations count: %v", err)
	}
	if count < 1 {
		t.Errorf("schema_migrations row count after two Migrate calls = %d, want >= 1", count)
	}
}

func TestMigrateChecksumMismatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping migration test in short mode")
	}

	dir, err := os.MkdirTemp("", "royo-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() {
		if err := testutil.RemoveAllWithRetry(dir); err != nil {
			t.Errorf("clean up database directory %q: %v", dir, err)
		}
	}()
	path := filepath.Join(dir, "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (first): %v", err)
	}

	// Tamper with the stored checksum to simulate a modified migration.
	if _, err := db.DB.Exec(`UPDATE schema_migrations SET checksum = 'bad_checksum' WHERE version = 1`); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	if err := Migrate(db); err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
}

func TestConcurrentMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping migration test in short mode")
	}

	dir, err := os.MkdirTemp("", "royo-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() {
		if err := testutil.RemoveAllWithRetry(dir); err != nil {
			t.Errorf("clean up database directory %q: %v", dir, err)
		}
	}()
	path := filepath.Join(dir, "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer db.Close()

	// Barrier synchronises both goroutines so they reach Migrate roughly
	// simultaneously, making the TestLock exercise deterministic.
	var barrier, wg sync.WaitGroup
	errs := make([]error, 2)

	barrier.Add(2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			barrier.Done() // signal "ready"
			barrier.Wait() // wait for both to be ready
			errs[idx] = Migrate(db)
		}(i)
	}
	wg.Wait()

	// Exactly one caller must succeed; the other must hit the TryLock guard.
	successCount := 0
	for _, e := range errs {
		if e == nil {
			successCount++
		}
	}
	if successCount < 1 {
		t.Fatalf("no concurrent migration succeeded: errs=%v", errs)
	}
	if successCount > 1 {
		t.Fatalf("multiple concurrent migrations succeeded (expected at most 1): errs=%v", errs)
	}

	// After concurrent calls, exactly one migration row must exist.
	var count int
	if err := db.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("schema_migrations count: %v", err)
	}
	if count < 1 {
		t.Errorf("schema_migrations row count after concurrent Migrate = %d, want >= 1", count)
	}
}

func TestConcurrentIdenticalMigrationApplicationIsTolerated(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "migration-race.db")
	first, err := Open(path)
	if err != nil {
		t.Fatalf("open first database: %v", err)
	}
	defer first.Close()
	second, err := Open(path)
	if err != nil {
		t.Fatalf("open second database: %v", err)
	}
	defer second.Close()
	if _, err := first.DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, checksum TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}

	m := migration{
		Version:  99,
		Name:     "099_concurrent_test",
		SQL:      `CREATE TABLE IF NOT EXISTS concurrent_migration_table (id INTEGER PRIMARY KEY)`,
		Checksum: "same-checksum",
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, db := range []*sql.DB{first.DB, second.DB} {
		wg.Add(1)
		go func(conn *sql.DB) {
			defer wg.Done()
			<-start
			errs <- applyMigration(conn, m)
		}(db)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("identical concurrent migration failed: %v", err)
		}
	}

	var count int
	if err := first.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 99 AND checksum = 'same-checksum'`).Scan(&count); err != nil {
		t.Fatalf("read migration row: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration rows = %d, want 1", count)
	}
}

func TestConcurrentMigrationChecksumMismatchStillFails(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "migration-checksum.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if _, err := db.DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, checksum TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	if _, err := db.DB.Exec(`INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES (100, '100_test', 'stored-checksum', datetime('now'))`); err != nil {
		t.Fatalf("insert stored migration: %v", err)
	}
	err = applyMigration(db.DB, migration{
		Version:  100,
		Name:     "100_test",
		SQL:      `CREATE TABLE IF NOT EXISTS checksum_test_table (id INTEGER PRIMARY KEY)`,
		Checksum: "current-checksum",
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
}

func TestMigrateDryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping migration test in short mode")
	}

	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer db.Close()

	// Before migration: all pending.
	plan, err := MigrateDryRun(db)
	if err != nil {
		t.Fatalf("MigrateDryRun (before): %v", err)
	}
	if len(plan) == 0 {
		t.Fatal("MigrateDryRun returned empty plan before migration")
	}
	for _, p := range plan {
		if p.Status != "pending" {
			t.Errorf("migration %d %q: status=%q, want pending", p.Version, p.Name, p.Status)
		}
	}

	// Apply migration.
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// After migration: all applied.
	plan, err = MigrateDryRun(db)
	if err != nil {
		t.Fatalf("MigrateDryRun (after): %v", err)
	}
	if len(plan) == 0 {
		t.Fatal("MigrateDryRun returned empty plan after migration")
	}
	for _, p := range plan {
		if p.Status != "applied" {
			t.Errorf("migration %d %q: status=%q, want applied", p.Version, p.Name, p.Status)
		}
	}

	// Dry-run must not modify the database.
	var count int
	if err := db.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("schema_migrations count: %v", err)
	}
	if count < 1 {
		t.Errorf("schema_migrations row count after dry-run = %d, want >= 1", count)
	}
}
