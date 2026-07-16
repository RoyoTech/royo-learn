package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"agent-royo-learn/internal/domain"
)

func TestCLIErrorModelEveryClassTranslates(t *testing.T) {
	t.Parallel()

	for _, code := range domain.AllErrorCodes() {
		de := &domain.DomainError{
			Code:        code,
			Message:     "boom",
			Recoverable: true,
			Details:     map[string]any{"k": "v"},
			NextAction:  "do the thing",
		}

		var stderr bytes.Buffer
		exit := writeDomainError(&stderr, de, "invalid_argument", `run "royo-learn --help"`, "op: ")
		if want := code.ExitCode(); exit != want {
			t.Errorf("code %q: exit = %d, want %d", code, exit, want)
		}

		var envelope map[string]any
		if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
			t.Fatalf("code %q: stderr not valid JSON: %v\n%s", code, err, stderr.String())
		}
		if envelope["code"] != string(code) {
			t.Errorf("code %q: envelope code = %v", code, envelope["code"])
		}
		details, ok := envelope["details"].(map[string]any)
		if !ok || details["k"] != "v" {
			t.Errorf("code %q: envelope lost details: %v", code, envelope["details"])
		}
		if envelope["next_action"] != "do the thing" || envelope["recoverable"] != true {
			t.Errorf("code %q: incomplete envelope: %v", code, envelope)
		}
	}
}

func TestCLIErrorModelPreservesRollbackRecoveryDetails(t *testing.T) {
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

	var stderr bytes.Buffer
	exit := writeDomainError(&stderr, wrapped, "invalid_argument", "fallback", "rollback: ")
	if exit != domain.ErrRollbackFailed.ExitCode() {
		t.Fatalf("exit = %d, want %d", exit, domain.ErrRollbackFailed.ExitCode())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("stderr not valid JSON: %v", err)
	}
	details, _ := envelope["details"].(map[string]any)
	if envelope["code"] != string(domain.ErrRollbackFailed) || details["recovery_artifact"] != artifact {
		t.Fatalf("rollback recovery envelope lost typed details: %v", envelope)
	}
}

func TestCLIErrorModelNonDomainErrorFallsBack(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	exit := writeDomainError(&stderr, errors.New("plain error"), "invalid_argument", "fallback", "op: ")
	if exit != domain.ErrInvalidArgument.ExitCode() {
		t.Fatalf("fallback exit = %d, want %d", exit, domain.ErrInvalidArgument.ExitCode())
	}
}
