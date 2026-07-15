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
	learningType, proposedDest, decision, seed string) string {
	t.Helper()

	capture := mustCallJSON(t, ts, ctx, "learning_capture", map[string]any{
		"title":                "Governance surface " + seed,
		"type":                 learningType,
		"context":              "While closing the contract recovery we found a governance hole (" + seed + ").",
		"observation":          "A non-preference learning routed to a sensitive destination published with no approval (" + seed + ").",
		"reusable_lesson":      "AGENTS.md and shared scope must always require explicit human approval (" + seed + ").",
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

	learningID := captureCurateForApproval(t, ts, ctx, "diagnostic", "agents_rule", "approve_agents_rule", "hole")

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

// previewFor returns the requires_approval flag and preview hash for a learning.
func previewFor(t *testing.T, ts *testServer, ctx context.Context, learningID string) (bool, string) {
	t.Helper()
	preview := mustCallJSON(t, ts, ctx, "learning_publication_preview", map[string]any{
		"learning_id": learningID,
		"actor":       map[string]any{"kind": "human", "name": "publisher"},
	})
	requires, _ := preview["requires_approval"].(bool)
	hash, _ := preview["preview_hash"].(string)
	return requires, hash
}

// approve grants a real approval through the public learning_approve tool.
func approve(t *testing.T, ts *testServer, ctx context.Context, learningID, previewHash string, extra map[string]any) map[string]any {
	t.Helper()
	args := map[string]any{
		"learning_id":       learningID,
		"preview_hash":      previewHash,
		"approved_by":       "release-owner",
		"reason":            "Reviewed the diff and authorize this exact preview.",
		"approval_evidence": "https://example.test/approvals/1",
		"actor":             map[string]any{"kind": "human", "name": "release-owner"},
	}
	for k, v := range extra {
		args[k] = v
	}
	return mustCallJSON(t, ts, ctx, "learning_approve", args)
}

// TestApprovalGate_SensitiveWithoutApprovalIsBlocked: a shared-scope publication
// with no approval is refused.
func TestApprovalGate_SensitiveWithoutApprovalIsBlocked(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "agent")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	learningID := captureCurateForApproval(t, ts, ctx, "architecture", "shared", "approve_shared_knowledge", "blocked")
	requires, hash := previewFor(t, ts, ctx, learningID)
	if !requires {
		t.Fatal("shared-scope publication must require approval")
	}

	result, err := ts.callTool(ctx, "learning_publish", map[string]any{
		"learning_id":  learningID,
		"preview_hash": hash,
		"actor":        map[string]any{"kind": "agent", "name": "recorrido-c"},
	})
	if err != nil {
		t.Fatalf("learning_publish: %v", err)
	}
	if !result.IsError {
		t.Fatal("shared-scope publication without approval must be blocked")
	}
	if !contains(toolText(result), "approval") {
		t.Errorf("error should mention approval, got: %s", toolText(result))
	}
}

// TestApprovalGate_ValidApprovalIsAccepted: preview -> approve -> publish with
// the matching approval_id succeeds and writes the file.
func TestApprovalGate_ValidApprovalIsAccepted(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "agent")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	learningID := captureCurateForApproval(t, ts, ctx, "architecture", "shared", "approve_shared_knowledge", "valid")
	requires, hash := previewFor(t, ts, ctx, learningID)
	if !requires {
		t.Fatal("shared-scope publication must require approval")
	}

	appr := approve(t, ts, ctx, learningID, hash, nil)
	approvalID, _ := appr["approval_id"].(string)
	if approvalID == "" {
		t.Fatalf("approve returned no approval_id: %v", appr)
	}

	published := mustCallJSON(t, ts, ctx, "learning_publish", map[string]any{
		"learning_id":  learningID,
		"preview_hash": hash,
		"approval_id":  approvalID,
		"apply":        true,
		"actor":        map[string]any{"kind": "agent", "name": "recorrido-c"},
	})
	if status, _ := published["status"].(string); status != "completed" {
		t.Fatalf("publication status = %q, want completed (%v)", status, published)
	}
}

// TestApprovalGate_ApprovalForDifferentPreviewIsRejected: an approval bound to
// one preview hash cannot authorize the publication of a different preview.
// This covers both "approval for a different preview hash" and "approval reused
// for a different preview".
func TestApprovalGate_ApprovalForDifferentPreviewIsRejected(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "agent")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Learning A: approved and its preview approved.
	learningA := captureCurateForApproval(t, ts, ctx, "architecture", "shared", "approve_shared_knowledge", "A")
	_, hashA := previewFor(t, ts, ctx, learningA)
	apprA := approve(t, ts, ctx, learningA, hashA, nil)
	approvalA, _ := apprA["approval_id"].(string)

	// Learning B: a different learning with its own, unapproved preview.
	learningB := captureCurateForApproval(t, ts, ctx, "architecture", "shared", "approve_shared_knowledge", "B")
	_, hashB := previewFor(t, ts, ctx, learningB)
	if hashA == hashB {
		t.Fatal("test setup: the two learnings produced identical preview hashes")
	}

	// Attempt to publish B by reusing A's approval id.
	result, err := ts.callTool(ctx, "learning_publish", map[string]any{
		"learning_id":  learningB,
		"preview_hash": hashB,
		"approval_id":  approvalA,
		"actor":        map[string]any{"kind": "agent", "name": "recorrido-c"},
	})
	if err != nil {
		t.Fatalf("learning_publish: %v", err)
	}
	if !result.IsError {
		t.Fatal("an approval bound to preview A must not authorize publishing preview B")
	}
}

// TestApprovalGate_ExpiredApprovalIsRejected: an approval whose expiry is in the
// past is rejected at publish time.
func TestApprovalGate_ExpiredApprovalIsRejected(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "agent")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	learningID := captureCurateForApproval(t, ts, ctx, "architecture", "shared", "approve_shared_knowledge", "expired")
	_, hash := previewFor(t, ts, ctx, learningID)

	past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	appr := approve(t, ts, ctx, learningID, hash, map[string]any{"expires_at": past})
	approvalID, _ := appr["approval_id"].(string)
	if approvalID == "" {
		t.Fatalf("approve returned no approval_id: %v", appr)
	}

	result, err := ts.callTool(ctx, "learning_publish", map[string]any{
		"learning_id":  learningID,
		"preview_hash": hash,
		"approval_id":  approvalID,
		"actor":        map[string]any{"kind": "agent", "name": "recorrido-c"},
	})
	if err != nil {
		t.Fatalf("learning_publish: %v", err)
	}
	if !result.IsError {
		t.Fatal("an expired approval must not authorize publication")
	}
	if !contains(toolText(result), "expired") {
		t.Errorf("error should mention expiry, got: %s", toolText(result))
	}
}
