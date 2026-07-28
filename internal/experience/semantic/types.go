// Package semantic owns the Job/JobFunc/JobResult contract and the
// taxonomy enum types (JobIntent, JobScope, JobRiskClass) that
// describe every job in the symmetric job engine introduced by Hito 11.
//
// The package is the single source of truth for the audit-event-name
// constants (job_pending, job_running, job_succeeded, job_failed)
// emitted by jobs.Service.RunOne, and the payload allow-list that
// enforces the Hito 10 SEVERE trace-leak invariant (no transcript text
// may appear in any audit event payload — see
// docs/24-EXPERIENCE-THREAT-MODEL.md §6 and
// openspec/changes/hito10-codex for the prior fix).
//
// The package has no dependency on internal/experience/jobs (the lease
// engine) or on any per-adapter package: it is a leaf package that the
// engine layer (jobs) and the per-adapter layers (opencode, claudecode,
// codex) both depend on.
package semantic

// JobIntent classifies the high-level purpose of a job. The system uses
// the value at audit-hook time and at operator inspection time ("which
// jobs are ingests?").
//
// JobIntentIngest is the only value used by the Hito 11 static jobs.
// The Promote/Rebuild/Cleanup constants are reserved for future hits
// and are exported so that later work can extend the registry without
// re-opening this package.
type JobIntent string

const (
	JobIntentIngest  JobIntent = "ingest"
	JobIntentPromote JobIntent = "promote"
	JobIntentRebuild JobIntent = "rebuild"
	JobIntentCleanup JobIntent = "cleanup"
)

// IsValid reports whether the intent value is one of the documented
// JobIntent constants. The check is used by the upsert path (Hito 11
// PR #14) to reject unknown values at write time.
func (i JobIntent) IsValid() bool {
	return IsValidIntent(i)
}

// IsValidIntent is the package-level validator for JobIntent values.
func IsValidIntent(i JobIntent) bool {
	switch i {
	case JobIntentIngest, JobIntentPromote, JobIntentRebuild, JobIntentCleanup:
		return true
	default:
		return false
	}
}

// JobScope is the boundary the job runs against. Project-scoped jobs
// operate on a single project's input; global jobs (future) operate
// across all registered projects.
type JobScope string

const (
	JobScopeProject JobScope = "project"
	JobScopeGlobal  JobScope = "global"
)

// IsValid reports whether the scope value is one of the documented
// JobScope constants.
func (s JobScope) IsValid() bool {
	return IsValidScope(s)
}

// IsValidScope is the package-level validator for JobScope values.
func IsValidScope(s JobScope) bool {
	switch s {
	case JobScopeProject, JobScopeGlobal:
		return true
	default:
		return false
	}
}

// JobRiskClass is the operator-visible hazard class of a job. The
// Hito 11 static ingest jobs all carry JobRiskClassLow. The Medium/High
// constants are reserved for future jobs (e.g. cleanup or promote)
// and are exported so that a later change can add higher-risk jobs
// without re-opening this package.
type JobRiskClass string

const (
	JobRiskClassLow    JobRiskClass = "low"
	JobRiskClassMedium JobRiskClass = "medium"
	JobRiskClassHigh   JobRiskClass = "high"
)

// IsValid reports whether the risk class value is one of the
// documented JobRiskClass constants.
func (r JobRiskClass) IsValid() bool {
	return IsValidRiskClass(r)
}

// IsValidRiskClass is the package-level validator for JobRiskClass
// values.
func IsValidRiskClass(r JobRiskClass) bool {
	switch r {
	case JobRiskClassLow, JobRiskClassMedium, JobRiskClassHigh:
		return true
	default:
		return false
	}
}
