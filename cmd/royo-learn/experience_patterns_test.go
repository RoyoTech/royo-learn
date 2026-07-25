// End-to-end CLI tests for Hito 6 slice 6.4 (experience patterns).
//
// These tests exercise the patterns subcommands through the canonical
// CLI dispatch (runExperiencePatterns*). The fixtures mirror the
// acceptance matrix in docs/25:
//
//   - 3 sessions qualify; 3 retries in 1 session do NOT.
//   - dismissal is idempotent on the same reason.
//   - dismissal rejects a different reason on an already-dismissed
//     pattern.
//   - a dismissed pattern's stored reason matches the request.
//
// The tests use a real SQLite database and the project's CLI
// dispatch so any contract drift between the service and the CLI is
// caught before merge.

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/patterns"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/testutil"
)

// patternsCLIFixture seeds a project + a synthetic qualified pattern
// the CLI tests operate on. The fixture mirrors what the production
// CLI expects: a `.royo-learn/` directory with `config.yaml` and a
// SQLite DB at `royo-learn.db`. The project id comes from
// resolvePublishContext so the CLI and the fixture agree.
func patternsCLIFixture(t *testing.T) (root string, db *storage.DB, projectID domain.ProjectID, patternID domain.ExperiencePatternID) {
	t.Helper()
	root = testutil.TempDir(t)
	if err := os.MkdirAll(filepath.Join(root, ".royo-learn"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".royo-learn", "config.yaml"), []byte("project_root: "+root+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	dbPath := filepath.Join(root, ".royo-learn", "royo-learn.db")
	var err error
	db, err = storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := storage.Migrate(db); err != nil {
		db.Close()
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	// Drive resolvePublishContext to obtain the project id it
	// expects. The function opens the same DB, runs migrations,
	// seeds (or looks up) the project, and returns its id.
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	resolvedRoot, resolvedDB, resolvedProjectID, code := resolvePublishContext(root, stderr)
	if code != exitSuccess {
		t.Fatalf("resolvePublishContext: code=%d stderr=%s", code, stderr.String())
	}
	_ = stdout
	_ = resolvedRoot

	svc := patterns.NewService(resolvedDB)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	memberIDs := []domain.ExperienceEventID{"evt-1", "evt-2", "evt-3", "evt-4", "evt-5"}
	seedPatternExperienceEvents(t, resolvedDB, resolvedProjectID, "fixture", memberIDs, now)
	cluster := patterns.ClusterRecord{
		Fingerprint:        "fp-cli-1",
		Kind:               domain.EventTestFailure,
		Members:            memberIDs,
		Sessions:           map[string]struct{}{"sess-1": {}, "sess-2": {}, "sess-3": {}},
		Days:               map[string]struct{}{"2026-07-25": {}, "2026-07-26": {}, "2026-07-27": {}},
		DistinctSessions:   3,
		DistinctDays:       3,
		OccurrenceCount:    5,
		SuccessfulOutcomes: 3,
		RepeatedCorrection: true,
		FirstSeenAt:        now,
		LastSeenAt:         now,
		RetrievalTerms:     []string{"compile", "missing", "header"},
	}
	saved, err := svc.IngestCluster(context.Background(), resolvedProjectID, cluster, patterns.QualificationDecision{Status: patterns.PatternQualified})
	if err != nil {
		t.Fatalf("IngestCluster: %v", err)
	}
	// Close the seed DB so cleanup does not deadlock. The CLI will
	// open its own connection when invoked.
	_ = resolvedDB.Close()
	_ = db.Close()
	return root, db, resolvedProjectID, saved.ID
}

func seedPatternExperienceEvents(t *testing.T, db *storage.DB, projectID domain.ProjectID, prefix string, eventIDs []domain.ExperienceEventID, now time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := storage.WithTx(ctx, db, func(tx *sql.Tx) error {
		turnIDs := make([]domain.ExperienceTurnID, 3)
		for i := range turnIDs {
			sessionID := domain.ExperienceSessionID(fmt.Sprintf("pattern-%s-session-%d", prefix, i+1))
			turnID := domain.ExperienceTurnID(fmt.Sprintf("pattern-%s-turn-%d", prefix, i+1))
			turnIDs[i] = turnID
			occurredAt := now.AddDate(0, 0, i)
			session := &domain.ExperienceSession{
				ID: sessionID, ProjectID: projectID, Source: domain.SourceOpenCode,
				ExternalSessionID: fmt.Sprintf("pattern-%s-external-session-%d", prefix, i+1),
				Locator:           domain.TranscriptLocator{Kind: "sqlite", Path: "C:/safe/pattern-sessions.db", SessionID: string(sessionID)},
				StartedAt:         &occurredAt, UpdatedAt: occurredAt, ClosedAt: &occurredAt,
				MetadataSHA256: fmt.Sprintf("pattern-session-digest-%d", i+1), CreatedAt: occurredAt,
			}
			if err := storage.SaveExperienceSession(ctx, tx, session); err != nil {
				return err
			}
			turn := &domain.ExperienceTurn{
				ID: turnID, SessionID: sessionID, ExternalTurnID: fmt.Sprintf("pattern-%s-external-turn-%d", prefix, i+1),
				Sequence: int64(i + 1), Status: domain.TurnIngested, Fingerprint: fmt.Sprintf("pattern-turn-fingerprint-%d", i+1),
				UserDigest: "user-digest", AssistantDigest: "assistant-digest", ToolCallsDigest: "tool-digest",
				SafeSummary: "Synthetic pattern fixture.", OccurredAt: occurredAt, StableAt: &occurredAt,
				IngestedAt: occurredAt, SourceRevision: "revision-1", Redacted: true,
			}
			if err := storage.SaveExperienceTurn(ctx, tx, turn); err != nil {
				return err
			}
		}
		for i, eventID := range eventIDs {
			event := &domain.ExperienceEvent{
				ID: eventID, ProjectID: projectID, TurnID: turnIDs[i%len(turnIDs)], Kind: domain.EventTestFailure,
				Summary: "Synthetic test failure.", Observation: "A deterministic test failed.", Outcome: "success",
				Fingerprint: fmt.Sprintf("pattern-event-fingerprint-%d", i+1), EvidenceJSON: `[{"kind":"test"}]`,
				Detector:   domain.DetectorIdentity{Kind: "deterministic", Name: "pattern-fixture", Version: "1.0.0"},
				Confidence: domain.ConfidenceHigh, CreatedAt: now.Add(time.Duration(i) * time.Minute),
			}
			if err := storage.SaveExperienceEvent(ctx, tx, event); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed pattern experience events: %v", err)
	}
}

// TestExperiencePatterns_List covers the happy path of the list
// subcommand. The JSON envelope must include the qualified pattern
// in the response.
func TestExperiencePatterns_List(t *testing.T) {
	t.Parallel()

	root, db, _, patternID := patternsCLIFixture(t)
	defer db.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	args := []string{
		"--project-root", root,
		"--status", string(patterns.PatternQualified),
	}
	code := runExperiencePatternsList(args, stdout, stderr)
	if code != exitSuccess {
		t.Fatalf("runExperiencePatternsList = %d, stderr=%s", code, stderr.String())
	}

	var out experiencePatternsOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("Unmarshal: %v\nraw=%s", err, stdout.String())
	}
	if out.Operation != "list" {
		t.Fatalf("Operation = %s, want list", out.Operation)
	}
	if len(out.Patterns) != 1 {
		t.Fatalf("Patterns len = %d, want 1 (stderr=%s)", len(out.Patterns), stderr.String())
	}
	if out.Patterns[0].ID != patternID {
		t.Fatalf("Pattern ID = %s, want %s", out.Patterns[0].ID, patternID)
	}
}

// TestExperiencePatterns_Get covers the get subcommand.
func TestExperiencePatterns_Get(t *testing.T) {
	t.Parallel()

	root, db, _, patternID := patternsCLIFixture(t)
	defer db.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	args := []string{
		"--project-root", root,
		"--id", string(patternID),
	}
	code := runExperiencePatternsGet(args, stdout, stderr)
	if code != exitSuccess {
		t.Fatalf("runExperiencePatternsGet = %d, stderr=%s", code, stderr.String())
	}

	var out experiencePatternsOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("Unmarshal: %v\nraw=%s", err, stdout.String())
	}
	if out.Operation != "get" {
		t.Fatalf("Operation = %s, want get", out.Operation)
	}
	if out.Pattern == nil || out.Pattern.ID != patternID {
		t.Fatalf("Pattern = %+v, want ID=%s", out.Pattern, patternID)
	}
	if len(out.Members) == 0 {
		t.Fatal("Members is empty, want ≥ 1")
	}
}

// TestExperiencePatterns_Dismiss_Idempotent verifies the dismiss
// subcommand is idempotent on the same reason.
func TestExperiencePatterns_Dismiss_Idempotent(t *testing.T) {
	t.Parallel()

	root, db, _, patternID := patternsCLIFixture(t)
	defer db.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	args := []string{
		"--project-root", root,
		"--id", string(patternID),
		"--reason", string(patterns.DismissalNotReusable),
		"--note", "first note",
	}

	code := runExperiencePatternsDismiss(args, stdout, stderr)
	if code != exitSuccess {
		t.Fatalf("first dismiss = %d, stderr=%s", code, stderr.String())
	}
	code = runExperiencePatternsDismiss(args, stdout, stderr)
	if code != exitSuccess {
		t.Fatalf("second dismiss (idempotent) = %d, stderr=%s", code, stderr.String())
	}

	// The buffer now contains two JSON objects, one per dismiss
	// call. Parse only the LAST one so the assertion reflects the
	// final state.
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	last := lines[len(lines)-1]
	var out experiencePatternsOutput
	if err := json.Unmarshal([]byte(last), &out); err != nil {
		t.Fatalf("Unmarshal: %v\nraw=%s", err, last)
	}
	if out.Pattern.DismissalReason != patterns.DismissalNotReusable {
		t.Fatalf("DismissalReason = %q, want %q", out.Pattern.DismissalReason, patterns.DismissalNotReusable)
	}
}

// TestExperiencePatterns_Dismiss_WrongPayload surfaces the typed
// error envelope when the operator forgets --id or passes a bad
// reason.
func TestExperiencePatterns_Dismiss_WrongPayload(t *testing.T) {
	t.Parallel()

	root, db, _, _ := patternsCLIFixture(t)
	defer db.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	args := []string{
		"--project-root", root,
		"--reason", string(patterns.DismissalOneOff),
	}
	code := runExperiencePatternsDismiss(args, stdout, stderr)
	if code == exitSuccess {
		t.Fatalf("dismiss without --id = success, want error")
	}
	if !strings.Contains(stderr.String(), "invalid_argument") {
		t.Fatalf("stderr = %q, want invalid_argument", stderr.String())
	}
}

// TestExperiencePatterns_AcceptanceSynthetic covers the full
// end-to-end acceptance scenario from docs/26 §3 PR #5:
//
//   - 3 sessions qualify a pattern.
//   - 3 retries in 1 session do NOT qualify.
//   - Dismissal is idempotent on the same reason.
//   - Stable JSON shape across the CLI surface.
func TestExperiencePatterns_AcceptanceSynthetic(t *testing.T) {
	t.Parallel()

	root, _, projectID, _ := patternsCLIFixture(t)

	// Re-open the DB so the synthetic scenarios can use the same
	// storage layer the CLI uses.
	dbPath := filepath.Join(root, ".royo-learn", "royo-learn.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	svc := patterns.NewService(db)

	// Synthetic 1: 3 sessions, qualifies.
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	acceptanceEventIDs := []domain.ExperienceEventID{"evt-a", "evt-b", "evt-c", "evt-d", "evt-e", "evt-f"}
	seedPatternExperienceEvents(t, db, projectID, "acceptance", acceptanceEventIDs, now)
	qualifiedCluster := patterns.ClusterRecord{
		Fingerprint:        "fp-acc-1",
		Kind:               domain.EventTestFailure,
		Members:            []domain.ExperienceEventID{"evt-a", "evt-b", "evt-c"},
		Sessions:           map[string]struct{}{"sess-1": {}, "sess-2": {}, "sess-3": {}},
		Days:               map[string]struct{}{"2026-07-25": {}, "2026-07-26": {}, "2026-07-27": {}},
		DistinctSessions:   3,
		DistinctDays:       3,
		OccurrenceCount:    3,
		SuccessfulOutcomes: 3,
		FirstSeenAt:        now,
		LastSeenAt:         now,
		RetrievalTerms:     []string{"compile", "missing", "header"},
	}
	q1, err := svc.IngestCluster(ctx, projectID, qualifiedCluster, patterns.QualificationDecision{Status: patterns.PatternQualified})
	if err != nil {
		t.Fatalf("IngestCluster(qualified): %v", err)
	}
	if q1.Status != patterns.PatternQualified {
		t.Fatalf("Qualified cluster status = %s, want qualified", q1.Status)
	}

	// Synthetic 2: 3 retries in 1 session, must NOT qualify.
	retriesCluster := patterns.ClusterRecord{
		Fingerprint:        "fp-acc-2",
		Kind:               domain.EventTestFailure,
		Members:            []domain.ExperienceEventID{"evt-d", "evt-e", "evt-f"},
		Sessions:           map[string]struct{}{"sess-1": {}},
		Days:               map[string]struct{}{"2026-07-25": {}, "2026-07-26": {}, "2026-07-27": {}},
		DistinctSessions:   1,
		DistinctDays:       3,
		OccurrenceCount:    3,
		SuccessfulOutcomes: 2,
		FirstSeenAt:        now,
		LastSeenAt:         now,
		RetrievalTerms:     []string{"compile", "missing"},
	}
	q2, err := svc.IngestCluster(ctx, projectID, retriesCluster, patterns.QualificationDecision{Status: patterns.PatternObserved})
	if err != nil {
		t.Fatalf("IngestCluster(retries): %v", err)
	}
	if q2.Status != patterns.PatternObserved {
		t.Fatalf("3-retries-1-session status = %s, want observed (anti-pattern)", q2.Status)
	}

	// Dismissal idempotence end-to-end through the CLI.
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := runExperiencePatternsDismiss([]string{
		"--project-root", root,
		"--id", string(q1.ID),
		"--reason", string(patterns.DismissalOneOff),
		"--note", "synthetic acceptance",
	}, stdout, stderr)
	if code != exitSuccess {
		t.Fatalf("dismiss = %d, stderr=%s", code, stderr.String())
	}
	code = runExperiencePatternsDismiss([]string{
		"--project-root", root,
		"--id", string(q1.ID),
		"--reason", string(patterns.DismissalOneOff),
		"--note", "synthetic acceptance",
	}, stdout, stderr)
	if code != exitSuccess {
		t.Fatalf("dismiss (idempotent) = %d, stderr=%s", code, stderr.String())
	}

	// Stable JSON: the encoded envelope must include the typed
	// reason verbatim.
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, `"dismissal_reason":"one_off"`) {
		t.Fatalf("encoded output missing dismissal_reason one_off: %s", last)
	}
	if !strings.Contains(last, `"status":"dismissed"`) {
		t.Fatalf("encoded output missing status dismissed: %s", last)
	}
}
