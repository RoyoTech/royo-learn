package main

import (
	"fmt"
	"io"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/logging"
)

// ---------------------------------------------------------------------------
// One error model on the CLI surface (Tramo 4 §4.3).
//
// Both helpers derive their exit code from the error code via
// domain.ErrorCode.ExitCode (docs/04-CLI-SPEC.md §Exit codes). No handler picks
// an exit constant by hand and no surface interprets an error by string
// matching. writeCodeError is for CLI-level validation (a code chosen at the
// call site); writeDomainError surfaces a service error faithfully, carrying its
// real code, message, recoverability, details and next action.
// ---------------------------------------------------------------------------

// writeCodeError writes the documented envelope for a hand-set code and returns
// the exit code the mapping assigns to it.
func writeCodeError(stderr io.Writer, code, next, format string, a ...any) int {
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        code,
		Message:     fmt.Sprintf(format, a...),
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  next,
	})
	return domain.ErrorCode(code).ExitCode()
}

// writeDomainError translates a service error. When err carries a domain error,
// its real code drives both the envelope and the exit code (approval_required
// exits 7, learning_not_found exits 5, ...). Otherwise it falls back to the
// supplied code. prefix is prepended to the message (e.g. "publish: ").
func writeDomainError(stderr io.Writer, err error, fallbackCode, fallbackNext, prefix string) int {
	if de, ok := domain.AsDomainError(err); ok {
		details := de.Details
		if details == nil {
			details = map[string]any{}
		}
		next := de.NextAction
		if next == "" {
			next = fallbackNext
		}
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:        string(de.Code),
			Message:     prefix + de.Message,
			Recoverable: de.Recoverable,
			Details:     details,
			NextAction:  next,
		})
		return de.Code.ExitCode()
	}
	return writeCodeError(stderr, fallbackCode, fallbackNext, "%s%v", prefix, err)
}
