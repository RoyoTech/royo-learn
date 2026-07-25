// Tests for `experience detect` (Hito 5 slice 5.3).
//
// The subcommand is a thin orchestrator over the detector registry.
// The tests verify the CLI surface (flags, error paths, JSON
// output) and the integration with the retry detector, which is
// the only registered detector for slice 5.3.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeRetryPayload writes a JSON-encoded RetryPayload to a temp
// file and returns the path. The current observation is always a
// failure with the supplied fingerprint; the recent list is built
// from the supplied failures (each at the supplied offset from now).
func writeRetryPayload(t *testing.T, fingerprint, tool string, recent []time.Duration) string {
	t.Helper()
	now := time.Now().UTC()

	current := observationJSON{
		Fingerprint: fingerprint,
		Tool:        tool,
		Result:      "fail",
		Timestamp:   now.Format(time.RFC3339Nano),
	}
	recentObs := make([]observationJSON, 0, len(recent))
	for _, off := range recent {
		recentObs = append(recentObs, observationJSON{
			Fingerprint: fingerprint,
			Tool:        tool,
			Result:      "fail",
			Timestamp:   now.Add(off).Format(time.RFC3339Nano),
		})
	}

	payload := retryPayloadJSON{Current: current, Recent: recentObs}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	path := filepath.Join(t.TempDir(), "payload.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	return path
}

type observationJSON struct {
	Fingerprint string `json:"fingerprint"`
	Tool        string `json:"tool"`
	Result      string `json:"result"`
	Timestamp   string `json:"timestamp"`
}

type retryPayloadJSON struct {
	Current observationJSON   `json:"current"`
	Recent  []observationJSON `json:"recent"`
}

func TestRunExperienceDetect_RequiresKind(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"detect", "--project-root", t.TempDir()}, &stdout, &stderr)
	if code == exitSuccess {
		t.Fatalf("expected non-zero exit code, got %d (stdout=%q stderr=%q)", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--kind") {
		t.Errorf("stderr = %q, want mention of --kind", stderr.String())
	}
}

func TestRunExperienceDetect_UnknownKind(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"detect", "--kind", "not_a_real_detector", "--project-root", t.TempDir()}, &stdout, &stderr)
	if code == exitSuccess {
		t.Fatalf("expected non-zero exit code, got %d (stdout=%q stderr=%q)", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "not_a_real_detector") {
		t.Errorf("stderr = %q, want mention of unknown kind", stderr.String())
	}
	if !strings.Contains(stderr.String(), "retry") {
		t.Errorf("stderr = %q, want mention of registered kinds (so the operator knows what's available)", stderr.String())
	}
}

func TestRunExperienceDetect_InputFileNotFound(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := runExperience([]string{
		"detect",
		"--kind", "retry",
		"--project-root", t.TempDir(),
		"--input", "/this/path/does/not/exist.json",
	}, &stdout, &stderr)
	if code == exitSuccess {
		t.Fatalf("expected non-zero exit code, got %d (stdout=%q stderr=%q)", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--input") {
		t.Errorf("stderr = %q, want mention of --input", stderr.String())
	}
}

func TestRunExperienceDetect_InvalidJSON(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runExperience([]string{
		"detect",
		"--kind", "retry",
		"--project-root", t.TempDir(),
		"--input", path,
	}, &stdout, &stderr)
	if code == exitSuccess {
		t.Fatalf("expected non-zero exit code, got %d (stdout=%q stderr=%q)", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "retry") {
		t.Errorf("stderr = %q, want mention of retry payload", stderr.String())
	}
}

func TestRunExperienceDetect_RetryAtThreshold(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	// 2 recent fails + current fail = 3 occurrences, exactly at threshold.
	payload := writeRetryPayload(t, "fp-cmd-fail-001", "npm test",
		[]time.Duration{-1 * time.Minute, -3 * time.Minute})

	var stdout, stderr bytes.Buffer
	code := runExperience([]string{
		"detect",
		"--kind", "retry",
		"--project-root", projectRoot,
		"--input", payload,
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitSuccess, stderr.String())
	}

	var out experienceDetectOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v (stdout=%q)", err, stdout.String())
	}
	if out.Status != "ok" {
		t.Errorf("Status = %q, want %q", out.Status, "ok")
	}
	if out.Kind != "retry" {
		t.Errorf("Kind = %q, want %q", out.Kind, "retry")
	}
	if out.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", out.Version, "0.1.0")
	}
	if out.TotalEvents != 1 {
		t.Errorf("TotalEvents = %d, want 1", out.TotalEvents)
	}
	if len(out.DetectedEvents) != 1 {
		t.Fatalf("DetectedEvents has %d entries, want 1", len(out.DetectedEvents))
	}
	ev := out.DetectedEvents[0]
	if ev.Kind != "retry" {
		t.Errorf("event Kind = %q, want %q", ev.Kind, "retry")
	}
	if ev.Tool != "npm test" {
		t.Errorf("event Tool = %q, want %q", ev.Tool, "npm test")
	}
	if occ, ok := ev.Extra["occurrences"].(float64); !ok || occ != 3 {
		t.Errorf("event Extra[occurrences] = %v (%T), want float64 3 (JSON round-trip coerces int to float64)", ev.Extra["occurrences"], ev.Extra["occurrences"])
	}
}

func TestRunExperienceDetect_RetryBelowThreshold(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	// Empty recent: only the current fail, count=1, below threshold.
	payload := writeRetryPayload(t, "fp-cmd-fail-001", "npm test", nil)

	var stdout, stderr bytes.Buffer
	code := runExperience([]string{
		"detect",
		"--kind", "retry",
		"--project-root", projectRoot,
		"--input", payload,
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitSuccess, stderr.String())
	}

	var out experienceDetectOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v (stdout=%q)", err, stdout.String())
	}
	if out.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d, want 0", out.TotalEvents)
	}
	if len(out.DetectedEvents) != 0 {
		t.Errorf("DetectedEvents has %d entries, want 0", len(out.DetectedEvents))
	}
}

func TestRunExperienceDetect_RetryWindowExcludesOld(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	// 2 recent fails, both outside the 5-minute window.
	payload := writeRetryPayload(t, "fp-cmd-fail-001", "npm test",
		[]time.Duration{-10 * time.Minute, -30 * time.Minute})

	var stdout, stderr bytes.Buffer
	code := runExperience([]string{
		"detect",
		"--kind", "retry",
		"--project-root", projectRoot,
		"--input", payload,
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitSuccess, stderr.String())
	}

	var out experienceDetectOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v (stdout=%q)", err, stdout.String())
	}
	if out.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d, want 0 (window excludes old observations)", out.TotalEvents)
	}
}

func TestRunExperienceDetect_DirectExecute(t *testing.T) {
	t.Parallel()
	// Drive executeDetect directly with a bytes.Buffer to verify the
	// core orchestration without going through the flag parser. This
	// is the unit-level hook the acceptance test (slice 5.4) will
	// reuse when wiring up capture.Service.
	projectRoot := t.TempDir()
	now := time.Now().UTC()
	payload := retryPayloadJSON{
		Current: observationJSON{
			Fingerprint: "fp-cmd-fail-001",
			Tool:        "npm test",
			Result:      "fail",
			Timestamp:   now.Format(time.RFC3339Nano),
		},
		Recent: []observationJSON{
			{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339Nano)},
			{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-3 * time.Minute).Format(time.RFC3339Nano)},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := executeDetect(stdout, stderr, "retry", projectRoot, bytes.NewReader(data))
	if code != exitSuccess {
		t.Fatalf("executeDetect exit code = %d, want %d (stderr=%q)", code, exitSuccess, stderr.String())
	}

	var out experienceDetectOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v (stdout=%q)", err, stdout.String())
	}
	if out.TotalEvents != 1 {
		t.Errorf("TotalEvents = %d, want 1", out.TotalEvents)
	}
}

func TestRunExperienceDetect_ExecuteUnknownKind(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := executeDetect(stdout, stderr, "not_a_kind", t.TempDir(), strings.NewReader("{}"))
	if code == exitSuccess {
		t.Fatalf("expected non-zero exit code, got %d", code)
	}
	if !strings.Contains(stderr.String(), "not_a_kind") {
		t.Errorf("stderr = %q, want mention of unknown kind", stderr.String())
	}
}

// TestRunExperienceDetect_PersistRequiresProjectRoot covers the
// --persist flag's contract: the canonical experience store is
// scoped to a project, so --project-root must be supplied when
// --persist is set. Defaulting the cwd would silently write to the
// wrong database.
func TestRunExperienceDetect_PersistRequiresProjectRoot(t *testing.T) {
	t.Parallel()

	// Build a temp dir with a valid payload, but DO NOT pass
	// --project-root alongside --persist.
	now := time.Now().UTC()
	payload, err := json.Marshal(retryPayloadJSON{
		Current: observationJSON{
			Fingerprint: "fp-cmd-fail-001",
			Tool:        "npm test",
			Result:      "fail",
			Timestamp:   now.Format(time.RFC3339Nano),
		},
		Recent: []observationJSON{
			{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339Nano)},
			{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-3 * time.Minute).Format(time.RFC3339Nano)},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	inputPath := filepath.Join(t.TempDir(), "payload.json")
	if err := os.WriteFile(inputPath, payload, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runExperience([]string{
		"detect",
		"--kind", "retry",
		"--input", inputPath,
		"--persist",
	}, &stdout, &stderr)
	if code == exitSuccess {
		t.Fatalf("expected non-zero exit code, got %d (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--project-root") {
		t.Errorf("stderr = %q, want mention of --project-root", stderr.String())
	}
}

// TestRunExperienceDetect_PersistEndToEnd is the slice 5.4
// acceptance scenario: a real project root, a real SQLite database,
// a detector event that crosses the retry threshold, and a
// verification that the persisted row's metadata is observable on
// the second run via the (session_id, external_turn_id)
// uniqueness constraint.
func TestRunExperienceDetect_PersistEndToEnd(t *testing.T) {
	t.Parallel()

	projectRoot := setupProjectRoot(t)
	payload := writeRetryPayload(t, "fp-cmd-fail-001", "npm test",
		[]time.Duration{-1 * time.Minute, -3 * time.Minute})

	// First run: persist the event.
	var firstStdout, firstStderr bytes.Buffer
	code := runExperience([]string{
		"detect",
		"--kind", "retry",
		"--project-root", projectRoot,
		"--input", payload,
		"--persist",
	}, &firstStdout, &firstStderr)
	if code != exitSuccess {
		t.Fatalf("first run exit code = %d, want %d (stderr=%q)", code, exitSuccess, firstStderr.String())
	}

	var firstOut experienceDetectOutput
	if err := json.Unmarshal(firstStdout.Bytes(), &firstOut); err != nil {
		t.Fatalf("decode first output: %v (stdout=%q)", err, firstStdout.String())
	}
	if firstOut.Status != "ok" {
		t.Errorf("first Status = %q, want %q", firstOut.Status, "ok")
	}
	if firstOut.TotalEvents != 1 {
		t.Errorf("first TotalEvents = %d, want 1", firstOut.TotalEvents)
	}
	if firstOut.PersistedCount != 1 {
		t.Errorf("first PersistedCount = %d, want 1", firstOut.PersistedCount)
	}
	if len(firstOut.Persisted) != 1 {
		t.Fatalf("first Persisted has %d entries, want 1", len(firstOut.Persisted))
	}
	firstEvent := firstOut.Persisted[0]
	if firstEvent.EventID == "" {
		t.Error("first EventID is empty")
	}
	if firstEvent.Fingerprint == "" {
		t.Error("first Fingerprint is empty")
	}
	if firstEvent.Duplicate {
		t.Error("first Duplicate = true on initial persist, want false")
	}

	// Second run with the same payload: idempotent. The detector
	// still emits the event (deterministic threshold rule), but
	// the persistence layer recognises the (session, turn) pair
	// and reports Duplicate = true.
	var secondStdout, secondStderr bytes.Buffer
	code = runExperience([]string{
		"detect",
		"--kind", "retry",
		"--project-root", projectRoot,
		"--input", payload,
		"--persist",
	}, &secondStdout, &secondStderr)
	if code != exitSuccess {
		t.Fatalf("second run exit code = %d, want %d (stderr=%q)", code, exitSuccess, secondStderr.String())
	}

	var secondOut experienceDetectOutput
	if err := json.Unmarshal(secondStdout.Bytes(), &secondOut); err != nil {
		t.Fatalf("decode second output: %v (stdout=%q)", err, secondStdout.String())
	}
	if secondOut.TotalEvents != 1 {
		t.Errorf("second TotalEvents = %d, want 1", secondOut.TotalEvents)
	}
	if secondOut.PersistedCount != 1 {
		t.Errorf("second PersistedCount = %d, want 1", secondOut.PersistedCount)
	}
	if len(secondOut.Persisted) != 1 {
		t.Fatalf("second Persisted has %d entries, want 1", len(secondOut.Persisted))
	}
	if !secondOut.Persisted[0].Duplicate {
		t.Errorf("second Duplicate = false, want true (idempotency proof)")
	}
	if secondOut.Persisted[0].EventID != firstEvent.EventID {
		t.Errorf("second EventID = %q, want %q (same row on retry)",
			secondOut.Persisted[0].EventID, firstEvent.EventID)
	}
	if secondOut.Persisted[0].Fingerprint != firstEvent.Fingerprint {
		t.Errorf("second Fingerprint = %q, want %q (deterministic fingerprint)",
			secondOut.Persisted[0].Fingerprint, firstEvent.Fingerprint)
	}
}

// TestRunExperienceDetect_PersistNoEvents verifies that a payload
// below threshold produces no persisted rows, not just zero
// detected events. This guards against a future refactor that
// persists the empty set on every run.
func TestRunExperienceDetect_PersistNoEvents(t *testing.T) {
	t.Parallel()

	projectRoot := setupProjectRoot(t)
	// No recent failures: count=1, below threshold of 3.
	payload := writeRetryPayload(t, "fp-cmd-fail-001", "npm test", nil)

	var stdout, stderr bytes.Buffer
	code := runExperience([]string{
		"detect",
		"--kind", "retry",
		"--project-root", projectRoot,
		"--input", payload,
		"--persist",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitSuccess, stderr.String())
	}

	var out experienceDetectOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v (stdout=%q)", err, stdout.String())
	}
	if out.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d, want 0", out.TotalEvents)
	}
	if out.PersistedCount != 0 {
		t.Errorf("PersistedCount = %d, want 0", out.PersistedCount)
	}
	if len(out.Persisted) != 0 {
		t.Errorf("Persisted has %d entries, want 0", len(out.Persisted))
	}
}
