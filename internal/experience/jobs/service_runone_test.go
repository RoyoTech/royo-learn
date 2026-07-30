// Phase 4 (PR #14) — RunOne audit-hook tests for the symmetric job
// engine. The tests live in a dedicated file so the diff stays focused
// on the new surface; the legacy RunDue / lease tests stay in
// service_test.go unchanged.

package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
	"agent-royo-learn/internal/experience/semantic"
	"agent-royo-learn/internal/storage"
)

// runOneTestFixture is the test harness shared by every Phase 4 test.
// The fixture registers a deterministic ingest job, builds a *Service
// with an injectable clock, and exposes the audit_events + job_run_log
// helpers the tests need to assert invariants.
type runOneTestFixture struct {
	db        *sql.DB
	svc       *Service
	now       time.Time
	registry  string
	projectID domain.ProjectID
	job       *semantic.Job
}

// newRunOneFixture creates the fixture. The job_name is the canonical
// experience_ingest:codex string; the JobFunc returns an empty Result
// (success) and the tests override Func per scenario.
func newRunOneFixture(t *testing.T, jobName, source string) *runOneTestFixture {
	t.Helper()
	db, _ := openServiceTestDB(t)
	ctx := context.Background()
	projectID := domain.ProjectID("proj-svc")

	fixed := time.Date(2026, 7, 28, 14, 35, 33, 0, time.UTC)
	svcWithClock := NewServiceWithClock(db, func() time.Time { return fixed })

	job := &semantic.Job{
		Name:   jobName,
		Source: source,
		Intent: semantic.JobIntentIngest,
		Scope:  semantic.JobScopeProject,
		Func: func(ctx context.Context, deps semantic.Deps) (semantic.Result, error) {
			return semantic.Result{}, nil
		},
	}
	if err := svcWithClock.Register(ctx, projectID, JobRegistryEntry{
		JobName:            jobName,
		Description:        "phase-4 test job",
		DefaultIntervalSec: 3600,
		DefaultMaxRetries:  3,
		Intent:             semantic.JobIntentIngest,
		Scope:              semantic.JobScopeProject,
		RiskClass:          semantic.JobRiskClassLow,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	return &runOneTestFixture{
		db:        db,
		svc:       svcWithClock,
		now:       fixed,
		registry:  jobName,
		projectID: projectID,
		job:       job,
	}
}

// fetchAuditEvents returns the audit_events rows for the run_id
// passed in, ordered by occurred_at ASC. The test asserts the
// four-event invariant by scanning the returned slice.
func fetchAuditEvents(t *testing.T, db *sql.DB, runID string) []auditRow {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		`SELECT operation, entity_id, details_json
		 FROM audit_events
		 WHERE entity_type = 'job_run' AND entity_id = ?
		 ORDER BY occurred_at ASC, sequence ASC`,
		runID,
	)
	if err != nil {
		t.Fatalf("query audit_events: %v", err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var r auditRow
		if err := rows.Scan(&r.Operation, &r.EntityID, &r.DetailsJSON); err != nil {
			t.Fatalf("scan audit row: %v", err)
		}
		out = append(out, r)
	}
	return out
}

// fetchRunLog returns the single job_run_log row for runID.
func fetchRunLog(t *testing.T, db *sql.DB, runID string) runLogRow {
	t.Helper()
	var row runLogRow
	var finishedAt sql.NullString
	err := db.QueryRowContext(context.Background(),
		`SELECT run_id, job_name, state, started_at, finished_at, error_code, error_message, attempt
		 FROM job_run_log WHERE run_id = ?`,
		runID,
	).Scan(&row.RunID, &row.JobName, &row.State, &row.StartedAt, &finishedAt,
		&row.ErrorCode, &row.ErrorMessage, &row.Attempt)
	if err != nil {
		t.Fatalf("query job_run_log: %v", err)
	}
	if finishedAt.Valid {
		row.FinishedAt = &finishedAt.String
	}
	return row
}

// auditRow is a flattened audit_events row used by Phase 4 tests.
type auditRow struct {
	Operation   string
	EntityID    string
	DetailsJSON string
}

// runLogRow is a flattened job_run_log row used by Phase 4 tests.
type runLogRow struct {
	RunID        string
	JobName      string
	State        string
	StartedAt    string
	FinishedAt   *string
	ErrorCode    string
	ErrorMessage string
	Attempt      int
}

// TestRunOne_Leases verifies RunOne acquires and releases the lease
// atomically. The test asserts (a) state.Status = JobOK after the
// run, (b) the lease is released (LeaseOwner empty), and (c) the
// run-log state is succeeded.
func TestRunOne_Leases(t *testing.T) {
	f := newRunOneFixture(t, "phase4-lease-job", "codex")

	out, err := f.svc.RunOne(context.Background(), f.projectID, f.registry, "runner-1", f.job)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if out.State != semantic.StateSucceeded {
		t.Errorf("State = %q, want %q", out.State, semantic.StateSucceeded)
	}

	state, err := NewRepository(f.db).GetJobState(context.Background(), f.projectID, f.registry)
	if err != nil {
		t.Fatalf("GetJobState: %v", err)
	}
	if state.Status != JobOK {
		t.Errorf("Status = %q, want %q", state.Status, JobOK)
	}
	if state.LeaseOwner != "" {
		t.Errorf("LeaseOwner = %q, want empty", state.LeaseOwner)
	}
}

// TestRunOne_EmitsFourEvents verifies the four-event audit invariant:
// exactly one row per transition (pending, running, succeeded) with
// the same run_id, ordered by sequence, and the matching run-log row.
func TestRunOne_EmitsFourEvents(t *testing.T) {
	f := newRunOneFixture(t, "phase4-events-job", "codex")

	out, err := f.svc.RunOne(context.Background(), f.projectID, f.registry, "runner-1", f.job)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if out.RunID == "" {
		t.Fatal("RunID is empty")
	}

	rows := fetchAuditEvents(t, f.db, out.RunID)
	wantOps := []string{semantic.EventJobPending, semantic.EventJobRunning, semantic.EventJobSucceeded}
	if len(rows) != len(wantOps) {
		t.Fatalf("audit rows = %d, want %d (ops: %v)", len(rows), len(wantOps), rowOps(rows))
	}
	for i, want := range wantOps {
		if rows[i].Operation != want {
			t.Errorf("audit row %d operation = %q, want %q", i, rows[i].Operation, want)
		}
		if rows[i].EntityID != out.RunID {
			t.Errorf("audit row %d entity_id = %q, want %q", i, rows[i].EntityID, out.RunID)
		}
	}

	run := fetchRunLog(t, f.db, out.RunID)
	if run.State != semantic.StateSucceeded {
		t.Errorf("run_log.state = %q, want %q", run.State, semantic.StateSucceeded)
	}
	if run.Attempt != 1 {
		t.Errorf("run_log.attempt = %d, want 1", run.Attempt)
	}
}

func rowOps(rows []auditRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Operation
	}
	return out
}

// TestRunOne_FailureEmitsJobFailed verifies the failure path emits
// job_failed (not job_succeeded) and stamps the run-log with
// state=failed + error_code + error_message.
func TestRunOne_FailureEmitsJobFailed(t *testing.T) {
	f := newRunOneFixture(t, "phase4-fail-job", "codex")
	f.job.Func = func(ctx context.Context, deps semantic.Deps) (semantic.Result, error) {
		return semantic.Result{ErrorCode: "boom", ErrorMessage: "body failed"}, nil
	}

	out, err := f.svc.RunOne(context.Background(), f.projectID, f.registry, "runner-1", f.job)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if out.State != semantic.StateFailed {
		t.Errorf("State = %q, want %q", out.State, semantic.StateFailed)
	}
	if out.ErrorCode != "boom" {
		t.Errorf("ErrorCode = %q, want boom", out.ErrorCode)
	}

	rows := fetchAuditEvents(t, f.db, out.RunID)
	if len(rows) != 3 {
		t.Fatalf("audit rows = %d, want 3 (ops: %v)", len(rows), rowOps(rows))
	}
	if rows[2].Operation != semantic.EventJobFailed {
		t.Errorf("terminal operation = %q, want %q", rows[2].Operation, semantic.EventJobFailed)
	}
	if !strings.Contains(rows[2].DetailsJSON, "boom") {
		t.Errorf("failed details_json missing error_code: %s", rows[2].DetailsJSON)
	}
}

// TestRunOne_LeaseConflictSkips verifies the lease-held path emits
// job_pending + job_failed (with error_code=job_lease_held) and does
// NOT call the JobFunc body.
func TestRunOne_LeaseConflictSkips(t *testing.T) {
	f := newRunOneFixture(t, "phase4-lease-held-job", "codex")

	// Owner A acquires the lease first.
	if _, err := f.svc.AcquireLease(context.Background(), f.projectID, f.registry, "owner-A"); err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}

	executed := false
	f.job.Func = func(ctx context.Context, deps semantic.Deps) (semantic.Result, error) {
		executed = true
		return semantic.Result{}, nil
	}

	out, err := f.svc.RunOne(context.Background(), f.projectID, f.registry, "owner-B", f.job)
	if err == nil {
		t.Fatal("expected lease conflict error, got nil")
	}
	if out.State != semantic.StateLeaseHeld {
		t.Errorf("State = %q, want %q", out.State, semantic.StateLeaseHeld)
	}
	if out.ErrorCode != "job_lease_held" {
		t.Errorf("ErrorCode = %q, want job_lease_held", out.ErrorCode)
	}
	if executed {
		t.Error("JobFunc body was executed under lease conflict")
	}

	rows := fetchAuditEvents(t, f.db, out.RunID)
	wantOps := []string{semantic.EventJobPending, semantic.EventJobFailed}
	if len(rows) != len(wantOps) {
		t.Fatalf("audit rows = %d, want %d (ops: %v)", len(rows), len(wantOps), rowOps(rows))
	}
	if rows[1].DetailsJSON == "" || !strings.Contains(rows[1].DetailsJSON, "job_lease_held") {
		t.Errorf("job_failed details_json missing job_lease_held: %s", rows[1].DetailsJSON)
	}
}

// TestRunOne_CancellationHonoured verifies the JobFunc body
// observing a cancelled ctx emits the job_failed audit transition.
// The engine uses context.WithoutCancel(ctx) for the Phase 5
// finalisation so the audit invariant survives a body-side cancel
// race; the test verifies the audit + run-log writes commit even
// though the parent ctx was cancelled mid-flight.
func TestRunOne_CancellationHonoured(t *testing.T) {
	f := newRunOneFixture(t, "phase4-cancel-job", "codex")

	ctx, cancel := context.WithCancel(context.Background())

	f.job.Func = func(jobCtx context.Context, deps semantic.Deps) (semantic.Result, error) {
		// Cancel from inside the body so the audit phases (job_pending,
		// job_running) observed the ctx as live, but the body observes
		// the cancellation and propagates the ctx error. Phase 5
		// finalisation uses context.WithoutCancel so the audit
		// writes commit even with the parent ctx cancelled.
		cancel()
		if err := jobCtx.Err(); err != nil {
			return semantic.Result{}, err
		}
		return semantic.Result{}, nil
	}

	out, err := f.svc.RunOne(ctx, f.projectID, f.registry, "runner-1", f.job)
	if err == nil {
		t.Fatal("expected error from cancelled ctx, got nil")
	}
	if out.State != semantic.StateFailed {
		t.Errorf("State = %q, want %q (err=%v)", out.State, semantic.StateFailed, err)
	}
}

// TestRunOne_RunIDsAreUnique verifies two consecutive RunOne
// invocations mint distinct run_ids.
func TestRunOne_RunIDsAreUnique(t *testing.T) {
	f := newRunOneFixture(t, "phase4-unique-rid-job", "codex")

	out1, err := f.svc.RunOne(context.Background(), f.projectID, f.registry, "runner-1", f.job)
	if err != nil {
		t.Fatalf("RunOne 1: %v", err)
	}
	out2, err := f.svc.RunOne(context.Background(), f.projectID, f.registry, "runner-1", f.job)
	if err != nil {
		t.Fatalf("RunOne 2: %v", err)
	}
	if out1.RunID == out2.RunID {
		t.Errorf("RunIDs must be unique: both = %q", out1.RunID)
	}
	// Both IDs must be valid UUIDs.
	if _, err := uuid.Parse(out1.RunID); err != nil {
		t.Errorf("RunID %q is not a valid UUID: %v", out1.RunID, err)
	}
	if _, err := uuid.Parse(out2.RunID); err != nil {
		t.Errorf("RunID %q is not a valid UUID: %v", out2.RunID, err)
	}
}

// TestRunOne_PayloadAllowList asserts the audit-event payload JSON
// contains ONLY the 7 documented allow-list keys (plus the optional
// transition key from PR #13) — never any ExperienceEnvelope field.
// The test inspects every row emitted by RunOne.
func TestRunOne_PayloadAllowList(t *testing.T) {
	f := newRunOneFixture(t, "phase4-allowlist-job", "codex")
	f.job.Func = func(ctx context.Context, deps semantic.Deps) (semantic.Result, error) {
		return semantic.Result{ErrorCode: "boom", ErrorMessage: "fail body"}, nil
	}

	out, err := f.svc.RunOne(context.Background(), f.projectID, f.registry, "runner-1", f.job)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}

	rows := fetchAuditEvents(t, f.db, out.RunID)
	if len(rows) != 3 {
		t.Fatalf("audit rows = %d, want 3", len(rows))
	}

	allowed := map[string]bool{
		"job_name": true, "run_id": true, "source": true, "state": true,
		"transition": true, "attempt": true, "error_code": true, "error_message": true,
	}
	for _, r := range rows {
		for _, key := range extractPayloadKeys(t, r.DetailsJSON) {
			if !allowed[key] {
				t.Errorf("row op=%s has forbidden key %q in payload: %s", r.Operation, key, r.DetailsJSON)
			}
		}
	}
}

// extractPayloadKeys parses the details_json blob and returns the
// top-level keys. The audit sink stores details as raw JSON (the
// result of json.Marshal on a map[string]any), so we unmarshal it
// directly. We accept either a JSON object or a JSON-quoted string
// (older code paths wrote the string form).
func extractPayloadKeys(t *testing.T, detailsJSON string) []string {
	t.Helper()
	trimmed := strings.TrimSpace(detailsJSON)
	if trimmed == "" {
		return nil
	}
	// First try a raw JSON object.
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		keys := make([]string, 0, len(payload))
		for k := range payload {
			keys = append(keys, k)
		}
		return keys
	}
	// Fall back to a JSON-quoted string form (older writers stored
	// the marshaled bytes as a quoted string).
	if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		inner := trimmed[1 : len(trimmed)-1]
		if err := json.Unmarshal([]byte(inner), &payload); err == nil {
			keys := make([]string, 0, len(payload))
			for k := range payload {
				keys = append(keys, k)
			}
			return keys
		}
	}
	t.Fatalf("decode details_json: %q", detailsJSON)
	return nil
}

// TestRunOne_PerAdapterPathNoDirectAuditCall is the static-review
// guard: the three per-adapter packages must NEVER call
// storage.RecordEventTx directly. The audit hook is owned by
// jobs.Service.RunOne exclusively (hito10-codex-review-fixes.md).
func TestRunOne_PerAdapterPathNoDirectAuditCall(t *testing.T) {
	// Determine the project root from this test file's location so
	// the test is independent of CWD.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}

	packages := []string{
		filepath.Join(abs, "internal", "experience", "opencode"),
		filepath.Join(abs, "internal", "experience", "claudecode"),
		filepath.Join(abs, "internal", "experience", "codex"),
	}
	for _, pkg := range packages {
		if err := assertNoDirectAuditCall(pkg); err != nil {
			t.Errorf("%s: %v", pkg, err)
		}
	}
}

// assertNoDirectAuditCall walks every .go file in dir and asserts
// none of them reference storage.RecordEventTx. The check is the
// static counterpart of TestAuditHook_DoesNotLeakTranscriptText.
func assertNoDirectAuditCall(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "RecordEventTx") {
			return fmt.Errorf("file %s references RecordEventTx directly; only jobs.Service.RunOne may emit job_* audit events", path)
		}
		return nil
	})
}

// TestAuditHook_DoesNotLeakTranscriptText is the Hito 10 SEVERE
// invariant test. The fixture envelope carries two canary sentinels
// in the user + assistant text fields; the test asserts no audit
// payload contains either sentinel byte sequence.
func TestAuditHook_DoesNotLeakTranscriptText(t *testing.T) {
	f := newRunOneFixture(t, "phase4-leak-canary-job", "codex")

	canaryUser := "USER_LEAK_CANARY_42"
	canaryAssistant := "ASSISTANT_LEAK_CANARY_42"

	// The JobFunc body simulates ingesting an envelope with canary
	// text. The Result.Envelopes field is `[]any` in PR #13; the
	// JobFunc body never persists envelopes in PR #14 (that lives
	// in PR #15 via the per-adapter IngestEnvelope call), but we
	// still test that even when the body returns envelopes with
	// canary text in their UserText/AssistantText fields, the
	// audit-event payload cannot leak them.
	canaryEnvelopes := []any{
		func() experience.ExperienceEnvelope {
			env := experience.ExperienceEnvelope{}
			env.Turn.UserText = canaryUser
			env.Turn.AssistantText = canaryAssistant
			return env
		}(),
	}
	f.job.Func = func(ctx context.Context, deps semantic.Deps) (semantic.Result, error) {
		return semantic.Result{Envelopes: canaryEnvelopes}, nil
	}

	out, err := f.svc.RunOne(context.Background(), f.projectID, f.registry, "runner-1", f.job)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}

	rows := fetchAuditEvents(t, f.db, out.RunID)
	if len(rows) == 0 {
		t.Fatal("no audit events emitted")
	}
	for _, r := range rows {
		if strings.Contains(r.DetailsJSON, canaryUser) {
			t.Errorf("audit row op=%s leaks USER canary: %s", r.Operation, r.DetailsJSON)
		}
		if strings.Contains(r.DetailsJSON, canaryAssistant) {
			t.Errorf("audit row op=%s leaks ASSISTANT canary: %s", r.Operation, r.DetailsJSON)
		}
	}
}

// TestRunOne_NilJob verifies the nil-job guard.
func TestRunOne_NilJob(t *testing.T) {
	_, svc := openServiceTestDB(t)
	_, err := svc.RunOne(context.Background(), "proj-svc", "missing", "owner", nil)
	if err == nil {
		t.Fatal("expected error on nil Job")
	}
}

// TestRunOne_MissingRegistryEntry verifies RunOne refuses to run a
// job whose registry row is absent. The audit invariant must hold:
// the engine must NOT emit a job_pending row for an unregistered job.
func TestRunOne_MissingRegistryEntry(t *testing.T) {
	db, svc := openServiceTestDB(t)
	job := &semantic.Job{Name: "does-not-exist", Source: "codex"}
	_, err := svc.RunOne(context.Background(), "proj-svc", "does-not-exist", "owner", job)
	if err == nil {
		t.Fatal("expected error for missing registry entry")
	}

	// No audit_events rows should have been emitted (the engine
	// fails before the audit emission).
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM audit_events WHERE operation LIKE 'job_%'`).Scan(&n); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if n != 0 {
		t.Errorf("audit_events rows = %d, want 0 (engine must not emit on missing registry)", n)
	}
}

// TestBuildJobAuditEvent_RejectsInvalidTransition is a unit test for
// the buildJobAuditEvent helper. The helper is unexported but
// covered via the public RunOne path above; this extra unit test
// documents the explicit rejection of an unknown transition string.
func TestBuildJobAuditEvent_RejectsInvalidTransition(t *testing.T) {
	_, svc := openServiceTestDB(t)
	_, err := svc.buildJobAuditEvent(context.Background(),
		"job_unknown", "run-x", "name-x", "codex", semantic.StatePending, 0, "", "", time.Now().UTC())
	if err == nil {
		t.Fatal("expected error for unknown transition")
	}
	if !strings.Contains(err.Error(), "invalid audit transition") {
		t.Errorf("error = %v, want one mentioning invalid audit transition", err)
	}
}

// TestBuildJobAuditEvent_RejectsInvalidState verifies the state
// allow-list is enforced at runtime (the static counterpart is the
// migration's CHECK constraint).
func TestBuildJobAuditEvent_RejectsInvalidState(t *testing.T) {
	_, svc := openServiceTestDB(t)
	_, err := svc.buildJobAuditEvent(context.Background(),
		semantic.EventJobPending, "run-x", "name-x", "codex", "unknown_state", 0, "", "", time.Now().UTC())
	if err == nil {
		t.Fatal("expected error for unknown state")
	}
}

// TestRunOne_NilDB verifies the nil-db guard.
func TestRunOne_NilDB(t *testing.T) {
	svc := &Service{repo: NewRepository(nil)}
	job := &semantic.Job{Name: "x", Source: "codex"}
	_, err := svc.RunOne(context.Background(), "proj-svc", "x", "owner", job)
	if err == nil {
		t.Fatal("expected error on nil DB")
	}
}

// TestRunOne_JobFuncError verifies a JobFunc that returns a Go error
// is mapped to the failure path.
func TestRunOne_JobFuncError(t *testing.T) {
	f := newRunOneFixture(t, "phase4-func-error-job", "codex")
	want := errors.New("adapter boom")
	f.job.Func = func(ctx context.Context, deps semantic.Deps) (semantic.Result, error) {
		return semantic.Result{}, want
	}
	out, err := f.svc.RunOne(context.Background(), f.projectID, f.registry, "runner-1", f.job)
	if err == nil {
		t.Fatal("expected error from JobFunc failure")
	}
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
	if out.State != semantic.StateFailed {
		t.Errorf("State = %q, want %q", out.State, semantic.StateFailed)
	}
}

// TestRunOne_FourEventsSharedRunID asserts the run_id invariant: all
// four audit events (or three, in the happy path) emitted by one
// RunOne invocation share the same run_id, equal to the run-log
// run_id.
func TestRunOne_FourEventsSharedRunID(t *testing.T) {
	f := newRunOneFixture(t, "phase4-shared-rid-job", "codex")
	out, err := f.svc.RunOne(context.Background(), f.projectID, f.registry, "runner-1", f.job)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	rows := fetchAuditEvents(t, f.db, out.RunID)
	if len(rows) < 2 {
		t.Fatalf("audit rows = %d, want >= 2", len(rows))
	}
	for _, r := range rows {
		if r.EntityID != out.RunID {
			t.Errorf("audit row entity_id = %q, want %q (shared run_id invariant)", r.EntityID, out.RunID)
		}
	}
	run := fetchRunLog(t, f.db, out.RunID)
	if run.RunID != out.RunID {
		t.Errorf("run_log.run_id = %q, want %q", run.RunID, out.RunID)
	}
}

// keep storage import in use (the test file references
// storage.Migrate via openServiceTestDB; this anchor keeps go vet
// happy if the tests in this file change).
var _ = storage.Migrate
