package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/semantic"
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

// UpsertJobStateTx is the tx-aware sibling of UpsertJobState. It
// runs the same INSERT ... ON CONFLICT upsert inside the caller's
// *sql.Tx so the audit-event emission + lease-state update +
// run-log writes commit atomically (Hito 11 PR #14).
func (r *Repository) UpsertJobStateTx(ctx context.Context, tx *sql.Tx, state JobState) error {
	if tx == nil {
		return fmt.Errorf("jobs: upsert state tx: nil tx")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := tx.ExecContext(ctx,
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
		return fmt.Errorf("jobs: upsert state tx: %w", err)
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

// GetJobStateTx is the tx-aware sibling of GetJobState. It runs the
// same SELECT inside the caller's *sql.Tx so the audit-event
// emission + lease-state update + run-log writes commit atomically
// (Hito 11 PR #14).
func (r *Repository) GetJobStateTx(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, jobName string) (*JobState, error) {
	if tx == nil {
		return nil, fmt.Errorf("jobs: get state tx: nil tx")
	}
	var (
		leaseExpiresAt, lastStartedAt, lastSuccessAt, lastFailedAt sql.NullString
		state                                                      JobState
		createdAt, updatedAt                                       string
	)
	row := tx.QueryRowContext(ctx,
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
		return nil, fmt.Errorf("jobs: get state tx: %w", err)
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
//
// Hito 11 (PR #14) extends the upsert path to write the three new
// taxonomy columns (intent, scope, risk_class) introduced by
// migration 008_job_semantics.sql. The three values are normalised
// before the SQL is issued:
//
//   - empty values are replaced with the migration's DEFAULTs
//     (intent=ingest, scope=project, risk_class=low) so the per-adapter
//     constructors that still zero-value the new fields (PR #15
//     rewrites them) keep working at boot.
//   - non-empty values are validated against the semantic enum
//     allow-list; an unknown value is rejected with a typed
//     domain.ErrInvalidArgument error.
//
// IMPORTANT: this method does NOT call JobRegistryEntry.Validate()
// because the per-adapter constructors in codex/jobs.go and
// claudecode/jobs.go (PR #15 scope) still zero-value the new
// fields; calling Validate() here would break the Hito 10 contract
// at boot. Validate() returns to the upsert path in PR #15 once the
// per-adapter constructors are rewritten to populate the fields.
// The normalisation + taxonomy-only validation below is the safe
// interim gate.
//
// TODO(hito11-pr15): call entry.Validate() once the per-adapter
// Job() accessors populate the three fields.
func (r *Repository) UpsertRegistryEntry(ctx context.Context, entry JobRegistryEntry) error {
	if r.db == nil {
		return fmt.Errorf("jobs: database is nil")
	}
	intent, scope, riskClass := normaliseTaxonomy(entry.Intent, entry.Scope, entry.RiskClass)
	if err := validateTaxonomy(intent, scope, riskClass); err != nil {
		return err
	}
	enabled := 0
	if entry.Enabled {
		enabled = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO job_registry (job_name, description, default_interval_sec,
			default_max_retries, intent, scope, risk_class, enabled, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(job_name) DO UPDATE SET
			description = excluded.description,
			default_interval_sec = excluded.default_interval_sec,
			default_max_retries = excluded.default_max_retries,
			intent = excluded.intent,
			scope = excluded.scope,
			risk_class = excluded.risk_class,
			enabled = excluded.enabled`,
		entry.JobName, entry.Description, entry.DefaultIntervalSec,
		entry.DefaultMaxRetries, string(intent), string(scope),
		string(riskClass), enabled, now,
	)
	if err != nil {
		return fmt.Errorf("jobs: upsert registry: %w", err)
	}
	return nil
}

// normaliseTaxonomy replaces any empty taxonomy value with the
// migration's documented DEFAULT so existing callers that still
// zero-value the fields (PR #14) keep working. PR #15 populates the
// fields explicitly; this helper is the safe interim gate.
//
// TODO(hito11-pr15): drop normaliseTaxonomy once every per-adapter
// constructor populates the three fields.
func normaliseTaxonomy(intent semantic.JobIntent, scope semantic.JobScope, riskClass semantic.JobRiskClass) (semantic.JobIntent, semantic.JobScope, semantic.JobRiskClass) {
	if intent == "" {
		intent = semantic.JobIntentIngest
	}
	if scope == "" {
		scope = semantic.JobScopeProject
	}
	if riskClass == "" {
		riskClass = semantic.JobRiskClassLow
	}
	return intent, scope, riskClass
}

// validateTaxonomy rejects unknown taxonomy values before they
// reach the SQLite NOT NULL columns. The helper exists separately
// from JobRegistryEntry.Validate() because the per-adapter
// constructors in PR #15 will populate the fields but the current
// (PR #14) per-adapter constructors still zero-value them; calling
// Validate() at upsert time would break those constructors.
//
// TODO(hito11-pr15): fold validateTaxonomy into JobRegistryEntry.Validate()
// and call entry.Validate() from UpsertRegistryEntry.
func validateTaxonomy(intent semantic.JobIntent, scope semantic.JobScope, riskClass semantic.JobRiskClass) error {
	if !semantic.IsValidIntent(intent) {
		return domain.NewValidationError(domain.ErrInvalidArgument, "jobs: registry entry has invalid Intent")
	}
	if !semantic.IsValidScope(scope) {
		return domain.NewValidationError(domain.ErrInvalidArgument, "jobs: registry entry has invalid Scope")
	}
	if !semantic.IsValidRiskClass(riskClass) {
		return domain.NewValidationError(domain.ErrInvalidArgument, "jobs: registry entry has invalid RiskClass")
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
		intent    string
		scope     string
		riskClass string
	)
	row := r.db.QueryRowContext(ctx,
		`SELECT job_name, description, default_interval_sec,
			default_max_retries, intent, scope, risk_class,
			enabled, created_at
		 FROM job_registry WHERE job_name = ?`,
		jobName,
	)
	err := row.Scan(
		&entry.JobName, &entry.Description, &entry.DefaultIntervalSec,
		&entry.DefaultMaxRetries, &intent, &scope, &riskClass,
		&enabled, &createdAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("jobs: registry entry not found: %s: %w", jobName, ErrJobNotFound)
		}
		return nil, fmt.Errorf("jobs: get registry: %w", err)
	}
	entry.Enabled = enabled != 0
	entry.Intent = semantic.JobIntent(intent)
	entry.Scope = semantic.JobScope(scope)
	entry.RiskClass = semantic.JobRiskClass(riskClass)
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
			default_max_retries, intent, scope, risk_class,
			enabled, created_at
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
			intent    string
			scope     string
			riskClass string
		)
		if err := rows.Scan(
			&entry.JobName, &entry.Description, &entry.DefaultIntervalSec,
			&entry.DefaultMaxRetries, &intent, &scope, &riskClass,
			&enabled, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("jobs: scan registry: %w", err)
		}
		entry.Enabled = enabled != 0
		entry.Intent = semantic.JobIntent(intent)
		entry.Scope = semantic.JobScope(scope)
		entry.RiskClass = semantic.JobRiskClass(riskClass)
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			entry.CreatedAt = t
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

// --- Job Run Log ------------------------------------------------------
//
// Hito 11 (PR #14) introduces the job_run_log table (see
// migrations/008_job_semantics.sql). The three methods below own the
// row lifecycle: RecordRunLog writes the initial pending stub,
// UpdateRunLogAttempt bumps the attempt counter when the engine
// transitions pending → running, and FinishRunLog stamps the
// terminal state + finished_at + error fields. All three methods
// accept a *sql.Tx so the engine layer can interleave the audit
// emission + lease release + run-log update inside one transaction.

// RecordRunLog inserts the initial pending stub row for a run. The
// row is committed inside the caller's tx so the audit-event
// emission and the run-log insert succeed or fail atomically.
func (r *Repository) RecordRunLog(ctx context.Context, tx *sql.Tx, log jobRunLog) error {
	if tx == nil {
		return fmt.Errorf("jobs: record run log: nil tx")
	}
	if log.RunID == "" {
		return fmt.Errorf("jobs: record run log: run_id is required")
	}
	if log.JobName == "" {
		return fmt.Errorf("jobs: record run log: job_name is required")
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO job_run_log (run_id, job_name, state, started_at, attempt)
		 VALUES (?, ?, ?, ?, ?)`,
		log.RunID, log.JobName, log.State,
		log.StartedAt.UTC().Format(time.RFC3339), log.Attempt,
	)
	if err != nil {
		return fmt.Errorf("jobs: record run log: %w", err)
	}
	return nil
}

// UpdateRunLogAttempt bumps the attempt counter on the run-log row
// when the engine transitions pending → running. The attempt value
// is always >= 1 (the pending stub uses 0 by convention; running
// and the terminal events use 1).
func (r *Repository) UpdateRunLogAttempt(ctx context.Context, tx *sql.Tx, runID string, attempt int) error {
	if tx == nil {
		return fmt.Errorf("jobs: update run log attempt: nil tx")
	}
	if runID == "" {
		return fmt.Errorf("jobs: update run log attempt: run_id is required")
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE job_run_log SET attempt = ? WHERE run_id = ?`,
		attempt, runID,
	)
	if err != nil {
		return fmt.Errorf("jobs: update run log attempt: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("jobs: update run log attempt: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("jobs: update run log attempt: run_id %q not found", runID)
	}
	return nil
}

// FinishRunLog stamps the terminal state + finished_at + error
// fields on the run-log row. The error fields are written
// unconditionally (empty strings on success) so the schema mirrors
// the documented SQL contract.
func (r *Repository) FinishRunLog(ctx context.Context, tx *sql.Tx, runID, state, errorCode, errorMessage string) error {
	if tx == nil {
		return fmt.Errorf("jobs: finish run log: nil tx")
	}
	if runID == "" {
		return fmt.Errorf("jobs: finish run log: run_id is required")
	}
	if state == "" {
		return fmt.Errorf("jobs: finish run log: state is required")
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE job_run_log
		 SET state = ?, finished_at = ?, error_code = ?, error_message = ?
		 WHERE run_id = ?`,
		state, time.Now().UTC().Format(time.RFC3339), errorCode, errorMessage, runID,
	)
	if err != nil {
		return fmt.Errorf("jobs: finish run log: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("jobs: finish run log: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("jobs: finish run log: run_id %q not found", runID)
	}
	return nil
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
