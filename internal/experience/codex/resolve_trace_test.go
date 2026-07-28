package codex

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

func fixtureHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func locatorFor(t *testing.T, rolloutPath, turnID string) domain.TranscriptLocator {
	t.Helper()
	return domain.TranscriptLocator{
		Kind: "rollout", Path: rolloutPath,
		SessionID: "session-001", TurnID: turnID, SourceHash: fixtureHash(t, rolloutPath),
	}
}

func TestResolveTrace_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := NewAdapter().ResolveTrace(ctx, domain.TranscriptLocator{
		Kind: "rollout", Path: "/tmp/rollout.jsonl", TurnID: "turn-1",
	}, TraceBounds{MaxBytes: 64})
	if result.Code != string(domain.ErrTimeout) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrTimeout)
	}
}

func TestResolveTrace_OK_ReturnsExcerpt(t *testing.T) {
	path := writeScanFile(t, "rollout-trace.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanEventMsg("event_msg.user_message", "turn-1", "user", "hello world")+"\n",
	)
	result := NewAdapter().ResolveTrace(context.Background(), locatorFor(t, path, "turn-1"), TraceBounds{MaxBytes: 1024})
	if result.Code != "" {
		t.Fatalf("ResolveTrace Code = %q, want empty (result=%+v)", result.Code, result)
	}
	if !strings.Contains(result.Excerpt, "hello world") {
		t.Fatalf("ResolveTrace Excerpt = %q, want it to contain the original content", result.Excerpt)
	}
	if result.SourceChanged || result.Redacted {
		t.Fatalf("ResolveTrace SourceChanged/Redacted = %v/%v, want both false", result.SourceChanged, result.Redacted)
	}
}

func TestResolveTrace_TruncatesToMaxBytes(t *testing.T) {
	big := strings.Repeat("A", 2048)
	path := writeScanFile(t, "rollout-trace-big.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanResponseFunctionCall("turn-big", "call-1", "shell", `{"cmd":"x"}`)+"\n"+
			`{"timestamp":"2026-07-27T12:00:02Z","type":"response_item","turn_id":"turn-big","payload":{"role":"assistant","type":"message","text":"`+big+`"}}`+"\n",
	)
	result := NewAdapter().ResolveTrace(context.Background(), locatorFor(t, path, "turn-big"), TraceBounds{MaxBytes: 256})
	if got := len(result.Excerpt); got > 256 {
		t.Fatalf("ResolveTrace Excerpt length = %d, want <= 256", got)
	}
	if !strings.HasSuffix(result.Excerpt, TraceExcerptSuffix) {
		t.Fatalf("ResolveTrace Excerpt = %q, want suffix %q", result.Excerpt, TraceExcerptSuffix)
	}
}

func TestResolveTrace_DefaultMaxBytes(t *testing.T) {
	content := strings.Repeat("B", 4096)
	path := writeScanFile(t, "rollout-trace-default.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanResponseFunctionCall("turn-big", "call-1", "shell", `{"cmd":"x"}`)+"\n"+
			`{"timestamp":"2026-07-27T12:00:02Z","type":"response_item","turn_id":"turn-big","payload":{"role":"assistant","type":"message","text":"`+content+`"}}`+"\n",
	)
	result := NewAdapter().ResolveTrace(context.Background(), locatorFor(t, path, "turn-big"), TraceBounds{MaxBytes: 0})
	if got := len(result.Excerpt); got > defaultTraceMaxBytes {
		t.Fatalf("ResolveTrace Excerpt length = %d, want <= %d", got, defaultTraceMaxBytes)
	}
}

func TestResolveTrace_RedactsSecrets(t *testing.T) {
	const secret = "sk-abc123def456ghi789jkl012mno345pq"
	path := writeScanFile(t, "rollout-trace-secret.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanEventMsg("event_msg.user_message", "turn-secret", "user", "prefix "+secret+" suffix")+"\n",
	)
	result := NewAdapter().ResolveTrace(context.Background(), locatorFor(t, path, "turn-secret"), TraceBounds{MaxBytes: 1024})
	if !result.Redacted {
		t.Fatalf("ResolveTrace Redacted = false, want true")
	}
	if strings.Contains(result.Excerpt, secret) {
		t.Fatalf("ResolveTrace Excerpt still contains the raw secret")
	}
}

func TestResolveTrace_SourceChangedReturnsNoExcerpt(t *testing.T) {
	path := writeScanFile(t, "rollout-trace-stale.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanEventMsg("event_msg.user_message", "turn-1", "user", "hello world")+"\n",
	)
	locator := locatorFor(t, path, "turn-1")
	locator.SourceHash = "stale-hash-does-not-match"
	result := NewAdapter().ResolveTrace(context.Background(), locator, TraceBounds{MaxBytes: 1024})
	if result.Code != "trace_source_changed" {
		t.Fatalf("ResolveTrace Code = %q, want trace_source_changed", result.Code)
	}
	if !result.SourceChanged {
		t.Fatalf("ResolveTrace SourceChanged = false, want true")
	}
	if result.Excerpt != "" {
		t.Fatalf("ResolveTrace Excerpt = %q, want empty when source changed", result.Excerpt)
	}
}

func TestResolveTrace_MatchedSourceHash(t *testing.T) {
	path := writeScanFile(t, "rollout-trace-fresh.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanEventMsg("event_msg.user_message", "turn-1", "user", "fresh")+"\n",
	)
	result := NewAdapter().ResolveTrace(context.Background(), locatorFor(t, path, "turn-1"), TraceBounds{MaxBytes: 1024})
	if result.Code != "" {
		t.Fatalf("ResolveTrace Code = %q, want empty on matched hash", result.Code)
	}
}

func TestResolveTrace_TurnNotFound(t *testing.T) {
	path := writeScanFile(t, "rollout-trace-missing.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanEventMsg("event_msg.user_message", "turn-1", "user", "x")+"\n",
	)
	result := NewAdapter().ResolveTrace(context.Background(), locatorFor(t, path, "missing"), TraceBounds{MaxBytes: 1024})
	if result.Code != "trace_source_unavailable" {
		t.Fatalf("ResolveTrace Code = %q, want trace_source_unavailable", result.Code)
	}
}

func TestResolveTrace_InvalidLocatorKind(t *testing.T) {
	result := NewAdapter().ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind: "jsonl", Path: "/tmp/rollout.jsonl", TurnID: "turn-1",
	}, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceLocatorInvalid) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceLocatorInvalid)
	}
}

func TestResolveTrace_EmptyPath(t *testing.T) {
	result := NewAdapter().ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind: "rollout", Path: "", TurnID: "turn-1",
	}, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceLocatorInvalid) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceLocatorInvalid)
	}
}

func TestResolveTrace_EmptyTurnID(t *testing.T) {
	path := writeScanFile(t, "rollout-trace-empty.jsonl", scanSessionMeta("session-001")+"\n")
	result := NewAdapter().ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind: "rollout", Path: path, TurnID: "",
	}, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceLocatorInvalid) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceLocatorInvalid)
	}
}

func TestResolveTrace_MissingFile(t *testing.T) {
	locator := domain.TranscriptLocator{
		Kind: "rollout", Path: filepath.Join(t.TempDir(), "no-such.jsonl"), TurnID: "turn-1",
	}
	result := NewAdapter().ResolveTrace(context.Background(), locator, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceSourceNotFound)
	}
}

// TestResolveTrace_DropsReasoning covers the response_item branch when the
// only payload for the turn is reasoning. design.md §Scan requires reasoning
// to be dropped at envelope-build time; ResolveTrace must follow the same
// rule so a trace never re-exposes reasoning content.
func TestResolveTrace_DropsReasoning(t *testing.T) {
	path := writeScanFile(t, "rollout-trace-reasoning.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanResponseReasoning("turn-reason")+"\n",
	)
	result := NewAdapter().ResolveTrace(context.Background(), locatorFor(t, path, "turn-reason"), TraceBounds{MaxBytes: 1024})
	if result.Code != "trace_source_unavailable" {
		t.Fatalf("ResolveTrace Code = %q, want trace_source_unavailable", result.Code)
	}
	if strings.Contains(result.Excerpt, "SECRET-LEAK from reasoning") {
		t.Fatalf("ResolveTrace Excerpt = %q leaks reasoning content", result.Excerpt)
	}
}

// TestResolveTrace_DropsFunctionCallOutput covers the response_item branch
// when the only payload for the turn is a function_call_output. Same rule
// as reasoning: dropped at envelope-build, must not re-appear in trace.
func TestResolveTrace_DropsFunctionCallOutput(t *testing.T) {
	const secret = "SECRET-FUNCTION-OUTPUT-12345"
	path := writeScanFile(t, "rollout-trace-fco.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanResponseFunctionCallOutput("call-1", secret)+"\n",
	)
	result := NewAdapter().ResolveTrace(context.Background(), locatorFor(t, path, "call-1"), TraceBounds{MaxBytes: 1024})
	if strings.Contains(result.Excerpt, secret) {
		t.Fatalf("ResolveTrace Excerpt = %q leaks function_call_output content", result.Excerpt)
	}
}

// TestResolveTrace_FallsThroughFromReasoningToMessage covers the case where
// a turn has BOTH a reasoning response_item and a message response_item.
// ResolveTrace must skip the reasoning and return the message text. This
// also serves as the redaction coverage finding from the reliability lens:
// a secret placed in the message text must be redacted, not leaked raw.
func TestResolveTrace_FallsThroughFromReasoningToMessage(t *testing.T) {
	const secret = "sk-abc123def456ghi789jkl012mno345pq"
	path := writeScanFile(t, "rollout-trace-reason-then-message.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanResponseReasoning("turn-mix")+"\n"+
			`{"timestamp":"2026-07-27T12:00:05Z","type":"response_item","turn_id":"turn-mix","payload":{"role":"assistant","type":"message","text":"prefix `+secret+` suffix"}}`+"\n",
	)
	result := NewAdapter().ResolveTrace(context.Background(), locatorFor(t, path, "turn-mix"), TraceBounds{MaxBytes: 1024})
	if result.Code != "" {
		t.Fatalf("ResolveTrace Code = %q, want empty", result.Code)
	}
	if !result.Redacted {
		t.Fatalf("ResolveTrace Redacted = false, want true")
	}
	if strings.Contains(result.Excerpt, secret) {
		t.Fatalf("ResolveTrace Excerpt = %q still contains raw secret", result.Excerpt)
	}
	if strings.Contains(result.Excerpt, "SECRET-LEAK from reasoning") {
		t.Fatalf("ResolveTrace Excerpt = %q leaked reasoning content", result.Excerpt)
	}
}
