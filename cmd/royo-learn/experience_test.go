package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-royo-learn/internal/testutil"
	_ "modernc.org/sqlite"
)

// opencodeFixtureSchema is the minimal OpenCode schema the CLI path expects.
// Mirrors internal/experience/opencode/opencodeTestSchema; the CLI never
// imports the opencode package's test helpers, so the schema lives here too.
const opencodeFixtureSchema = `
CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    started_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    closed_at   INTEGER
);
CREATE TABLE messages (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL,
    sequence    INTEGER NOT NULL,
    role        TEXT NOT NULL,
    content     TEXT,
    finish      TEXT,
    created_at  INTEGER NOT NULL,
    complete    INTEGER NOT NULL DEFAULT 1,
    revision    TEXT
);
CREATE INDEX idx_messages_session ON messages(session_id, sequence);
`

// setupProjectRoot creates a project root with .royo-learn/config.yaml so
// resolvePublishContext accepts it.
func setupProjectRoot(t *testing.T) string {
	t.Helper()
	root := testutil.TempDir(t)
	if err := os.MkdirAll(filepath.Join(root, ".royo-learn"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".royo-learn", "config.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// writeOpencodeFixture writes a minimal OpenCode-shaped SQLite database at
// the given path. The schema is created; the populate callback (when non-nil)
// can insert sessions and messages. Returns the same path for chaining.
func writeOpencodeFixture(t *testing.T, path string, populate func(*sql.DB)) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	if _, err := db.Exec(opencodeFixtureSchema); err != nil {
		_ = db.Close()
		t.Fatalf("create schema: %v", err)
	}
	if populate != nil {
		populate(db)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
}

// fixtureSubdir is a stable, well-known subdirectory inside the project root
// that the CLI tests use to host their OpenCode-shaped fixture. Keeping the
// fixture inside the project root is required because the core service rejects
// experience envelopes whose locator path is outside the stored project root.
const fixtureSubdir = ".opencode-fixture"

// makeOpencodeFixtureWithTurns writes a fixture containing one session and
// one complete turn (role=assistant, content="hello"). The fixture is created
// inside `root` so the locator validation accepts it.
func makeOpencodeFixtureWithTurns(t *testing.T, root string) string {
	t.Helper()
	dbPath := filepath.Join(root, fixtureSubdir, "opencode.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeOpencodeFixture(t, dbPath, func(db *sql.DB) {
		if _, err := db.Exec(
			"INSERT INTO sessions(id, project_id, started_at, updated_at) VALUES(?, ?, ?, ?)",
			"session-1", "project-1", int64(1700000000000), int64(1700000001000),
		); err != nil {
			t.Fatalf("insert session: %v", err)
		}
		if _, err := db.Exec(
			"INSERT INTO messages(id, session_id, sequence, role, content, finish, created_at, complete, revision) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)",
			"turn-1", "session-1", int64(1), "assistant", "hello", "stop", int64(1700000001000), 1, "rev-1",
		); err != nil {
			t.Fatalf("insert message: %v", err)
		}
	})
	return dbPath
}

// makeOpencodeFixtureEmpty writes a fixture with the schema but no rows.
// Created inside `root` for the same reason as makeOpencodeFixtureWithTurns.
func makeOpencodeFixtureEmpty(t *testing.T, root string) string {
	t.Helper()
	dbPath := filepath.Join(root, fixtureSubdir, "opencode.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeOpencodeFixture(t, dbPath, nil)
	return dbPath
}

func TestRunExperienceOpencodeRequiresSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"opencode"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero; stderr=%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "subcommand is required") {
		t.Fatalf("stderr = %q, want it to mention a required subcommand", stderr.String())
	}
}

func TestRunExperienceOpencodeUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"opencode", "watch"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero; stderr=%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown subcommand") {
		t.Fatalf("stderr = %q, want unknown-subcommand message", stderr.String())
	}
}

func TestRunExperienceOpencodeScanMissingProjectRoot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"opencode", "scan"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero; stderr=%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--project-root") {
		t.Fatalf("stderr = %q, want it to mention --project-root", stderr.String())
	}
}

func TestRunExperienceOpencodeScanInvalidProjectRoot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"opencode", "scan", "--project-root", "/definitely/does/not/exist/royo-learn-cli-test"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero; stderr=%s", stderr.String())
	}
}

func TestRunExperienceOpencodeScanEmptyDiscovery(t *testing.T) {
	root := setupProjectRoot(t)
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"opencode", "scan", "--project-root", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s, stdout=%s", code, stderr.String(), stdout.String())
	}
	var out struct {
		Source    string `json:"source"`
		Status    string `json:"status"`
		Instances []any  `json:"instances"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout: %v (%s)", err, stdout.String())
	}
	if out.Source != "opencode" {
		t.Fatalf("source = %q, want opencode", out.Source)
	}
	if out.Status != "ok" {
		t.Fatalf("status = %q, want ok", out.Status)
	}
	if len(out.Instances) != 0 {
		t.Fatalf("instances = %d, want 0", len(out.Instances))
	}
}

func TestRunExperienceOpencodeScanFixture(t *testing.T) {
	root := setupProjectRoot(t)
	fixture := makeOpencodeFixtureEmpty(t, root)
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"opencode", "scan", "--project-root", root, "--fixture", fixture}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s, stdout=%s", code, stderr.String(), stdout.String())
	}
	var out struct {
		Source         string `json:"source"`
		Status         string `json:"status"`
		Instances      []struct {
			DBPath         string `json:"db_path"`
			Status         string `json:"status"`
			IngestedTurns  int    `json:"ingested_turns"`
			Duplicates     int    `json:"duplicates"`
			SkippedIncomp  int    `json:"skipped_incomplete"`
			EnvelopesTotal int    `json:"envelopes_total"`
		} `json:"instances"`
		IngestedTurns  int `json:"ingested_turns"`
		Duplicates     int `json:"duplicates"`
		SkippedIncomp  int `json:"skipped_incomplete"`
		EnvelopesTotal int `json:"envelopes_total"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout: %v (%s)", err, stdout.String())
	}
	if out.Source != "opencode" {
		t.Fatalf("source = %q, want opencode", out.Source)
	}
	if out.Status != "ok" {
		t.Fatalf("status = %q, want ok", out.Status)
	}
	if len(out.Instances) != 1 {
		t.Fatalf("instances = %d, want 1", len(out.Instances))
	}
	if out.Instances[0].Status != "ok" {
		t.Fatalf("instance status = %q, want ok", out.Instances[0].Status)
	}
	if out.EnvelopesTotal != 0 {
		t.Fatalf("envelopes_total = %d, want 0 (empty fixture)", out.EnvelopesTotal)
	}
}

func TestRunExperienceOpencodeScanFixtureIngestAndIdempotent(t *testing.T) {
	root := setupProjectRoot(t)
	fixture := makeOpencodeFixtureWithTurns(t, root)

	// First run: ingests the one complete turn.
	var stdout1, stderr1 bytes.Buffer
	code1 := runExperience([]string{"opencode", "scan", "--project-root", root, "--fixture", fixture}, &stdout1, &stderr1)
	if code1 != 0 {
		t.Fatalf("first run: exit=%d, stderr=%s, stdout=%s", code1, stderr1.String(), stdout1.String())
	}
	var out1 struct {
		Status         string `json:"status"`
		IngestedTurns  int    `json:"ingested_turns"`
		Duplicates     int    `json:"duplicates"`
		EnvelopesTotal int    `json:"envelopes_total"`
		Instances      []struct {
			Status         string `json:"status"`
			IngestedTurns  int    `json:"ingested_turns"`
			Duplicates     int    `json:"duplicates"`
			EnvelopesTotal int    `json:"envelopes_total"`
		} `json:"instances"`
	}
	if err := json.Unmarshal(stdout1.Bytes(), &out1); err != nil {
		t.Fatalf("first run stdout: %v (%s)", err, stdout1.String())
	}
	if out1.IngestedTurns != 1 {
		t.Fatalf("first run ingested_turns = %d, want 1; out=%+v", out1.IngestedTurns, out1)
	}
	if out1.EnvelopesTotal != 1 {
		t.Fatalf("first run envelopes_total = %d, want 1", out1.EnvelopesTotal)
	}
	if out1.Status != "ok" {
		t.Fatalf("first run status = %q, want ok", out1.Status)
	}
	if len(out1.Instances) != 1 || out1.Instances[0].IngestedTurns != 1 {
		t.Fatalf("first run per-instance report = %+v, want 1 instance with ingested_turns=1", out1.Instances)
	}

	// Second run: same fixture, same project root → the service must report
	// the turn as a duplicate (idempotency by source + external IDs).
	var stdout2, stderr2 bytes.Buffer
	code2 := runExperience([]string{"opencode", "scan", "--project-root", root, "--fixture", fixture}, &stdout2, &stderr2)
	if code2 != 0 {
		t.Fatalf("second run: exit=%d, stderr=%s, stdout=%s", code2, stderr2.String(), stdout2.String())
	}
	var out2 struct {
		Status         string `json:"status"`
		IngestedTurns  int    `json:"ingested_turns"`
		Duplicates     int    `json:"duplicates"`
		EnvelopesTotal int    `json:"envelopes_total"`
	}
	if err := json.Unmarshal(stdout2.Bytes(), &out2); err != nil {
		t.Fatalf("second run stdout: %v (%s)", err, stdout2.String())
	}
	if out2.IngestedTurns != 0 {
		t.Fatalf("second run ingested_turns = %d, want 0", out2.IngestedTurns)
	}
	if out2.Duplicates != 1 {
		t.Fatalf("second run duplicates = %d, want 1", out2.Duplicates)
	}
}

func TestRunExperienceOpencodeScanFixtureSkipsIncomplete(t *testing.T) {
	root := setupProjectRoot(t)
	if err := os.MkdirAll(filepath.Join(root, fixtureSubdir), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(root, fixtureSubdir, "opencode.db")
	writeOpencodeFixture(t, fixture, func(db *sql.DB) {
		if _, err := db.Exec(
			"INSERT INTO sessions(id, project_id, started_at, updated_at) VALUES(?, ?, ?, ?)",
			"session-1", "project-1", int64(1700000000000), int64(1700000001000),
		); err != nil {
			t.Fatalf("insert session: %v", err)
		}
		// One complete turn and one incomplete turn.
		if _, err := db.Exec(
			"INSERT INTO messages(id, session_id, sequence, role, content, finish, created_at, complete, revision) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)",
			"turn-complete", "session-1", int64(1), "assistant", "hello", "stop", int64(1700000001000), 1, "rev-1",
		); err != nil {
			t.Fatalf("insert complete: %v", err)
		}
		if _, err := db.Exec(
			"INSERT INTO messages(id, session_id, sequence, role, content, finish, created_at, complete, revision) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)",
			"turn-incomplete", "session-1", int64(2), "assistant", "in-flight", "", int64(1700000002000), 0, "rev-1",
		); err != nil {
			t.Fatalf("insert incomplete: %v", err)
		}
	})

	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"opencode", "scan", "--project-root", root, "--fixture", fixture}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s, stdout=%s", code, stderr.String(), stdout.String())
	}
	var out struct {
		Instances []struct {
			IngestedTurns int `json:"ingested_turns"`
			SkippedIncomp int `json:"skipped_incomplete"`
		} `json:"instances"`
		SkippedIncomp int `json:"skipped_incomplete"`
		IngestedTurns int `json:"ingested_turns"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout: %v (%s)", err, stdout.String())
	}
	if len(out.Instances) != 1 {
		t.Fatalf("instances = %d, want 1", len(out.Instances))
	}
	if out.Instances[0].IngestedTurns != 1 {
		t.Fatalf("ingested_turns = %d, want 1", out.Instances[0].IngestedTurns)
	}
	if out.Instances[0].SkippedIncomp != 1 {
		t.Fatalf("skipped_incomplete = %d, want 1", out.Instances[0].SkippedIncomp)
	}
	if out.SkippedIncomp != 1 {
		t.Fatalf("total skipped_incomplete = %d, want 1", out.SkippedIncomp)
	}
}

func TestRunExperienceOpencodeScanFixtureSymlinkRejected(t *testing.T) {
	root := setupProjectRoot(t)
	target := makeOpencodeFixtureWithTurns(t, root)
	dir := testutil.TempDir(t)
	link := filepath.Join(dir, "opencode-link.db")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported in this env: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"opencode", "scan", "--project-root", root, "--fixture", link}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit = 0, want non-zero (symlink should be rejected); stdout=%s", stdout.String())
	}
}

// makeOpencodeFixtureWithSecretTurn writes a fixture containing one complete
// turn whose assistant content embeds a pattern that the core redacts (a
// fake API key). Used by the security roundtrip test below.
func makeOpencodeFixtureWithSecretTurn(t *testing.T, root string) string {
	t.Helper()
	dbPath := filepath.Join(root, fixtureSubdir, "opencode.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeOpencodeFixture(t, dbPath, func(db *sql.DB) {
		if _, err := db.Exec(
			"INSERT INTO sessions(id, project_id, started_at, updated_at) VALUES(?, ?, ?, ?)",
			"session-secret", "project-secret", int64(1700000000000), int64(1700000001000),
		); err != nil {
			t.Fatalf("insert session: %v", err)
		}
		if _, err := db.Exec(
			"INSERT INTO messages(id, session_id, sequence, role, content, finish, created_at, complete, revision) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)",
			"turn-secret", "session-secret", int64(1), "assistant", "export API_KEY=sk-abc123secretvalue", "stop", int64(1700000001000), 1, "rev-secret",
		); err != nil {
			t.Fatalf("insert message: %v", err)
		}
	})
	return dbPath
}

// TestRunExperienceOpencodeScanFixtureOutsideRootRejected verifies that the
// core service rejects envelopes whose locator path is outside the stored
// project root. The fixture is created in a separate temp dir, so the scan
// must produce envelopes whose locator is outside `root` and the service
// must refuse to ingest them, surfacing the error to the CLI.
func TestRunExperienceOpencodeScanFixtureOutsideRootRejected(t *testing.T) {
	root := setupProjectRoot(t)
	outside := makeOpencodeFixtureWithTurns(t, root)
	// Move the fixture to a sibling temp dir that is NOT under `root` so the
	// canonical locator falls outside the stored project root.
	sibling := testutil.TempDir(t)
	moved := filepath.Join(sibling, "opencode.db")
	if err := os.Rename(outside, moved); err != nil {
		t.Fatalf("move fixture: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"opencode", "scan", "--project-root", root, "--fixture", moved}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit = 0, want non-zero; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

// TestRunExperienceOpencodeScanFixtureSecretIdempotent verifies that a turn
// whose content embeds a secret-like pattern is ingested on the first pass
// and recognized as a duplicate on the second pass. The fingerprint must
// remain stable across the redaction step in the core service, otherwise
// idempotency would break for any turn that contains a secret.
func TestRunExperienceOpencodeScanFixtureSecretIdempotent(t *testing.T) {
	root := setupProjectRoot(t)
	fixture := makeOpencodeFixtureWithSecretTurn(t, root)

	var stdout1, stderr1 bytes.Buffer
	code1 := runExperience([]string{"opencode", "scan", "--project-root", root, "--fixture", fixture}, &stdout1, &stderr1)
	if code1 != 0 {
		t.Fatalf("first run: exit=%d, stderr=%s, stdout=%s", code1, stderr1.String(), stdout1.String())
	}
	var out1 struct {
		IngestedTurns int `json:"ingested_turns"`
		Duplicates    int `json:"duplicates"`
	}
	if err := json.Unmarshal(stdout1.Bytes(), &out1); err != nil {
		t.Fatalf("first stdout: %v (%s)", err, stdout1.String())
	}
	if out1.IngestedTurns != 1 {
		t.Fatalf("first run ingested_turns = %d, want 1 (secret content)", out1.IngestedTurns)
	}

	var stdout2, stderr2 bytes.Buffer
	code2 := runExperience([]string{"opencode", "scan", "--project-root", root, "--fixture", fixture}, &stdout2, &stderr2)
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
	if out2.Duplicates != 1 {
		t.Fatalf("second run duplicates = %d, want 1 (idempotent across redaction)", out2.Duplicates)
	}
	if out2.IngestedTurns != 0 {
		t.Fatalf("second run ingested_turns = %d, want 0", out2.IngestedTurns)
	}
}

func TestRunExperienceInjectFixture(t *testing.T) {
	root := setupProjectRoot(t)
	envelope := `{"schema_version":1,"source":"opencode","project_root":"` + filepath.ToSlash(root) + `","session":{"external_id":"session-cli","updated_at":"2026-07-21T10:00:00Z","locator":{"kind":"sqlite","path":"` + filepath.ToSlash(filepath.Join(root, "source.db")) + `","session_id":"native-session","turn_id":"native-turn"}},"turn":{"external_id":"turn-cli","sequence":1,"complete":true,"finish_reason":"stop","occurred_at":"2026-07-21T10:01:00Z","source_revision":"revision-1","user_text":"fix","assistant_text":"fixed"},"actor":{"kind":"agent","name":"test","model":"test","session_id":"actor"}}`
	fixture := filepath.Join(root, "envelope.json")
	if err := os.WriteFile(fixture, []byte(envelope), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"inject", "--envelope", fixture, "--project-root", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	var got struct {
		SessionID   string `json:"session_id"`
		TurnID      string `json:"turn_id"`
		Fingerprint string `json:"fingerprint"`
		Duplicate   bool   `json:"duplicate"`
		Skipped     bool   `json:"skipped"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout: %v (%s)", err, stdout.String())
	}
	if got.SessionID == "" || got.TurnID == "" || got.Fingerprint == "" || got.Duplicate || got.Skipped {
		t.Fatalf("result = %+v", got)
	}
}
