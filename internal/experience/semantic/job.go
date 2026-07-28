// Package semantic — the runtime Job contract. See types.go for the
// package-level doc.

package semantic

import (
	"context"
	"database/sql"
	"time"
)

// JobFunc is the runtime contract every adapter implements. The body
// MUST respect ctx cancellation/timeout and MUST NOT emit job_*
// audit events directly (those are owned by jobs.Service.RunOne —
// the Hito 10 SEVERE trace-leak invariant
// (hito10-codex-review-fixes.md) forbids the per-adapter code from
// touching the audit sink with any payload that could carry transcript
// content).
//
// The function returns a Result describing the run's outcome. A nil
// error AND an empty Result.ErrorCode indicates a successful run; any
// non-nil error or non-empty ErrorCode indicates a failed run and the
// audit hook will emit job_failed instead of job_succeeded.
//
// TODO(hito11-pr14): narrow Deps.SourceInstance once the
// experience.ExperienceSource binding is finalised; today it is typed
// as `any` to keep the PR #13 slice free of any per-adapter import.
type JobFunc func(ctx context.Context, deps Deps) (Result, error)

// Deps is the immutable bundle every JobFunc depends on. The fields
// are populated once at RunOne entry; the engine never mutates them
// after that.
//
// TODO(hito11-pr14): narrow SourceInstance to a typed
// experience.ExperienceSource binding once the contract is finalised;
// today it is intentionally `any` so PR #13 ships without taking a
// hard import on any per-adapter package.
type Deps struct {
	// DB is the *sql.DB handle the JobFunc body opens inner
	// transactions on (the engine owns the audit-tx, not this handle).
	DB *sql.DB

	// Now is the package-level injectable clock. The Codex Job()
	// accessor exposes jobNow for exactly this reason
	// (internal/experience/codex/jobs.go).
	Now func() time.Time

	// SourceInstance is the per-adapter runtime binding. PR #13 does
	// not type it; PR #14 narrows it.
	// TODO(hito11-pr14): narrow type.
	SourceInstance any
}

// Result is the typed outcome of one JobFunc invocation. The fields
// here are the union of what the three static ingest jobs need; future
// jobs (Promote/Rebuild/Cleanup) may add their own counters via a
// follow-up PR.
//
// TODO(hito11-pr14): narrow Envelopes from `any` to
// experience.ExperienceEnvelope once the contract is finalised.
type Result struct {
	// Envelopes is the slice of ingested envelopes produced by this
	// run. PR #13 types it as `any` to keep the contract decoupled from
	// the per-adapter ScanResult shapes.
	// TODO(hito11-pr14): narrow type.
	Envelopes []any

	// SkippedMalformed is the number of source records that failed the
	// adapter's well-formedness check.
	SkippedMalformed int

	// SkippedIncomplete is the number of source records that were
	// well-formed but lacked required fields (the per-adapter
	// "incomplete" bucket).
	SkippedIncomplete int

	// NextCursor is the checkpoint the JobFunc body computed; the
	// engine persists it via the existing IngestionCursor path on the
	// next iteration of RunDue.
	NextCursor string

	// ErrorCode is a structured failure code ("" on success). The
	// audit hook writes it into the job_failed event's payload.
	ErrorCode string
}

// Job binds a registry entry to its runtime closure. The static
// fields (Name, Source, Intent, Scope, RiskClass, Enabled,
// DefaultIntervalSec, DefaultMaxRetries) are populated by the per-
// adapter Job() accessor; the Func field is the runtime closure
// wired by the same accessor.
type Job struct {
	// Name is the registry row name (e.g. experience_ingest:codex).
	Name string

	// Source identifies which adapter owns this job.
	Source string

	// Intent classifies the job's purpose (see JobIntent).
	Intent JobIntent

	// Scope is the boundary the job runs against (see JobScope).
	Scope JobScope

	// RiskClass is the operator-visible hazard class (see
	// JobRiskClass).
	RiskClass JobRiskClass

	// Enabled mirrors job_registry.enabled; the Hito 11 static rows
	// ship disabled so Hito 3 (--watch, PR #10) is the single switch
	// that flips it on.
	Enabled bool

	// DefaultIntervalSec is the registry's default interval.
	DefaultIntervalSec int

	// DefaultMaxRetries is the registry's default retry budget.
	DefaultMaxRetries int

	// Func is the runtime closure. A nil Func is treated by
	// jobs.Service.RunOne as a programming error (it returns early
	// with a typed "missing func" error).
	Func JobFunc
}
