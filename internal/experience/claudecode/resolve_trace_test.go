package claudecode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-royo-learn/internal/domain"
)

// fixtureHash returns the hex-encoded SHA-256 of the file at path. It is the
// client-side companion to the hash ResolveTrace computes internally; tests
// use it to build a locator whose SourceHash matches the on-disk content.
func fixtureHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

const lineUser = `{"type":"user","uuid":"turn-user-001","sessionId":"session-001","timestamp":"2026-01-02T03:04:05Z","stop_reason":"end_turn","message":{"content":"hello world"}}`
const lineAssistant = `{"type":"assistant","uuid":"turn-assistant-001","sessionId":"session-001","timestamp":"2026-01-02T03:04:06Z","stop_reason":"end_turn","message":{"content":[{"type":"thinking","thinking":"private"},{"type":"text","text":"redacted content also here"}]}}`

// locatorFor builds a TranscriptLocator for the JSONL path with the current
// fixture hash so the trace path can clear the source-changed branch.
func locatorFor(t *testing.T, jsonlPath, turnID string) domain.TranscriptLocator {
	t.Helper()
	return domain.TranscriptLocator{
		Kind:       "jsonl",
		Path:       jsonlPath,
		SessionID:  "session-001",
		TurnID:     turnID,
		SourceHash: fixtureHash(t, jsonlPath),
	}
}

// TestResolveTrace_ContextCanceled verifies the contract: a cancelled
// context short-circuits before any source I/O. This pins the contract test
// at claudecode_test.go:54.
func TestResolveTrace_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	adapter := NewAdapter()
	result := adapter.ResolveTrace(ctx, domain.TranscriptLocator{
		Kind: "jsonl", Path: "/tmp/claude.jsonl", TurnID: "turn-1",
	}, TraceBounds{MaxBytes: 64})
	if result.Code != string(domain.ErrTimeout) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrTimeout)
	}
}

// TestResolveTrace_OK_ReturnsExcerpt verifies the happy path: a known turn
// returns its text content with an empty Code and the original text
// observable in the excerpt.
func TestResolveTrace_OK_ReturnsExcerpt(t *testing.T) {
	path := writeJSONL(t, "session.jsonl", []byte(lineUser+"\n"+lineAssistant+"\n"))
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locatorFor(t, path, "turn-user-001"), TraceBounds{MaxBytes: 1024})
	if result.Code != "" {
		t.Fatalf("ResolveTrace Code = %q, want empty (result=%+v)", result.Code, result)
	}
	if !strings.Contains(result.Excerpt, "hello world") {
		t.Fatalf("ResolveTrace Excerpt = %q, want it to contain the original content", result.Excerpt)
	}
	if result.SourceChanged {
		t.Fatalf("ResolveTrace SourceChanged = true, want false on a fresh locator")
	}
	if result.Redacted {
		t.Fatalf("ResolveTrace Redacted = true, want false on benign content")
	}
}

// TestResolveTrace_AssistantConcatenatesTextBlocks verifies that an
// assistant turn whose message.content is a list of blocks is reduced to
// the concatenation of its text blocks (thinking blocks dropped).
func TestResolveTrace_AssistantConcatenatesTextBlocks(t *testing.T) {
	path := writeJSONL(t, "session.jsonl", []byte(lineUser+"\n"+lineAssistant+"\n"))
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locatorFor(t, path, "turn-assistant-001"), TraceBounds{MaxBytes: 1024})
	if result.Code != "" {
		t.Fatalf("ResolveTrace Code = %q, want empty", result.Code)
	}
	if strings.Contains(result.Excerpt, "private") {
		t.Fatalf("ResolveTrace Excerpt leaked a thinking block: %q", result.Excerpt)
	}
	if !strings.Contains(result.Excerpt, "redacted content also here") {
		t.Fatalf("ResolveTrace Excerpt = %q, want it to contain the assistant text", result.Excerpt)
	}
}

// TestResolveTrace_TruncatesToMaxBytes verifies the excerpt is capped at
// bounds.MaxBytes and ends with the truncation marker.
func TestResolveTrace_TruncatesToMaxBytes(t *testing.T) {
	big := strings.Repeat("A", 2048)
	path := writeJSONL(t, "session.jsonl", []byte(
		`{"type":"user","uuid":"turn-big","sessionId":"session-001","timestamp":"2026-01-02T03:04:05Z","stop_reason":"end_turn","message":{"content":"`+big+`"}}`+"\n",
	))
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locatorFor(t, path, "turn-big"), TraceBounds{MaxBytes: 256})
	if got := len(result.Excerpt); got > 256 {
		t.Fatalf("ResolveTrace Excerpt length = %d, want <= 256", got)
	}
	if !strings.HasSuffix(result.Excerpt, TraceExcerptSuffix) {
		t.Fatalf("ResolveTrace Excerpt = %q, want suffix %q", result.Excerpt, TraceExcerptSuffix)
	}
}

// TestResolveTrace_DefaultMaxBytes verifies that a zero MaxBytes falls
// back to the contract default (1 KiB).
func TestResolveTrace_DefaultMaxBytes(t *testing.T) {
	content := strings.Repeat("B", 4096)
	path := writeJSONL(t, "session.jsonl", []byte(
		`{"type":"user","uuid":"turn-big","sessionId":"session-001","timestamp":"2026-01-02T03:04:05Z","stop_reason":"end_turn","message":{"content":"`+content+`"}}`+"\n",
	))
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locatorFor(t, path, "turn-big"), TraceBounds{MaxBytes: 0})
	if got := len(result.Excerpt); got > defaultTraceMaxBytes {
		t.Fatalf("ResolveTrace Excerpt length = %d, want <= defaultTraceMaxBytes=%d", got, defaultTraceMaxBytes)
	}
}

// TestResolveTrace_RedactsSecrets verifies that obvious credential-like
// strings inside the content are scrubbed before the excerpt is returned.
func TestResolveTrace_RedactsSecrets(t *testing.T) {
	const secret = "sk-abc123def456ghi789jkl012mno345pq"
	body := "prefix " + secret + " suffix"
	path := writeJSONL(t, "session.jsonl", []byte(
		`{"type":"user","uuid":"turn-secret","sessionId":"session-001","timestamp":"2026-01-02T03:04:05Z","stop_reason":"end_turn","message":{"content":"`+body+`"}}`+"\n",
	))
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locatorFor(t, path, "turn-secret"), TraceBounds{MaxBytes: 1024})
	if !result.Redacted {
		t.Fatalf("ResolveTrace Redacted = false, want true when the content carried a secret")
	}
	if strings.Contains(result.Excerpt, secret) {
		t.Fatalf("ResolveTrace Excerpt still contains the raw secret %q", secret)
	}
}

// TestResolveTrace_SourceChanged verifies that a stale SourceHash on the
// locator surfaces as Code="trace_source_changed" while the excerpt is
// still returned (advisory).
func TestResolveTrace_SourceChanged(t *testing.T) {
	path := writeJSONL(t, "session.jsonl", []byte(lineUser+"\n"+lineAssistant+"\n"))
	locator := locatorFor(t, path, "turn-user-001")
	locator.SourceHash = "stale-hash-does-not-match-the-current-file"

	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locator, TraceBounds{MaxBytes: 1024})
	if result.Code != "trace_source_changed" {
		t.Fatalf("ResolveTrace Code = %q, want trace_source_changed", result.Code)
	}
	if !result.SourceChanged {
		t.Fatalf("ResolveTrace SourceChanged = false, want true")
	}
	if !strings.Contains(result.Excerpt, "hello world") {
		t.Fatalf("ResolveTrace Excerpt = %q, want it to still contain the original content (advisory)", result.Excerpt)
	}
}

// TestResolveTrace_MatchedSourceHash verifies the matched-hash branch:
// the locator's hash lines up with the on-disk file and the trace path
// returns empty Code.
func TestResolveTrace_MatchedSourceHash(t *testing.T) {
	path := writeJSONL(t, "session.jsonl", []byte(lineUser+"\n"))
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locatorFor(t, path, "turn-user-001"), TraceBounds{MaxBytes: 1024})
	if result.Code != "" {
		t.Fatalf("ResolveTrace Code = %q, want empty on matched hash", result.Code)
	}
}

// TestResolveTrace_TurnNotFound verifies the "turn disappeared" case
// surfaces as Code="trace_source_unavailable".
func TestResolveTrace_TurnNotFound(t *testing.T) {
	path := writeJSONL(t, "session.jsonl", []byte(lineUser+"\n"))
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locatorFor(t, path, "missing-turn"), TraceBounds{MaxBytes: 1024})
	if result.Code != "trace_source_unavailable" {
		t.Fatalf("ResolveTrace Code = %q, want trace_source_unavailable", result.Code)
	}
}

// TestResolveTrace_InvalidLocatorKind rejects locators whose Kind is not
// jsonl; the adapter only resolves Claude Code JSONL transcripts.
func TestResolveTrace_InvalidLocatorKind(t *testing.T) {
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind:   "sqlite",
		Path:   "/tmp/claude.jsonl",
		TurnID: "turn-1",
	}, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceLocatorInvalid) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceLocatorInvalid)
	}
}

// TestResolveTrace_EmptyPath rejects locators whose Path is empty.
func TestResolveTrace_EmptyPath(t *testing.T) {
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind:   "jsonl",
		Path:   "",
		TurnID: "turn-1",
	}, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceLocatorInvalid) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceLocatorInvalid)
	}
}

// TestResolveTrace_EmptyTurnID rejects locators whose TurnID is empty.
func TestResolveTrace_EmptyTurnID(t *testing.T) {
	path := writeJSONL(t, "session.jsonl", []byte(lineUser+"\n"))
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind: "jsonl", Path: path, TurnID: "",
	}, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceLocatorInvalid) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceLocatorInvalid)
	}
}

// TestResolveTrace_MissingFile rejects locators whose Path no longer
// resolves to a readable file.
func TestResolveTrace_MissingFile(t *testing.T) {
	locator := domain.TranscriptLocator{
		Kind:   "jsonl",
		Path:   filepath.Join(t.TempDir(), "no-such.jsonl"),
		TurnID: "turn-1",
	}
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locator, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceSourceNotFound)
	}
}
