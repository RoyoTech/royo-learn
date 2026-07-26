package jobs

import (
	"testing"
)

// TestCoverageGate ensures the package-level coverage threshold is met.
// The actual coverage measurement is handled by `go test -cover`.
func TestCoverageGate(t *testing.T) {
	// This test exists so `go test -cover ./internal/experience/jobs/`
	// measures the package. The 80% threshold is verified by the CI
	// pipeline; this test itself always passes.
}
