package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The tests in this file drive the acceptance criterion of Recorrido B:
//
//	captured -> needs_evidence -> evidence_attached -> approved
//
// EVERY step goes through the public CLI. Nothing here calls
// storage.SaveEvidence, opens the database, or writes SQL. A test that can only
// pass by reaching around the public interface proves nothing about the product.

// captureJSON runs a CLI capture and returns the decoded --json payload.
func captureJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run(append([]string{"capture"}, args...), &stdout, &stderr); code != exitSuccess {
		t.Fatalf("capture exit = %d, want %d\nstderr: %s", code, exitSuccess, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("capture stdout is not JSON: %v\n%s", err, stdout.String())
	}
	return out
}

func initProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	var buf bytes.Buffer
	if code := run([]string{"init", "--project-root", root}, &buf, &buf); code != exitSuccess {
		t.Fatalf("init failed: %s", buf.String())
	}
	// Windows Defender real-time protection locks the SQLite WAL/SHM
	// files briefly after the CLI closes them, which can race t.TempDir's
	// RemoveAll cleanup. Registering a t.Cleanup that waits briefly lets
	// the AV scan complete and releases file handles before t.TempDir
	// cleanup runs.
	t.Cleanup(func() {
		if runtime.GOOS == "windows" {
			time.Sleep(50 * time.Millisecond)
		}
	})
	return root
}

// TestCLI_EvidenceUnblocksApproval is THE acceptance criterion of Recorrido B.
//
// A learning captured with no evidence records cannot be approved. Attaching
// evidence afterwards, through the public `evidence add` command, unblocks it.
// No direct SQLite manipulation anywhere.
func TestCLI_EvidenceUnblocksApproval(t *testing.T) {
	root := initProject(t)

	// 1. captured — with a sufficient declared level but ZERO evidence records.
	captured := captureJSON(t,
		"--project-root", root,
		"--title", "Evidence gate must be reachable",
		"--context", "Recovery of the approval chain",
		"--observation", "checkEvidenceThreshold requires a persisted evidence record",
		"--lesson", "Every threshold needs a public interface that can satisfy it",
		"--destination", "project",
		"--evidence-level", "moderate",
		"--json",
	)
	learningID, _ := captured["learning_id"].(string)
	if learningID == "" {
		t.Fatal("capture returned no learning_id")
	}
	if status, _ := captured["status"].(string); status != "captured" {
		t.Fatalf("status = %q, want %q", status, "captured")
	}

	// 2. needs_evidence — the curator sends it back for evidence.
	var neOut, neErr bytes.Buffer
	if code := run([]string{
		"curate", "--project-root", root,
		"--learning-id", learningID,
		"--action", "needs_evidence",
		"--rationale", "No evidence records are attached to this learning",
		"--json",
	}, &neOut, &neErr); code != exitSuccess {
		t.Fatalf("curate needs_evidence exit = %d: %s", code, neErr.String())
	}
	var neResult map[string]any
	_ = json.Unmarshal(neOut.Bytes(), &neResult)
	if got, _ := neResult["new_status"].(string); got != "needs_evidence" {
		t.Fatalf("new_status = %q, want %q", got, "needs_evidence")
	}

	// 3. Approval MUST be blocked while no evidence record exists.
	var blockOut, blockErr bytes.Buffer
	blockCode := run([]string{
		"curate", "--project-root", root,
		"--learning-id", learningID,
		"--action", "approve",
		"--rationale", "Trying to approve without any evidence record",
		"--json",
	}, &blockOut, &blockErr)
	if blockCode == exitSuccess {
		t.Fatalf("approval succeeded with zero evidence records; the D3 threshold is not enforced.\nstdout: %s", blockOut.String())
	}
	if !strings.Contains(blockErr.String(), "evidence") {
		t.Fatalf("blocked approval error does not mention evidence: %s", blockErr.String())
	}

	// 4. evidence_attached — through the public CLI, not through SQL.
	var addOut, addErr bytes.Buffer
	if code := run([]string{
		"evidence", "add", learningID,
		"--project-root", root,
		"--kind", "test",
		"--summary", "The approval chain test reproduces the blocked approval",
		"--source", "go test ./cmd/royo-learn",
		"--content", "--- PASS: TestCLI_EvidenceUnblocksApproval",
		"--json",
	}, &addOut, &addErr); code != exitSuccess {
		t.Fatalf("evidence add exit = %d, want %d\nstderr: %s", code, exitSuccess, addErr.String())
	}
	var addResult map[string]any
	if err := json.Unmarshal(addOut.Bytes(), &addResult); err != nil {
		t.Fatalf("evidence add stdout is not JSON: %v\n%s", err, addOut.String())
	}
	if count, _ := addResult["evidence_count"].(float64); count != 1 {
		t.Fatalf("evidence_count = %v, want 1", addResult["evidence_count"])
	}

	// 5. approved — the same approval that was blocked now succeeds.
	var okOut, okErr bytes.Buffer
	if code := run([]string{
		"curate", "--project-root", root,
		"--learning-id", learningID,
		"--action", "approve",
		"--rationale", "Evidence is now attached and the threshold is satisfied",
		"--json",
	}, &okOut, &okErr); code != exitSuccess {
		t.Fatalf("approval after evidence exit = %d, want %d\nstderr: %s", code, exitSuccess, okErr.String())
	}
	var okResult map[string]any
	_ = json.Unmarshal(okOut.Bytes(), &okResult)
	if got, _ := okResult["new_status"].(string); got != "approved" {
		t.Fatalf("new_status = %q, want %q", got, "approved")
	}
}

// TestCLI_CaptureAcceptsEmbeddedEvidence proves capture itself can satisfy the
// threshold, so a learning does not have to detour through needs_evidence.
func TestCLI_CaptureAcceptsEmbeddedEvidence(t *testing.T) {
	root := initProject(t)

	evidenceFile := filepath.Join(t.TempDir(), "evidence.json")
	payload := `[
	  {"kind":"test","summary":"Suite green after the fix","source":"go test ./...","content":"ok agent-royo-learn/internal/curate"}
	]`
	if err := os.WriteFile(evidenceFile, []byte(payload), 0o644); err != nil {
		t.Fatalf("write evidence file: %v", err)
	}

	captured := captureJSON(t,
		"--project-root", root,
		"--title", "Capture carries its own evidence",
		"--context", "Recorrido B",
		"--observation", "Capture accepts an evidence array",
		"--lesson", "Evidence must be attachable at capture time",
		"--destination", "project",
		"--evidence-level", "strong",
		"--evidence-file", evidenceFile,
		"--json",
	)
	learningID, _ := captured["learning_id"].(string)
	if count, _ := captured["evidence_count"].(float64); count != 1 {
		t.Fatalf("evidence_count = %v, want 1", captured["evidence_count"])
	}

	// Approval must succeed immediately: no needs_evidence detour.
	var out, errBuf bytes.Buffer
	if code := run([]string{
		"curate", "--project-root", root,
		"--learning-id", learningID,
		"--action", "approve",
		"--rationale", "Evidence was supplied at capture time",
		"--json",
	}, &out, &errBuf); code != exitSuccess {
		t.Fatalf("approve exit = %d, want %d\nstderr: %s", code, exitSuccess, errBuf.String())
	}
	var result map[string]any
	_ = json.Unmarshal(out.Bytes(), &result)
	if got, _ := result["new_status"].(string); got != "approved" {
		t.Fatalf("new_status = %q, want approved", got)
	}
}

// TestCLI_SecretIsRedactedBeforeEverySink feeds a secret through the public
// capture interface and asserts it never reaches ANY sink: not SQLite, not the
// blob store, not the Markdown record, not the audit log, not the JSON response.
//
// Redaction is a write condition, not an output filter. This test fails if it is
// implemented as the latter.
func TestCLI_SecretIsRedactedBeforeEverySink(t *testing.T) {
	root := initProject(t)

	const secret = "sk-proj-abcdefghijklmnopqrstuvwxyz0123456789"

	captured := captureJSON(t,
		"--project-root", root,
		"--title", "Secret redaction must run before persistence",
		"--context", "A log line contained a live API key",
		"--observation", "The key "+secret+" leaked into the captured output",
		"--lesson", "Redact secrets before any write, never on the way out",
		"--destination", "project",
		"--evidence-level", "strong",
		"--evidence-summary", "Raw log containing the key "+secret,
		"--evidence-content", "ERROR authorization failed for "+secret,
		"--evidence-kind", "text",
		"--json",
	)
	learningID, _ := captured["learning_id"].(string)
	if learningID == "" {
		t.Fatal("capture returned no learning_id")
	}

	// Sink 1: the CLI JSON response itself.
	raw, _ := json.Marshal(captured)
	if bytes.Contains(raw, []byte(secret)) {
		t.Errorf("SINK LEAK — CLI JSON response contains the secret:\n%s", raw)
	}

	// Sinks 2..N: every byte persisted under .royo-learn. This covers the SQLite
	// file, the blob store, the Markdown records and the audit log in one sweep,
	// and it will also catch any sink added later.
	storeDir := filepath.Join(root, ".royo-learn")
	var leaks []string
	err := filepath.Walk(storeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if bytes.Contains(data, []byte(secret)) {
			leaks = append(leaks, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk store: %v", err)
	}
	for _, leak := range leaks {
		rel, _ := filepath.Rel(root, leak)
		t.Errorf("SINK LEAK — the secret is persisted in %s", rel)
	}

	// The learning read back through the public interface must be redacted too.
	var getOut, getErr bytes.Buffer
	if code := run([]string{
		"evidence", "list", learningID,
		"--project-root", root,
		"--json",
	}, &getOut, &getErr); code != exitSuccess {
		t.Fatalf("evidence list exit = %d: %s", code, getErr.String())
	}
	if bytes.Contains(getOut.Bytes(), []byte(secret)) {
		t.Errorf("SINK LEAK — evidence list response contains the secret:\n%s", getOut.String())
	}
	if !bytes.Contains(getOut.Bytes(), []byte("[REDACTED:")) {
		t.Errorf("evidence list shows no redaction marker; redaction did not run:\n%s", getOut.String())
	}
}

// TestCLI_IdempotencyKeyDoesNotDuplicateEvidence covers D5: the same
// idempotency_key on a retry is a technical retry. It creates neither a second
// learning nor a second copy of the evidence.
func TestCLI_IdempotencyKeyDoesNotDuplicateEvidence(t *testing.T) {
	root := initProject(t)

	args := []string{
		"--project-root", root,
		"--title", "Idempotent capture",
		"--context", "The client retried after a network timeout",
		"--observation", "The same capture arrived twice",
		"--lesson", "A technical retry is not a new learning",
		"--destination", "project",
		"--evidence-level", "moderate",
		"--idempotency-key", "session-42/task-7/lesson-1",
		"--evidence-summary", "The retry produced an identical payload",
		"--evidence-kind", "text",
		"--evidence-content", "attempt 1 and attempt 2 are byte-identical",
		"--json",
	}

	first := captureJSON(t, args...)
	firstID, _ := first["learning_id"].(string)
	if newFlag, _ := first["new"].(bool); !newFlag {
		t.Fatal("first capture reported new = false")
	}
	if count, _ := first["evidence_count"].(float64); count != 1 {
		t.Fatalf("first capture evidence_count = %v, want 1", first["evidence_count"])
	}

	second := captureJSON(t, args...)
	secondID, _ := second["learning_id"].(string)

	if secondID != firstID {
		t.Fatalf("retry created a SECOND learning: %s != %s", secondID, firstID)
	}
	if newFlag, _ := second["new"].(bool); newFlag {
		t.Error("retry reported new = true; the same idempotency key must be a technical retry")
	}

	// The decisive assertion: the evidence was not duplicated.
	var listOut, listErr bytes.Buffer
	if code := run([]string{
		"evidence", "list", firstID,
		"--project-root", root,
		"--json",
	}, &listOut, &listErr); code != exitSuccess {
		t.Fatalf("evidence list exit = %d: %s", code, listErr.String())
	}
	var listed map[string]any
	if err := json.Unmarshal(listOut.Bytes(), &listed); err != nil {
		t.Fatalf("evidence list stdout is not JSON: %v\n%s", err, listOut.String())
	}
	if count, _ := listed["evidence_count"].(float64); count != 1 {
		t.Fatalf("evidence_count after retry = %v, want 1: the retry DUPLICATED the evidence", listed["evidence_count"])
	}
}

// TestCLI_CaptureExposesDestinationAndEvidenceLevel guards the two flags whose
// absence made two of the five curate actions structurally unusable from the
// CLI, and made every CLI learning born `insufficient`.
func TestCLI_CaptureExposesDestinationAndEvidenceLevel(t *testing.T) {
	root := initProject(t)

	captured := captureJSON(t,
		"--project-root", root,
		"--title", "Destination flag reaches the skill decision",
		"--context", "Recorrido B",
		"--observation", "Without a destination flag approve_new_skill always fails",
		"--lesson", "A CLI that cannot express a destination cannot approve a skill",
		"--type", "procedure",
		"--destination", "skill",
		"--evidence-level", "strong",
		"--confidence", "high",
		"--evidence-summary", "Curate accepted approve_new_skill",
		"--evidence-kind", "test",
		"--json",
	)
	learningID, _ := captured["learning_id"].(string)

	// approve_new_skill demands proposed destination "skill". Before --destination
	// existed this always failed with `got "project"`.
	var out, errBuf bytes.Buffer
	if code := run([]string{
		"curate", "--project-root", root,
		"--learning-id", learningID,
		"--action", "approve_new_skill",
		"--rationale", "The learning is a reusable procedure worth a skill",
		"--json",
	}, &out, &errBuf); code != exitSuccess {
		t.Fatalf("approve_new_skill exit = %d, want %d\nstderr: %s", code, exitSuccess, errBuf.String())
	}
	var result map[string]any
	_ = json.Unmarshal(out.Bytes(), &result)
	if got, _ := result["new_status"].(string); got != "approved" {
		t.Fatalf("new_status = %q, want approved", got)
	}
}
