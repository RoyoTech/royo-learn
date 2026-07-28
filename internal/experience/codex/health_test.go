package codex

import (
	"context"
	"os"
	"path/filepath"
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
