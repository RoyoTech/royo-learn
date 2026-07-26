package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// Repository is the persistence layer for job state and registry.
// It uses the database/sql handle directly, consistent with the
// pattern in internal/experience/trace/repository.go.
type Repository struct {
	db *sql.DB
}

// NewRepository wires a Repository with the given database handle.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// DB returns the underlying database handle for use by the Service
// when it needs direct queries.
func (r *Repository) DB() *sql.DB {
	return r.db
}

// --- Job State --------------------------------------------------------

// UpsertJobState inserts or updates the runtime row for a job. It is
// idempotent on (project_id, job_name).
func (r *Repository) UpsertJobState(ctx context.Context, state JobState) error {
	if r.db == nil {
		return fmt.Errorf("jobs: database is nil")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO job_state (project_id, job_name, status, input_digest,
			lease_owner, lease_expires_at, retry_count, max_retries,
			last_started_at, last_success_at, last_failed_at,
			last_error_code, last_error, metrics_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, job_name) DO UPDATE SET
			status = excluded.status,
			input_digest = excluded.input_digest,
			lease_owner = excluded.lease_owner,
			lease_expires_at = excluded.lease_expires_at,
			retry_count = excluded.retry_count,
			max_retries = excluded.max_retries,
			last_started_at = excluded.last_started_at,
			last_success_at = excluded.last_success_at,
			last_failed_at = excluded.last_failed_at,
			last_error_code = excluded.last_error_code,
			last_error = excluded.last_error,
			metrics_json = excluded.metrics_json,
			updated_at = excluded.updated_at`,
		string(state.ProjectID), state.JobName, string(state.Status),
		state.InputDigest, state.LeaseOwner, nullableTime(state.LeaseExpiresAt),
		state.RetryCount, state.MaxRetries,
		nullableTime(state.LastStartedAt), nullableTime(state.LastSuccessAt),
		nullableTime(state.LastFailedAt),
		state.LastErrorCode, state.LastError, state.MetricsJSON,
		now, now,
	)
	if err != nil {
		return fmt.Errorf("jobs: upsert state: %w", err)
	}
	return nil
}

// GetJobState retrieves a single job state row.
func (r *Repository) GetJobState(ctx context.Context, projectID domain.ProjectID, jobName string) (*JobState, error) {
	if r.db == nil {
		return nil, fmt.Errorf("jobs: database is nil")
	}
	var (
		leaseExpiresAt, lastStartedAt, lastSuccessAt, lastFailedAt sql.NullString
		state                                                      JobState
		createdAt, updatedAt                                       string
	)
	row := r.db.QueryRowContext(ctx,
		`SELECT project_id, job_name, status, input_digest,
			lease_owner, lease_expires_at, retry_count, max_retries,
			last_started_at, last_success_at, last_failed_at,
			last_error_code, last_error, metrics_json, created_at, updated_at
		 FROM job_state WHERE project_id = ? AND job_name = ?`,
		string(projectID), jobName,
	)
	err := row.Scan(
		&state.ProjectID, &state.JobName, &state.Status, &state.InputDigest,
		&state.LeaseOwner, &leaseExpiresAt, &state.RetryCount, &state.MaxRetries,
		&lastStartedAt, &lastSuccessAt, &lastFailedAt,
		&state.LastErrorCode, &state.LastError, &state.MetricsJSON,
		&createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("jobs: state not found: %s/%s: %w", string(projectID), jobName, ErrJobNotFound)
		}
		return nil, fmt.Errorf("jobs: get state: %w", err)
	}
	state.LeaseExpiresAt = parseNullTime(leaseExpiresAt)
	state.LastStartedAt = parseNullTime(lastStartedAt)
	state.LastSuccessAt = parseNullTime(lastSuccessAt)
	state.LastFailedAt = parseNullTime(lastFailedAt)
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		state.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		state.UpdatedAt = t
	}
	return &state, nil
}

// ListJobStates returns all job state rows for a project.
func (r *Repository) ListJobStates(ctx context.Context, projectID domain.ProjectID) ([]JobStateSummary, error) {
	if r.db == nil {
		return nil, fmt.Errorf("jobs: database is nil")
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT job_name, status, last_success_at, last_failed_at,
			last_error_code, retry_count, lease_owner
		 FROM job_state WHERE project_id = ? ORDER BY job_name`,
		string(projectID),
	)
	if err != nil {
		return nil, fmt.Errorf("jobs: list states: %w", err)
	}
	defer rows.Close()

	var result []JobStateSummary
	for rows.Next() {
		var (
			s                           JobStateSummary
			lastSuccessAt, lastFailedAt sql.NullString
		)
		if err := rows.Scan(
			&s.JobName, &s.Status, &lastSuccessAt, &lastFailedAt,
			&s.LastErrorCode, &s.RetryCount, &s.LeaseOwner,
		); err != nil {
			return nil, fmt.Errorf("jobs: scan state: %w", err)
		}
		s.LastSuccessAt = parseNullTime(lastSuccessAt)
		s.LastFailedAt = parseNullTime(lastFailedAt)
		result = append(result, s)
	}
	return result, rows.Err()
}

// --- Job Registry -----------------------------------------------------

// UpsertRegistryEntry inserts or updates a static job registration.
func (r *Repository) UpsertRegistryEntry(ctx context.Context, entry JobRegistryEntry) error {
	if r.db == nil {
		return fmt.Errorf("jobs: database is nil")
	}
	enabled := 0
	if entry.Enabled {
		enabled = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO job_registry (job_name, description, default_interval_sec,
			default_max_retries, enabled, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(job_name) DO UPDATE SET
			description = excluded.description,
			default_interval_sec = excluded.default_interval_sec,
			default_max_retries = excluded.default_max_retries,
			enabled = excluded.enabled`,
		entry.JobName, entry.Description, entry.DefaultIntervalSec,
		entry.DefaultMaxRetries, enabled, now,
	)
	if err != nil {
		return fmt.Errorf("jobs: upsert registry: %w", err)
	}
	return nil
}

// GetRegistryEntry retrieves a single registry entry.
func (r *Repository) GetRegistryEntry(ctx context.Context, jobName string) (*JobRegistryEntry, error) {
	if r.db == nil {
		return nil, fmt.Errorf("jobs: database is nil")
	}
	var (
		entry     JobRegistryEntry
		enabled   int
		createdAt string
	)
	row := r.db.QueryRowContext(ctx,
		`SELECT job_name, description, default_interval_sec,
			default_max_retries, enabled, created_at
		 FROM job_registry WHERE job_name = ?`,
		jobName,
	)
	err := row.Scan(
		&entry.JobName, &entry.Description, &entry.DefaultIntervalSec,
		&entry.DefaultMaxRetries, &enabled, &createdAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("jobs: registry entry not found: %s: %w", jobName, ErrJobNotFound)
		}
		return nil, fmt.Errorf("jobs: get registry: %w", err)
	}
	entry.Enabled = enabled != 0
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		entry.CreatedAt = t
	}
	return &entry, nil
}

// ListRegistryEntries returns all registered job entries.
func (r *Repository) ListRegistryEntries(ctx context.Context) ([]JobRegistryEntry, error) {
	if r.db == nil {
		return nil, fmt.Errorf("jobs: database is nil")
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT job_name, description, default_interval_sec,
			default_max_retries, enabled, created_at
		 FROM job_registry ORDER BY job_name`,
	)
	if err != nil {
		return nil, fmt.Errorf("jobs: list registry: %w", err)
	}
	defer rows.Close()

	var result []JobRegistryEntry
	for rows.Next() {
		var (
			entry     JobRegistryEntry
			enabled   int
			createdAt string
		)
		if err := rows.Scan(
			&entry.JobName, &entry.Description, &entry.DefaultIntervalSec,
			&entry.DefaultMaxRetries, &enabled, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("jobs: scan registry: %w", err)
		}
		entry.Enabled = enabled != 0
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			entry.CreatedAt = t
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

// --- Helpers ----------------------------------------------------------

func nullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func parseNullTime(ns sql.NullString) *time.Time {
	if !ns.Valid {
		return nil
	}
	t, err := time.Parse(time.RFC3339, ns.String)
	if err != nil {
		return nil
	}
	return &t
}
