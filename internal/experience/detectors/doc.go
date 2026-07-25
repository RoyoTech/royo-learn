// Package detectors implements the deterministic experience detector
// surface for Hito 5 of the discovery layer.
//
// A detector inspects a raw operational observation (a tool error,
// a test failure, a retry, a config change, ...) and, when it
// recognizes a candidate pattern, emits one or more CandidateEvent
// values. The ingestion layer
// (internal/experience.Service) is responsible for schema validation,
// redaction, fingerprint computation, idempotency and persistence via
// capture.Service. Detectors MUST NOT persist directly.
//
// Scope (per docs/23-PATTERN-MINING.md §2.1 and the Hito 5 gates in
// docs/26-IMPLEMENTATION-ROADMAP.md §3 PR #4):
//
//   - Deterministic: same input + same version → same output.
//   - Precision over recall: zero events in routine chat is the bar
//     that matters. Speculative matches are not emitted.
//   - Five canonical kinds for v1: correction, command_outcome,
//     tests, retry, tool_limit.
//   - No shell, no network, no LLM provider in v1. The detector
//     inspects structured payloads only.
//
// Boundaries:
//
//   - Detectors do not read the upstream platform database. Adapters
//     (internal/experience/opencode, ...) own the read-only scan and
//     forward a typed payload through DetectInput.Payload.
//   - Detectors do not persist. Promotion of a CandidateEvent into
//     the durable ExperienceEvent stream is the job of the ingestion
//     service, which also re-validates the detector contract.
//   - Detectors do not publish or mutate a Learning. Per
//     docs/24-EXPERIENCE-THREAT-MODEL.md, the only bridge between
//     observed experience and approved knowledge is promotion via
//     capture.Service in a separate package.
//
// Package version: detectors/v1. See docs/22-ADAPTER-CONTRACT.md §7
// for the rule that bumps this version when the contract changes.
package detectors
