package main

import (
	"fmt"
	"io"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/logging"
)

func writeCodeError(stderr io.Writer, code, next, format string, args ...any) int {
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        code,
		Message:     fmt.Sprintf(format, args...),
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  next,
	})
	return domain.ErrorCode(code).ExitCode()
}

// writeDomainError faithfully projects a service error onto the CLI contract.
func writeDomainError(stderr io.Writer, err error, fallbackCode, fallbackNext, prefix string) int {
	if domainErr, ok := domain.AsDomainError(err); ok {
		details := domainErr.Details
		if details == nil {
			details = map[string]any{}
		}
		nextAction := domainErr.NextAction
		if nextAction == "" {
			nextAction = fallbackNext
		}
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:        string(domainErr.Code),
			Message:     prefix + domainErr.Message,
			Recoverable: domainErr.Recoverable,
			Details:     details,
			NextAction:  nextAction,
		})
		return domainErr.Code.ExitCode()
	}
	return writeCodeError(stderr, fallbackCode, fallbackNext, "%s%v", prefix, err)
}
