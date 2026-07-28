package codex

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
)

func TestScan_BuildsNeutralEnvelopesAndDropsReasoning(t *testing.T) {
	rollout := writeScanFile(t, "rollout-scan.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanTurnContext("turn-1")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-1", "user", "hello")+"\n"+
			scanEventMsg("event_msg.user_message", "", "user", "hi")+"\n"+
			scanResponseFunctionCall("turn-1", "call-1", "shell", `{"cmd":"ls"}`)+"\n"+
			scanResponseReasoning("turn-1")+"\n"+
			scanResponseFunctionCallOutput("call-1", "SECRET-LEAK")+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-1", "assistant", "done")+"\n")

	adapter := NewAdapter()
	req := ScanRequest{
		ProjectRoot: filepath.Dir(rollout),
		Instance:    SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	}
	result, err := adapter.Scan(context.Background(), req)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 1 {
		t.Fatalf("Scan returned %d envelopes, want 1", len(result.Envelopes))
	}
	env := result.Envelopes[0]
	if env.Source != domain.SourceCodex || env.Actor.Name != "codex" {
		t.Fatalf("envelope source/actor = %q/%q", env.Source, env.Actor.Name)
	}
	if env.Session.ExternalID != "session-001" || env.Turn.ExternalID != "turn-1" {
		t.Fatalf("envelope ids = %q/%q", env.Session.ExternalID, env.Turn.ExternalID)
	}
	if env.Session.Locator.Kind != "rollout" || env.Session.Locator.Path != rollout {
		t.Fatalf("envelope locator = %+v", env.Session.Locator)
	}
	if !env.Turn.Complete || env.Turn.FinishReason != "task_complete" {
		t.Fatalf("envelope completion = %+v", env.Turn)
	}
	if env.Turn.UserText != "hi" || env.Turn.AssistantText != "done" {
		t.Fatalf("envelope texts = %q/%q", env.Turn.UserText, env.Turn.AssistantText)
	}
	if len(env.Turn.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(env.Turn.ToolCalls))
	}
	call := env.Turn.ToolCalls[0]
	if call.Name != "shell" || call.Outcome != "call-1" {
		t.Fatalf("tool call name/outcome = %q/%q", call.Name, call.Outcome)
	}
	if call.OutputHash == "" || call.OutputHint == "" {
		t.Fatalf("tool call hash/hint empty: %+v", call)
	}
	if containsLeak(env.Turn.AssistantText) || containsLeak(env.Turn.UserText) {
		t.Fatalf("envelope persisted leaked value: %+v", env.Turn)
	}
	if result.NextCursor["last_session_id"] != "session-001" || result.NextCursor["last_turn_uuid"] != "turn-1" {
		t.Fatalf("NextCursor = %+v", result.NextCursor)
	}
	if result.Status != "ok" || result.SkippedIncomplete != 0 || result.SkippedMalformed != 0 {
		t.Fatalf("Scan counters = %+v", result)
	}
}

func TestScan_CursorAdvancesAndSkipsEarlierTurns(t *testing.T) {
	rollout := writeScanFile(t, "rollout-cursor.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanTurnContext("turn-1")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-1", "user", "first")+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-1", "assistant", "first")+"\n"+
			scanTurnContext("turn-2")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-2", "user", "second")+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-2", "assistant", "second")+"\n",
	)
	req := ScanRequest{
		ProjectRoot: filepath.Dir(rollout),
		Instance:    SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
		Cursor:      map[string]any{"last_session_id": "session-001", "last_turn_uuid": "turn-1"},
	}
	result, err := NewAdapter().Scan(context.Background(), req)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 1 || result.Envelopes[0].Turn.ExternalID != "turn-2" {
		t.Fatalf("envelopes = %+v", result.Envelopes)
	}
	if result.NextCursor["last_turn_uuid"] != "turn-2" {
		t.Fatalf("NextCursor = %+v", result.NextCursor)
	}
}

func TestScan_SortsEnvelopesAcrossSessions(t *testing.T) {
	rollout := writeScanFile(t, "rollout-multi.jsonl",
		scanSessionMeta("session-002")+"\n"+
			scanTurnContext("turn-1")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-1", "user", "")+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-1", "assistant", "")+"\n"+
			scanSessionMeta("session-001")+"\n"+
			scanTurnContext("turn-1")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-1", "user", "")+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-1", "assistant", "")+"\n",
	)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(rollout),
		Instance:    SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	keys := make([]string, len(result.Envelopes))
	for i, env := range result.Envelopes {
		keys[i] = env.Session.ExternalID + "/" + env.Turn.ExternalID
	}
	sort.Strings(keys)
	for i, key := range keys {
		got := result.Envelopes[i].Session.ExternalID + "/" + result.Envelopes[i].Turn.ExternalID
		if got != key {
			t.Fatalf("envelope %d = %s, sorted key = %s", i, got, key)
		}
	}
}

func TestScan_SkipsMalformedAndIncompleteTurns(t *testing.T) {
	rollout := writeScanFile(t, "rollout-skips.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanTurnContext("turn-1")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-1", "user", "started")+"\n"+
			"this-is-not-json\n"+
			scanEventMsg("event_msg.task_complete", "turn-2", "assistant", "no-starter")+"\n",
	)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(rollout),
		Instance:    SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 0 {
		t.Fatalf("envelopes = %+v, want 0", result.Envelopes)
	}
	if result.SkippedMalformed == 0 || result.SkippedIncomplete == 0 {
		t.Fatalf("counters = %+v", result)
	}
}

func TestScan_ContextCanceledReturnsError(t *testing.T) {
	rollout := writeScanFile(t, "rollout-cancel.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanTurnContext("turn-1")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-1", "user", "")+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-1", "assistant", "")+"\n",
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewAdapter().Scan(ctx, ScanRequest{
		ProjectRoot: filepath.Dir(rollout),
		Instance:    SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	}); err != context.Canceled {
		t.Fatalf("Scan(canceled) error = %v, want context.Canceled", err)
	}
}

func containsLeak(s string) bool {
	for _, leak := range []string{"SECRET-LEAK", "secret", "leak"} {
		if len(s) >= len(leak) && indexOf(s, leak) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func writeScanFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func scanSessionMeta(sessionID string) string {
	return `{"timestamp":"2026-07-27T12:00:00Z","type":"session_meta","payload":{"codex_session_id":"` + sessionID + `","cwd":"/tmp/project","cli_version":"1.2.3"}}`
}

func scanTurnContext(turnID string) string {
	return `{"timestamp":"2026-07-27T12:00:01Z","type":"turn_context","payload":{"turn_id":"` + turnID + `"}}`
}

func scanEventMsg(eventType, turnID, role, text string) string {
	payload := `{"type":"` + eventType + `","turn_id":"` + turnID + `","message":"` + text + `","role":"` + role + `"}`
	return `{"timestamp":"2026-07-27T12:00:02Z","type":"event_msg","payload":` + payload + `}`
}

func scanResponseFunctionCall(turnID, callID, name, args string) string {
	payload := `{"role":"assistant","type":"function_call","name":"` + name + `","arguments":` + args + `,"call_id":"` + callID + `"}`
	return `{"timestamp":"2026-07-27T12:00:03Z","type":"response_item","turn_id":"` + turnID + `","payload":` + payload + `}`
}

func scanResponseReasoning(turnID string) string {
	payload := `{"role":"assistant","type":"reasoning","text":"SECRET-LEAK from reasoning"}`
	return `{"timestamp":"2026-07-27T12:00:04Z","type":"response_item","turn_id":"` + turnID + `","payload":` + payload + `}`
}

func scanResponseFunctionCallOutput(callID, output string) string {
	payload := `{"type":"function_call_output","call_id":"` + callID + `","output":"` + output + `"}`
	return `{"timestamp":"2026-07-27T12:00:05Z","type":"response_item","payload":` + payload + `}`
}

var _ = experience.ExperienceEnvelope{}
var _ = time.Time{}

// TestScan_RejectsNonCodexSource covers scan.go:48-50: a Scan request whose
// instance source is not Codex must surface a validation error before any
// file I/O.
func TestScan_RejectsNonCodexSource(t *testing.T) {
	_, err := NewAdapter().Scan(context.Background(), ScanRequest{
		Instance: SourceInstance{Source: domain.SourceOpenCode, RolloutPath: "/tmp/foo"},
	})
	if domainCode(err) != domain.ErrInvalidArgument {
		t.Fatalf("Scan(wrong source) error = %v, want %q", err, domain.ErrInvalidArgument)
	}
}

// TestScan_ReadFileError covers scan.go:51-54: a missing rollout path
// must surface the os.ReadFile error verbatim.
func TestScan_ReadFileError(t *testing.T) {
	_, err := NewAdapter().Scan(context.Background(), ScanRequest{
		Instance: SourceInstance{Source: domain.SourceCodex, RolloutPath: filepath.Join(t.TempDir(), "missing.jsonl")},
	})
	if err == nil {
		t.Fatal("Scan(missing) returned no error")
	}
	if !os.IsNotExist(err) && !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("Scan(missing) error = %v, want os.IsNotExist", err)
	}
}

// TestScan_ContextCanceledInLoop covers scan.go:67-70: a context that is
// already canceled when Scan reads the rollout must return context.Canceled.
func TestScan_ContextCanceledInLoop(t *testing.T) {
	rollout := writeScanFile(t, "rollout-cancel-loop.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanTurnContext("turn-1")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-1", "user", "hi")+"\n",
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewAdapter().Scan(ctx, ScanRequest{
		Instance: SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Scan(canceled in loop) error = %v, want context.Canceled", err)
	}
}

// TestScan_TurnContextWithoutID covers scan.go:97-98: a turn_context line
// with no turn_id must be skipped silently rather than crashing.
func TestScan_TurnContextWithoutID(t *testing.T) {
	rollout := writeScanFile(t, "rollout-no-turn.jsonl",
		scanSessionMeta("session-001")+"\n"+
			`{"timestamp":"2026-07-27T12:00:01Z","type":"turn_context","payload":{}}`+"\n"+
			scanTurnContext("turn-1")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-1", "user", "hi")+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-1", "assistant", "done")+"\n",
	)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{
		Instance: SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 1 || result.Envelopes[0].Turn.ExternalID != "turn-1" {
		t.Fatalf("envelopes = %+v, want 1 with turn-1", result.Envelopes)
	}
}

// TestScan_TaskStartedWithoutTurnID covers scan.go:108-110: a task_started
// event with no turn_id must lazily create a new turn under a derived ID.
func TestScan_TaskStartedWithoutTurnID(t *testing.T) {
	rollout := writeScanFile(t, "rollout-orphan-start.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanTurnContext("turn-1")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-1", "user", "seed")+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-1", "assistant", "seed")+"\n"+
			scanEventMsg("event_msg.task_started", "", "user", "orphan")+"\n"+
			scanEventMsg("event_msg.task_complete", "", "assistant", "orphan")+"\n",
	)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{
		Instance: SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) < 1 {
		t.Fatalf("envelopes = %+v, want >=1", result.Envelopes)
	}
}

// TestScan_UserMessageWithoutAnchor covers scan.go:118-120: a user_message
// event with neither an explicit turn_id nor a previous turn in the order
// must be dropped without producing a partial envelope.
func TestScan_UserMessageWithoutAnchor(t *testing.T) {
	rollout := writeScanFile(t, "rollout-orphan-user.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanEventMsg("event_msg.user_message", "", "user", "no anchor")+"\n",
	)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{
		Instance: SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 0 {
		t.Fatalf("envelopes = %+v, want 0 (orphan user_message dropped)", result.Envelopes)
	}
}

// TestScan_UserMessageCreatesNewTurn covers scan.go:121-124: a user_message
// anchored to the most recent turn id, when no prior turn exists, must
// lazily create one and seed it as StarterSeen.
func TestScan_UserMessageCreatesNewTurn(t *testing.T) {
	rollout := writeScanFile(t, "rollout-user-newturn.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanEventMsg("event_msg.user_message", "fresh-turn", "user", "hello")+"\n"+
			scanEventMsg("event_msg.task_complete", "fresh-turn", "assistant", "done")+"\n",
	)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{
		Instance: SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 1 {
		t.Fatalf("envelopes = %+v, want 1", result.Envelopes)
	}
	if got := result.Envelopes[0].Turn.ExternalID; got != "fresh-turn" {
		t.Fatalf("envelope turn id = %q, want fresh-turn", got)
	}
}

// TestScan_TaskCompleteWithoutAnchor covers scan.go:129-130: a
// task_complete with no matching turn must be silently ignored.
func TestScan_TaskCompleteWithoutAnchor(t *testing.T) {
	rollout := writeScanFile(t, "rollout-orphan-complete.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanEventMsg("event_msg.task_complete", "", "assistant", "ghost")+"\n",
	)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{
		Instance: SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	// Orphan task_complete is dropped silently; no envelope is produced.
	if len(result.Envelopes) != 0 {
		t.Fatalf("envelopes = %+v, want 0", result.Envelopes)
	}
}

// TestScan_ResponseItemWithoutTurnID covers scan.go:145-146: a
// response_item with no turn_id must fall back to the last turn in order
// when one exists, or be dropped otherwise.
func TestScan_ResponseItemWithoutTurnID(t *testing.T) {
	rollout := writeScanFile(t, "rollout-orphan-response.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanTurnContext("turn-1")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-1", "user", "hi")+"\n"+
			scanResponseFunctionCall("", "call-1", "shell", `{"cmd":"ls"}`)+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-1", "assistant", "done")+"\n",
	)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{
		Instance: SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 1 || len(result.Envelopes[0].Turn.ToolCalls) != 1 {
		t.Fatalf("envelopes/tool calls = %+v, want 1 envelope with 1 call", result.Envelopes)
	}
}

// TestScan_ResponseItemCreatesNewTurn covers scan.go:148-151: a
// response_item addressed to a never-seen turn id must lazily create the
// turn slot under that id.
func TestScan_ResponseItemCreatesNewTurn(t *testing.T) {
	rollout := writeScanFile(t, "rollout-response-newturn.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanResponseFunctionCall("seed-turn", "call-1", "shell", `{"cmd":"ls"}`)+"\n"+
			scanEventMsg("event_msg.task_started", "seed-turn", "user", "seed")+"\n"+
			scanEventMsg("event_msg.task_complete", "seed-turn", "assistant", "done")+"\n",
	)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{
		Instance: SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 1 || len(result.Envelopes[0].Turn.ToolCalls) != 1 {
		t.Fatalf("envelopes/tool calls = %+v, want 1 envelope with 1 call", result.Envelopes)
	}
}

// TestScan_FunctionCallBadJSON covers scan.go:160-162: a function_call
// whose arguments field is not a JSON object must still be retained with
// the raw payload wrapped under a "raw" key.
func TestScan_FunctionCallBadJSON(t *testing.T) {
	rollout := writeScanFile(t, "rollout-bad-args.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanTurnContext("turn-1")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-1", "user", "hi")+"\n"+
			scanResponseFunctionCall("turn-1", "call-1", "shell", `[1,2,3]`)+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-1", "assistant", "done")+"\n",
	)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{
		Instance: SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 1 || len(result.Envelopes[0].Turn.ToolCalls) != 1 {
		t.Fatalf("envelopes/tool calls = %+v, want 1 envelope with 1 call", result.Envelopes)
	}
	rawArg, ok := result.Envelopes[0].Turn.ToolCalls[0].Arguments["raw"]
	if !ok {
		t.Fatalf("tool call arguments = %+v, want raw key", result.Envelopes[0].Turn.ToolCalls[0].Arguments)
	}
	if rawArg != "[1,2,3]" {
		t.Fatalf("raw argument = %v, want [1,2,3]", rawArg)
	}
}

// TestScan_SessionIDFallback covers scan.go:171-176: when no session_meta
// is observed in the first pass, Scan must fall back to a second pass that
// extracts the session id from any later session_meta line.
func TestScan_SessionIDFallback(t *testing.T) {
	// The first scanSessionMeta yields an empty codex_session_id, so the
	// first pass leaves sessionID == "". A later session_meta with a real
	// id must be picked up by the fallback loop.
	emptyMeta := `{"timestamp":"2026-07-27T11:00:00Z","type":"session_meta","payload":{"codex_session_id":"","cwd":"/tmp","cli_version":"1.0"}}`
	realMeta := `{"timestamp":"2026-07-27T12:00:00Z","type":"session_meta","payload":{"codex_session_id":"session-FALLBACK","cwd":"/tmp","cli_version":"1.0"}}`
	rollout := writeScanFile(t, "rollout-fallback.jsonl",
		emptyMeta+"\n"+
			realMeta+"\n"+
			scanTurnContext("turn-1")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-1", "user", "hi")+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-1", "assistant", "done")+"\n",
	)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{
		Instance: SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 1 || result.Envelopes[0].Session.ExternalID != "session-FALLBACK" {
		t.Fatalf("envelopes = %+v, want session-FALLBACK", result.Envelopes)
	}
}

// TestScan_EmptySessionIDSkipped covers scan.go:190-192: when no
// session_meta with a usable id is ever observed, every turn must be
// counted as incomplete and no envelope must be emitted.
func TestScan_EmptySessionIDSkipped(t *testing.T) {
	emptyMeta := `{"timestamp":"2026-07-27T11:00:00Z","type":"session_meta","payload":{"codex_session_id":"","cwd":"/tmp","cli_version":"1.0"}}`
	rollout := writeScanFile(t, "rollout-empty-session.jsonl",
		emptyMeta+"\n"+
			scanTurnContext("turn-1")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-1", "user", "hi")+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-1", "assistant", "done")+"\n",
	)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{
		Instance: SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 0 || result.SkippedIncomplete == 0 {
		t.Fatalf("envelopes = %+v, skipped = %d, want 0 envelopes with skipped_incomplete > 0", result.Envelopes, result.SkippedIncomplete)
	}
}

// TestScan_SortsEnvelopesByTurnID covers scan.go:200-205: the sort step
// must order envelopes by their (session, turn) external ids. Within one
// rollout the session id is constant, so the final comparator falls back
// to the turn id branch.
func TestScan_SortsEnvelopesByTurnID(t *testing.T) {
	rollout := writeScanFile(t, "rollout-sort.jsonl",
		scanSessionMeta("session-001")+"\n"+
			scanTurnContext("turn-z")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-z", "user", "")+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-z", "assistant", "")+"\n"+
			scanTurnContext("turn-a")+"\n"+
			scanEventMsg("event_msg.task_started", "turn-a", "user", "")+"\n"+
			scanEventMsg("event_msg.task_complete", "turn-a", "assistant", "")+"\n",
	)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{
		Instance: SourceInstance{Source: domain.SourceCodex, RolloutPath: rollout, Schema: SchemaTag},
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 2 {
		t.Fatalf("envelopes = %+v, want 2", result.Envelopes)
	}
	if result.Envelopes[0].Turn.ExternalID != "turn-a" || result.Envelopes[1].Turn.ExternalID != "turn-z" {
		t.Fatalf("envelope order = %s, %s; want turn-a, turn-z",
			result.Envelopes[0].Turn.ExternalID, result.Envelopes[1].Turn.ExternalID)
	}
	if got := result.NextCursor["last_turn_uuid"]; got != "turn-z" {
		t.Fatalf("NextCursor.last_turn_uuid = %v, want turn-z", got)
	}
}

// TestMergeText covers all three branches of mergeText (scan.go:235-243).
func TestMergeText(t *testing.T) {
	cases := []struct {
		prev, next, want string
	}{
		{"", "hello", "hello"},
		{"hello", "", "hello"},
		{"hello", "world", "hello\nworld"},
	}
	for _, tc := range cases {
		if got := mergeText(tc.prev, tc.next); got != tc.want {
			t.Errorf("mergeText(%q, %q) = %q, want %q", tc.prev, tc.next, got, tc.want)
		}
	}
}

// TestMergeTime covers all four branches of mergeTime (scan.go:245-256).
func TestMergeTime(t *testing.T) {
	zero := time.Time{}
	earlier := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	later := time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC)
	cases := []struct {
		name             string
		prev, next, want time.Time
	}{
		{"zero next", later, zero, later},
		{"zero prev", zero, later, later},
		{"next earlier", later, earlier, earlier},
		{"next later", earlier, later, earlier},
	}
	for _, tc := range cases {
		if got := mergeTime(tc.prev, tc.next); !got.Equal(tc.want) {
			t.Errorf("mergeTime(%v, %v) = %v, want %v", tc.prev, tc.next, got, tc.want)
		}
	}
}

// TestLastTurnID covers scan.go:258-265 including the empty-order fallback.
func TestLastTurnID(t *testing.T) {
	if got := lastTurnID(nil, nil); got != "" {
		t.Errorf("lastTurnID(nil, nil) = %q, want empty", got)
	}
	if got := lastTurnID([]string{}, map[string]*codexTurn{}); got != "" {
		t.Errorf("lastTurnID(empty) = %q, want empty", got)
	}
	if got := lastTurnID([]string{"turn-1", "turn-2"}, map[string]*codexTurn{}); got != "" {
		t.Errorf("lastTurnID(missing turns) = %q, want empty", got)
	}
	if got := lastTurnID([]string{"turn-1", "turn-2"}, map[string]*codexTurn{"turn-2": {ID: "turn-2"}}); got != "turn-2" {
		t.Errorf("lastTurnID = %q, want turn-2", got)
	}
	if got := lastTurnID([]string{"turn-1", "turn-2"}, map[string]*codexTurn{"turn-1": {ID: "turn-1"}}); got != "turn-1" {
		t.Errorf("lastTurnID = %q, want turn-1 (only turn-1 present)", got)
	}
}

// TestDigestCodexOutput covers scan.go:314-320 including the empty-input
// branch and the deterministic hashing invariant.
func TestDigestCodexOutput(t *testing.T) {
	if got := digestCodexOutput(""); got != "" {
		t.Errorf("digestCodexOutput(\"\") = %q, want empty", got)
	}
	first := digestCodexOutput("hello world")
	if first == "" {
		t.Fatal("digestCodexOutput(\"hello world\") = empty")
	}
	if second := digestCodexOutput("hello world"); second != first {
		t.Errorf("digestCodexOutput not deterministic: %q vs %q", first, second)
	}
}

// TestBoundedOmissionHint covers scan.go:322-328 including the long-output
// branch which exercises itoa internally.
func TestBoundedOmissionHint(t *testing.T) {
	if got := boundedOmissionHint("short"); got != "[omitted]" {
		t.Errorf("boundedOmissionHint(short) = %q, want [omitted]", got)
	}
	long := boundedOmissionHint(strings.Repeat("x", 200))
	if !strings.HasPrefix(long, "[omitted ") || !strings.HasSuffix(long, " bytes]") {
		t.Errorf("boundedOmissionHint(long) = %q, want [omitted N bytes]", long)
	}
}

// TestItoa covers the standalone itoa helper (scan.go:330-342).
func TestItoa(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{9, "9"},
		{42, "42"},
		{200, "200"},
		{1234567, "1234567"},
	}
	for _, tc := range cases {
		if got := itoa(tc.in); got != tc.want {
			t.Errorf("itoa(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestParseTimestamp covers scan.go:344-358 including the empty-raw
// early return, the RFC3339Nano success, the RFC3339 success, and the
// final zero fallback when neither parse succeeds.
func TestParseTimestamp(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want time.Time
	}{
		{"empty raw", "", time.Time{}},
		{"rfc3339nano", `"2026-07-27T12:00:00.123456789Z"`, time.Date(2026, 7, 27, 12, 0, 0, 123456789, time.UTC)},
		{"rfc3339", `"2026-07-27T12:00:00Z"`, time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)},
		{"invalid json", `not-json`, time.Time{}},
		{"valid json unparseable date", `"not-a-timestamp"`, time.Time{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTimestamp(json.RawMessage(tc.in))
			if !got.Equal(tc.want) {
				t.Fatalf("parseTimestamp(%s) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
