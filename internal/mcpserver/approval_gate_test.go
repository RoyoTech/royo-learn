package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// Recorrido C — the governance hole and the approval gate.
//
// These tests drive the real MCP session end to end. They never fabricate an
// Approval by writing to storage: every approval is obtained through the public
// learning_approve tool, and every publication is attempted through the public
// learning_publish tool.
// ---------------------------------------------------------------------------

// captureCurateForApproval captures a learning proposing the given destination,
// attaches evidence, and curates it with the given decision — all through the
// public MCP tools — leaving the learning in status approved. It returns the
// learning id.
func captureCurateForApproval(t *testing.T, ts *testServer, ctx context.Context,
	learningType, proposedDest, decision string) string {
	t.Helper()

	capture := mustCallJSON(t, ts, ctx, "learning_capture", map[string]any{
		"title":                "Route imports through the shared allowlist",
		"type":                 learningType,
		"context":              "While closing the contract recovery we found a governance hole.",
		"observation":          "A non-preference learning routed to AGENTS.md published with no approval.",
		"reusable_lesson":      "AGENTS.md and shared scope must always require explicit human approval.",
		"scope_guess":          "project",
		"confidence":           "high",
		"evidence_level":       "moderate",
		"proposed_destination": proposedDest,
		"actor":                map[string]any{"kind": "agent", "name": "recorrido-c"},
		"evidence": []map[string]any{
			{
				"kind":    "test",
				"summary": "The failing test reproduces the hole before the fix",
				"source":  "test://recorrido-c",
				"content": "--- FAIL: TestApprovalGate",
			},
		},
	})
	learningID, _ := capture["learning_id"].(string)
	if learningID == "" {
		t.Fatalf("capture returned no learning_id: %v", capture)
	}

	mustCallJSON(t, ts, ctx, "learning_curate", map[string]any{
		"learning_id": learningID,
		"decision":    decision,
		"rationale":   "Evidence proves the learning is reusable and belongs in the shared governance surface.",
		"actor":       map[string]any{"kind": "human", "name": "curator"},
	})

	return learningID
}

// mustCallJSON calls a tool, fails on transport or tool error, and returns the
// decoded JSON body.
func mustCallJSON(t *testing.T, ts *testServer, ctx context.Context, name string, args map[string]any) map[string]any {
	t.Helper()
	result, err := ts.callTool(ctx, name, args)
	if err != nil {
		t.Fatalf("%s: transport error: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s: tool error: %s", name, toolText(result))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(toolText(result)), &out); err != nil {
		t.Fatalf("%s: response not JSON object: %v (%s)", name, err, toolText(result))
	}
	return out
}

func toolText(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	if tc, ok := result.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

// TestApprovalGate_NonPreferenceAgentsRuleRequiresApproval is the RED test that
// demonstrates the governance hole. Today the destination-based policies are
// tautologies: a non-preference learning routed to AGENTS.md publishes with
// requires_approval=false and no human ever authorizes touching the file that
// governs every agent. The correct contract is the opposite: AGENTS.md always
// requires approval.
func TestApprovalGate_NonPreferenceAgentsRuleRequiresApproval(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "admin")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	learningID := captureCurateForApproval(t, ts, ctx, "diagnostic", "agents_rule", "approve_agents_rule")

	preview := mustCallJSON(t, ts, ctx, "learning_publication_preview", map[string]any{
		"learning_id": learningID,
		"actor":       map[string]any{"kind": "human", "name": "publisher"},
	})

	requiresApproval, _ := preview["requires_approval"].(bool)
	if !requiresApproval {
		t.Fatal("GOVERNANCE HOLE: a non-preference learning routed to AGENTS.md reports " +
			"requires_approval=false; publishing global rules must always require human approval")
	}

	previewHash, _ := preview["preview_hash"].(string)

	// With approval required and none granted, publishing must be blocked.
	result, err := ts.callTool(ctx, "learning_publish", map[string]any{
		"learning_id":  learningID,
		"preview_hash": previewHash,
		"actor":        map[string]any{"kind": "agent", "name": "recorrido-c"},
	})
	if err != nil {
		t.Fatalf("learning_publish: transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("GOVERNANCE HOLE: AGENTS.md was published with no approval record")
	}
}
