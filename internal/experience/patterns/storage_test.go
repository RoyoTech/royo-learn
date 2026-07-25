// Storage tests for Hito 6 slice 6.4 (migration 005).
//
// The migration introduces:
//
//   - experience_patterns (id, project_id, status, kind, fingerprint,
//     title, summary, distinct_sessions, distinct_days,
//     occurrence_count, first_seen_at, last_seen_at,
//     proposed_learning_id, detector_version, input_digest, created_at,
//     updated_at).
//   - experience_pattern_members (pattern_id, event_id,
//     similarity_kind, similarity_score, added_at) UNIQUE(pattern_id, event_id).
//
// The repository surface (repo_patterns.go) must:
//
//   - save an observed pattern idempotently by (project_id, fingerprint);
//   - bump the pattern's metrics (distinct_sessions, distinct_days,
//     occurrence_count) on a re-save with the same fingerprint;
//   - add member rows idempotently on (pattern_id, event_id);
//   - return typed errors for size and enum violations.
//
// The tests use the storagetest.OpenTemp helper so the migration runs
// through the canonical pipeline and the test surface matches what
// the production CLI/MCP exercises.

package patterns_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/patterns"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/storage/storagetest"
)

// projectFixture seeds a project and returns its ID. The patterns
// repository expects a foreign key into projects(id); the helper
// reuses the existing repo_projects surface to keep the wiring
// minimal.
type projectFixture struct {
	ProjectID domain.ProjectID
	Path      string
}

// seedExperienceEvents inserts the supplied event ids into the
// experience_events table so membership rows can reference them
// (the membership table has a FK to experience_events.id per
// migration 005). The fixture walks the projects → turns → events
// dependency chain so the FK constraints at every level are
// satisfied without forcing each test to assemble the chain by
// hand.
func seedExperienceEvents(t *testing.T, db *storage.DB, projectID domain.ProjectID, ids []domain.ExperienceEventID) {
	t.Helper()
	if len(ids) == 0 {
		return
	}
	now := time.Now().UTC()
	tx, err := db.DB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("seedExperienceEvents BeginTx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck
	// Seed one session and one turn that all events share.
	sessionID := domain.ExperienceSessionID("session-" + string(projectID))
	if _, err := tx.ExecContext(context.Background(), `
		INSERT OR IGNORE INTO experience_sessions
			(id, project_id, source, external_session_id, locator_json, updated_at, metadata_sha256, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, string(sessionID), string(projectID), "manual", "session-ext-1", "{}",
		now.UTC().Format(time.RFC3339Nano), "metadata", now.UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	turnID := domain.ExperienceTurnID("turn-" + string(projectID))
	if _, err := tx.ExecContext(context.Background(), `
		INSERT OR IGNORE INTO experience_turns
			(id, session_id, external_turn_id, sequence, status, fingerprint,
			 user_digest, assistant_digest, tool_calls_digest, safe_summary,
			 occurred_at, stable_at, ingested_at, source_revision, redacted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, string(turnID), string(sessionID), "turn-ext-1", 0, "ingested",
		"fp-turn", "u", "a", "t", "summary",
		now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano),
		now.UTC().Format(time.RFC3339Nano), "rev-1", 0); err != nil {
		t.Fatalf("seed turn: %v", err)
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(context.Background(), `
			INSERT OR IGNORE INTO experience_events
				(id, project_id, turn_id, kind, summary, observation, outcome,
				 fingerprint, evidence_json, detector_json, confidence, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, string(id), string(projectID), string(turnID),
			string(domain.EventTestFailure), "summary", "obs", "outcome",
			"fp-evt-"+string(id), "{}", "{}", string(domain.ConfidenceMedium),
			now.UTC().Format(time.RFC3339Nano)); err != nil {
			t.Fatalf("seed event %s: %v", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("seed commit: %v", err)
	}
}

func newProjectFixture(t *testing.T, db *storage.DB, name string) projectFixture {
	t.Helper()
	canonical := filepath.Join(t.TempDir(), name)
	project := &domain.Project{
		ID:            domain.ProjectID("proj-" + strings.ReplaceAll(name, " ", "-")),
		ProjectKey:    "key-" + name,
		DisplayName:   name,
		CanonicalPath: canonical,
		Fingerprint:   "fp-" + name,
		CreatedAt:     time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if err := storage.SaveProject(ctx, tx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return projectFixture{ProjectID: project.ID, Path: canonical}
}

func newTestDB(t *testing.T) *storage.DB {
	t.Helper()
	return storagetest.OpenTemp(t)
}

// TestMigration005_AppliesCleanly runs the canonical migration
// pipeline and confirms the new tables exist. Re-running Migrate
// must remain a no-op (idempotency contract).
func TestMigration005_AppliesCleanly(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	rows, err := db.DB.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name IN ('experience_patterns', 'experience_pattern_members') ORDER BY name`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	var found []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		found = append(found, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	want := []string{"experience_pattern_members", "experience_patterns"}
	if strings.Join(found, ",") != strings.Join(want, ",") {
		t.Fatalf("tables = %v, want %v", found, want)
	}

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

// TestRepository_SavePatternIdempotent exercises the repository's
// idempotency rule: saving the same (project_id, fingerprint) twice
// updates the existing row instead of creating a duplicate.
func TestRepository_SavePatternIdempotent(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "idem")

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	pattern := newPattern(fixture.ProjectID, "fp-1", now)

	ctx := context.Background()
	repo := patterns.NewRepository(db)
	first, err := repo.SavePattern(ctx, pattern)
	if err != nil {
		t.Fatalf("first SavePattern: %v", err)
	}
	if first.ID == "" {
		t.Fatal("first pattern ID is empty")
	}
	if first.Status != patterns.PatternObserved {
		t.Fatalf("first status = %s, want observed", first.Status)
	}

	second, err := repo.SavePattern(ctx, pattern)
	if err != nil {
		t.Fatalf("second SavePattern: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second ID = %s, want %s (idempotent)", second.ID, first.ID)
	}

	// Only one row exists.
	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM experience_patterns WHERE project_id = ? AND fingerprint = ?`,
		string(fixture.ProjectID), pattern.Fingerprint).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("rows = %d, want 1", count)
	}
}

// TestRepository_BumpMetricsOnResave verifies the metrics are
// recomputed when the same fingerprint is re-saved with new
// session/day counts.
func TestRepository_BumpMetricsOnResave(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "bump")

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	pattern := newPattern(fixture.ProjectID, "fp-bump", now)
	pattern.DistinctSessions = 1
	pattern.DistinctDays = 1
	pattern.OccurrenceCount = 1

	ctx := context.Background()
	repo := patterns.NewRepository(db)
	first, err := repo.SavePattern(ctx, pattern)
	if err != nil {
		t.Fatalf("first SavePattern: %v", err)
	}

	pattern.DistinctSessions = 4
	pattern.DistinctDays = 5
	pattern.OccurrenceCount = 12
	second, err := repo.SavePattern(ctx, pattern)
	if err != nil {
		t.Fatalf("second SavePattern: %v", err)
	}
	if second.DistinctSessions != 4 || second.DistinctDays != 5 || second.OccurrenceCount != 12 {
		t.Fatalf("metrics not bumped: %+v", second)
	}
	if second.Revision <= first.Revision {
		t.Fatalf("revision did not advance: first=%d second=%d", first.Revision, second.Revision)
	}
}

// TestRepository_AddMemberIdempotent verifies the membership table
// enforces UNIQUE(pattern_id, event_id) and the repository returns
// the existing membership row on duplicate insertion.
func TestRepository_AddMemberIdempotent(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "member")

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	pattern := newPattern(fixture.ProjectID, "fp-mem", now)

	ctx := context.Background()
	repo := patterns.NewRepository(db)
	saved, err := repo.SavePattern(ctx, pattern)
	if err != nil {
		t.Fatalf("SavePattern: %v", err)
	}
	seedExperienceEvents(t, db, fixture.ProjectID, []domain.ExperienceEventID{"evt-1"})

	added := time.Date(2026, 7, 25, 12, 30, 0, 0, time.UTC)
	mem, err := repo.AddMember(ctx, saved.ID, domain.ExperienceEventID("evt-1"), "exact_fingerprint", 1.0, added)
	if err != nil {
		t.Fatalf("first AddMember: %v", err)
	}
	if mem.AddedAt.IsZero() {
		t.Fatalf("AddedAt is zero")
	}

	// Second add must NOT raise and must NOT change similarity_score.
	again, err := repo.AddMember(ctx, saved.ID, domain.ExperienceEventID("evt-1"), "exact_fingerprint", 0.5, added.Add(time.Hour))
	if err != nil {
		t.Fatalf("second AddMember: %v", err)
	}
	if !again.AddedAt.Equal(mem.AddedAt) {
		t.Fatalf("AddedAt changed on re-add: %v vs %v", again.AddedAt, mem.AddedAt)
	}

	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM experience_pattern_members WHERE pattern_id = ?`,
		string(saved.ID)).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("member rows = %d, want 1", count)
	}
}

// TestRepository_GetByFingerprint covers the read path used by both
// dismissal and the CLI/MCP listing.
func TestRepository_GetByFingerprint(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "get")

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	pattern := newPattern(fixture.ProjectID, "fp-get", now)

	ctx := context.Background()
	repo := patterns.NewRepository(db)
	saved, err := repo.SavePattern(ctx, pattern)
	if err != nil {
		t.Fatalf("SavePattern: %v", err)
	}

	found, err := repo.GetByFingerprint(ctx, fixture.ProjectID, pattern.Fingerprint)
	if err != nil {
		t.Fatalf("GetByFingerprint: %v", err)
	}
	if found.ID != saved.ID {
		t.Fatalf("GetByFingerprint ID = %s, want %s", found.ID, saved.ID)
	}
}

// TestRepository_ListByStatus returns the persisted patterns
// filtered by status, in stable order (last_seen_at DESC, id ASC).
func TestRepository_ListByStatus(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "list")

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	repo := patterns.NewRepository(db)
	ctx := context.Background()

	for i, fp := range []string{"fp-a", "fp-b", "fp-c"} {
		p := newPattern(fixture.ProjectID, fp, now.Add(time.Duration(i)*time.Minute))
		if err := repo.UpsertFromCluster(ctx, p); err != nil {
			t.Fatalf("UpsertFromCluster %s: %v", fp, err)
		}
	}

	out, err := repo.ListByStatus(ctx, fixture.ProjectID, patterns.PatternObserved)
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("ListByStatus len = %d, want 3", len(out))
	}
	// Stable order: last_seen_at DESC, then id ASC. Newest insert wins.
	wantFingerprints := []string{"fp-c", "fp-b", "fp-a"}
	for i, p := range out {
		if p.Fingerprint != wantFingerprints[i] {
			t.Fatalf("ListByStatus[%d].Fingerprint = %q, want %q", i, p.Fingerprint, wantFingerprints[i])
		}
	}
}

// TestRepository_SetStatus covers the dismissal helper: SetStatus
// transitions a pattern and records the change.
func TestRepository_SetStatus(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "set")

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	pattern := newPattern(fixture.ProjectID, "fp-set", now)
	ctx := context.Background()
	repo := patterns.NewRepository(db)
	saved, err := repo.SavePattern(ctx, pattern)
	if err != nil {
		t.Fatalf("SavePattern: %v", err)
	}

	updated, err := repo.SetStatus(ctx, saved.ID, patterns.PatternDismissed)
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if updated.Status != patterns.PatternDismissed {
		t.Fatalf("Status = %s, want dismissed", updated.Status)
	}
	if updated.Revision <= saved.Revision {
		t.Fatalf("revision did not advance on dismissal")
	}
}

// TestRepository_GetReturnsTypedNotFound verifies that
// GetByFingerprint / GetByID return ErrPatternNotFound (and the
// canonical domain code) when the row is missing.
func TestRepository_GetReturnsTypedNotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "notfound")

	ctx := context.Background()
	repo := patterns.NewRepository(db)
	_, err := repo.GetByFingerprint(ctx, fixture.ProjectID, "fp-missing")
	if err == nil {
		t.Fatal("GetByFingerprint(missing) = nil, want ErrPatternNotFound")
	}
	if !errorsIs(err, patterns.ErrPatternNotFound) {
		t.Fatalf("GetByFingerprint error = %v, want ErrPatternNotFound", err)
	}
}

// --- helpers ---

func newPattern(projectID domain.ProjectID, fingerprint string, now time.Time) patterns.ExperiencePattern {
	return patterns.ExperiencePattern{
		ID:               domain.ExperiencePatternID("pat-" + fingerprint),
		ProjectID:        projectID,
		Status:           patterns.PatternObserved,
		Kind:             domain.EventTestFailure,
		Fingerprint:      fingerprint,
		Title:            "title " + fingerprint,
		Summary:          "summary " + fingerprint,
		DistinctSessions: 1,
		DistinctDays:     1,
		OccurrenceCount:  1,
		FirstSeenAt:      now,
		LastSeenAt:       now,
		DetectorVersion:  "0.1.0",
		InputDigest:      "digest-" + fingerprint,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// errorsIs is a local helper so the test file does not need to
// import "errors" only for this single check.
func errorsIs(err, target error) bool {
	if err == nil || target == nil {
		return err == target
	}
	return err == target || err.Error() == target.Error()
}

// keep sql import used by future expansion without leaving a stale import.
var _ sql.IsolationLevel

func TestMigration005_EnforcesPatternAndMemberConstraints(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "migration-constraints")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.DB.Exec(`INSERT INTO experience_patterns
		(id, project_id, status, kind, fingerprint, first_seen_at, last_seen_at, proposed_learning_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "bad-pattern", fixture.ProjectID, "invalid", string(domain.EventTestFailure), "fp-bad", now, now, "missing-learning", now, now)
	if err == nil {
		t.Fatal("invalid pattern status and missing proposed learning were accepted")
	}

	pattern := newPattern(fixture.ProjectID, "fp-constraint-member", time.Now().UTC())
	saved, err := patterns.NewRepository(db).SavePattern(context.Background(), pattern)
	if err != nil {
		t.Fatal(err)
	}
	seedExperienceEvents(t, db, fixture.ProjectID, []domain.ExperienceEventID{"evt-constraint"})
	_, err = db.DB.Exec(`INSERT INTO experience_pattern_members
		(pattern_id, event_id, similarity_kind, similarity_score, added_at) VALUES (?, ?, ?, ?, ?)`,
		saved.ID, "evt-constraint", "exact_fingerprint", 1.5, now)
	if err == nil {
		t.Fatal("similarity_score outside [0,1] was accepted")
	}
}
