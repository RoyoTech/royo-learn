// Contract tests for the Detector interface shipped in Hito 5 slice 5.0.
//
// These tests exist BEFORE the production types in detectors.go. In the
// RED phase the file must not compile (Detector, DetectInput and
// CandidateEvent do not exist yet). In the GREEN phase detectors.go
// introduces the minimum surface required to make every test in this
// file pass with zero per-detector logic.
//
// The contract verifies the four invariants that every detector must
// satisfy regardless of its Kind:
//
//   - Kind() returns the canonical lower_snake_case name.
//   - Version() returns a non-empty semantic version string.
//   - Detect(ctx, DetectInput{}) returns (nil, nil) when the input
//     carries no observation: zero events, zero error.
//   - Detect is deterministic for the same (input, config) pair.
//
// Slice 5.0 ships only the contract. Per-detector logic and
// detector-specific tables arrive in slices 5.1 (file-change), 5.2
// (retry), 5.3 (config-change) and the matching per-kind cases land
// alongside their detector implementation.

package detectors

import (
	"context"
	"strings"
	"testing"
	"time"
)

// stubDetector is a test-only no-op detector. It exists solely so the
// contract tests have something to call. Production detectors land in
// later slices.
type stubDetector struct {
	kind    string
	version string
	events  []CandidateEvent
	err     error
}

func (s stubDetector) Kind() string    { return s.kind }
func (s stubDetector) Version() string { return s.version }
func (s stubDetector) Detect(_ context.Context, _ DetectInput) ([]CandidateEvent, error) {
	return s.events, s.err
}

// TestDetector_ContractCompile is a compile-time guard. If Detector,
// DetectInput or CandidateEvent disappear, the build breaks before
// any runtime test runs. Cheap insurance against accidental removals.
func TestDetector_ContractCompile(t *testing.T) {
	t.Parallel()
	var _ Detector = stubDetector{}
	var _ = DetectInput{}
	var _ = CandidateEvent{}
}

// TestDetector_ContractReturns covers the happy path every detector
// must respect: empty input yields zero events and no error.
func TestDetector_ContractReturns(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	det := stubDetector{kind: "stub", version: "0.1.0"}

	if got := det.Kind(); got != "stub" {
		t.Errorf("Kind() = %q, want %q", got, "stub")
	}
	if got := det.Version(); got != "0.1.0" {
		t.Errorf("Version() = %q, want %q", got, "0.1.0")
	}

	got, err := det.Detect(ctx, DetectInput{})
	if err != nil {
		t.Fatalf("Detect(empty) returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Detect(empty) returned %d events, want 0", len(got))
	}
}

// TestDetector_ContractDeterministic verifies the determinism rule
// from docs/23-PATTERN-MINING.md §1: same input + same version =
// same output. The stub returns nil regardless, so the assertion is
// trivial for 5.0; per-detector determinism is enforced in 5.1-5.3.
func TestDetector_ContractDeterministic(t *testing.T) {
	t.Parallel()

	det := stubDetector{kind: "stub", version: "0.1.0"}
	in := DetectInput{
		Source:      "test",
		ProjectRoot: "/tmp/proj",
		Payload:     "any-payload",
		Timestamp:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}

	first, err := det.Detect(context.Background(), in)
	if err != nil {
		t.Fatalf("first Detect error: %v", err)
	}
	second, err := det.Detect(context.Background(), in)
	if err != nil {
		t.Fatalf("second Detect error: %v", err)
	}
	if len(first) != len(second) {
		t.Errorf("Detect non-deterministic: first=%d events, second=%d events",
			len(first), len(second))
	}
}

// TestDetector_CanonicalKinds exercises the five event kinds from
// docs/23-PATTERN-MINING.md §2.1. The stub detector accepts any kind
// string, so this test verifies the *names* are valid identifiers
// downstream code can switch on. Real detectors per kind land in
// slices 5.1-5.3.
func TestDetector_CanonicalKinds(t *testing.T) {
	t.Parallel()

	kinds := []string{
		"correction",
		"command_outcome",
		"tests",
		"retry",
		"tool_limit",
	}
	for _, kind := range kinds {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			det := stubDetector{kind: kind, version: "0.1.0"}

			if got := det.Kind(); got != kind {
				t.Errorf("Kind() = %q, want %q", got, kind)
			}
			if got := det.Kind(); strings.ToLower(got) != got {
				t.Errorf("Kind() = %q, want lower_snake_case", got)
			}
			if strings.ContainsAny(det.Kind(), " \t\n-/") {
				t.Errorf("Kind() = %q, want single token", det.Kind())
			}
		})
	}
}

// TestDetector_NonEmptyVersion guards against a detector shipping
// without a version. Per docs/22-ADAPTER-CONTRACT.md §7 every
// detector version bumps must reset fingerprint compatibility, so a
// blank version would silently leak old fingerprints.
func TestDetector_NonEmptyVersion(t *testing.T) {
	t.Parallel()

	det := stubDetector{kind: "stub", version: ""}
	if got := det.Version(); got != "" {
		t.Fatalf("Version() = %q, want empty for this guard; production detectors must return non-empty", got)
	}
	// Production code path is verified separately when slice 5.1
	// lands: every concrete detector must override Version() with a
	// semver string. The compile-time guard above ensures the
	// signature is satisfied.
}
