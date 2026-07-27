package claudecode

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

// validJSONLInstance wraps a freshly-created JSONL path into a SourceInstance
// suitable for the Health adapter call. Mirrors opencode's helper shape.
func validJSONLInstance(t *testing.T, jsonlPath string) SourceInstance {
	t.Helper()
	return SourceInstance{
		Source:      domain.SourceClaudeCode,
		ProjectRoot: filepath.Dir(jsonlPath),
		JSONLPath:   jsonlPath,
		Schema:      SchemaTag,
		Discovered:  time.Unix(0, 0).UTC(),
	}
}

// writeJSONL writes the given bytes to a fresh file under t.TempDir() with
// the supplied suffix and returns the absolute path.
func writeJSONL(t *testing.T, name string, content []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
	return path
}

// validHeaderObject is the canonical Claude Code JSONL header shape the
// Health probe accepts as proof that the file is a real session transcript.
const validHeaderObject = `{"type":"user","uuid":"11111111-1111-1111-1111-111111111111","sessionId":"22222222-2222-2222-2222-222222222222","timestamp":"2026-07-24T12:00:00Z","message":{"role":"user","content":"hello"}}`

// TestHealth_OK verifies the canonical happy path: a JSONL file whose first
// 1 KiB contains at least one object with non-empty type / uuid / sessionId
// yields Status="ok" with both flags set.
func TestHealth_OK(t *testing.T) {
	jsonlPath := writeJSONL(t, "session-ok.jsonl", []byte(validHeaderObject+"\n"))
	adapter := NewAdapter()
	adapter.Now = func() time.Time { return time.Unix(0, 0).UTC() }

	result := adapter.Health(context.Background(), validJSONLInstance(t, jsonlPath))
	if result.Status != "ok" {
		t.Fatalf("Health Status = %q, want %q (result=%+v)", result.Status, "ok", result)
	}
	if !result.Readable || !result.SchemaOK {
		t.Fatalf("Health Readable=%v SchemaOK=%v, want both true", result.Readable, result.SchemaOK)
	}
	if result.Code != "" {
		t.Fatalf("Health Code = %q, want empty on success", result.Code)
	}
	if result.JSONLPath != jsonlPath {
		t.Fatalf("Health JSONLPath = %q, want %q", result.JSONLPath, jsonlPath)
	}
	if result.CheckedAt.IsZero() {
		t.Fatalf("Health CheckedAt is zero")
	}
}

// TestHealth_NoSourceSideEffects verifies that Health did not mutate the
// source JSONL. The gate "cero side effects en la fuente Claude Code"
// (docs/24 T8) depends on this property. Mtime must stay identical across
// three Health probes.
func TestHealth_NoSourceSideEffects(t *testing.T) {
	jsonlPath := writeJSONL(t, "session-no-side-effects.jsonl", []byte(validHeaderObject+"\n"))
	before := fileMtime(t, jsonlPath)

	adapter := NewAdapter()
	for i := 0; i < 3; i++ {
		result := adapter.Health(context.Background(), validJSONLInstance(t, jsonlPath))
		if result.Status != "ok" {
			t.Fatalf("Health iteration %d status = %q, want ok (result=%+v)", i, result.Status, result)
		}
	}
	after := fileMtime(t, jsonlPath)
	if before != after {
		t.Fatalf("Health mutated source: mtime before=%d after=%d", before, after)
	}
}

// TestHealth_MissingFile returns degraded + experience_source_not_found when
// the JSONL does not exist. The caller treats this as "source disappeared"
// and reports degraded ingestion; it is never a fatal error.
func TestHealth_MissingFile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.jsonl")
	adapter := NewAdapter()
	result := adapter.Health(context.Background(), validJSONLInstance(t, missing))
	if result.Status != "degraded" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "degraded")
	}
	if result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrExperienceSourceNotFound)
	}
	if result.Readable || result.SchemaOK {
		t.Fatalf("Health flags readable=%v schemaOK=%v, want both false", result.Readable, result.SchemaOK)
	}
}

// TestHealth_PathIsDirectory rejects a directory at the JSONL path. Health
// must not open a directory as a JSONL; this signals "source layout changed"
// and degrades gracefully with experience_source_not_found.
func TestHealth_PathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-dir.jsonl")
	if err := os.MkdirAll(jsonlPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	adapter := NewAdapter()
	result := adapter.Health(context.Background(), validJSONLInstance(t, jsonlPath))
	if result.Status != "degraded" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "degraded")
	}
	if result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrExperienceSourceNotFound)
	}
}

// TestHealth_NonJSONL rejects a file that does not contain a parseable JSON
// object in the first 1 KiB. Health reads the header, fails to decode, and
// degrades with experience_source_schema_unsupported per docs/22 §6.
func TestHealth_NonJSONL(t *testing.T) {
	jsonlPath := writeJSONL(t, "garbage.jsonl", []byte("this is not JSONL at all, definitely not a session\n"))
	adapter := NewAdapter()
	result := adapter.Health(context.Background(), validJSONLInstance(t, jsonlPath))
	if result.Status != "degraded" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "degraded")
	}
	if result.Code != string(domain.ErrExperienceSchemaUnsupported) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrExperienceSchemaUnsupported)
	}
	if !result.Readable {
		t.Fatalf("Health Readable = false, want true (file did open)")
	}
	if result.SchemaOK {
		t.Fatalf("Health SchemaOK = true, want false on non-JSONL")
	}
}

// TestHealth_TruncatedNoObject rejects a JSONL shorter than 1 KiB that
// contains no complete JSON object (e.g. only a partial line). Per docs/24
// T8 ("truncated JSONL"), this degrades with schema_unsupported.
func TestHealth_TruncatedNoObject(t *testing.T) {
	partial := []byte(`{"type":"us`) // 11 bytes, no closing brace, no object complete
	jsonlPath := writeJSONL(t, "truncated.jsonl", partial)
	adapter := NewAdapter()
	result := adapter.Health(context.Background(), validJSONLInstance(t, jsonlPath))
	if result.Status != "degraded" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "degraded")
	}
	if result.Code != string(domain.ErrExperienceSchemaUnsupported) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrExperienceSchemaUnsupported)
	}
}

// TestHealth_HeaderMissingRequiredField rejects a JSONL whose first object
// has the right shape but lacks one of type / uuid / sessionId. The probe
// must surface experience_source_schema_unsupported.
func TestHealth_HeaderMissingRequiredField(t *testing.T) {
	// Missing "sessionId".
	bad := `{"type":"user","uuid":"11111111-1111-1111-1111-111111111111","timestamp":"2026-07-24T12:00:00Z"}`
	jsonlPath := writeJSONL(t, "missing-field.jsonl", []byte(bad+"\n"))
	adapter := NewAdapter()
	result := adapter.Health(context.Background(), validJSONLInstance(t, jsonlPath))
	if result.Status != "degraded" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "degraded")
	}
	if result.Code != string(domain.ErrExperienceSchemaUnsupported) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrExperienceSchemaUnsupported)
	}
}

// TestHealth_RejectsWrongSource rejects an instance whose Source is not
// domain.SourceClaudeCode. The adapter refuses to health-check stores
// belonging to other adapters; the typed error is ErrInvalidArgument.
func TestHealth_RejectsWrongSource(t *testing.T) {
	jsonlPath := writeJSONL(t, "wrong-source.jsonl", []byte(validHeaderObject+"\n"))
	instance := validJSONLInstance(t, jsonlPath)
	instance.Source = domain.SourceOpenCode
	adapter := NewAdapter()
	result := adapter.Health(context.Background(), instance)
	if result.Status != "error" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "error")
	}
	if result.Code != string(domain.ErrInvalidArgument) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrInvalidArgument)
	}
}

// TestHealth_RejectsEmptyJSONLPath rejects an instance without a JSONLPath.
// The caller must have run Discover first to obtain a SourceInstance; bare
// invocations cannot be trusted.
func TestHealth_RejectsEmptyJSONLPath(t *testing.T) {
	adapter := NewAdapter()
	result := adapter.Health(context.Background(), SourceInstance{
		Source:      domain.SourceClaudeCode,
		ProjectRoot: t.TempDir(),
		JSONLPath:   "",
		Schema:      SchemaTag,
	})
	if result.Status != "error" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "error")
	}
	if result.Code != string(domain.ErrInvalidArgument) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrInvalidArgument)
	}
}

// TestHealth_ContextCanceled verifies that Health surfaces context
// cancellation as ErrTimeout. The contract requires every method to honor
// cancellation (docs/22 §1) and the claudecode_test.go contract table
// pins Health specifically to domain.ErrTimeout.
func TestHealth_ContextCanceled(t *testing.T) {
	jsonlPath := writeJSONL(t, "canceled.jsonl", []byte(validHeaderObject+"\n"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	adapter := NewAdapter()
	result := adapter.Health(ctx, validJSONLInstance(t, jsonlPath))
	if result.Status != "error" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "error")
	}
	if result.Code != string(domain.ErrTimeout) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrTimeout)
	}
}

// TestHealth_CheckedAtUTC verifies the CheckedAt timestamp is in UTC so
// downstream JSON consumers never have to deal with local-time drift.
func TestHealth_CheckedAtUTC(t *testing.T) {
	jsonlPath := writeJSONL(t, "utc.jsonl", []byte(validHeaderObject+"\n"))
	adapter := NewAdapter()
	adapter.Now = func() time.Time {
		loc, _ := time.LoadLocation("America/Buenos_Aires")
		return time.Date(2026, 7, 24, 12, 0, 0, 0, loc)
	}
	result := adapter.Health(context.Background(), validJSONLInstance(t, jsonlPath))
	if result.CheckedAt.Location() != time.UTC {
		t.Fatalf("Health CheckedAt location = %v, want UTC", result.CheckedAt.Location())
	}
}

// TestHealth_RealisticEncodedSlug exercises a JSONL placed under a realistic
// `~/.claude/projects/<encoded>/<uuid>.jsonl` layout (the layout Discover
// produces) to confirm Health works on real upstream paths, not just
// t.TempDir()-only paths.
func TestHealth_RealisticEncodedSlug(t *testing.T) {
	root := t.TempDir()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	encoded := strings.ReplaceAll(filepath.ToSlash(abs), ":", "")
	encoded = url.PathEscape(encoded)
	dir := filepath.Join(root, ".claude", "projects", encoded)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir encoded: %v", err)
	}
	path := filepath.Join(dir, "33333333-3333-3333-3333-333333333333.jsonl")
	if err := os.WriteFile(path, []byte(validHeaderObject+"\n"), 0o600); err != nil {
		t.Fatalf("write realistic: %v", err)
	}
	adapter := NewAdapter()
	result := adapter.Health(context.Background(), validJSONLInstance(t, path))
	if result.Status != "ok" {
		t.Fatalf("Health Status = %q, want ok (result=%+v)", result.Status, result)
	}
}

// fileMtime returns the mtime of path in seconds since epoch. Used by the
// no-side-effects test to verify Health never writes to the source.
func fileMtime(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.ModTime().Unix()
}
