package logging

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteDiagnosticUsesProvidedStream(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	if err := WriteDiagnostic(&stderr, "invalid arguments"); err != nil {
		t.Fatalf("WriteDiagnostic() error = %v", err)
	}

	var diagnostic Diagnostic
	if err := json.Unmarshal(stderr.Bytes(), &diagnostic); err != nil {
		t.Fatalf("diagnostic is not valid JSON: %v", err)
	}
	if diagnostic.Level != "error" || diagnostic.Message != "invalid arguments" {
		t.Errorf("diagnostic = %#v", diagnostic)
	}
}
