package claudecode

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"agent-royo-learn/internal/domain"
)

func TestScan_JSONLFixture(t *testing.T) {
	path, err := filepath.Abs("testdata/fixtures/session-001.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(path)
	result, err := NewAdapter().Scan(context.Background(), ScanRequest{ProjectRoot: root, Instance: SourceInstance{Source: domain.SourceClaudeCode, ProjectRoot: root, JSONLPath: path}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Envelopes) != 2 {
		t.Fatalf("envelopes = %d, want 2", len(result.Envelopes))
	}
	if result.SkippedMalformed != 1 || result.SkippedSystem != 1 || result.SkippedIncomplete != 1 {
		t.Fatalf("skip counters = malformed:%d system:%d incomplete:%d", result.SkippedMalformed, result.SkippedSystem, result.SkippedIncomplete)
	}
	assistant := result.Envelopes[0]
	if assistant.Turn.ExternalID != "turn-assistant-001" || assistant.Turn.AssistantText != "Inspection complete." {
		t.Fatalf("assistant envelope = %#v", assistant.Turn)
	}
	if len(assistant.Turn.ToolCalls) != 1 || assistant.Turn.ToolCalls[0].Name != "Read" {
		t.Fatalf("tool calls = %#v", assistant.Turn.ToolCalls)
	}
	if assistant.Session.Locator.Kind != "jsonl" || assistant.Session.Locator.SourceHash == "" {
		t.Fatalf("locator = %#v", assistant.Session.Locator)
	}
	if assistant.Actor.Kind != "agent" || assistant.Actor.Name != "claude_code" {
		t.Fatalf("actor = %#v", assistant.Actor)
	}
	if got := result.Envelopes[1].Turn.UserText; got != "Please inspect the fake example." {
		t.Fatalf("user text = %q", got)
	}
}

func TestScan_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewAdapter().Scan(ctx, ScanRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
