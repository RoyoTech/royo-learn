package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunExperienceInjectFixture(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".royo-learn"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".royo-learn", "config.yaml"), []byte(`version: 1
`), 0o600); err != nil {
		t.Fatal(err)
	}
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
