package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The tests in this file drive Recorrido B through the MCP surface only.
// Nothing here calls storage.SaveEvidence. If a test can only pass by reaching
// around the public interface, it is not proving that the product works.

// resultJSON decodes the single text content of a tool result.
func resultJSON(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("tool result has no content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("tool result content is %T, want *mcp.TextContent", res.Content[0])
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
		t.Fatalf("tool result is not a JSON object: %v\n%s", err, text.Text)
	}
	return out
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		return ""
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		return ""
	}
	return text.Text
}

func testActor() map[string]any {
	return map[string]any{"kind": "agent", "name": "mcp-test"}
}

// TestMCP_AddEvidenceUnblocksApproval is the acceptance criterion of Recorrido B
// driven entirely over MCP: captured -> needs_evidence -> evidence_attached ->
// approved, with no direct SQLite manipulation.
func TestMCP_AddEvidenceUnblocksApproval(t *testing.T) {
	ctx := context.Background()
	ts := newTestServer(t, profileAgent)

	// 1. captured, with no evidence records.
	capRes, err := ts.callTool(ctx, "learning_capture", map[string]any{
		"title":           "MCP evidence gate",
		"type":            "procedure",
		"context":         "Recorrido B over MCP",
		"observation":     "Approval requires a persisted evidence record",
		"reusable_lesson": "Every threshold needs a public interface that satisfies it",
		"scope_guess":     "project",
		"confidence":      "high",
		"evidence_level":  "moderate",
		"actor":           testActor(),
	})
	if err != nil {
		t.Fatalf("learning_capture: %v", err)
	}
	if capRes.IsError {
		t.Fatalf("learning_capture failed: %s", resultText(t, capRes))
	}
	captured := resultJSON(t, capRes)
	learningID, _ := captured["learning_id"].(string)
	if learningID == "" {
		t.Fatal("learning_capture returned no learning_id")
	}

	// 2. Approval MUST be blocked: no evidence record exists.
	blocked, err := ts.callTool(ctx, "learning_curate", map[string]any{
		"learning_id": learningID,
		"decision":    "approve_project_knowledge",
		"rationale":   "Attempting approval with zero evidence records",
		"actor":       testActor(),
	})
	if err != nil {
		t.Fatalf("learning_curate: %v", err)
	}
	if !blocked.IsError {
		t.Fatalf("approval succeeded with zero evidence records; the D3 threshold is not enforced: %s",
			resultText(t, blocked))
	}
	if !strings.Contains(resultText(t, blocked), "evidence") {
		t.Fatalf("blocked approval does not mention evidence: %s", resultText(t, blocked))
	}

	// 3. evidence_attached — through learning_add_evidence, not through SQL.
	addRes, err := ts.callTool(ctx, "learning_add_evidence", map[string]any{
		"learning_id": learningID,
		"evidence": []any{
			map[string]any{
				"kind":    "test",
				"summary": "The MCP flow reproduces the blocked approval",
				"source":  "go test ./internal/mcpserver",
				"content": "--- PASS: TestMCP_AddEvidenceUnblocksApproval",
			},
		},
		"actor": testActor(),
	})
	if err != nil {
		t.Fatalf("learning_add_evidence: %v", err)
	}
	if addRes.IsError {
		t.Fatalf("learning_add_evidence failed: %s", resultText(t, addRes))
	}
	added := resultJSON(t, addRes)
	if count, _ := added["evidence_count"].(float64); count != 1 {
		t.Fatalf("evidence_count = %v, want 1", added["evidence_count"])
	}

	// 4. approved — the same call that was blocked now succeeds.
	okRes, err := ts.callTool(ctx, "learning_curate", map[string]any{
		"learning_id": learningID,
		"decision":    "approve_project_knowledge",
		"rationale":   "Evidence is attached and the threshold is satisfied",
		"actor":       testActor(),
	})
	if err != nil {
		t.Fatalf("learning_curate (approve): %v", err)
	}
	if okRes.IsError {
		t.Fatalf("approval after attaching evidence still fails: %s", resultText(t, okRes))
	}
	approved := resultJSON(t, okRes)
	if got, _ := approved["new_status"].(string); got != "approved" {
		t.Fatalf("new_status = %q, want %q", got, "approved")
	}
}

// TestMCP_CaptureAcceptsEmbeddedEvidence proves learning_capture persists the
// evidence[] array it accepts, in the same coherent operation as the learning.
func TestMCP_CaptureAcceptsEmbeddedEvidence(t *testing.T) {
	ctx := context.Background()
	ts := newTestServer(t, profileAgent)

	capRes, err := ts.callTool(ctx, "learning_capture", map[string]any{
		"title":           "Capture carries evidence over MCP",
		"type":            "procedure",
		"context":         "Recorrido B",
		"observation":     "learning_capture accepts an evidence array",
		"reusable_lesson": "Evidence must be attachable at capture time",
		"scope_guess":     "project",
		"confidence":      "high",
		"evidence_level":  "strong",
		"evidence": []any{
			map[string]any{
				"kind":    "command",
				"summary": "The suite is green after the fix",
				"source":  "go test ./...",
				"content": "ok agent-royo-learn/internal/curate 0.4s",
			},
		},
		"actor": testActor(),
	})
	if err != nil {
		t.Fatalf("learning_capture: %v", err)
	}
	if capRes.IsError {
		t.Fatalf("learning_capture failed: %s", resultText(t, capRes))
	}
	captured := resultJSON(t, capRes)
	if count, _ := captured["evidence_count"].(float64); count != 1 {
		t.Fatalf("evidence_count = %v, want 1", captured["evidence_count"])
	}
	learningID, _ := captured["learning_id"].(string)

	// Approval must succeed straight away.
	okRes, err := ts.callTool(ctx, "learning_curate", map[string]any{
		"learning_id": learningID,
		"decision":    "approve_project_knowledge",
		"rationale":   "Evidence was supplied at capture time",
		"actor":       testActor(),
	})
	if err != nil {
		t.Fatalf("learning_curate: %v", err)
	}
	if okRes.IsError {
		t.Fatalf("approval after capture-with-evidence failed: %s", resultText(t, okRes))
	}
}

// TestMCP_SecretIsRedactedBeforeTheResponse asserts the MCP response is not a
// leak channel. Redaction runs before persistence, so it is also before this
// response is built.
func TestMCP_SecretIsRedactedBeforeTheResponse(t *testing.T) {
	ctx := context.Background()
	ts := newTestServer(t, profileAgent)

	const secret = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"

	capRes, err := ts.callTool(ctx, "learning_capture", map[string]any{
		"title":           "Token leaked into a log line",
		"type":            "security",
		"context":         "CI printed a GitHub token",
		"observation":     "The token " + secret + " appeared in the build log",
		"reusable_lesson": "Redact secrets before any write",
		"scope_guess":     "project",
		"confidence":      "high",
		"evidence_level":  "strong",
		"evidence": []any{
			map[string]any{
				"kind":    "text",
				"summary": "Build log containing " + secret,
				"content": "fatal: authentication failed using " + secret,
			},
		},
		"actor": testActor(),
	})
	if err != nil {
		t.Fatalf("learning_capture: %v", err)
	}
	if capRes.IsError {
		t.Fatalf("learning_capture failed: %s", resultText(t, capRes))
	}

	if strings.Contains(resultText(t, capRes), secret) {
		t.Errorf("SINK LEAK — the MCP capture response contains the secret:\n%s", resultText(t, capRes))
	}

	captured := resultJSON(t, capRes)
	learningID, _ := captured["learning_id"].(string)

	getRes, err := ts.callTool(ctx, "learning_get", map[string]any{"learning_id": learningID})
	if err != nil {
		t.Fatalf("learning_get: %v", err)
	}
	if strings.Contains(resultText(t, getRes), secret) {
		t.Errorf("SINK LEAK — learning_get returns the secret from SQLite:\n%s", resultText(t, getRes))
	}
	if !strings.Contains(resultText(t, getRes), "[REDACTED:") {
		t.Errorf("learning_get shows no redaction marker; redaction did not run:\n%s", resultText(t, getRes))
	}
}

// TestMCP_AddEvidenceRejectsEmptyPayload — a call that attaches nothing must not
// report success. Otherwise an agent could believe it satisfied the threshold.
func TestMCP_AddEvidenceRejectsEmptyPayload(t *testing.T) {
	ctx := context.Background()
	ts := newTestServer(t, profileAgent)

	capRes, err := ts.callTool(ctx, "learning_capture", map[string]any{
		"title":           "Empty evidence payload",
		"type":            "procedure",
		"context":         "Recorrido B",
		"observation":     "An empty evidence array must be rejected",
		"reusable_lesson": "Never report success for an attachment that attached nothing",
		"scope_guess":     "project",
		"confidence":      "high",
		"evidence_level":  "moderate",
		"actor":           testActor(),
	})
	if err != nil {
		t.Fatalf("learning_capture: %v", err)
	}
	learningID, _ := resultJSON(t, capRes)["learning_id"].(string)

	res, err := ts.callTool(ctx, "learning_add_evidence", map[string]any{
		"learning_id": learningID,
		"evidence":    []any{},
		"actor":       testActor(),
	})
	if err != nil {
		t.Fatalf("learning_add_evidence: %v", err)
	}
	if !res.IsError {
		t.Fatalf("empty evidence payload reported success: %s", resultText(t, res))
	}
}

// TestMCP_AddEvidenceIsNotServedByReadProfile — learning_add_evidence is a
// write. It must not appear in the read profile (D2).
func TestMCP_AddEvidenceIsNotServedByReadProfile(t *testing.T) {
	for _, tc := range []struct {
		profile string
		want    bool
	}{
		{profileRead, false},
		{profileAgent, true},
		{profileAdmin, true},
	} {
		served := false
		for _, tool := range profileTools(tc.profile) {
			if tool.name == "learning_add_evidence" {
				served = true
			}
		}
		if served != tc.want {
			t.Errorf("profile %q serves learning_add_evidence = %v, want %v", tc.profile, served, tc.want)
		}
	}
}
