// Hito 12 — drift CLI tests.
// These tests verify the JSON envelope shape and the PII redaction contract
// (REQ-DCM-3, drift-cli-mcp spec).

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/publish/drift"
	"agent-royo-learn/internal/storage"
)

// driftCLIFixture creates a valid project root with .royo-learn/config.yaml
// and a real SQLite DB at the canonical location (.royo-learn/royo-learn.db)
// that resolvePublishContext will open. Returns the root path, an open DB,
// and a cleanup function.
func driftCLIFixture(t *testing.T) (string, *storage.DB, func()) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".royo-learn"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".royo-learn", "config.yaml"), []byte("project_root: "+root+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	dbPath := filepath.Join(root, ".royo-learn", "royo-learn.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	// Disable FK enforcement for the drift CLI surface: it does not own
	// the publications table, so the CLI can be exercised without seeding
	// parent rows in every test.
	if _, err := db.DB.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		_ = db.Close()
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if err := storage.Migrate(db); err != nil {
		_ = db.Close()
		t.Fatalf("storage.Migrate: %v", err)
	}
	return root, db, func() { _ = db.Close() }
}

func TestRunExperienceDrift_EnvelopeShape(t *testing.T) {
	root, db, cleanup := driftCLIFixture(t)
	defer cleanup()
	repo := drift.NewRepository(db.DB, nil)
	_ = root

	fixedTime := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	rows := []drift.DriftRow{
		{
			PublicationID: "01HZXPUB00000000000000000A",
			Source:        "opencode",
			TargetPath:    "/home/alice/.opencode/sessions/abc.json",
			ExpectedHash:  "expected-aaaa",
			ActualHash:    "actual-aaaa",
			Status:        drift.StatusOK,
			CheckedAt:     fixedTime,
			RunID:         "run-1",
		},
		{
			PublicationID: "01HZXPUB00000000000000000B",
			Source:        "claudecode",
			TargetPath:    "/Users/bob/.claude/sessions/def.jsonl",
			ExpectedHash:  "expected-bbbb",
			ActualHash:    "actual-cccc",
			Status:        drift.StatusDrifted,
			CheckedAt:     fixedTime,
			RunID:         "run-1",
		},
		{
			PublicationID: "01HZXPUB00000000000000000C",
			Source:        "codex",
			TargetPath:    "C:\\Users\\carol\\.codex\\sessions\\ghi.jsonl",
			ExpectedHash:  "expected-dddd",
			ActualHash:    "",
			Status:        drift.StatusTargetMissing,
			CheckedAt:     fixedTime,
			RunID:         "run-1",
		},
	}
	for _, r := range rows {
		if err := repo.RecordDrift(context.Background(), r); err != nil {
			t.Fatalf("RecordDrift: %v", err)
		}
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exit := runExperienceDrift(
		[]string{"--project-root", root, "--all-sources"},
		stdout, stderr,
	)
	if exit != exitSuccess {
		t.Fatalf("runExperienceDrift exit = %d, want %d (stderr=%q)", exit, exitSuccess, stderr.String())
	}

	var out experienceDriftOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal: %v\nstdout: %s", err, stdout.String())
	}

	if out.Status != "ok" {
		t.Errorf("Status = %q, want ok", out.Status)
	}
	if out.Total != 3 {
		t.Errorf("Total = %d, want 3", out.Total)
	}
	if len(out.Publications) != 3 {
		t.Fatalf("Publications len = %d, want 3", len(out.Publications))
	}

	// Verify the four canonical sources are always present (stable shape).
	wantSources := map[string]bool{"opencode": false, "claudecode": false, "codex": false}
	for _, s := range out.Sources {
		if _, ok := wantSources[s.Source]; ok {
			wantSources[s.Source] = true
		}
		switch s.Source {
		case "opencode":
			if s.OK != 1 || s.Drifted != 0 || s.Missing != 0 || s.Unreadable != 0 {
				t.Errorf("opencode summary = %+v, want OK=1", s)
			}
		case "claudecode":
			if s.Drifted != 1 {
				t.Errorf("claudecode summary = %+v, want Drifted=1", s)
			}
		case "codex":
			if s.Missing != 1 {
				t.Errorf("codex summary = %+v, want Missing=1", s)
			}
		}
	}
	for src, present := range wantSources {
		if !present {
			t.Errorf("source %q missing from envelope", src)
		}
	}
}

func TestRunExperienceDrift_RedactsTargetPath(t *testing.T) {
	root, db, cleanup := driftCLIFixture(t)
	defer cleanup()
	repo := drift.NewRepository(db.DB, nil)
	_ = root

	row := drift.DriftRow{
		PublicationID: "01HZXPUB00000000000000000X",
		Source:        "opencode",
		// Unix-style user path: contains /home/alice
		TargetPath:   "/home/alice/.opencode/sessions/secret.json",
		ExpectedHash: "expected",
		ActualHash:   "actual",
		Status:       drift.StatusDrifted,
		CheckedAt:    time.Now().UTC(),
		RunID:        "run-x",
	}
	if err := repo.RecordDrift(context.Background(), row); err != nil {
		t.Fatalf("RecordDrift: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if exit := runExperienceDrift(
		[]string{"--project-root", root, "--source=opencode"},
		stdout, stderr,
	); exit != exitSuccess {
		t.Fatalf("runExperienceDrift exit = %d, stderr=%q", exit, stderr.String())
	}

	body := stdout.String()
	for _, banned := range []string{"/home/", "/Users/", "alice", "C:\\Users"} {
		if strings.Contains(body, banned) {
			t.Errorf("output leaks PII substring %q: %s", banned, body)
		}
	}
	// Must still surface the basename so the operator sees the file name.
	if !strings.Contains(body, "secret.json") {
		t.Errorf("output should contain basename secret.json, got: %s", body)
	}
}

func TestRunExperienceDrift_RequiresProjectRoot(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exit := runExperienceDrift([]string{}, stdout, stderr)
	if exit == exitSuccess {
		t.Fatalf("runExperienceDrift exit = %d, want failure when --project-root missing", exit)
	}
	if !strings.Contains(stderr.String(), "--project-root is required") {
		t.Errorf("stderr = %q, want it to mention --project-root", stderr.String())
	}
}

func TestRunExperienceDrift_RejectsInvalidSource(t *testing.T) {
	root, _, cleanup2 := driftCLIFixture(t)
	defer cleanup2()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exit := runExperienceDrift(
		[]string{"--project-root", root, "--source=invalid"},
		stdout, stderr,
	)
	if exit == exitSuccess {
		t.Fatalf("runExperienceDrift exit = %d, want failure on invalid --source", exit)
	}
	if !strings.Contains(stderr.String(), "invalid --source") {
		t.Errorf("stderr = %q, want it to mention invalid --source", stderr.String())
	}
}
