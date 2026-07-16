package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"agent-royo-learn/internal/domain"
)

// ---------------------------------------------------------------------------
// One error model on the CLI surface (Tramo 4 §4.3).
//
// writeDomainError translates a domain error faithfully: the envelope carries
// the domain error's real code and the exit code is derived from that code via
// domain.ErrorCode.ExitCode — never a hand-picked constant, never a string
// match. This table has one row per error class.
// ---------------------------------------------------------------------------

func TestCLIErrorModel_EveryClassTranslates(t *testing.T) {
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

		var env map[string]any
		if err := json.Unmarshal(stderr.Bytes(), &env); err != nil {
			t.Fatalf("code %q: stderr not valid JSON envelope: %v\n%s", code, err, stderr.String())
		}
		if env["code"] != string(code) {
			t.Errorf("code %q: envelope code = %v, want %q", code, env["code"], code)
		}
		if _, ok := env["message"].(string); !ok {
			t.Errorf("code %q: envelope missing message", code)
		}
		if _, ok := env["next_action"].(string); !ok {
			t.Errorf("code %q: envelope missing next_action", code)
		}
		if _, ok := env["details"].(map[string]any); !ok {
			t.Errorf("code %q: envelope missing details object", code)
		}
	}
}

// A non-domain error falls back to the supplied code and its exit code, so the
// surface never crashes on an unexpected error type.
func TestCLIErrorModel_NonDomainErrorFallsBack(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	exit := writeDomainError(&stderr, errorsNew("plain error"), "invalid_argument", `run "royo-learn --help"`, "op: ")
	if want := domain.ErrInvalidArgument.ExitCode(); exit != want {
		t.Errorf("fallback exit = %d, want %d", exit, want)
	}
	var env map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &env); err != nil {
		t.Fatalf("stderr not valid JSON: %v", err)
	}
	if env["code"] != "invalid_argument" {
		t.Errorf("fallback envelope code = %v, want invalid_argument", env["code"])
	}
}

func errorsNew(msg string) error { return &plainError{msg} }

type plainError struct{ s string }

func (e *plainError) Error() string { return e.s }
