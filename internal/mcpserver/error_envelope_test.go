package mcpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"agent-royo-learn/internal/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mcpErrorBody(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	if !result.IsError || len(result.Content) == 0 {
		t.Fatalf("expected a populated MCP error result")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("error content is not text: %T", result.Content[0])
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(text.Text), &body); err != nil {
		t.Fatalf("error content not valid JSON: %v\n%s", err, text.Text)
	}
	inner, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope not nested under error: %v", body)
	}
	return inner
}

func TestMCPErrorModelEveryClassTranslates(t *testing.T) {
	t.Parallel()

	for _, code := range domain.AllErrorCodes() {
		result, _, _ := toolDomainError(&domain.DomainError{
			Code:        code,
			Message:     "boom",
			Recoverable: true,
			Details:     map[string]any{"k": "v"},
			NextAction:  "do the thing",
		}, "operation_failed")
		inner := mcpErrorBody(t, result)
		if inner["code"] != string(code) {
			t.Errorf("code %q: envelope code = %v", code, inner["code"])
		}
		details, ok := inner["details"].(map[string]any)
		if !ok || details["k"] != "v" {
			t.Errorf("code %q: envelope lost details: %v", code, inner["details"])
		}
		if inner["next_action"] != "do the thing" || inner["recoverable"] != true {
			t.Errorf("code %q: incomplete envelope: %v", code, inner)
		}
	}
}

func TestMCPErrorModelPreservesRollbackRecoveryDetails(t *testing.T) {
	t.Parallel()

	artifact := "C:/project/.royo-learn/recovery/publication-target-1.patch"
	wrapped := fmt.Errorf("rollback: %w", &domain.DomainError{
		Code:        domain.ErrRollbackFailed,
		Message:     "rollback could not safely restore one target",
		Recoverable: true,
		Details: map[string]any{
			"publication_id":    "publication-1",
			"recovery_artifact": artifact,
			"conflicts":         []any{map[string]any{"path": "skills/demo/SKILL.md"}},
		},
		NextAction: "review the reversal artifact",
	})
	result, _, _ := toolDomainError(wrapped, "rollback_failed")
	inner := mcpErrorBody(t, result)
	details, _ := inner["details"].(map[string]any)
	if inner["code"] != string(domain.ErrRollbackFailed) || details["recovery_artifact"] != artifact {
		t.Fatalf("rollback recovery envelope lost typed details: %v", inner)
	}
}

func TestMCPErrorModelFallbackAndToolErrorsAreNested(t *testing.T) {
	t.Parallel()

	result, _, _ := toolDomainError(errors.New("boom"), "operation_failed")
	if inner := mcpErrorBody(t, result); inner["code"] != "operation_failed" {
		t.Fatalf("fallback error code = %v", inner["code"])
	}
	result, _, _ = toolError("invalid_argument", "bad input")
	if inner := mcpErrorBody(t, result); inner["code"] != "invalid_argument" {
		t.Fatalf("tool error code = %v", inner["code"])
	}
}
