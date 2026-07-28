package codex

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

const validSessionMeta = `{"timestamp":"2026-07-27T12:00:00Z","type":"session_meta","payload":{"codex_session_id":"session-001","cwd":"/tmp/project","cli_version":"1.2.3"}}`

func TestHealth_OKAndReadOnly(t *testing.T) {
	path := writeHealthRollout(t, "rollout-ok.jsonl", validSessionMeta+"\n")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	result := NewAdapter().Health(context.Background(), validHealthInstance(path))
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || !result.Readable || !result.SchemaOK || result.Code != "" {
		t.Fatalf("Health(valid) = %+v", result)
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("Health mutated the rollout source")
	}
}

func TestHealth_MissingAndInvalidSchema(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.jsonl")
	result := NewAdapter().Health(context.Background(), validHealthInstance(missing))
	if result.Status != "degraded" || result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("Health(missing) = %+v", result)
	}

	invalid := writeHealthRollout(t, "rollout-invalid.jsonl", `{"type":"event_msg","payload":{"type":"user_message"}}`+"\n")
	result = NewAdapter().Health(context.Background(), validHealthInstance(invalid))
	if result.Status != "degraded" || result.Code != string(domain.ErrExperienceSchemaUnsupported) || !result.Readable {
		t.Fatalf("Health(invalid schema) = %+v", result)
	}
}

func TestHealth_RequiresCodexInstance(t *testing.T) {
	result := NewAdapter().Health(context.Background(), SourceInstance{Source: domain.SourceOpenCode, RolloutPath: "ignored"})
	if result.Status != "error" || result.Code != string(domain.ErrInvalidArgument) {
		t.Fatalf("Health(wrong source) = %+v", result)
	}
	result = NewAdapter().Health(context.Background(), SourceInstance{Source: domain.SourceCodex})
	if result.Status != "error" || result.Code != string(domain.ErrInvalidArgument) {
		t.Fatalf("Health(empty path) = %+v", result)
	}
}

func TestHealth_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := NewAdapter().Health(ctx, SourceInstance{})
	if result.Status != "error" || result.Code != string(domain.ErrTimeout) {
		t.Fatalf("Health(canceled) = %+v", result)
	}
}

func TestHealth_HeaderMustFitFirstKiB(t *testing.T) {
	path := writeHealthRollout(t, "rollout-late-meta.jsonl", string(make([]byte, healthHeaderBytes))+validSessionMeta+"\n")
	result := NewAdapter().Health(context.Background(), validHealthInstance(path))
	if result.Status != "degraded" || result.Code != string(domain.ErrExperienceSchemaUnsupported) {
		t.Fatalf("Health(late session_meta) = %+v", result)
	}
}

func validHealthInstance(path string) SourceInstance {
	return SourceInstance{Source: domain.SourceCodex, ProjectRoot: filepath.Dir(path), RolloutPath: path, Schema: SchemaTag}
}

func writeHealthRollout(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHealth_CheckedAtUTC(t *testing.T) {
	path := writeHealthRollout(t, "rollout-utc.jsonl", validSessionMeta+"\n")
	adapter := NewAdapter()
	loc := time.FixedZone("test", -3*60*60)
	adapter.Now = func() time.Time { return time.Date(2026, 7, 27, 9, 0, 0, 0, loc) }
	if got := adapter.Health(context.Background(), validHealthInstance(path)).CheckedAt.Location(); got != time.UTC {
		t.Fatalf("CheckedAt location = %v, want UTC", got)
	}
}

// TestHealth_SessionMetaMissingRequiredFields covers the validation branch
// in verifyCodexHeader (health.go:81-83) where one or more required fields
// of session_meta are empty.
func TestHealth_SessionMetaMissingRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		meta string
	}{
		{
			"empty cwd",
			`{"type":"session_meta","payload":{"codex_session_id":"s","cwd":"","cli_version":"1.0"}}`,
		},
		{
			"empty cli_version",
			`{"type":"session_meta","payload":{"codex_session_id":"s","cwd":"/tmp","cli_version":""}}`,
		},
		{
			"empty session id",
			`{"type":"session_meta","payload":{"cwd":"/tmp","cli_version":"1.0"}}`,
		},
		{
			"whitespace session id",
			`{"type":"session_meta","payload":{"codex_session_id":"   ","cwd":"/tmp","cli_version":"1.0"}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeHealthRollout(t, "rollout-"+tc.name+".jsonl", tc.meta+"\n")
			result := NewAdapter().Health(context.Background(), validHealthInstance(path))
			if result.Status != "degraded" || result.Code != string(domain.ErrExperienceSchemaUnsupported) {
				t.Fatalf("Health(%s) = %+v", tc.name, result)
			}
			if !result.Readable {
				t.Fatalf("Health(%s).Readable = false, want true (file was readable)", tc.name)
			}
		})
	}
}

// TestHealth_HeaderOpenFailsOnUnreadableFile covers verifyCodexHeader's
// os.Open error path (health.go:55-57). chmod 0 on a file leaves Lstat
// working but makes Open fail with EACCES.
func TestHealth_HeaderOpenFailsOnUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0 file test relies on Unix file permissions")
	}
	if os.Geteuid() == 0 {
		t.Skip("chmod 0 has no effect when running as root")
	}
	path := writeHealthRollout(t, "rollout-unreadable.jsonl", validSessionMeta+"\n")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	result := NewAdapter().Health(context.Background(), validHealthInstance(path))
	if result.Status != "degraded" || result.Code != string(domain.ErrExperienceSchemaUnsupported) {
		t.Fatalf("Health(unreadable) = %+v", result)
	}
	if !result.Readable {
		t.Fatalf("Health(unreadable).Readable = false, want true (stat succeeded)")
	}
}

// TestHealth_DirectoryPathReturnsDegraded covers the IsDir branch of the
// Stat check (health.go:35). Passing a directory path must surface as
// "degraded" with the documented source-not-found code, without attempting
// to read it as a rollout.
func TestHealth_DirectoryPathReturnsDegraded(t *testing.T) {
	dir := t.TempDir()
	result := NewAdapter().Health(context.Background(), validHealthInstance(dir))
	if result.Status != "degraded" || result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("Health(directory) = %+v", result)
	}
}

// TestHealth_EmptyFileReturnsDegraded covers the io.EOF branch in
// verifyCodexHeader (health.go:72-73): when the file is empty (or
// whitespace-only within the first KiB), session_meta is not found.
func TestHealth_EmptyFileReturnsDegraded(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"truly empty", ""},
		{"whitespace only", "   \n\t\n"},
		{"garbage before meta", "not-json-at-all\nstill-not-json\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeHealthRollout(t, "rollout-"+tc.name+".jsonl", tc.content)
			result := NewAdapter().Health(context.Background(), validHealthInstance(path))
			if result.Status != "degraded" || result.Code != string(domain.ErrExperienceSchemaUnsupported) {
				t.Fatalf("Health(%s) = %+v", tc.name, result)
			}
		})
	}
}

// TestHealth_MalformedJSONReturnsDegraded covers the Decode error branch
// in verifyCodexHeader (health.go:75-76): when a JSONL line is invalid
// JSON, the probe must surface a stable schema-unsupported error.
func TestHealth_MalformedJSONReturnsDegraded(t *testing.T) {
	path := writeHealthRollout(t, "rollout-malformed.jsonl", `{"type":"session_meta","payload":{`+"\n")
	result := NewAdapter().Health(context.Background(), validHealthInstance(path))
	if result.Status != "degraded" || result.Code != string(domain.ErrExperienceSchemaUnsupported) {
		t.Fatalf("Health(malformed) = %+v", result)
	}
}

// TestHealth_SessionMetaNotInFirstKiB covers the case where the first KiB
// of a valid file does NOT contain session_meta, even if the file is
// shorter than 1 KiB. The documented stable outcome is "degraded" with
// ErrExperienceSchemaUnsupported.
func TestHealth_SessionMetaNotInFirstKiB(t *testing.T) {
	path := writeHealthRollout(t, "rollout-no-meta.jsonl",
		`{"type":"event_msg","payload":{"type":"user_message","message":"hi"}}`+"\n"+
			`{"type":"response_item","payload":{"text":"hello"}}`+"\n")
	result := NewAdapter().Health(context.Background(), validHealthInstance(path))
	if result.Status != "degraded" || result.Code != string(domain.ErrExperienceSchemaUnsupported) {
		t.Fatalf("Health(no meta) = %+v", result)
	}
}
