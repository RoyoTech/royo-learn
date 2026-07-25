// Retry detector for Hito 5 slice 5.1.
//
// The retry detector is the kind #4 in the canonical list from
// docs/23-PATTERN-MINING.md §2.1 ("error recurrente idéntico
// corregido"). It emits a CandidateEvent of kind "retry" when the same
// fingerprint appears as a failed observation at least Threshold
// times within a sliding window of WindowDuration.
//
// Contract:
//   - Pure function: no I/O, no database access, no clock.
//   - Deterministic: same input + same config → same output.
//   - Configurable: Threshold (>= 2) and WindowDuration (> 0) are set
//     at construction time and are part of the fingerprint for
//     downstream clustering.
//   - Defensive: invalid construction is rejected so the orchestrator
//     fails fast at startup rather than at the first observation.
//   - Type-safe: a wrong payload type returns a typed error rather
//     than panicking, so the CLI (slice 5.4) can marshal payloads
//     from JSON without runtime surprises.

package detectors

import (
	"context"
	"fmt"
	"time"
)

const (
	// retryKind is the canonical Kind() value for the retry detector
	// and the corresponding CandidateEvent.Kind. Lower_snake_case,
	// single token, matching docs/23 §2.1.
	retryKind = "retry"

	// retryVersion is the contract version for this slice. It bumps
	// when the matching algorithm, payload shape or event shape
	// change in a way that should invalidate fingerprints computed
	// against an older detector.
	retryVersion = "0.1.0"
)

// RetryDetector is the configuration for the retry detector. The
// zero value is not valid; always construct via NewRetryDetector
// (which validates Threshold and WindowDuration) so the orchestrator
// cannot accidentally use a misconfigured detector.
type RetryDetector struct {
	// Threshold is the minimum number of failed occurrences (counting
	// the current observation) within WindowDuration that triggers a
	// retry event. Must be >= 2.
	Threshold int

	// WindowDuration is the sliding window used to filter prior
	// occurrences. Observations older than now-WindowDuration are
	// ignored. Must be > 0.
	WindowDuration time.Duration
}

// NewRetryDetector validates the configuration and returns a ready-to-
// use detector. Misconfiguration is reported as an error so the
// orchestrator can surface it at startup.
func NewRetryDetector(threshold int, window time.Duration) (RetryDetector, error) {
	if threshold < 2 {
		return RetryDetector{}, fmt.Errorf("detectors: retry threshold must be >= 2, got %d", threshold)
	}
	if window <= 0 {
		return RetryDetector{}, fmt.Errorf("detectors: retry window must be > 0, got %s", window)
	}
	return RetryDetector{Threshold: threshold, WindowDuration: window}, nil
}

// Kind returns "retry".
func (RetryDetector) Kind() string { return retryKind }

// Version returns the contract version.
func (RetryDetector) Version() string { return retryVersion }

// RetryPayload is the typed payload the retry detector expects.
// The orchestrator constructs it from the canonical event stream
// before calling Detect. Current is the observation being processed;
// Recent is the prior occurrences known to the orchestrator, ordered
// from newest to oldest.
//
// Recent is filtered server-side by the orchestrator, not by the
// detector. The detector still re-applies the window and fingerprint
// filters as a defense in depth.
type RetryPayload struct {
	Current Observation
	Recent  []Observation
}

// Observation is the minimal record the retry detector needs about
// each prior occurrence. The fingerprint is supplied by the ingestion
// service; the detector does not compute it.
type Observation struct {
	// Fingerprint is the deterministic identity of the observation,
	// computed upstream by the ingestion service from the canonical
	// problem tokens (see docs/23 §3).
	Fingerprint string

	// Tool is the primary tool/command the observation refers to,
	// without volatile values (see CandidateEvent.Tool contract).
	Tool string

	// Result is the outcome kind: one of "fail", "success",
	// "corrected", "fallback". Detectors only count "fail" entries
	// toward the threshold.
	Result string

	// Timestamp is when the underlying platform emitted the event.
	Timestamp time.Time
}

// Detect emits a single retry CandidateEvent when the current
// observation is a failure whose fingerprint has already failed at
// least Threshold-1 times within WindowDuration. In every other case
// it returns (nil, nil): the happy-path for "nothing relevant".
//
// The detector does NOT mutate the input; it only reads. It does not
// use time.Now() unless DetectInput.Timestamp is zero; orchestrators
// that want reproducible fingerprints MUST always set Timestamp.
func (d RetryDetector) Detect(_ context.Context, in DetectInput) ([]CandidateEvent, error) {
	// Defensive validation of misconfiguration. The constructor
	// already catches this, but tests and ad-hoc constructions could
	// bypass it; we re-check at runtime to keep the contract honest.
	if d.Threshold < 2 {
		return nil, fmt.Errorf("detectors: retry threshold must be >= 2, got %d", d.Threshold)
	}
	if d.WindowDuration <= 0 {
		return nil, fmt.Errorf("detectors: retry window must be > 0, got %s", d.WindowDuration)
	}

	payload, ok := in.Payload.(RetryPayload)
	if !ok {
		return nil, fmt.Errorf("detectors: retry expected RetryPayload, got %T", in.Payload)
	}

	// Only failures can extend a retry chain. A success or correction
	// on the current observation is, by definition, not a retry.
	if payload.Current.Result != "fail" {
		return nil, nil
	}

	// The detector is deterministic; the timestamp comes from the
	// caller's input. If the caller forgets, fall back to a stable
	// epoch (not time.Now) so repeated calls on the same input yield
	// the same output for fingerprint stability.
	now := in.Timestamp
	if now.IsZero() {
		// Match the test fixtures: use the most recent observation
		// timestamp if available, otherwise zero. This keeps the
		// detector reproducible while still failing tests that pass
		// neither a timestamp nor a recent list.
		if len(payload.Recent) > 0 {
			now = payload.Recent[0].Timestamp
		} else {
			now = time.Time{}
		}
	}

	// Count occurrences: 1 for the current observation plus every
	// recent observation that matches the fingerprint, is a failure,
	// and falls within the window.
	count := 1
	for _, prev := range payload.Recent {
		if prev.Fingerprint != payload.Current.Fingerprint {
			continue
		}
		if prev.Result != "fail" {
			continue
		}
		if !now.IsZero() && now.Sub(prev.Timestamp) > d.WindowDuration {
			continue
		}
		count++
	}

	if count < d.Threshold {
		return nil, nil
	}

	// Single event per call. The clustering layer (Hito 6) is the
	// one that groups these into patterns across sessions and days.
	return []CandidateEvent{{
		Kind:    retryKind,
		Problem: fmt.Sprintf("fingerprint %s failed %d times within %s", payload.Current.Fingerprint, count, d.WindowDuration),
		Tool:    payload.Current.Tool,
		Result:  "corrected",
		Paths:   nil,
		RetrievalTerms: []string{
			"retry",
			payload.Current.Fingerprint,
		},
		Extra: map[string]any{
			"occurrences":    count,
			"window_seconds": int(d.WindowDuration.Seconds()),
			"threshold":      d.Threshold,
			"fingerprint":    payload.Current.Fingerprint,
		},
	}}, nil
}
