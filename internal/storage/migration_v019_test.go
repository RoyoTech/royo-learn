package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/testutil"

	"github.com/google/uuid"
)

// buildV019Base produces a database whose schema is exactly the one royo-learn
// shipped at tag v0.1.9: migrations 001_init and 002_recurrence applied, and
// nothing else. Those two files are byte-identical between v0.1.9 and HEAD
// (verified in-repo), so applying them through the real migration runner — same
// SQL, same checksum recording — reconstructs the genuine v0.1.9 base rather
// than a hand-built fake.
func buildV019Base(t *testing.T, conn *sql.DB) {
	t.Helper()
	files, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, name TEXT NOT NULL, checksum TEXT NOT NULL, applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	applied := 0
	for _, m := range files {
		if m.Version > 2 {
			continue // v0.1.9 shipped only 001 and 002.
		}
		if err := applyMigration(conn, m); err != nil {
			t.Fatalf("apply v0.1.9 migration %d: %v", m.Version, err)
		}
		applied++
	}
	if applied != 2 {
		t.Fatalf("v0.1.9 base should apply exactly 2 migrations, applied %d", applied)
	}
}

func newV019FileDB(t *testing.T) (*DB, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "v019-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	path := filepath.Join(dir, "royo-learn.db")
	db, err := Open(path)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_ = testutil.RemoveAllWithRetry(dir)
	})
	buildV019Base(t, db.DB)
	return db, dir
}

func countRows(t *testing.T, db *DB, table string) int {
	t.Helper()
	var n int
	if err := db.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func appliedVersions(t *testing.T, db *DB) []int {
	t.Helper()
	rows, err := db.DB.Query("SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan version: %v", err)
		}
		out = append(out, v)
	}
	return out
}

// TestMigrate_FromRealV019Base runs the full migration chain on a genuine v0.1.9
// database and asserts the three §4.8 guarantees: no data loss, old records land
// in a safe non-approved state, and an idempotent re-run is a no-op.
func TestMigrate_FromRealV019Base(t *testing.T) {
	db, _ := newV019FileDB(t)
	ctx := context.Background()

	// The v0.1.9 base must NOT yet have the later schema.
	if hasColumn(t, db, "recurrence_records", "outcome") {
		t.Fatal("v0.1.9 base already has migration 003 columns; base is not authentic")
	}
	if hasColumn(t, db, "learning_relations", "status") {
		t.Fatal("v0.1.9 base already has migration 004 columns; base is not authentic")
	}

	// Seed real v0.1.9 data: a project, and learnings in pre-approval states.
	now := time.Now().UTC().Truncate(time.Second)
	proj := &domain.Project{
		ID: domain.ProjectID(uuid.Must(uuid.NewV7()).String()), ProjectKey: "v019", DisplayName: "v019",
		CanonicalPath: "/tmp/v019", Fingerprint: "fp", CreatedAt: now, UpdatedAt: now,
	}
	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveProject(ctx, tx, proj); err != nil {
		tx.Rollback()
		t.Fatalf("SaveProject: %v", err)
	}
	captured := newTestLearning(proj.ID)
	captured.Status = domain.StatusCaptured
	captured.Title = "old captured"
	if err := SaveLearning(ctx, tx, captured); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning captured: %v", err)
	}
	needsEvidence := newTestLearning(proj.ID)
	needsEvidence.Status = domain.StatusNeedsEvidence
	needsEvidence.Title = "old needs-evidence"
	needsEvidence.NormalizedHash = "hash002"
	needsEvidence.Fingerprint = "fp-learn-002"
	if err := SaveLearning(ctx, tx, needsEvidence); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning needsEvidence: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit seed: %v", err)
	}

	learningsBefore := countRows(t, db, "learnings")

	// Run the full chain (001..latest) on the v0.1.9 base.
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate from v0.1.9: %v", err)
	}

	// (a) The chain advanced to the latest version, additively.
	got := appliedVersions(t, db)
	if len(got) < 4 || got[0] != 1 || got[1] != 2 || got[2] != 3 || got[3] != 4 {
		t.Fatalf("applied versions after upgrade = %v, want at least 1,2,3,4", got)
	}
	if !hasColumn(t, db, "recurrence_records", "outcome") {
		t.Fatal("migration 003 did not add the occurrence columns")
	}
	if !hasColumn(t, db, "learning_relations", "status") {
		t.Fatal("migration 004 did not add the relation lifecycle columns")
	}

	// (b) NO DATA LOSS: every seeded learning survived.
	if after := countRows(t, db, "learnings"); after != learningsBefore {
		t.Fatalf("learning count changed across migration: before=%d after=%d", learningsBefore, after)
	}

	// (c) OLD RECORDS ARE NOT AUTO-APPROVED: statuses are untouched, and no
	// migration fabricated an approval or a publication.
	assertStatus(t, db, captured.ID, string(domain.StatusCaptured))
	assertStatus(t, db, needsEvidence.ID, string(domain.StatusNeedsEvidence))
	var approvedCount int
	if err := db.DB.QueryRow(
		"SELECT COUNT(*) FROM learnings WHERE status IN ('approved','published')").Scan(&approvedCount); err != nil {
		t.Fatalf("count approved/published: %v", err)
	}
	if approvedCount != 0 {
		t.Fatalf("migration elevated %d learning(s) into an approved/published state", approvedCount)
	}
	if n := countRows(t, db, "approvals"); n != 0 {
		t.Fatalf("migration fabricated %d approval(s)", n)
	}
	if n := countRows(t, db, "publications"); n != 0 {
		t.Fatalf("migration fabricated %d publication(s)", n)
	}

	// A backup of the pre-upgrade store must have been written (plan 4.8).
	backups, _ := filepath.Glob(filepath.Join(filepath.Dir(db.path), "backups", "pre-migration-*.db"))
	if len(backups) == 0 {
		t.Fatal("no pre-migration backup was written before the upgrade")
	}

	// (d) IDEMPOTENT RE-RUN: a second Migrate is a no-op.
	versionsBefore := appliedVersions(t, db)
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	versionsAfter := appliedVersions(t, db)
	if len(versionsBefore) != len(versionsAfter) {
		t.Fatalf("re-run changed applied migrations: %v -> %v", versionsBefore, versionsAfter)
	}
	if after := countRows(t, db, "learnings"); after != learningsBefore {
		t.Fatalf("re-run changed learning count: %d", after)
	}
}

func assertStatus(t *testing.T, db *DB, id domain.LearningID, want string) {
	t.Helper()
	var got string
	if err := db.DB.QueryRow("SELECT status FROM learnings WHERE id = ?", string(id)).Scan(&got); err != nil {
		t.Fatalf("read status for %s: %v", id, err)
	}
	if got != want {
		t.Fatalf("learning %s status = %q, want %q (migration must not change it)", id, got, want)
	}
}

func hasColumn(t *testing.T, db *DB, table, column string) bool {
	t.Helper()
	rows, err := db.DB.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("table_info %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}
