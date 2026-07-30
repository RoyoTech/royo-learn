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

// codexFixtureSubdir is the stable, well-known subdirectory inside the
// project root that the CLI tests use to host their Codex-shaped fixture.
const codexFixtureSubdir = ".codex-fixture"

// writeCodexJSONLFixture writes the given JSONL content verbatim under
// the project root so the locator validation accepts it.
func writeCodexJSONLFixture(t *testing.T, root, name, jsonl string) string {
	t.Helper()
	jsonlPath := filepath.Join(root, codexFixtureSubdir, name)
	if err := os.MkdirAll(filepath.Dir(jsonlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonlPath, []byte(jsonl), 0o600); err != nil {
		t.Fatal(err)
	}
	return jsonlPath
}

const minimalCodexJSONL = `{"timestamp":"2026-01-02T03:04:05Z","type":"session_meta","payload":{"codex_session_id":"session-1","cwd":"/tmp/project","cli_version":"1.2.3"}}
{"timestamp":"2026-01-02T03:04:06Z","type":"turn_context","payload":{"turn_id":"turn-1"}}
{"timestamp":"2026-01-02T03:04:07Z","type":"event_msg","payload":{"type":"event_msg.task_started","turn_id":"turn-1","role":"user","message":"hello"}}
{"timestamp":"2026-01-02T03:04:08Z","type":"event_msg","payload":{"type":"event_msg.task_complete","turn_id":"turn-1","role":"assistant","message":"hi"}}
`

func TestRunExperienceCodexRequiresSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"scan", "--source=codex"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero; stderr=%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--project-root") {
		t.Fatalf("stderr = %q, want it to mention a required subcommand", stderr.String())
	}
}

func TestRunExperienceCodexUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"scan", "--source=codex"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero; stderr=%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--project-root") {
		t.Fatalf("stderr = %q, want unknown-subcommand message", stderr.String())
	}
}

func TestRunExperienceCodexScanMissingProjectRoot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"scan", "--source=codex", "scan"}, &stdout, &stderr)
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

func TestRunExperienceCodexScanHappyPath(t *testing.T) {
	root := setupProjectRoot(t)
	fixture := writeCodexJSONLFixture(t, root, "rollout.jsonl", minimalCodexJSONL)

	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"scan", "--source=codex", "--project-root", root, "--fixture", fixture}, &stdout, &stderr)
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
			RolloutPath    string `json:"rollout_path"`
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
	if out.Source != "codex" {
		t.Fatalf("source = %q, want codex", out.Source)
	}
	if out.Status != "ok" {
		t.Fatalf("status = %q, want ok", out.Status)
	}
	if len(out.Instances) != 1 {
		t.Fatalf("instances = %d, want 1", len(out.Instances))
	}
	if out.Instances[0].RolloutPath != fixture {
		t.Fatalf("instance rollout_path = %q, want %q", out.Instances[0].RolloutPath, fixture)
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
}

func TestRunExperienceCodexScanIdempotent(t *testing.T) {
	root := setupProjectRoot(t)
	fixture := writeCodexJSONLFixture(t, root, "rollout.jsonl", minimalCodexJSONL)

	var stdout1, stderr1 bytes.Buffer
	code1 := runExperience([]string{"scan", "--source=codex", "--project-root", root, "--fixture", fixture}, &stdout1, &stderr1)
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

	var stdout2, stderr2 bytes.Buffer
	code2 := runExperience([]string{"scan", "--source=codex", "--project-root", root, "--fixture", fixture}, &stdout2, &stderr2)
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

func TestRunExperienceCodexScanSymlinkRejected(t *testing.T) {
	root := setupProjectRoot(t)
	target := writeCodexJSONLFixture(t, root, "rollout.jsonl", minimalCodexJSONL)
	dir := testutil.TempDir(t)
	link := filepath.Join(dir, "rollout-link.jsonl")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported in this env: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"scan", "--source=codex", "--project-root", root, "--fixture", link}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit = 0, want non-zero (symlink should be rejected); stdout=%s", stdout.String())
	}
}
