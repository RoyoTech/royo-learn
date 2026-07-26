// Package jobs implements the lease-based job engine for incremental
// work scheduling (Hito 8). Jobs are registered statically at startup
// and executed with SQLite-leased coordination, digest-based idempotency,
// and crash-recovery safety.
//
// Slices 8.0–8.4:
//
//   - 8.0 ships migration 007, domain types, and repository scaffold.
//   - 8.1 adds the job registry and SQLite lease mechanism.
//   - 8.2 adds run-due, retry, and crash recovery.
//   - 8.3 adds CLI and MCP surface with acceptance tests.
//
// See docs/21-EXPERIENCE-DOMAIN.md §8 and
// PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md §Hito 8.

package jobs

import (
	"errors"
	"time"

	"agent-royo-learn/internal/domain"
)

// --- Typed errors ----------------------------------------------------

var (
	// ErrJobNotFound is returned when a job state or registry entry
	// is not found for the given (project_id, job_name).
	ErrJobNotFound = domain.NewNotFoundError(domain.ErrJobNotFound, "job")
)

// ErrorIs exposes package-level typed errors so callers and tests
// can compare with errors.Is without depending on variable identity.
func ErrorIs(err, target error) bool {
	return errors.Is(err, target)
}

// JobStatus is the closed enum governing the lifecycle of a job.
// The CHECK constraint in migration 007 mirrors these values.
type JobStatus string

const (
	JobIdle     JobStatus = "idle"
	JobRunning  JobStatus = "running"
	JobOK       JobStatus = "ok"
	JobDegraded JobStatus = "degraded"
	JobError    JobStatus = "error"
)

// IsTerminal reports whether the status is a final state that the
// runner will not attempt again without an explicit reset.
func (s JobStatus) IsTerminal() bool {
	return s == JobOK || s == JobError
}

// IsActive reports whether the job is currently owned by a lease.
func (s JobStatus) IsActive() bool {
	return s == JobRunning
}

// JobState is the runtime row for one job. It carries the lease,
// digest, status, retry counters, and last-success anchor. SQLite
// is the sole coordination authority.
type JobState struct {
	ProjectID      domain.ProjectID `json:"project_id"`
	JobName        string           `json:"job_name"`
	Status         JobStatus        `json:"status"`
	InputDigest    string           `json:"input_digest"`
	LeaseOwner     string           `json:"lease_owner"`
	LeaseExpiresAt *time.Time       `json:"lease_expires_at,omitempty"`
	RetryCount     int              `json:"retry_count"`
	MaxRetries     int              `json:"max_retries"`
	LastStartedAt  *time.Time       `json:"last_started_at,omitempty"`
	LastSuccessAt  *time.Time       `json:"last_success_at,omitempty"`
	LastFailedAt   *time.Time       `json:"last_failed_at,omitempty"`
	LastErrorCode  string           `json:"last_error_code"`
	LastError      string           `json:"last_error"`
	MetricsJSON    string           `json:"metrics_json"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

// JobRegistryEntry is the static registration row that lists a known
// job name, its default config, and whether it is enabled.
type JobRegistryEntry struct {
	JobName            string    `json:"job_name"`
	Description        string    `json:"description"`
	DefaultIntervalSec int       `json:"default_interval_sec"`
	DefaultMaxRetries  int       `json:"default_max_retries"`
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"created_at"`
}

// LeaseBounds parameterise a lease acquisition. LeaseDuration is how
// long the lease lives before it can be considered expired by another
// process.
type LeaseBounds struct {
	LeaseDuration time.Duration
}

// DefaultLeaseBounds returns the conservative defaults: a 5-minute
// lease window, which is long enough for most jobs but short enough
// that a crash doesn't block progress for long.
func DefaultLeaseBounds() LeaseBounds {
	return LeaseBounds{
		LeaseDuration: 5 * time.Minute,
	}
}

// RunResult is the structured outcome of a single job execution.
type RunResult struct {
	Status   JobStatus     `json:"status"`
	Code     string        `json:"code,omitempty"`
	Message  string        `json:"message,omitempty"`
	Duration time.Duration `json:"duration"`
}

// JobStateSummary is a lightweight view of all job states used by
// the CLI/MCP list commands.
type JobStateSummary struct {
	JobName       string     `json:"job_name"`
	Status        JobStatus  `json:"status"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastFailedAt  *time.Time `json:"last_failed_at,omitempty"`
	LastErrorCode string     `json:"last_error_code"`
	RetryCount    int        `json:"retry_count"`
	LeaseOwner    string     `json:"lease_owner"`
}
