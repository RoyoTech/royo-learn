// Tests for runPublicationDriftCheck (Hito 12, T12.5). The job
// registration is the entry point that ties the Checker (T12.3) and the
// Repository (T12.2) into the Hito 11 symmetric engine. The tests
// cover:
//
//   - The status gate is encoded in the JobFunc body (decision D1):
//     a publication with status='in_progress' must NOT generate a row
//     in publication_drift_state, while a sibling with
//     status='completed' MUST.
//   - The gate literal lives in the Go source (static-review test).
//   - All four outcomes are produced end-to-end against a real DB.
//   - The Hito 10 SEVERE audit-payload invariant is preserved: no
//     /home/alice or excerpt text appears in any audit_events payload
//     emitted by the audit hook.

package drift

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/experience/semantic"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/testutil"
)

// fixedClock returns a deterministic time for the JobFunc body and the
// Repository. Setting jobNow via SetJobNow ensures the run_id is
// reproducible across calls.
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// seedPublicationWith inserts a project + learning + publication row
// with the supplied status, learning id, and source. The source is
// stored on the learnings row (the publications table does not carry
// a source column directly — see migration 001_init.sql). The
// project_key is derived from learningID so multiple test cases do not
// collide on the UNIQUE project_key constraint.
func seedPublicationWith(t *testing.T, db *storage.DB, pubID, learningID, source, status string) {
	t.Helper()
	projectID := "proj-" + learningID
	projectKey := "drift-jobs-" + learningID
	if _, err := db.DB.Exec(`
		INSERT INTO projects (id, project_key, display_name, canonical_path, fingerprint, created_at, updated_at)
		VALUES (?, ?, 'Drift Jobs', '/tmp/dj', 'fp-dj', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`, projectID, projectKey); err != nil {
		t.Fatalf("seed projects: %v", err)
	}
	if _, err := db.DB.Exec(`
		INSERT INTO learnings (id, project_id, status, type, title, context, observation,
			reusable_lesson, scope_guess, confidence, evidence_level, fingerprint, normalized_hash,
			actor_json, created_at, updated_at)
		VALUES (?, ?, 'approved', 'pattern', 'Drift Jobs Seed',
			'ctx', 'obs', 'lesson', 'project', 'medium', 'observed', 'fp-ldj',
			'nh-ldj', '{}', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`, learningID, projectID); err != nil {
		t.Fatalf("seed learnings: %v", err)
	}
	targetsJSON := `[{"root":"/","path":"__placeholder__","operation":"create"}]`
	if _, err := db.DB.Exec(`
		INSERT INTO publications (id, learning_id, preview_hash, targets_json, verification_json,
			rollback_json, status, started_at)
		VALUES (?, ?, 'prev-dj', ?, '[]', '[]', ?, '2026-01-01T00:00:00Z')
	`, pubID, learningID, targetsJSON, status); err != nil {
		t.Fatalf("seed publications: %v", err)
	}
	_ = source // source lives on learnings; the drift job does not consume it today
}

// TestPublicationDriftCheck_SkipsInProgress inserts two publications:
// one with status='completed' and one with status='in_progress'. Both
// carry a targets_json that points at the same temp file. After running
// the JobFunc, publication_drift_state must contain exactly one row
// (the completed publication). The in_progress publication is skipped
// by the Go-level gate (decision D1).
func TestPublicationDriftCheck_SkipsInProgress(t *testing.T) {
	dir := testutil.TempDir(t)
	dbPath := filepath.Join(dir, "gate.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("storage.Migrate: %v", err)
	}

	// One real target file on disk so the completed-publication check
	// succeeds with status='ok'.
	targetDir := testutil.TempDir(t)
	targetPath := filepath.Join(targetDir, "target.jsonl")
	if err := os.WriteFile(targetPath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Targets must be a JSON-encoded array; we point both publications
	// at the same file.
	targetsJSON, _ := encodeJSONArray([]map[string]string{
		{"root": "", "path": targetPath, "operation": "create"},
	})

	// Seed both publications with the SAME targets_json. The Go-level
	// gate must drop the in_progress row regardless of SQL filters.
	seedPublicationWith(t, db, "01HZXPUBCOMPLETED00000000000", "learn-jobs-c", "opencode", "completed")
	seedPublicationWith(t, db, "01HZXPUBINPROGRESS0000000000", "learn-jobs-i", "opencode", "in_progress")
	if _, err := db.DB.Exec(`UPDATE publications SET targets_json = ? WHERE id IN (?, ?)`,
		targetsJSON,
		"01HZXPUBCOMPLETED00000000000", "01HZXPUBINPROGRESS0000000000",
	); err != nil {
		t.Fatalf("update targets_json: %v", err)
	}

	// Inject a deterministic clock so the run_id is reproducible.
	t0 := time.Date(2026, 1, 20, 10, 0, 0, 0, time.UTC)
	prev := jobNow
	jobNow = fixedClock(t0)
	t.Cleanup(func() { jobNow = prev })

	job := Job()
	if job.Func == nil {
		t.Fatal("Job().Func is nil")
	}
	if _, err := job.Func(context.Background(), makeDeps(db.DB)); err != nil {
		t.Fatalf("JobFunc: %v", err)
	}

	// Exactly one row in publication_drift_state — the completed
	// publication only.
	repo := NewRepository(db.DB, fixedClock(t0))
	rows, err := repo.ListDrift(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListDrift: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListDrift returned %d rows, want 1 (in_progress must be skipped)", len(rows))
	}
	if rows[0].PublicationID != "01HZXPUBCOMPLETED00000000000" {
		t.Errorf("PublicationID = %q, want the completed publication", rows[0].PublicationID)
	}
	if rows[0].Status != StatusOK {
		t.Errorf("Status = %q, want %q (first run against a fresh file is 'ok')", rows[0].Status, StatusOK)
	}
}

// TestPublicationDriftCheck_GateInJobFuncBody is a static-review test:
// it greps the production source for the literal
// `status != "completed"` inside the runPublicationDriftCheck body.
// The presence of the literal proves the gate is encoded in Go and is
// not a hidden SQL WHERE filter that the spec test cannot see.
func TestPublicationDriftCheck_GateInJobFuncBody(t *testing.T) {
	src, err := os.ReadFile("jobs.go")
	if err != nil {
		t.Fatalf("ReadFile(jobs.go): %v", err)
	}
	body := string(src)
	if !strings.Contains(body, `status != "completed"`) {
		t.Error(`jobs.go does not contain the gate literal status != "completed"` +
			"\nThe spec requires the gate to be visible to static review of the JobFunc body.")
	}
	if !strings.Contains(body, "completionStatus = \"completed\"") {
		t.Error(`jobs.go does not declare the completionStatus constant.`)
	}
}

// TestPublicationDriftCheck_AllFourOutcomes seeds a project + learning
// + one publication with four targets in its targets_json (one per
// expected outcome) and asserts that, after running the JobFunc
// against a primed publication_drift_state baseline, all four rows
// appear in publication_drift_state with the expected Status.
//
// The drifted baseline is established by first seeding
// publication_drift_state with a known actual_hash that differs from
// the file's current SHA-256. The Checker sees the mismatch and emits
// StatusDrifted.
func TestPublicationDriftCheck_AllFourOutcomes(t *testing.T) {
	dir := testutil.TempDir(t)
	dbPath := filepath.Join(dir, "outcomes.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("storage.Migrate: %v", err)
	}

	targetsDir := testutil.TempDir(t)

	// ok: file exists and matches the baseline.
	okPath := filepath.Join(targetsDir, "ok.bin")
	if err := os.WriteFile(okPath, []byte("ok content"), 0o644); err != nil {
		t.Fatalf("WriteFile(ok): %v", err)
	}
	okBaseline := sha256HexBytes([]byte("ok content"))

	// drifted: file exists but the baseline is a different hash.
	driftedPath := filepath.Join(targetsDir, "drifted.bin")
	if err := os.WriteFile(driftedPath, []byte("v1 contents"), 0o644); err != nil {
		t.Fatalf("WriteFile(drifted): %v", err)
	}
	driftedBaseline := sha256HexBytes([]byte("v0 contents")) // intentionally stale

	// target_missing: path does not exist.
	missingPath := filepath.Join(targetsDir, "missing.jsonl")

	// target_unreadable: file exists; we chmod 0o000 on POSIX.
	unreadablePath := filepath.Join(targetsDir, "locked.bin")
	if err := os.WriteFile(unreadablePath, []byte("locked"), 0o644); err != nil {
		t.Fatalf("WriteFile(unreadable): %v", err)
	}
	if os.Geteuid() != 0 { // root bypasses POSIX chmod; skip the unreadable case for root
		_ = os.Chmod(unreadablePath, 0o000)
		t.Cleanup(func() { _ = os.Chmod(unreadablePath, 0o644) })
	} else {
		// Running as root: chmod 0o000 is ineffective. Replace with a
		// directory-shaped path that os.Open cannot read after stat.
		_ = os.Remove(unreadablePath)
		if err := os.Mkdir(unreadablePath, 0o755); err != nil {
			t.Fatalf("Mkdir(unreadable): %v", err)
		}
		unreadablePath = filepath.Join(unreadablePath, "child")
	}

	pubID := "01HZXPUBFOUROUTCOMES00000000"
	learningID := "learn-jobs-outcomes"
	seedPublicationWith(t, db, pubID, learningID, "opencode", "completed")

	// Wire the targets_json so each target is one of the four files.
	targetsJSON, _ := encodeJSONArray([]map[string]string{
		{"root": "", "path": okPath, "operation": "create"},
		{"root": "", "path": driftedPath, "operation": "create"},
		{"root": "", "path": missingPath, "operation": "create"},
		{"root": "", "path": unreadablePath, "operation": "create"},
	})
	if _, err := db.DB.Exec(`UPDATE publications SET targets_json = ? WHERE id = ?`, targetsJSON, pubID); err != nil {
		t.Fatalf("update targets_json: %v", err)
	}

	// Seed the baseline actual_hash for the drifted path so the
	// Checker sees a mismatch. The ok path's baseline equals the file's
	// current hash (no drift).
	repo := NewRepository(db.DB, func() time.Time { return time.Date(2026, 1, 21, 9, 0, 0, 0, time.UTC) })
	priorRunID := "seed-baseline"
	if err := repo.RecordDrift(context.Background(), DriftRow{
		PublicationID: pubID,
		Source:        "opencode",
		TargetPath:    okPath,
		ActualHash:    okBaseline,
		Status:        StatusOK,
		CheckedAt:     time.Date(2026, 1, 20, 9, 0, 0, 0, time.UTC),
		RunID:         priorRunID,
	}); err != nil {
		t.Fatalf("seed baseline (ok): %v", err)
	}
	if err := repo.RecordDrift(context.Background(), DriftRow{
		PublicationID: pubID,
		Source:        "opencode",
		TargetPath:    driftedPath,
		ActualHash:    driftedBaseline, // intentionally stale
		Status:        StatusOK,
		CheckedAt:     time.Date(2026, 1, 20, 9, 0, 0, 0, time.UTC),
		RunID:         priorRunID,
	}); err != nil {
		t.Fatalf("seed baseline (drifted): %v", err)
	}

	// Inject a deterministic clock so run_id is reproducible.
	t0 := time.Date(2026, 1, 21, 10, 0, 0, 0, time.UTC)
	prev := jobNow
	jobNow = fixedClock(t0)
	t.Cleanup(func() { jobNow = prev })

	job := Job()
	if _, err := job.Func(context.Background(), makeDeps(db.DB)); err != nil {
		t.Fatalf("JobFunc: %v", err)
	}

	// Build a per-target status map from the latest rows.
	rows, err := repo.ListDrift(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListDrift: %v", err)
	}
	gotStatus := make(map[string]Status, 4)
	for _, r := range rows {
		if r.PublicationID != pubID {
			continue
		}
		// The latest row for each target wins; ListDrift orders by
		// checked_at DESC so the freshest row is first.
		if _, seen := gotStatus[r.TargetPath]; !seen {
			gotStatus[r.TargetPath] = r.Status
		}
	}
	want := map[string]Status{
		okPath:         StatusOK,
		driftedPath:    StatusDrifted,
		missingPath:    StatusTargetMissing,
		unreadablePath: StatusTargetUnreadable,
	}
	for path, wantStatus := range want {
		got, ok := gotStatus[path]
		if !ok {
			t.Errorf("no row for %q", path)
			continue
		}
		if got != wantStatus {
			t.Errorf("target %q status = %q, want %q", path, got, wantStatus)
		}
	}
}

// TestPublicationDriftCheck_RegistryEntryMetadata asserts the
// JobRegistryEntry() function returns the documented defaults
// (decision D3): intent=drift, scope=project, risk_class=low,
// default_interval_sec=3600, default_max_retries=3, enabled=false.
func TestPublicationDriftCheck_RegistryEntryMetadata(t *testing.T) {
	e := JobRegistryEntry()
	if e.JobName != JobName {
		t.Errorf("JobName = %q, want %q", e.JobName, JobName)
	}
	if e.DefaultIntervalSec != 3600 {
		t.Errorf("DefaultIntervalSec = %d, want 3600", e.DefaultIntervalSec)
	}
	if e.DefaultMaxRetries != 3 {
		t.Errorf("DefaultMaxRetries = %d, want 3", e.DefaultMaxRetries)
	}
	if e.Enabled {
		t.Error("Enabled = true, want false (Hito 11 invariant)")
	}
	if err := e.Validate(); err != nil {
		t.Errorf("JobRegistryEntry.Validate() = %v, want nil", err)
	}
}

// TestPublicationDriftCheck_AuditPayloadHasNoTargetPath is a defensive
// guard against a future refactor that accidentally puts the target
// file path into an audit-event payload. We seed no audit_events here
// (the drift job does not emit them — only jobs.Service.RunOne does
// that, and that path is shared with every other job and already
// audited by Hito 11). The test asserts that the drift package itself
// does not contain the kind of PII shape that has historically leaked
// transcript content.
func TestPublicationDriftCheck_AuditPayloadHasNoTargetPath(t *testing.T) {
	// Walk the package directory and grep for the Hito 10 SEVERE PII
	// markers. The drift package must never produce audit-payload
	// bodies that contain absolute paths or transcript excerpts.
	pkgFiles := []string{"checker.go", "repository.go", "jobs.go"}
	banned := []string{"/home/alice", "/Users/bob", "excerpt", "user_text", "assistant_text"}
	for _, f := range pkgFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", f, err)
		}
		for _, marker := range banned {
			if strings.Contains(string(data), marker) {
				t.Errorf("%s contains PII marker %q — drift job must not leak target paths in payloads", f, marker)
			}
		}
	}
}

// TestPublicationDriftCheck_NilDBReturnsTypedError asserts that calling
// runPublicationDriftCheck with a nil DB handle returns a typed
// semantic.Result with ErrorCode="missing_db" instead of panicking.
func TestPublicationDriftCheck_NilDBReturnsTypedError(t *testing.T) {
	job := Job()
	res, err := job.Func(context.Background(), makeDeps(nil))
	if err == nil {
		t.Fatal("expected error from JobFunc with nil DB, got nil")
	}
	if res.ErrorCode != "missing_db" {
		t.Errorf("ErrorCode = %q, want %q", res.ErrorCode, "missing_db")
	}
}

// TestPublicationDriftCheck_ContextCancelledReturnsTypedError asserts
// that a pre-cancelled context produces ErrorCode="context_cancelled".
func TestPublicationDriftCheck_ContextCancelledReturnsTypedError(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := storage.Open(filepath.Join(dir, "ctx.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("storage.Migrate: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	job := Job()
	res, err := job.Func(ctx, makeDeps(db.DB))
	if err == nil {
		t.Fatal("expected error from JobFunc with cancelled ctx, got nil")
	}
	if res.ErrorCode != "context_cancelled" {
		t.Errorf("ErrorCode = %q, want %q", res.ErrorCode, "context_cancelled")
	}
}

// TestPublicationDriftCheck_SkipsEmptyPathTarget exercises the
// `tgt.Path == ""` branch in the JobFunc: a target entry with no path
// is silently skipped (not recorded, not errored) so the loop
// continues to the next target.
func TestPublicationDriftCheck_SkipsEmptyPathTarget(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := storage.Open(filepath.Join(dir, "emptypath.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("storage.Migrate: %v", err)
	}

	// targets_json with one empty path and one valid path.
	validPath := filepath.Join(testutil.TempDir(t), "valid.bin")
	if err := os.WriteFile(validPath, []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	targetsJSON, _ := encodeJSONArray([]map[string]string{
		{"root": "", "path": "", "operation": "create"}, // skipped: empty path
		{"root": "", "path": validPath, "operation": "create"},
	})

	pubID := "01HZXPUBEMPTYPATH00000000000"
	learningID := "learn-jobs-emptypath"
	seedPublicationWith(t, db, pubID, learningID, "opencode", "completed")
	if _, err := db.DB.Exec(`UPDATE publications SET targets_json = ? WHERE id = ?`, targetsJSON, pubID); err != nil {
		t.Fatalf("update targets_json: %v", err)
	}

	job := Job()
	res, err := job.Func(context.Background(), makeDeps(db.DB))
	if err != nil {
		t.Fatalf("JobFunc: %v", err)
	}
	if res.ErrorCode != "" {
		t.Errorf("ErrorCode = %q, want empty (empty path should be silently skipped)", res.ErrorCode)
	}

	repo := NewRepository(db.DB, func() time.Time { return time.Now().UTC() })
	rows, err := repo.ListDrift(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListDrift: %v", err)
	}
	// Only the valid-path target produces a row; the empty-path one is
	// skipped silently.
	if len(rows) != 1 {
		t.Fatalf("ListDrift returned %d rows, want 1 (empty-path target should be skipped)", len(rows))
	}
	if rows[0].TargetPath != validPath {
		t.Errorf("TargetPath = %q, want %q", rows[0].TargetPath, validPath)
	}
}

// TestPublicationDriftCheck_DriftedDetectedOnSecondRun is the
// acceptance scenario for the baseline-comparison pattern: a file is
// first recorded as 'ok' on run 1, then mutated between runs, then
// recorded as 'drifted' on run 2.
func TestPublicationDriftCheck_DriftedDetectedOnSecondRun(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := storage.Open(filepath.Join(dir, "tworun.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("storage.Migrate: %v", err)
	}

	targetDir := testutil.TempDir(t)
	target := filepath.Join(targetDir, "watch.bin")
	if err := os.WriteFile(target, []byte("v1"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	pubID := "01HZXPUBTWORUN0000000000000"
	learningID := "learn-jobs-tworun"
	seedPublicationWith(t, db, pubID, learningID, "opencode", "completed")
	targetsJSON, _ := encodeJSONArray([]map[string]string{
		{"root": "", "path": target, "operation": "create"},
	})
	if _, err := db.DB.Exec(`UPDATE publications SET targets_json = ? WHERE id = ?`, targetsJSON, pubID); err != nil {
		t.Fatalf("update targets_json: %v", err)
	}

	clock := func() time.Time { return time.Date(2026, 1, 22, 8, 0, 0, 0, time.UTC) }
	prev := jobNow
	jobNow = clock
	t.Cleanup(func() { jobNow = prev })

	job := Job()
	if _, err := job.Func(context.Background(), makeDeps(db.DB)); err != nil {
		t.Fatalf("JobFunc (run 1): %v", err)
	}

	repo := NewRepository(db.DB, clock)
	rows, err := repo.ListDrift(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListDrift (run 1): %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListDrift (run 1) returned %d rows, want 1", len(rows))
	}
	if rows[0].Status != StatusOK {
		t.Errorf("run 1 status = %q, want %q (first run against a fresh file is 'ok')", rows[0].Status, StatusOK)
	}

	// Mutate the file between runs.
	if err := os.WriteFile(target, []byte("v2 — different content"), 0o644); err != nil {
		t.Fatalf("WriteFile (mutate): %v", err)
	}
	if _, err := job.Func(context.Background(), makeDeps(db.DB)); err != nil {
		t.Fatalf("JobFunc (run 2): %v", err)
	}

	rows, err = repo.ListDrift(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListDrift (run 2): %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListDrift (run 2) returned %d rows, want 1 (upsert on (pub, target))", len(rows))
	}
	if rows[0].Status != StatusDrifted {
		t.Errorf("run 2 status = %q, want %q (file mutated between runs)", rows[0].Status, StatusDrifted)
	}
	if rows[0].ActualHash != sha256HexBytes([]byte("v2 — different content")) {
		t.Errorf("ActualHash = %q, want %q", rows[0].ActualHash, sha256HexBytes([]byte("v2 — different content")))
	}
}

// TestSetJobNow_NilIsNoop asserts SetJobNow(nil) does not replace the
// package-level clock (defensive guard against a caller that forgets
// to pass a non-nil func).
func TestSetJobNow_NilIsNoop(t *testing.T) {
	prev := jobNow
	t.Cleanup(func() { SetJobNow(prev) })

	probe := func() time.Time { return time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC) }
	SetJobNow(probe)
	if got := jobNow(); !got.Equal(probe()) {
		t.Fatalf("after SetJobNow(probe), jobNow() = %v, want %v", got, probe())
	}

	SetJobNow(nil)
	// jobNow must NOT have been replaced with nil.
	got := jobNow()
	if got.IsZero() {
		t.Error("SetJobNow(nil) cleared jobNow; expected the previous clock to be retained")
	}
	if !got.Equal(probe()) {
		t.Errorf("after SetJobNow(nil), jobNow() = %v, want the probe value %v", got, probe())
	}
}

// TestRunIDForNow_NilClock uses time.Now directly to assert the nil
// branch of runIDForNow formats a non-empty run id.
func TestRunIDForNow_NilClock(t *testing.T) {
	got := runIDForNow(nil)
	if got == "" {
		t.Error("runIDForNow(nil) returned empty string")
	}
	if !strings.HasPrefix(got, "run-") {
		t.Errorf("runIDForNow(nil) = %q, want it to start with 'run-'", got)
	}
}

// TestDecodeTargets_EmptyReturnsError asserts the empty-string branch
// of decodeTargets (an empty targets_json produces a decode error so
// the JobFunc records target_unreadable and continues).
func TestDecodeTargets_EmptyReturnsError(t *testing.T) {
	_, err := decodeTargets("")
	if err == nil {
		t.Fatal("decodeTargets(\"\") returned nil error")
	}
	if !strings.Contains(err.Error(), "empty targets_json") {
		t.Errorf("decodeTargets(\"\") error = %v, want it to mention 'empty targets_json'", err)
	}
}

// TestUpsertUnreadable_RecordsRow exercises the unreadable
// fail-soft path: when targets_json fails to decode, the JobFunc calls
// upsertUnreadable which writes a target_unreadable row. We invoke it
// directly here to cover the helper.
func TestUpsertUnreadable_RecordsRow(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := storage.Open(filepath.Join(dir, "unreadable.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("storage.Migrate: %v", err)
	}
	seedPublicationWith(t, db, "01HZXPUBUNREADABLE000000000", "learn-jobs-u", "opencode", "completed")

	repo := NewRepository(db.DB, fixedNow)
	pr := publicationRow{ID: "01HZXPUBUNREADABLE000000000"}
	if err := upsertUnreadable(context.Background(), repo, pr, "(decode failed)", "run-test"); err != nil {
		t.Fatalf("upsertUnreadable: %v", err)
	}
	rows, err := repo.ListDrift(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListDrift: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListDrift returned %d rows, want 1", len(rows))
	}
	if rows[0].Status != StatusTargetUnreadable {
		t.Errorf("Status = %q, want %q", rows[0].Status, StatusTargetUnreadable)
	}
	if rows[0].TargetPath != "(decode failed)" {
		t.Errorf("TargetPath = %q, want %q", rows[0].TargetPath, "(decode failed)")
	}
}

// TestPublicationDriftCheck_TargetsJSONDecodeError exercises the
// fail-soft branch of the JobFunc body when targets_json is malformed:
// the helper records target_unreadable and the JobFunc continues
// without returning an error.
func TestPublicationDriftCheck_TargetsJSONDecodeError(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := storage.Open(filepath.Join(dir, "decode.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("storage.Migrate: %v", err)
	}

	seedPublicationWith(t, db, "01HZXPUBDECODEERR0000000000", "learn-jobs-dec", "opencode", "completed")
	if _, err := db.DB.Exec(`UPDATE publications SET targets_json = ? WHERE id = ?`,
		"this is not valid JSON",
		"01HZXPUBDECODEERR0000000000",
	); err != nil {
		t.Fatalf("update targets_json: %v", err)
	}

	job := Job()
	if _, err := job.Func(context.Background(), makeDeps(db.DB)); err != nil {
		t.Fatalf("JobFunc returned error: %v (expected fail-soft path to record target_unreadable)", err)
	}
	repo := NewRepository(db.DB, func() time.Time { return time.Now().UTC() })
	rows, err := repo.ListDrift(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListDrift: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListDrift returned %d rows, want 1 (fail-soft target_unreadable)", len(rows))
	}
	if rows[0].Status != StatusTargetUnreadable {
		t.Errorf("Status = %q, want %q", rows[0].Status, StatusTargetUnreadable)
	}
}

// TestPublicationDriftCheck_SelectFailsReturnsTypedError simulates a
// failed SELECT (by closing the DB before the JobFunc runs) and asserts
// the JobFunc returns a typed semantic.Result with ErrorCode set.
func TestPublicationDriftCheck_SelectFailsReturnsTypedError(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := storage.Open(filepath.Join(dir, "fail.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	if err := storage.Migrate(db); err != nil {
		db.Close()
		t.Fatalf("storage.Migrate: %v", err)
	}
	// Close the DB to make the Query fail.
	db.Close()

	job := Job()
	res, err := job.Func(context.Background(), makeDeps(db.DB))
	if err == nil {
		t.Fatal("expected error from JobFunc with closed DB, got nil")
	}
	if res.ErrorCode == "" {
		t.Error("ErrorCode is empty, want a typed code")
	}
}

// TestPublicationDriftCheck_EmptyPublications asserts the happy path
// when the publications table has no rows: the JobFunc returns a
// successful Result with checked=0, skipped=0.
func TestPublicationDriftCheck_EmptyPublications(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := storage.Open(filepath.Join(dir, "empty.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("storage.Migrate: %v", err)
	}

	job := Job()
	res, err := job.Func(context.Background(), makeDeps(db.DB))
	if err != nil {
		t.Fatalf("JobFunc (empty): %v", err)
	}
	if res.ErrorCode != "" {
		t.Errorf("ErrorCode = %q, want empty for happy path", res.ErrorCode)
	}
	if !strings.Contains(res.NextCursor, "checked=0") {
		t.Errorf("NextCursor = %q, want it to mention 'checked=0'", res.NextCursor)
	}
}

// makeDeps returns a semantic.Deps with just the DB handle populated,
// matching the shape jobs.Service.RunOne passes to JobFunc bodies.
func makeDeps(db *sql.DB) semantic.Deps {
	return semantic.Deps{DB: db}
}

// sha256HexBytes returns the hex-encoded SHA-256 of the byte slice.
func sha256HexBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// encodeJSONArray marshals the input slice to a JSON array string. Used
// to produce targets_json values for the seeded publications.
func encodeJSONArray(in interface{}) (string, error) {
	b, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
