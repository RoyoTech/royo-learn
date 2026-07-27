package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-royo-learn/internal/testutil"
)

// claudeCodeFixtureSubdir is the stable, well-known subdirectory inside the
// project root that the CLI tests use to host their Claude Code-shaped
// fixture. Mirrors the opencode precedent in experience_test.go.
const claudeCodeFixtureSubdir = ".claude-fixture"

// writeClaudeCodeJSONLFixture writes the given JSONL content verbatim under
// the project root so the locator validation accepts it.
func writeClaudeCodeJSONLFixture(t *testing.T, root string, jsonl string) string {
	t.Helper()
	jsonlPath := filepath.Join(root, claudeCodeFixtureSubdir, "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(jsonlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonlPath, []byte(jsonl), 0o600); err != nil {
		t.Fatal(err)
	}
	return jsonlPath
}

// minimalClaudeCodeJSONL contains one user turn followed by an assistant
// turn with stop_reason set, so the adapter emits exactly one envelope.
const minimalClaudeCodeJSONL = `{"type":"user","uuid":"turn-user-1","sessionId":"session-1","timestamp":"2026-01-02T03:04:05Z","message":{"content":"hello"}}
{"type":"assistant","uuid":"turn-assistant-1","sessionId":"session-1","timestamp":"2026-01-02T03:04:06Z","stop_reason":"end_turn","message":{"content":"hi"}}
`

func TestRunExperienceClaudecodeRequiresSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"claude-code"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero; stderr=%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "subcommand is required") {
		t.Fatalf("stderr = %q, want it to mention a required subcommand", stderr.String())
	}
}

func TestRunExperienceClaudecodeUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"claude-code", "watch"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero; stderr=%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown subcommand") {
		t.Fatalf("stderr = %q, want unknown-subcommand message", stderr.String())
	}
}

func TestRunExperienceClaudecodeScanMissingProjectRoot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"claude-code", "scan"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero; stderr=%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--project-root") {
		t.Fatalf("stderr = %q, want it to mention --project-root", stderr.String())
	}
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (invalid_argument); stderr=%s", code, stderr.String())
	}
}

// TestRunExperienceClaudecodeScanHappyPath verifies that with a synthetic
// JSONL fixture inside the project root, the orchestrator runs end-to-end:
// discovers nothing extra, scans the fixture, ingests the one complete turn,
// and emits a JSON envelope with source=claude_code, status=ok.
func TestRunExperienceClaudecodeScanHappyPath(t *testing.T) {
	root := setupProjectRoot(t)
	fixture := writeClaudeCodeJSONLFixture(t, root, minimalClaudeCodeJSONL)

	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"claude-code", "scan", "--project-root", root, "--fixture", fixture}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s, stdout=%s", code, stderr.String(), stdout.String())
	}
	var out struct {
		Source         string `json:"source"`
		Status         string `json:"status"`
		IngestedTurns  int    `json:"ingested_turns"`
		Duplicates     int    `json:"duplicates"`
		SkippedIncomp  int    `json:"skipped_incomplete"`
		SkippedMalform int    `json:"skipped_malformed"`
		EnvelopesTotal int    `json:"envelopes_total"`
		Instances      []struct {
			JSONLPath      string `json:"jsonl_path"`
			Status         string `json:"status"`
			IngestedTurns  int    `json:"ingested_turns"`
			Duplicates     int    `json:"duplicates"`
			SkippedIncomp  int    `json:"skipped_incomplete"`
			SkippedMalform int    `json:"skipped_malformed"`
			EnvelopesTotal int    `json:"envelopes_total"`
		} `json:"instances"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout: %v (%s)", err, stdout.String())
	}
	if out.Source != "claude_code" {
		t.Fatalf("source = %q, want claude_code", out.Source)
	}
	if out.Status != "ok" {
		t.Fatalf("status = %q, want ok", out.Status)
	}
	if len(out.Instances) != 1 {
		t.Fatalf("instances = %d, want 1", len(out.Instances))
	}
	if out.Instances[0].JSONLPath != fixture {
		t.Fatalf("instance jsonl_path = %q, want %q", out.Instances[0].JSONLPath, fixture)
	}
	if out.Instances[0].Status != "ok" {
		t.Fatalf("instance status = %q, want ok", out.Instances[0].Status)
	}
	if out.Instances[0].IngestedTurns != 1 {
		t.Fatalf("instance ingested_turns = %d, want 1", out.Instances[0].IngestedTurns)
	}
	if out.Instances[0].EnvelopesTotal != 1 {
		t.Fatalf("instance envelopes_total = %d, want 1", out.Instances[0].EnvelopesTotal)
	}
	if out.IngestedTurns != 1 {
		t.Fatalf("top-level ingested_turns = %d, want 1", out.IngestedTurns)
	}
	if out.EnvelopesTotal != 1 {
		t.Fatalf("top-level envelopes_total = %d, want 1", out.EnvelopesTotal)
	}
}

// TestRunExperienceClaudecodeScanIdempotent verifies that running the scan
// twice against the same fixture produces IngestedTurns=0 and Duplicates=1
// on the second run.
func TestRunExperienceClaudecodeScanIdempotent(t *testing.T) {
	root := setupProjectRoot(t)
	fixture := writeClaudeCodeJSONLFixture(t, root, minimalClaudeCodeJSONL)

	// First run.
	var stdout1, stderr1 bytes.Buffer
	code1 := runExperience([]string{"claude-code", "scan", "--project-root", root, "--fixture", fixture}, &stdout1, &stderr1)
	if code1 != 0 {
		t.Fatalf("first run: exit=%d, stderr=%s, stdout=%s", code1, stderr1.String(), stdout1.String())
	}
	var out1 struct {
		IngestedTurns int `json:"ingested_turns"`
	}
	if err := json.Unmarshal(stdout1.Bytes(), &out1); err != nil {
		t.Fatalf("first stdout: %v (%s)", err, stdout1.String())
	}
	if out1.IngestedTurns != 1 {
		t.Fatalf("first run ingested_turns = %d, want 1", out1.IngestedTurns)
	}

	// Second run: idempotent.
	var stdout2, stderr2 bytes.Buffer
	code2 := runExperience([]string{"claude-code", "scan", "--project-root", root, "--fixture", fixture}, &stdout2, &stderr2)
	if code2 != 0 {
		t.Fatalf("second run: exit=%d, stderr=%s, stdout=%s", code2, stderr2.String(), stdout2.String())
	}
	var out2 struct {
		IngestedTurns int `json:"ingested_turns"`
		Duplicates    int `json:"duplicates"`
	}
	if err := json.Unmarshal(stdout2.Bytes(), &out2); err != nil {
		t.Fatalf("second stdout: %v (%s)", err, stdout2.String())
	}
	if out2.IngestedTurns != 0 {
		t.Fatalf("second run ingested_turns = %d, want 0", out2.IngestedTurns)
	}
	if out2.Duplicates != 1 {
		t.Fatalf("second run duplicates = %d, want 1", out2.Duplicates)
	}
}

// TestRunExperienceClaudecodeScanSymlinkRejected verifies that --fixture
// pointing at a symlink is rejected by the same symlink guard discover() uses.
func TestRunExperienceClaudecodeScanSymlinkRejected(t *testing.T) {
	root := setupProjectRoot(t)
	target := writeClaudeCodeJSONLFixture(t, root, minimalClaudeCodeJSONL)
	dir := testutil.TempDir(t)
	link := filepath.Join(dir, "session-link.jsonl")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported in this env: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"claude-code", "scan", "--project-root", root, "--fixture", link}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit = 0, want non-zero (symlink should be rejected); stdout=%s", stdout.String())
	}
}
