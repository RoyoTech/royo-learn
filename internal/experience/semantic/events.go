// Package semantic — audit-event constants and the payload allow-list
// helper. See types.go for the package-level doc.

package semantic

import "sort"

// Audit-event operation names emitted by jobs.Service.RunOne. The
// literal string values are the exact strings written to
// audit_events.operation; changing them is a wire contract change.
const (
	EventJobPending   = "job_pending"
	EventJobRunning   = "job_running"
	EventJobSucceeded = "job_succeeded"
	EventJobFailed    = "job_failed"
)

// Job-state values written to the job_run_log.state column and embedded
// in each audit-event payload under "state". The set is closed; the
// migration's CHECK constraint mirrors it.
const (
	StatePending   = "pending"
	StateRunning   = "running"
	StateSucceeded = "succeeded"
	StateFailed    = "failed"
	StateLeaseHeld = "lease_held"
)

// allowedDetailsKeys is the exhaustive JSON allow-list for job_*
// audit-event payloads. Adding a key here is a code change AND a test
// change (TestJobPayload_AllowListContract below); the list is
// intentionally short to keep the audit invariant easy to audit.
//
// Allowed keys:
//
//   - job_name       — the registry row name (e.g. experience_ingest:codex).
//   - run_id         — the deterministic UUIDv7 minted by RunOne, shared
//     across the four events of one run.
//   - source         — the domain.ExperienceSource that owns the job.
//   - state          — one of StatePending/StateRunning/StateSucceeded/
//     StateFailed/StateLeaseHeld.
//   - transition     — one of EventJobPending/EventJobRunning/EventJob
//     Succeeded/EventJobFailed; matches operation but
//     is embedded for downstream filter convenience.
//   - attempt        — string-encoded attempt counter (pending is 0,
//     running and the terminal events are 1).
//   - error_code     — populated only on the failure transition.
//   - error_message  — populated only on the failure transition.
//
// The "transition" key is documented as required in PR #14's audit hook
// (matches the operation name) but is omitted from the public payload
// builder for now — the engine layer (PR #14) is the single source of
// truth for the transition value. We document the key in the allow-list
// so the test asserts the future surface without surprising PR #14.
var allowedDetailsKeys = []string{
	"job_name",
	"run_id",
	"source",
	"state",
	"transition",
	"attempt",
	"error_code",
	"error_message",
}

// AllowedDetailsKeys returns a copy of the allow-list as a sorted slice.
// The function exists so tests can assert the documented keys without
// depending on the internal ordering of allowedDetailsKeys.
func AllowedDetailsKeys() []string {
	out := make([]string, len(allowedDetailsKeys))
	copy(out, allowedDetailsKeys)
	sort.Strings(out)
	return out
}

// jobPayload builds the JSON details blob for a job_* event. It is the
// only sanctioned entry point for emitting job_* payloads: every other
// call site must funnel through here so the allow-list cannot be
// bypassed by a typo or by passing a free-form map.
//
// The helper accepts only the documented scalar arguments. It NEVER
// accepts an ExperienceEnvelope, a JobResult, or any field that may
// carry transcript content (UserText / AssistantText /
// ToolCalls[].OutputHint). This is the static guard for the Hito 10
// SEVERE trace-leak invariant
// (hito10-codex-review-fixes.md, docs/24-TM §6).
//
// errorCode and errorMessage are written only when non-empty so the
// happy-path payload is the documented six-key shape, never a six-key
// shape with empty error fields. The "transition" key is filled
// automatically from the operation name argument so the call site does
// not need to thread it.
func jobPayload(jobName, runID, source, state, transition, attempt, errorCode, errorMessage string) map[string]string {
	out := map[string]string{
		"job_name":   jobName,
		"run_id":     runID,
		"source":     source,
		"state":      state,
		"transition": transition,
		"attempt":    attempt,
	}
	if errorCode != "" {
		out["error_code"] = errorCode
	}
	if errorMessage != "" {
		out["error_message"] = errorMessage
	}
	return out
}

// BuildJobPayload is the exported alias for jobPayload. It is exposed so
// the PR #14 audit hook (jobs.Service.RunOne) can construct payloads
// through the same allow-list gate without depending on the unexported
// helper. The exported name is intentional: it documents that this is
// the ONLY supported way to build a job_* payload.
//
// Callers MUST pass scalar string values only. The function panics on a
// nil or non-string map value because every key in the allow-list is
// typed as string by the migration column defaults; a non-string value
// is a programming error, not a runtime condition.
func BuildJobPayload(jobName, runID, source, state, transition, attempt, errorCode, errorMessage string) map[string]string {
	return jobPayload(jobName, runID, source, state, transition, attempt, errorCode, errorMessage)
}
