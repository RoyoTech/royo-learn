// Hito 11 (PR #14) — RunOne types and helpers for the symmetric job
// engine. The audit hook (RunOne) and the run-log table
// (job_run_log) live in this file so the engine layer can be reviewed
// in one slice without crossing the repository surface.
//
// This file holds ONLY the typed outcome and the run-log row struct.
// The RunOne method itself lives in service.go because it is a method
// on *Service. The audit helper recordJobAuditTx lives next to
// RunOne in service.go for the same reason (it carries the *Service
// receiver to reuse the Now clock and the audit sink).

package jobs

import "time"

// JobRunOutcome is the structured result of one RunOne invocation.
// The CLI dispatcher (cmd/royo-learn/experience.go, PR #15) renders
// the outcome as JSON; the audit sink consumes the four event
// transitions independently.
type JobRunOutcome struct {
	// RunID is the deterministic UUID minted at RunOne entry. The
	// same RunID is shared by all four audit_events rows emitted
	// inside the same RunOne invocation and by the single
	// job_run_log row.
	RunID string

	// JobName is the registry row name (e.g. experience_ingest:codex).
	JobName string

	// State is the final run-log state (pending, running, succeeded,
	// failed, lease_held).
	State string

	// Attempt is the attempt counter stamped into job_run_log
	// (0 for the pending stub, 1 for the running + terminal events).
	Attempt int

	// ErrorCode is empty on success; populated from
	// semantic.Result.ErrorCode or the JobFunc error.
	ErrorCode string

	// ErrorMessage is empty on success; populated from
	// semantic.Result.ErrorMessage or the JobFunc error message.
	ErrorMessage string

	// StartedAt is the timestamp stamped into job_run_log.started_at.
	StartedAt time.Time

	// FinishedAt is the timestamp stamped into job_run_log.finished_at;
	// nil if the run did not reach the terminal event.
	FinishedAt *time.Time
}

// jobRunLog is the internal row shape used by the repository's
// RecordRunLog / UpdateRunLogAttempt / FinishRunLog methods. The
// fields map one-to-one to the job_run_log table columns introduced
// by migration 008_job_semantics.sql (PR #13).
//
// The struct is unexported because callers only ever populate it
// from inside the engine; the repository methods accept it
// positionally to keep the SQL layer free of stringly-typed maps.
type jobRunLog struct {
	RunID        string
	JobName      string
	State        string
	StartedAt    time.Time
	FinishedAt   *time.Time
	ErrorCode    string
	ErrorMessage string
	Attempt      int
}
