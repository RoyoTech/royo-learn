package mcpserver

import (
	"encoding/json"
	"testing"

	"agent-royo-learn/internal/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// One error model on the MCP surface (Tramo 4 §4.3).
//
// MCP errors use the nested envelope docs/05-MCP-SPEC.md specifies:
//
//	{ "error": { "code", "message", "recoverable", "details", "next_action" } }
//
// toolDomainError translates a domain error faithfully: the envelope carries the
// domain error's real code, never a hand-picked string and never a string match.
// This table has one row per error class.
// ---------------------------------------------------------------------------

// mcpErrorBody decodes the nested error envelope from a tool result.
func mcpErrorBody(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	if !result.IsError {
		t.Fatalf("expected an error result")
	}
	if len(result.Content) == 0 {
		t.Fatal("error result has no content")
	}
	txt, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("error content is not text: %T", result.Content[0])
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(txt.Text), &body); err != nil {
		t.Fatalf("error content not valid JSON: %v\n%s", err, txt.Text)
	}
	inner, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope not nested under \"error\": %v", body)
	}
	return inner
}

func TestMCPErrorModel_EveryClassTranslates(t *testing.T) {
	t.Parallel()

	for _, code := range domain.AllErrorCodes() {
		de := &domain.DomainError{
			Code:        code,
			Message:     "boom",
			Recoverable: true,
			Details:     map[string]any{"k": "v"},
			NextAction:  "do the thing",
		}
		result, _, _ := toolDomainError(de, "op_failed")
		inner := mcpErrorBody(t, result)

		if inner["code"] != string(code) {
			t.Errorf("code %q: envelope error.code = %v, want %q", code, inner["code"], code)
		}
		if _, ok := inner["message"].(string); !ok {
			t.Errorf("code %q: envelope missing message", code)
		}
		if _, ok := inner["next_action"]; !ok {
			t.Errorf("code %q: envelope missing next_action", code)
		}
		if _, ok := inner["details"].(map[string]any); !ok {
			t.Errorf("code %q: envelope missing details object", code)
		}
		if _, ok := inner["recoverable"].(bool); !ok {
			t.Errorf("code %q: envelope missing recoverable", code)
		}
	}
}

// A non-domain error falls back to the supplied code, still nested.
func TestMCPErrorModel_NonDomainFallsBack(t *testing.T) {
	t.Parallel()
	result, _, _ := toolDomainError(errPlain("boom"), "op_failed")
	inner := mcpErrorBody(t, result)
	if inner["code"] != "op_failed" {
		t.Errorf("fallback error.code = %v, want op_failed", inner["code"])
	}
}

// toolError emits the same nested envelope shape for hand-set codes.
func TestMCPErrorModel_ToolErrorIsNested(t *testing.T) {
	t.Parallel()
	result, _, _ := toolError("invalid_argument", "bad input")
	inner := mcpErrorBody(t, result)
	if inner["code"] != "invalid_argument" {
		t.Errorf("error.code = %v, want invalid_argument", inner["code"])
	}
	if inner["message"] != "bad input" {
		t.Errorf("error.message = %v, want \"bad input\"", inner["message"])
	}
}

type plainErr struct{ s string }

func (e *plainErr) Error() string { return e.s }
func errPlain(s string) error     { return &plainErr{s} }
