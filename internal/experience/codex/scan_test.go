package codex

import (
	"context"
	"os"
	"path/filepath"
	"sort"
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
