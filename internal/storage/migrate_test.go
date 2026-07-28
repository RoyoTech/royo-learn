// Tests for migration 008_job_semantics.sql (Hito 11, PR #13).
//
// The runner at migrate.go is forward-only — there is no down migration.
// These tests cover the forward path only:
//   - the new schema applies cleanly on a fresh DB;
//   - the new schema applies idempotently (Migrate twice is a no-op);
//   - the runner's stored SHA-256 of 008 matches the embedded file.

package storage

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"

	"agent-royo-learn/internal/testutil"
)

// TestMigrate_008_Forward applies migration 008 against a fresh SQLite
// database and asserts the three new job_registry columns and the
// job_run_log table exist with the documented shape.
func TestMigrate_008_Forward(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// The three taxonomy columns must exist on job_registry.
	for _, col := range []string{"intent", "scope", "risk_class"} {
		var n int
		if err := db.DB.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info('job_registry') WHERE name = ?",
			col,
		).Scan(&n); err != nil {
			t.Fatalf("pragma_table_info query for %q: %v", col, err)
		}
		if n != 1 {
			t.Errorf("job_registry.%s missing after migration 008 (found %d rows)", col, n)
		}
	}

	// The job_run_log table must exist.
	var tableExists int
	if err := db.DB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='job_run_log'",
	).Scan(&tableExists); err != nil {
		t.Fatalf("sqlite_master query for job_run_log: %v", err)
	}
	if tableExists != 1 {
		t.Fatal("job_run_log table missing after migration 008")
	}

	// Required columns and constraints on job_run_log. Note: pragma_table_info
	// reports NOT NULL=0 for PRIMARY KEY columns declared without an explicit
	// NOT NULL clause, even though PRIMARY KEY implies NOT NULL at the SQLite
	// level — so we only assert NOT NULL for non-PK columns.
	jobRunLogColumns := map[string]struct {
		wantNotNull bool
		dflt        string
		pk          int
	}{
		"run_id":        {wantNotNull: false, pk: 1}, // PK implies NOT NULL
		"job_name":      {wantNotNull: true},
		"state":         {wantNotNull: true},
		"started_at":    {wantNotNull: true},
		"finished_at":   {wantNotNull: false},
		"error_code":    {wantNotNull: true, dflt: ""},
		"error_message": {wantNotNull: true, dflt: ""},
		"attempt":       {wantNotNull: true, dflt: "0"},
	}
	for col, want := range jobRunLogColumns {
		var (
			notNull, pk int
			dfltValue   *string
		)
		err := db.DB.QueryRow(
			"SELECT \"notnull\", dflt_value, pk FROM pragma_table_info('job_run_log') WHERE name = ?",
			col,
		).Scan(&notNull, &dfltValue, &pk)
		if err != nil {
			t.Fatalf("pragma_table_info query for job_run_log.%s: %v", col, err)
		}
		gotNotNull := notNull == 1
		if gotNotNull != want.wantNotNull {
			t.Errorf("job_run_log.%s NOT NULL = %v, want %v", col, gotNotNull, want.wantNotNull)
		}
		if pk != want.pk {
			t.Errorf("job_run_log.%s PK = %d, want %d", col, pk, want.pk)
		}
		if want.dflt != "" {
			if dfltValue == nil || *dfltValue != want.dflt {
				var got string
				if dfltValue != nil {
					got = *dfltValue
				}
				t.Errorf("job_run_log.%s DEFAULT = %q, want %q", col, got, want.dflt)
			}
		}
	}

	// The schema_migrations row proves the runner accepted the migration.
	// (We don't attempt an INSERT into job_run_log with NULL run_id here
	// because modernc.org/sqlite doesn't enforce PK-implied NOT NULL by
	// default; the engine layer (jobs.Service) treats the empty run_id as
	// a programming error instead.)
	var appliedName string
	if err := db.DB.QueryRow(
		"SELECT name FROM schema_migrations WHERE version = 8",
	).Scan(&appliedName); err != nil {
		t.Fatalf("schema_migrations lookup for version 8: %v", err)
	}
	if appliedName != "008_job_semantics" {
		t.Errorf("schema_migrations name for version 8 = %q, want 008_job_semantics", appliedName)
	}
}

// TestMigrate_008_Idempotent calls Migrate twice on the same DB. The
// second call must be a no-op: no error, no extra rows in
// schema_migrations, the schema is unchanged.
func TestMigrate_008_Idempotent(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (first): %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (second): %v", err)
	}

	// Exactly one row for version 8.
	var count int
	if err := db.DB.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations WHERE version = 8",
	).Scan(&count); err != nil {
		t.Fatalf("schema_migrations count for version 8: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_migrations rows for version 8 = %d, want 1", count)
	}

	// Schema is unchanged: the three columns + table still present.
	for _, col := range []string{"intent", "scope", "risk_class"} {
		var n int
		if err := db.DB.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info('job_registry') WHERE name = ?",
			col,
		).Scan(&n); err != nil {
			t.Fatalf("pragma_table_info query for %q (second pass): %v", col, err)
		}
		if n != 1 {
			t.Errorf("job_registry.%s missing after second Migrate call", col)
		}
	}
	var tableExists int
	if err := db.DB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='job_run_log'",
	).Scan(&tableExists); err != nil {
		t.Fatalf("sqlite_master query for job_run_log (second pass): %v", err)
	}
	if tableExists != 1 {
		t.Error("job_run_log table missing after second Migrate call")
	}
}

// TestMigrate_008_ChecksumStable verifies that the runner stores a
// SHA-256 of 008 that matches the embedded file's actual checksum. This
// is the operator's safety net: if anyone edits 008 after it ships, the
// runner refuses to apply a divergent copy on a subsequent Migrate call.
func TestMigrate_008_ChecksumStable(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var stored string
	if err := db.DB.QueryRow(
		"SELECT checksum FROM schema_migrations WHERE version = 8",
	).Scan(&stored); err != nil {
		t.Fatalf("schema_migrations read for version 8: %v", err)
	}
	if stored == "" {
		t.Fatal("schema_migrations checksum for version 8 is empty")
	}

	// Recompute the SHA-256 of the embedded 008 file.
	data, readErr := migrationsFS.ReadFile("migrations/008_job_semantics.sql")
	if readErr != nil {
		t.Fatalf("read 008_job_semantics.sql: %v", readErr)
	}
	want := fmt.Sprintf("%x", sha256.Sum256(data))
	if stored != want {
		t.Errorf("stored checksum for 008 = %s, want %s", stored, want)
	}
}
