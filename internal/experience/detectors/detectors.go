// Detector contract and shared types for Hito 5 slice 5.0.
//
// This file ships the surface only. Per-detector implementations
// (correction, command_outcome, tests, retry, tool_limit) land in
// slices 5.1, 5.2 and 5.3 and live in their own files alongside
// per-kind tests.

package detectors

import (
	"context"
	"time"
)

// Detector is the contract every experience detector must satisfy.
//
// A detector is a pure function over a DetectInput. It is invoked by
// the ingestion orchestrator once per observation produced by the
// upstream adapter. The contract is intentionally small so the
// per-kind logic (which differs widely) can compose without leaking
// shape into the interface.
//
// Detectors MUST be deterministic. Same (input, config) MUST yield
// the same output. Same input across versions MAY yield different
// output; that is why Version() exists and is consumed downstream.
type Detector interface {
	// Kind returns the canonical event kind this detector produces.
	// It MUST match the detector's filename and one of the enums
	// documented in docs/23-PATTERN-MINING.md §2.1, lower_snake_case,
	// single token, no whitespace.
	Kind() string

	// Version returns the detector's semantic version. Bumping the
	// version resets fingerprint compatibility so downstream
	// clustering re-evaluates events produced by this detector.
	// Per docs/22-ADAPTER-CONTRACT.md §7, the version MUST be
	// non-empty.
	Version() string

	// Detect evaluates the input and returns zero or more candidate
	// events. A nil error with len(events) == 0 is the expected
	// happy-path when nothing relevant is present.
	//
	// Detect MUST NOT panic, MUST NOT mutate the input, MUST NOT
	// perform I/O outside the package's read-only access surface
	// (no network, no shell, no daemon). Errors are reserved for
	// configuration and contract violations, not for "no event
	// found": that case is reported as an empty slice.
	Detect(ctx context.Context, in DetectInput) ([]CandidateEvent, error)
}

// DetectInput is the minimal information every detector needs.
//
// Detectors that need richer context type-assert Payload to a
// detector-specific struct agreed with the adapter contract in
// docs/22-ADAPTER-CONTRACT.md. The ingestion orchestrator constructs
// the DetectInput and is responsible for filling the four fields
// below; detectors MUST NOT assume defaults.
type DetectInput struct {
	// Source identifies the platform that produced the observation
	// (e.g. "opencode", "claude_code", "codex"). Used by detectors
	// that tune behavior per source.
	Source string

	// ProjectRoot is the canonical root inside which all paths must
	// resolve. Detectors MUST reject paths outside this root with
	// a typed error; the orchestrator supplies the canonical form
	// (see internal/project.Canonicalize).
	ProjectRoot string

	// Payload is the raw observation. The shape is agreed with the
	// detector's Kind; see docs/23-PATTERN-MINING.md §2.1.
	Payload any

	// Timestamp is the wall-clock time at which the underlying
	// platform emitted the event, in UTC. Detectors MUST NOT use
	// time.Now() in lieu of this field; the value is part of the
	// deterministic input.
	Timestamp time.Time
}

// CandidateEvent is the structured event a detector emits BEFORE
// ingestion normalizes and persists it. The ingestion layer is
// responsible for schema validation, redaction, fingerprint
// computation, idempotency and persistence via capture.Service.
// Detectors MUST NOT persist directly.
//
// All string fields are normalized outputs: no secrets, no
// project-root-external paths, no UUIDs, no timestamps. Detection
// happens upstream; persistence ownership is downstream.
type CandidateEvent struct {
	// Kind matches the emitting detector's Kind(). Required.
	Kind string

	// Problem is a normalized, redacted description of the
	// observation. Free of secrets, paths outside the project root,
	// timestamps and UUIDs. Used for fingerprinting and retrieval.
	Problem string

	// Tool is the primary tool/command without volatile values
	// (arguments, ports, file names that are not canonical). Empty
	// when no tool is involved.
	Tool string

	// Result is the outcome kind. One of: "fail", "success",
	// "corrected", "fallback". Detectors that need a richer
	// vocabulary MUST keep the four canonical values and put
	// supplementary data in Extra.
	Result string

	// Paths are project-root-relative, normalized paths touched by
	// the observation. Empty when no file is involved. Detectors
	// MUST reject paths outside ProjectRoot.
	Paths []string

	// RetrievalTerms are normalized terms for downstream lexical
	// retrieval. See docs/23-PATTERN-MINING.md §3. Stable across
	// runs: no timestamps, no UUIDs, no per-run tokens.
	RetrievalTerms []string

	// Extra carries detector-specific structured data. The shape is
	// agreed per Kind. The value MUST be JSON-marshalable; no
	// functions, no channels, no unsafe pointers. Detectors MUST
	// keep Extra small: candidate events are reviewed by humans
	// and too much data defeats the precision-over-recall rule.
	Extra map[string]any
}
