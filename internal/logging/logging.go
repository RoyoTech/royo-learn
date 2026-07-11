// Package logging owns structured diagnostics written to an injected stream.
package logging

import (
	"encoding/json"
	"io"
)

// Diagnostic is a machine-readable diagnostic written only to stderr by callers.
type Diagnostic struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// ErrorEnvelope is the documented machine-readable error contract.
type ErrorEnvelope struct {
	Code        string         `json:"code"`
	Message     string         `json:"message"`
	Recoverable bool           `json:"recoverable"`
	Details     map[string]any `json:"details"`
	NextAction  string         `json:"next_action"`
}

// WriteDiagnostic writes one JSON diagnostic to the supplied diagnostic stream.
func WriteDiagnostic(destination io.Writer, message string) error {
	return json.NewEncoder(destination).Encode(Diagnostic{
		Level:   "error",
		Message: message,
	})
}

// WriteError writes one documented error envelope to the supplied diagnostic stream.
func WriteError(destination io.Writer, envelope ErrorEnvelope) error {
	return json.NewEncoder(destination).Encode(envelope)
}
