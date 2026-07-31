// Tests for migration 009_publication_drift.sql (Hito 12, PR #1).
//
// The runner at migrate.go is forward-only — there is no down migration.
// These tests cover the forward path only:
//   - the new schema applies cleanly on a fresh DB;
//   - the new schema applies idempotently (Migrate twice is a no-op);
//   - the runner's stored SHA-256 of 009 matches the embedded file;
//   - the CHECK constraint on publication_drift_state.status rejects
//     any value outside the four enum strings;
//   - the CHECK constraint on publication_drift_state.source rejects
//     any value outside the three adapter names.
//
// The tests are colocated with TestMigrate_008_* because the runner is
// shared; splitting them into a separate file keeps the new migration
// easy to find during review.

package storage

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"agent-royo-learn/internal/testutil"
)

// TestMigrate_009_Forward applies migration 009 against a fresh SQLite
// database and asserts the publication_drift_state table exists with
// the documented columns, the CHECK constraints, the FK to publications,
// and the three indexes.
func TestMigrate_009_Forward(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// The publication_drift_state table must exist.
	var tableExists int
	if err := db.DB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='publication_drift_state'",
	).Scan(&tableExists); err != nil {
		t.Fatalf("sqlite_master query for publication_drift_state: %v", err)
	}
	if tableExists != 1 {
		t.Fatal("publication_drift_state table missing after migration 009")
	}

	// Required columns. PRIMARY KEY columns report NOT NULL=0 here even
	// though PK implies NOT NULL at the SQLite level — we only assert
	// NOT NULL for non-PK columns. publication_id has pk=1 (first
	// column of composite PK); target_path has pk=2 (second column).
	wantCols := map[string]struct {
		wantNotNull bool
		dflt        string
		pk          int
	}{
		"publication_id": {wantNotNull: true, pk: 1},
		"source":         {wantNotNull: true, pk: 0},
		"target_path":    {wantNotNull: true, pk: 2},
		"expected_hash":  {wantNotNull: true, pk: 0},
		"actual_hash":    {wantNotNull: true, dflt: "", pk: 0},
		"status":         {wantNotNull: true, pk: 0},
		"checked_at":     {wantNotNull: true, pk: 0},
		"run_id":         {wantNotNull: true, pk: 0},
	}
	for col, want := range wantCols {
		var (
			notNull, pk int
			dfltValue   *string
		)
		err := db.DB.QueryRow(
			"SELECT \"notnull\", dflt_value, pk FROM pragma_table_info('publication_drift_state') WHERE name = ?",
			col,
		).Scan(&notNull, &dfltValue, &pk)
		if err != nil {
			t.Fatalf("pragma_table_info query for publication_drift_state.%s: %v", col, err)
		}
		gotNotNull := notNull == 1
		if gotNotNull != want.wantNotNull {
			t.Errorf("publication_drift_state.%s NOT NULL = %v, want %v", col, gotNotNull, want.wantNotNull)
		}
		if pk != want.pk {
			t.Errorf("publication_drift_state.%s PK = %d, want %d", col, pk, want.pk)
		}
		if want.dflt != "" {
			if dfltValue == nil || *dfltValue != want.dflt {
				var got string
				if dfltValue != nil {
					got = *dfltValue
				}
				t.Errorf("publication_drift_state.%s DEFAULT = %q, want %q", col, got, want.dflt)
			}
		}
	}

	// Composite PRIMARY KEY (publication_id, target_path): publication_id
	// holds pk=1 (first column of the PK) and target_path holds pk=2
	// (second column). This subtest is a belt-and-suspenders check on
	// top of the per-column assertion in the wantCols loop above.
	for _, c := range []struct {
		col    string
		wantPK int
	}{
		{"publication_id", 1},
		{"target_path", 2},
	} {
		var pk int
		if err := db.DB.QueryRow(
			"SELECT pk FROM pragma_table_info('publication_drift_state') WHERE name = ?",
			c.col,
		).Scan(&pk); err != nil {
			t.Fatalf("pragma_table_info query for %s.pk: %v", c.col, err)
		}
		if pk != c.wantPK {
			t.Errorf("publication_drift_state.%s PK = %d, want %d (composite PK column)", c.col, pk, c.wantPK)
		}
	}

	// The CHECK constraint on status must accept the four documented
	// enum values and reject any other value. We assert the literal
	// regex used by the migration so a refactor that broadens or narrows
	// the enum is caught at test time.
	checkStatusSQL := readCheckConstraint(t, db, "publication_drift_state", "status")
	for _, want := range []string{"ok", "drifted", "target_missing", "target_unreadable"} {
		if !strings.Contains(checkStatusSQL, "'"+want+"'") {
			t.Errorf("CHECK on status does not mention %q in: %s", want, checkStatusSQL)
		}
	}
	for _, banned := range []string{"corrupted", "in_progress", "published"} {
		if strings.Contains(checkStatusSQL, "'"+banned+"'") {
			t.Errorf("CHECK on status mentions banned value %q in: %s", banned, checkStatusSQL)
		}
	}

	// The CHECK constraint on source must accept the three documented
	// adapter values.
	checkSourceSQL := readCheckConstraint(t, db, "publication_drift_state", "source")
	for _, want := range []string{"opencode", "claudecode", "codex"} {
		if !strings.Contains(checkSourceSQL, "'"+want+"'") {
			t.Errorf("CHECK on source does not mention %q in: %s", want, checkSourceSQL)
		}
	}

	// The three indexes must exist.
	for _, idx := range []string{
		"idx_drift_status_checked",
		"idx_drift_run_id",
		"idx_drift_publication",
	} {
		var n int
		if err := db.DB.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?",
			idx,
		).Scan(&n); err != nil {
			t.Fatalf("sqlite_master query for index %q: %v", idx, err)
		}
		if n != 1 {
			t.Errorf("index %q missing after migration 009", idx)
		}
	}

	// The schema_migrations row proves the runner accepted 009.
	var appliedName string
	if err := db.DB.QueryRow(
		"SELECT name FROM schema_migrations WHERE version = 9",
	).Scan(&appliedName); err != nil {
		t.Fatalf("schema_migrations lookup for version 9: %v", err)
	}
	if appliedName != "009_publication_drift" {
		t.Errorf("schema_migrations name for version 9 = %q, want 009_publication_drift", appliedName)
	}
}

// TestMigrate_009_Idempotent calls Migrate twice on the same DB. The
// second call must be a no-op: no error, no extra rows in
// schema_migrations, the schema is unchanged.
func TestMigrate_009_Idempotent(t *testing.T) {
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

	var count int
	if err := db.DB.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations WHERE version = 9",
	).Scan(&count); err != nil {
		t.Fatalf("schema_migrations count for version 9: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_migrations rows for version 9 = %d, want 1", count)
	}

	// Schema unchanged: the table and the three indexes are still
	// present after the second Migrate call.
	var tableExists int
	if err := db.DB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='publication_drift_state'",
	).Scan(&tableExists); err != nil {
		t.Fatalf("sqlite_master query (second pass): %v", err)
	}
	if tableExists != 1 {
		t.Error("publication_drift_state table missing after second Migrate call")
	}
	for _, idx := range []string{
		"idx_drift_status_checked",
		"idx_drift_run_id",
		"idx_drift_publication",
	} {
		var n int
		if err := db.DB.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?",
			idx,
		).Scan(&n); err != nil {
			t.Fatalf("sqlite_master query for index %q (second pass): %v", idx, err)
		}
		if n != 1 {
			t.Errorf("index %q missing after second Migrate call", idx)
		}
	}
}

// TestMigrate_009_ChecksumStable verifies that the runner stores a
// SHA-256 of 009 that matches the embedded file's actual checksum. This
// is the operator's safety net: if anyone edits 009 after it ships, the
// runner refuses to apply a divergent copy on a subsequent Migrate call.
func TestMigrate_009_ChecksumStable(t *testing.T) {
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
		"SELECT checksum FROM schema_migrations WHERE version = 9",
	).Scan(&stored); err != nil {
		t.Fatalf("schema_migrations read for version 9: %v", err)
	}
	if stored == "" {
		t.Fatal("schema_migrations checksum for version 9 is empty")
	}

	data, readErr := migrationsFS.ReadFile("migrations/009_publication_drift.sql")
	if readErr != nil {
		t.Fatalf("read 009_publication_drift.sql: %v", readErr)
	}
	want := fmt.Sprintf("%x", sha256.Sum256(data))
	if stored != want {
		t.Errorf("stored checksum for 009 = %s, want %s", stored, want)
	}
}

// TestMigrate_009_CheckConstraintRejectsUnknownStatus asserts that the
// CHECK constraint on publication_drift_state.status rejects a value
// outside the four enum strings. We seed a valid publications row so the
// FK on publication_id does not pre-empt the CHECK violation.
func TestMigrate_009_CheckConstraintRejectsUnknownStatus(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Seed a minimal publications row so the FK on publication_id does
	// not pre-empt the CHECK violation. We need at least one
	// (id, learning_id, preview_hash, targets_json, verification_json,
	// rollback_json, status, started_at) tuple to satisfy the FK.
	seedMinimalPublications(t, db)

	// Attempt to insert with status='corrupted' — must fail with a
	// CHECK violation. modernc.org/sqlite surfaces CHECK errors as
	// plain text containing "CHECK constraint failed".
	_, err = db.DB.Exec(`
		INSERT INTO publication_drift_state
			(publication_id, source, target_path, expected_hash, actual_hash, status, checked_at, run_id)
		VALUES (?, 'opencode', '/tmp/foo', 'deadbeef', 'deadbeef', 'corrupted', '2026-01-01T00:00:00Z', 'run-x')
	`, "01HZXPUB0000000000000000000")
	if err == nil {
		t.Fatal("expected CHECK violation on status='corrupted', got nil")
	}
	if !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("expected error to mention CHECK constraint, got: %v", err)
	}
}

// TestMigrate_009_CheckConstraintRejectsUnknownSource asserts that the
// CHECK constraint on publication_drift_state.source rejects a value
// outside the three documented adapter names.
func TestMigrate_009_CheckConstraintRejectsUnknownSource(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	seedMinimalPublications(t, db)

	_, err = db.DB.Exec(`
		INSERT INTO publication_drift_state
			(publication_id, source, target_path, expected_hash, actual_hash, status, checked_at, run_id)
		VALUES (?, 'bogus', '/tmp/foo', 'deadbeef', 'deadbeef', 'ok', '2026-01-01T00:00:00Z', 'run-x')
	`, "01HZXPUB0000000000000000000")
	if err == nil {
		t.Fatal("expected CHECK violation on source='bogus', got nil")
	}
	if !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("expected error to mention CHECK constraint, got: %v", err)
	}
}

// readCheckConstraint returns the CHECK constraint SQL fragment for the
// given (table, column). It walks sqlite_master looking for a CHECK that
// mentions the column name.
func readCheckConstraint(t *testing.T, db *DB, table, column string) string {
	t.Helper()
	rows, err := db.DB.Query(
		"SELECT sql FROM sqlite_master WHERE type='table' AND name=?",
		table,
	)
	if err != nil {
		t.Fatalf("sqlite_master query for %s: %v", table, err)
	}
	defer rows.Close()
	var sql string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan sqlite_master.sql: %v", err)
		}
		sql = s
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if sql == "" {
		t.Fatalf("no CREATE TABLE SQL for %s", table)
	}
	// The CHECK constraints appear inline in the CREATE TABLE statement
	// because the migration uses column-level CHECK (no separate
	// table-level CHECK objects in sqlite_master). We return the entire
	// CREATE TABLE SQL and let the caller pattern-match.
	return sql
}

// seedMinimalPublications inserts the minimum publications row required
// to satisfy the FK on publication_drift_state.publication_id during the
// CHECK violation tests. We do not need the row to be otherwise valid
// for the drift logic — we only need it to exist so the FK is not the
// first failure.
func seedMinimalPublications(t *testing.T, db *DB) {
	t.Helper()

	// We need a learning row first (publications.learning_id has a FK to
	// learnings(id)). And learnings needs a project (project_id FK). And
	// projects is self-contained.
	if _, err := db.DB.Exec(`
		INSERT INTO projects (id, project_key, display_name, canonical_path, fingerprint, created_at, updated_at)
		VALUES ('proj-drift-001', 'drift-proj-001', 'Drift Test', '/tmp/drift-proj', 'fp-drift-001', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("seed projects: %v", err)
	}
	if _, err := db.DB.Exec(`
		INSERT INTO learnings (id, project_id, status, type, title, context, observation,
			reusable_lesson, scope_guess, confidence, evidence_level, fingerprint, normalized_hash,
			actor_json, created_at, updated_at)
		VALUES ('learn-drift-001', 'proj-drift-001', 'approved', 'pattern', 'Drift seed',
			'ctx', 'obs', 'lesson', 'project', 'medium', 'observed', 'fp-learn-drift-001',
			'nh-learn-drift-001', '{}', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("seed learnings: %v", err)
	}
	if _, err := db.DB.Exec(`
		INSERT INTO publications (id, learning_id, preview_hash, targets_json, verification_json,
			rollback_json, status, started_at)
		VALUES ('01HZXPUB0000000000000000000', 'learn-drift-001', 'prev-hash-drift-001', '[]', '[]', '[]',
			'completed', '2026-01-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("seed publications: %v", err)
	}
}

// silenceUnusedImport keeps errors in the test file's import set quiet if
// a future refactor drops the only reference to the imported symbol.
// (no-op placeholder; remove when no imports are unused)
var _ = fmt.Sprintf
