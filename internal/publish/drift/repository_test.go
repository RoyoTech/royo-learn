// Tests for Repository.RecordDrift and Repository.ListDrift
// (Hito 12, T12.2). The repository is the per-row upsert layer between
// the Checker (T12.3) and the publication_drift_state table introduced
// by migration 009 (T12.1).

package drift

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/testutil"
)

// fixedNow returns a deterministic clock for tests so checked_at values
// are reproducible.
func fixedNow() time.Time {
	return time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
}

// openTestDB creates a fresh SQLite database with migration 009 applied
// and returns the open *storage.DB plus a cleanup func.
func openTestDB(t *testing.T) (*storage.DB, func()) {
	t.Helper()
	dir := testutil.TempDir(t)
	db, err := storage.Open(filepath.Join(dir, "drift.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	if err := storage.Migrate(db); err != nil {
		db.Close()
		t.Fatalf("storage.Migrate: %v", err)
	}
	seedPublication(t, db, "01HZXPUB0000000000000000000")
	return db, func() { db.Close() }
}

// seedPublication inserts the minimum publications row required by the FK
// on publication_drift_state.publication_id.
func seedPublication(t *testing.T, db *storage.DB, id string) {
	t.Helper()
	if _, err := db.DB.Exec(`
		INSERT INTO projects (id, project_key, display_name, canonical_path, fingerprint, created_at, updated_at)
		VALUES ('proj-drift-repo', 'drift-repo', 'Drift Repo', '/tmp/dr', 'fp-dr', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("seed projects: %v", err)
	}
	if _, err := db.DB.Exec(`
		INSERT INTO learnings (id, project_id, status, type, title, context, observation,
			reusable_lesson, scope_guess, confidence, evidence_level, fingerprint, normalized_hash,
			actor_json, created_at, updated_at)
		VALUES ('learn-drift-repo', 'proj-drift-repo', 'approved', 'pattern', 'Drift Repo Seed',
			'ctx', 'obs', 'lesson', 'project', 'medium', 'observed', 'fp-ldr',
			'nh-ldr', '{}', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("seed learnings: %v", err)
	}
	if _, err := db.DB.Exec(`
		INSERT INTO publications (id, learning_id, preview_hash, targets_json, verification_json,
			rollback_json, status, started_at)
		VALUES (?, 'learn-drift-repo', 'prev-dr', '[]', '[]', '[]', 'completed', '2026-01-01T00:00:00Z')
	`, id); err != nil {
		t.Fatalf("seed publications: %v", err)
	}
}

// TestRecordDrift_RoundTrip inserts one row, reads it back via ListDrift,
// and asserts every column matches the input.
func TestRecordDrift_RoundTrip(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	repo := NewRepository(db.DB, fixedNow)
	row := DriftRow{
		PublicationID: "01HZXPUB0000000000000000000",
		Source:        "opencode",
		TargetPath:    "/var/data/x.jsonl",
		ExpectedHash:  "aaaa1111",
		ActualHash:    "bbbb2222",
		Status:        StatusDrifted,
		RunID:         "run-001",
	}
	if err := repo.RecordDrift(context.Background(), row); err != nil {
		t.Fatalf("RecordDrift: %v", err)
	}

	got, err := repo.ListDrift(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListDrift: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListDrift returned %d rows, want 1", len(got))
	}
	g := got[0]
	if g.PublicationID != row.PublicationID {
		t.Errorf("PublicationID = %q, want %q", g.PublicationID, row.PublicationID)
	}
	if g.Source != row.Source {
		t.Errorf("Source = %q, want %q", g.Source, row.Source)
	}
	if g.TargetPath != row.TargetPath {
		t.Errorf("TargetPath = %q, want %q", g.TargetPath, row.TargetPath)
	}
	if g.ExpectedHash != row.ExpectedHash {
		t.Errorf("ExpectedHash = %q, want %q", g.ExpectedHash, row.ExpectedHash)
	}
	if g.ActualHash != row.ActualHash {
		t.Errorf("ActualHash = %q, want %q", g.ActualHash, row.ActualHash)
	}
	if g.Status != row.Status {
		t.Errorf("Status = %q, want %q", g.Status, row.Status)
	}
	if g.RunID != row.RunID {
		t.Errorf("RunID = %q, want %q", g.RunID, row.RunID)
	}
	if !g.CheckedAt.Equal(fixedNow()) {
		t.Errorf("CheckedAt = %v, want %v", g.CheckedAt, fixedNow())
	}
}

// TestRecordDrift_UpsertOnConflict inserts a row, then inserts a second
// row with the same (publication_id, target_path) but different hashes
// and status. The composite PRIMARY KEY triggers ON CONFLICT DO UPDATE,
// so only one row exists after the second insert and the row reflects
// the second insert's values.
func TestRecordDrift_UpsertOnConflict(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	repo := NewRepository(db.DB, fixedNow)
	first := DriftRow{
		PublicationID: "01HZXPUB0000000000000000000",
		Source:        "opencode",
		TargetPath:    "/var/data/y.jsonl",
		ExpectedHash:  "first-expected",
		ActualHash:    "first-actual",
		Status:        StatusOK,
		RunID:         "run-001",
	}
	if err := repo.RecordDrift(context.Background(), first); err != nil {
		t.Fatalf("RecordDrift (first): %v", err)
	}
	second := first
	second.ExpectedHash = "second-expected"
	second.ActualHash = "second-actual"
	second.Status = StatusDrifted
	second.RunID = "run-002"
	if err := repo.RecordDrift(context.Background(), second); err != nil {
		t.Fatalf("RecordDrift (second): %v", err)
	}

	got, err := repo.ListDrift(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListDrift: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListDrift returned %d rows, want 1 (upsert should overwrite, not duplicate)", len(got))
	}
	g := got[0]
	if g.ExpectedHash != "second-expected" {
		t.Errorf("ExpectedHash = %q, want second-expected", g.ExpectedHash)
	}
	if g.ActualHash != "second-actual" {
		t.Errorf("ActualHash = %q, want second-actual", g.ActualHash)
	}
	if g.Status != StatusDrifted {
		t.Errorf("Status = %q, want %q", g.Status, StatusDrifted)
	}
	if g.RunID != "run-002" {
		t.Errorf("RunID = %q, want run-002", g.RunID)
	}
}

// TestRecordDrift_RejectsUnknownStatus attempts to insert a row with
// status='corrupted' (outside the four enum values). The CHECK
// constraint must reject the insert with a non-nil error.
func TestRecordDrift_RejectsUnknownStatus(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	repo := NewRepository(db.DB, fixedNow)
	row := DriftRow{
		PublicationID: "01HZXPUB0000000000000000000",
		Source:        "opencode",
		TargetPath:    "/var/data/z.jsonl",
		ExpectedHash:  "e",
		ActualHash:    "a",
		Status:        Status("corrupted"), // outside the four-value enum
		RunID:         "run-x",
	}
	err := repo.RecordDrift(context.Background(), row)
	if err == nil {
		t.Fatal("RecordDrift: expected CHECK violation, got nil")
	}
	if !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("RecordDrift error = %v, want it to mention CHECK", err)
	}
}

// TestListDrift_FilterBySource inserts three rows with different sources
// and asserts ListDrift with ListFilter{Source: "claudecode"} returns
// only the matching rows.
func TestListDrift_FilterBySource(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	repo := NewRepository(db.DB, fixedNow)
	rows := []DriftRow{
		{PublicationID: "01HZXPUB0000000000000000000", Source: "opencode", TargetPath: "/a", Status: StatusOK, RunID: "r1", CheckedAt: fixedNow()},
		{PublicationID: "01HZXPUB0000000000000000000", Source: "claudecode", TargetPath: "/b", Status: StatusDrifted, RunID: "r1", CheckedAt: fixedNow()},
		{PublicationID: "01HZXPUB0000000000000000000", Source: "codex", TargetPath: "/c", Status: StatusTargetMissing, RunID: "r1", CheckedAt: fixedNow()},
	}
	for _, r := range rows {
		if err := repo.RecordDrift(context.Background(), r); err != nil {
			t.Fatalf("RecordDrift(%v): %v", r.TargetPath, err)
		}
	}

	got, err := repo.ListDrift(context.Background(), ListFilter{Source: "claudecode"})
	if err != nil {
		t.Fatalf("ListDrift: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListDrift returned %d rows, want 1 (filter by source)", len(got))
	}
	if got[0].Source != "claudecode" {
		t.Errorf("Source = %q, want claudecode", got[0].Source)
	}
	if got[0].Status != StatusDrifted {
		t.Errorf("Status = %q, want %q", got[0].Status, StatusDrifted)
	}
}

// TestListDrift_FilterByRunID inserts four rows across two run_ids and
// asserts the run_id filter narrows the result to that run.
func TestListDrift_FilterByRunID(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	repo := NewRepository(db.DB, fixedNow)
	rows := []DriftRow{
		{PublicationID: "01HZXPUB0000000000000000000", Source: "opencode", TargetPath: "/p1", Status: StatusOK, RunID: "run-A", CheckedAt: fixedNow()},
		{PublicationID: "01HZXPUB0000000000000000000", Source: "opencode", TargetPath: "/p2", Status: StatusOK, RunID: "run-A", CheckedAt: fixedNow()},
		{PublicationID: "01HZXPUB0000000000000000000", Source: "opencode", TargetPath: "/p3", Status: StatusOK, RunID: "run-B", CheckedAt: fixedNow()},
		{PublicationID: "01HZXPUB0000000000000000000", Source: "opencode", TargetPath: "/p4", Status: StatusOK, RunID: "run-B", CheckedAt: fixedNow()},
	}
	for _, r := range rows {
		if err := repo.RecordDrift(context.Background(), r); err != nil {
			t.Fatalf("RecordDrift(%v): %v", r.TargetPath, err)
		}
	}

	got, err := repo.ListDrift(context.Background(), ListFilter{RunID: "run-A"})
	if err != nil {
		t.Fatalf("ListDrift: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListDrift returned %d rows, want 2 (filter by run-A)", len(got))
	}
	for _, g := range got {
		if g.RunID != "run-A" {
			t.Errorf("RunID = %q, want run-A", g.RunID)
		}
	}
}

// TestCountByStatus_AllFour inserts one row per outcome and asserts the
// per-status counts match.
func TestCountByStatus_AllFour(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	repo := NewRepository(db.DB, fixedNow)
	rows := []DriftRow{
		{PublicationID: "01HZXPUB0000000000000000000", Source: "opencode", TargetPath: "/a", Status: StatusOK, RunID: "r"},
		{PublicationID: "01HZXPUB0000000000000000000", Source: "opencode", TargetPath: "/b", Status: StatusDrifted, RunID: "r"},
		{PublicationID: "01HZXPUB0000000000000000000", Source: "opencode", TargetPath: "/c", Status: StatusTargetMissing, RunID: "r"},
		{PublicationID: "01HZXPUB0000000000000000000", Source: "opencode", TargetPath: "/d", Status: StatusTargetUnreadable, RunID: "r"},
	}
	for _, r := range rows {
		if err := repo.RecordDrift(context.Background(), r); err != nil {
			t.Fatalf("RecordDrift(%v): %v", r.TargetPath, err)
		}
	}

	counts, err := repo.CountByStatus(context.Background(), ListFilter{RunID: "r"})
	if err != nil {
		t.Fatalf("CountByStatus: %v", err)
	}
	want := map[Status]int{
		StatusOK:               1,
		StatusDrifted:          1,
		StatusTargetMissing:    1,
		StatusTargetUnreadable: 1,
	}
	for k, v := range want {
		if counts[k] != v {
			t.Errorf("counts[%q] = %d, want %d", k, counts[k], v)
		}
	}
}

// TestRecordDrift_DefaultCheckedAt asserts that omitting CheckedAt on
// the input row makes the Repository stamp the row with the injectable
// clock's value.
func TestRecordDrift_DefaultCheckedAt(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	repo := NewRepository(db.DB, fixedNow)
	row := DriftRow{
		PublicationID: "01HZXPUB0000000000000000000",
		Source:        "opencode",
		TargetPath:    "/var/data/default-time.jsonl",
		ExpectedHash:  "e",
		ActualHash:    "a",
		Status:        StatusOK,
		RunID:         "r",
		// CheckedAt deliberately zero
	}
	if err := repo.RecordDrift(context.Background(), row); err != nil {
		t.Fatalf("RecordDrift: %v", err)
	}

	got, err := repo.ListDrift(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListDrift: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListDrift returned %d rows, want 1", len(got))
	}
	if !got[0].CheckedAt.Equal(fixedNow()) {
		t.Errorf("CheckedAt = %v, want %v (zero value should use injected clock)", got[0].CheckedAt, fixedNow())
	}
}

// TestNewRepository_NilNowFn asserts the production path that passes
// nil to NewRepository falls back to time.Now.UTC.
func TestNewRepository_NilNowFn(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	repo := NewRepository(db.DB, nil)
	if repo == nil {
		t.Fatal("NewRepository returned nil")
	}
	// Inject a row with CheckedAt=zero; the default nowFn should stamp it.
	row := DriftRow{
		PublicationID: "01HZXPUB0000000000000000000",
		Source:        "opencode",
		TargetPath:    "/var/data/nil-clock.jsonl",
		ExpectedHash:  "e",
		ActualHash:    "a",
		Status:        StatusOK,
		RunID:         "r",
	}
	before := time.Now().UTC().Add(-1 * time.Second)
	if err := repo.RecordDrift(context.Background(), row); err != nil {
		t.Fatalf("RecordDrift: %v", err)
	}
	after := time.Now().UTC().Add(1 * time.Second)

	got, err := repo.ListDrift(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListDrift: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListDrift returned %d rows, want 1", len(got))
	}
	if got[0].CheckedAt.Before(before) || got[0].CheckedAt.After(after) {
		t.Errorf("CheckedAt = %v, want a value between %v and %v (time.Now fallback)", got[0].CheckedAt, before, after)
	}
}
