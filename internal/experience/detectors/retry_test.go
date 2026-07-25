// Retry detector tests for Hito 5 slice 5.1.
//
// The table covers nine scenarios that together guarantee the gates
// from docs/26-IMPLEMENTATION-ROADMAP.md §3 PR #4:
//
//   - Determinism: same input + same config → same output.
//   - Precision over recall: zero events in routine chat, only when
//     the same fingerprint fails Threshold times within WindowDuration.
//   - Boundary safety: prior success breaks the chain, old observations
//     outside the window are excluded, different fingerprints do not
//     accumulate, current success never emits.
//   - Contract safety: wrong payload type returns a typed error,
//     misconfiguration is rejected at construction time.
//   - Event shape: structured extras and retrieval terms are present
//     so downstream clustering and lexical retrieval work.

package detectors

import (
	"context"
	"strings"
	"testing"
	"time"
)

// retryDetector is a small helper that constructs a detector with the
// canonical configuration (threshold=3, window=5min) used by most
// tests. Tests that need different settings build their own.
func retryDetector(t *testing.T) RetryDetector {
	t.Helper()
	det, err := NewRetryDetector(3, 5*time.Minute)
	if err != nil {
		t.Fatalf("NewRetryDetector(3, 5m): %v", err)
	}
	return det
}

func TestRetryDetector_Contract(t *testing.T) {
	t.Parallel()
	det, err := NewRetryDetector(3, 5*time.Minute)
	if err != nil {
		t.Fatalf("NewRetryDetector: %v", err)
	}

	var _ Detector = det
	if got := det.Kind(); got != "retry" {
		t.Errorf("Kind() = %q, want %q", got, "retry")
	}
	if got := det.Version(); got != "0.1.0" {
		t.Errorf("Version() = %q, want %q", got, "0.1.0")
	}
}

// TestRetryDetector_BelowThreshold — a single failure with no prior
// history is below threshold and must not emit.
func TestRetryDetector_BelowThreshold(t *testing.T) {
	t.Parallel()
	det := retryDetector(t)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	in := DetectInput{
		Source:      "test",
		ProjectRoot: "/tmp/proj",
		Timestamp:   now,
		Payload: RetryPayload{
			Current: Observation{
				Fingerprint: "fp-cmd-fail-001",
				Tool:        "npm test",
				Result:      "fail",
				Timestamp:   now,
			},
			Recent: nil,
		},
	}

	got, err := det.Detect(context.Background(), in)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Detect returned %d events, want 0", len(got))
	}
}

// TestRetryDetector_AtThreshold — two prior fails + current fail =
// three occurrences, exactly at threshold, must emit one event.
func TestRetryDetector_AtThreshold(t *testing.T) {
	t.Parallel()
	det := retryDetector(t)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	in := DetectInput{
		Source:      "test",
		ProjectRoot: "/tmp/proj",
		Timestamp:   now,
		Payload: RetryPayload{
			Current: Observation{
				Fingerprint: "fp-cmd-fail-001",
				Tool:        "npm test",
				Result:      "fail",
				Timestamp:   now,
			},
			Recent: []Observation{
				{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-1 * time.Minute)},
				{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-3 * time.Minute)},
			},
		},
	}

	got, err := det.Detect(context.Background(), in)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Detect returned %d events, want 1", len(got))
	}
	if got[0].Kind != "retry" {
		t.Errorf("event Kind = %q, want %q", got[0].Kind, "retry")
	}
	if got[0].Tool != "npm test" {
		t.Errorf("event Tool = %q, want %q", got[0].Tool, "npm test")
	}
	if !strings.Contains(got[0].Problem, "fp-cmd-fail-001") {
		t.Errorf("event Problem = %q, want contains fingerprint", got[0].Problem)
	}
}

// TestRetryDetector_PriorSuccessBreaksChain — a prior success for the
// same fingerprint does not count toward retries; the chain is broken
// even if other entries were failures.
func TestRetryDetector_PriorSuccessBreaksChain(t *testing.T) {
	t.Parallel()
	det := retryDetector(t)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	in := DetectInput{
		Source:      "test",
		ProjectRoot: "/tmp/proj",
		Timestamp:   now,
		Payload: RetryPayload{
			Current: Observation{
				Fingerprint: "fp-cmd-fail-001",
				Tool:        "npm test",
				Result:      "fail",
				Timestamp:   now,
			},
			Recent: []Observation{
				{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "success", Timestamp: now.Add(-1 * time.Minute)},
				{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-3 * time.Minute)},
			},
		},
	}

	got, err := det.Detect(context.Background(), in)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Detect returned %d events, want 0 (success breaks chain)", len(got))
	}
}

// TestRetryDetector_WindowExcludesOldObservations — observations older
// than the window are not counted, even when the same fingerprint is
// involved.
func TestRetryDetector_WindowExcludesOldObservations(t *testing.T) {
	t.Parallel()
	det := retryDetector(t)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	in := DetectInput{
		Source:      "test",
		ProjectRoot: "/tmp/proj",
		Timestamp:   now,
		Payload: RetryPayload{
			Current: Observation{
				Fingerprint: "fp-cmd-fail-001",
				Tool:        "npm test",
				Result:      "fail",
				Timestamp:   now,
			},
			Recent: []Observation{
				{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-10 * time.Minute)},
				{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-30 * time.Minute)},
			},
		},
	}

	got, err := det.Detect(context.Background(), in)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Detect returned %d events, want 0 (prior fails outside window)", len(got))
	}
}

// TestRetryDetector_CurrentSuccessEmitsNothing — when the current
// observation is a success the detector never emits, regardless of
// recent failures.
func TestRetryDetector_CurrentSuccessEmitsNothing(t *testing.T) {
	t.Parallel()
	det := retryDetector(t)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	in := DetectInput{
		Source:      "test",
		ProjectRoot: "/tmp/proj",
		Timestamp:   now,
		Payload: RetryPayload{
			Current: Observation{
				Fingerprint: "fp-cmd-fail-001",
				Tool:        "npm test",
				Result:      "success",
				Timestamp:   now,
			},
			Recent: []Observation{
				{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-1 * time.Minute)},
				{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-3 * time.Minute)},
			},
		},
	}

	got, err := det.Detect(context.Background(), in)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Detect returned %d events, want 0 (current success)", len(got))
	}
}

// TestRetryDetector_DifferentFingerprintIgnored — recent failures
// with different fingerprints do not accumulate toward the current
// fingerprint's threshold.
func TestRetryDetector_DifferentFingerprintIgnored(t *testing.T) {
	t.Parallel()
	det := retryDetector(t)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	in := DetectInput{
		Source:      "test",
		ProjectRoot: "/tmp/proj",
		Timestamp:   now,
		Payload: RetryPayload{
			Current: Observation{
				Fingerprint: "fp-A",
				Tool:        "npm test",
				Result:      "fail",
				Timestamp:   now,
			},
			Recent: []Observation{
				{Fingerprint: "fp-B", Tool: "npm test", Result: "fail", Timestamp: now.Add(-1 * time.Minute)},
				{Fingerprint: "fp-C", Tool: "npm test", Result: "fail", Timestamp: now.Add(-3 * time.Minute)},
			},
		},
	}

	got, err := det.Detect(context.Background(), in)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Detect returned %d events, want 0 (different fingerprints)", len(got))
	}
}

// TestRetryDetector_Deterministic — same input + same detector config
// produces the same output. Required by docs/23 §1: "Pipeline
// auditable, mismo input + misma versión = mismo output".
func TestRetryDetector_Deterministic(t *testing.T) {
	t.Parallel()
	det := retryDetector(t)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	in := DetectInput{
		Source:      "test",
		ProjectRoot: "/tmp/proj",
		Timestamp:   now,
		Payload: RetryPayload{
			Current: Observation{
				Fingerprint: "fp-cmd-fail-001",
				Tool:        "npm test",
				Result:      "fail",
				Timestamp:   now,
			},
			Recent: []Observation{
				{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-1 * time.Minute)},
				{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-3 * time.Minute)},
			},
		},
	}

	first, err := det.Detect(context.Background(), in)
	if err != nil {
		t.Fatalf("first Detect: %v", err)
	}
	second, err := det.Detect(context.Background(), in)
	if err != nil {
		t.Fatalf("second Detect: %v", err)
	}
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected 1 event in each call, got first=%d second=%d", len(first), len(second))
	}
	if first[0].Problem != second[0].Problem {
		t.Errorf("Problem differs:\n  first=%q\n  second=%q", first[0].Problem, second[0].Problem)
	}
	if first[0].Kind != second[0].Kind {
		t.Errorf("Kind differs: first=%q second=%q", first[0].Kind, second[0].Kind)
	}
	if first[0].Tool != second[0].Tool {
		t.Errorf("Tool differs: first=%q second=%q", first[0].Tool, second[0].Tool)
	}
}

// TestRetryDetector_WrongPayloadType — when the orchestrator passes
// the wrong payload shape the detector returns a typed error rather
// than panicking. Critical for the CLI (slice 5.4) which marshals
// payloads from JSON.
func TestRetryDetector_WrongPayloadType(t *testing.T) {
	t.Parallel()
	det := retryDetector(t)

	in := DetectInput{
		Source:      "test",
		ProjectRoot: "/tmp/proj",
		Timestamp:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		Payload:     "not a RetryPayload",
	}

	got, err := det.Detect(context.Background(), in)
	if err == nil {
		t.Fatalf("Detect accepted wrong payload type; got events=%v", got)
	}
	if !strings.Contains(err.Error(), "RetryPayload") {
		t.Errorf("error = %q, want mention of RetryPayload", err.Error())
	}
}

// TestRetryDetector_InvalidConstruction — the constructor rejects
// misconfiguration so the orchestrator fails fast at startup rather
// than at the first observation.
func TestRetryDetector_InvalidConstruction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		threshold int
		window    time.Duration
	}{
		{"threshold zero", 0, 5 * time.Minute},
		{"threshold one", 1, 5 * time.Minute},
		{"threshold negative", -1, 5 * time.Minute},
		{"window zero", 3, 0},
		{"window negative", 3, -time.Second},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewRetryDetector(tc.threshold, tc.window)
			if err == nil {
				t.Errorf("NewRetryDetector(%d, %s) accepted, want error", tc.threshold, tc.window)
			}
		})
	}
}

// TestRetryDetector_EventShape — the emitted CandidateEvent carries
// every field the downstream ingestion service needs for clustering
// and lexical retrieval: occurrences count, window in seconds,
// threshold, fingerprint, tool, retrieval terms.
func TestRetryDetector_EventShape(t *testing.T) {
	t.Parallel()
	det := retryDetector(t)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	in := DetectInput{
		Source:      "test",
		ProjectRoot: "/tmp/proj",
		Timestamp:   now,
		Payload: RetryPayload{
			Current: Observation{
				Fingerprint: "fp-cmd-fail-001",
				Tool:        "npm test",
				Result:      "fail",
				Timestamp:   now,
			},
			Recent: []Observation{
				{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-1 * time.Minute)},
				{Fingerprint: "fp-cmd-fail-001", Tool: "npm test", Result: "fail", Timestamp: now.Add(-3 * time.Minute)},
			},
		},
	}

	got, err := det.Detect(context.Background(), in)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	ev := got[0]

	if ev.Kind != "retry" {
		t.Errorf("Kind = %q, want %q", ev.Kind, "retry")
	}
	if ev.Tool != "npm test" {
		t.Errorf("Tool = %q, want %q", ev.Tool, "npm test")
	}
	if ev.Result != "corrected" {
		t.Errorf("Result = %q, want %q", ev.Result, "corrected")
	}
	if occ, ok := ev.Extra["occurrences"].(int); !ok || occ != 3 {
		t.Errorf("Extra[occurrences] = %v (%T), want int 3", ev.Extra["occurrences"], ev.Extra["occurrences"])
	}
	if ws, ok := ev.Extra["window_seconds"].(int); !ok || ws != 300 {
		t.Errorf("Extra[window_seconds] = %v (%T), want int 300", ev.Extra["window_seconds"], ev.Extra["window_seconds"])
	}
	if thr, ok := ev.Extra["threshold"].(int); !ok || thr != 3 {
		t.Errorf("Extra[threshold] = %v (%T), want int 3", ev.Extra["threshold"], ev.Extra["threshold"])
	}
	if fp, ok := ev.Extra["fingerprint"].(string); !ok || fp != "fp-cmd-fail-001" {
		t.Errorf("Extra[fingerprint] = %v (%T), want string fp-cmd-fail-001", ev.Extra["fingerprint"], ev.Extra["fingerprint"])
	}
	if len(ev.RetrievalTerms) == 0 {
		t.Error("RetrievalTerms is empty, want at least one term")
	}
}
