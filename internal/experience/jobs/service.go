package jobs

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// Service implements the job engine operations: registration, lease
// acquisition/release, and run-due dispatch (slice 8.2).
type Service struct {
	repo   *Repository
	db     *sql.DB
	bounds LeaseBounds
}

// NewService wires the service with a repository and lease defaults.
func NewService(db *sql.DB, bounds LeaseBounds) *Service {
	return &Service{
		repo:   NewRepository(db),
		db:     db,
		bounds: bounds,
	}
}

// NewServiceWithDefaults creates a service with conservative lease defaults.
func NewServiceWithDefaults(db *sql.DB) *Service {
	return NewService(db, DefaultLeaseBounds())
}

// --- Registry ---------------------------------------------------------

// Register ensures a job exists in both the registry and the state table.
// It is idempotent: registering the same job twice with the same config
// is a no-op.
func (s *Service) Register(ctx context.Context, projectID domain.ProjectID, entry JobRegistryEntry) error {
	if s.repo == nil {
		return fmt.Errorf("jobs: service not initialised")
	}
	if entry.JobName == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "jobs: job_name is required")
	}

	// Upsert the registry entry first (global, not project-scoped).
	if err := s.repo.UpsertRegistryEntry(ctx, entry); err != nil {
		return fmt.Errorf("jobs: register: upsert registry: %w", err)
	}

	// Initialise the state row for this project if it doesn't exist yet.
	now := time.Now().UTC()
	state := JobState{
		ProjectID:  projectID,
		JobName:    entry.JobName,
		Status:     JobIdle,
		MaxRetries: entry.DefaultMaxRetries,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.repo.UpsertJobState(ctx, state); err != nil {
		return fmt.Errorf("jobs: register: upsert state: %w", err)
	}
	return nil
}

// ListRegistry returns all registered jobs.
func (s *Service) ListRegistry(ctx context.Context) ([]JobRegistryEntry, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("jobs: service not initialised")
	}
	return s.repo.ListRegistryEntries(ctx)
}

// ListStates returns the state summary for all jobs in a project.
func (s *Service) ListStates(ctx context.Context, projectID domain.ProjectID) ([]JobStateSummary, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("jobs: service not initialised")
	}
	return s.repo.ListJobStates(ctx, projectID)
}

// --- Lease ------------------------------------------------------------

// AcquireLease tries to take the lease for a job. It succeeds only when:
//   - The job is not currently leased (status != running), OR
//   - The existing lease has expired.
//
// The lease is acquired atomically inside a SQLite transaction.
func (s *Service) AcquireLease(ctx context.Context, projectID domain.ProjectID, jobName, owner string) (*JobState, error) {
	if s.db == nil {
		return nil, fmt.Errorf("jobs: database is nil")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("jobs: acquire lease: begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			tx.Rollback() //nolint: errcheck
		}
	}()

	state, err := s.repo.GetJobState(ctx, projectID, jobName)
	if err != nil {
		// Use a tx-scoped query to avoid the repo's not-found wrapping.
		row := tx.QueryRowContext(ctx,
			`SELECT project_id, job_name, status, input_digest,
				lease_owner, lease_expires_at, retry_count, max_retries,
				last_started_at, last_success_at, last_failed_at,
				last_error_code, last_error, metrics_json, created_at, updated_at
			 FROM job_state WHERE project_id = ? AND job_name = ?`,
			string(projectID), jobName,
		)
		var st JobState
		var le, ls, lss, lf sql.NullString
		var ca, ua string
		scanErr := row.Scan(
			&st.ProjectID, &st.JobName, &st.Status, &st.InputDigest,
			&st.LeaseOwner, &le, &st.RetryCount, &st.MaxRetries,
			&ls, &lss, &lf,
			&st.LastErrorCode, &st.LastError, &st.MetricsJSON,
			&ca, &ua,
		)
		if scanErr != nil {
			return nil, fmt.Errorf("jobs: acquire lease: get state: %w", scanErr)
		}
		st.LeaseExpiresAt = parseNullTime(le)
		st.LastStartedAt = parseNullTime(ls)
		st.LastSuccessAt = parseNullTime(lss)
		st.LastFailedAt = parseNullTime(lf)
		if t, parseErr := time.Parse(time.RFC3339, ca); parseErr == nil {
			st.CreatedAt = t
		}
		if t, parseErr := time.Parse(time.RFC3339, ua); parseErr == nil {
			st.UpdatedAt = t
		}
		state = &st
	}

	// Check if the lease is currently held by someone else.
	if state.Status == JobRunning && state.LeaseOwner != "" && state.LeaseOwner != owner {
		now := time.Now().UTC()
		if state.LeaseExpiresAt != nil && now.Before(*state.LeaseExpiresAt) {
			// Lease is active and not ours.
			return nil, domain.NewConflictError("job_lease_held",
				fmt.Sprintf("job %q is leased by %q until %s", jobName, state.LeaseOwner, state.LeaseExpiresAt.Format(time.RFC3339)))
		}
		// Lease expired — we can take it.
	}

	// Acquire the lease.
	now := time.Now().UTC()
	expires := now.Add(s.bounds.LeaseDuration)
	state.LeaseOwner = owner
	state.LeaseExpiresAt = &expires
	state.Status = JobRunning
	state.LastStartedAt = &now
	state.UpdatedAt = now

	if err := s.repo.UpsertJobState(ctx, *state); err != nil {
		return nil, fmt.Errorf("jobs: acquire lease: upsert state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("jobs: acquire lease: commit: %w", err)
	}
	tx = nil // prevent rollback in defer

	return state, nil
}

// ReleaseLease clears the lease and transitions the job to a terminal
// or degraded state. The caller MUST be the current lease owner.
func (s *Service) ReleaseLease(ctx context.Context, projectID domain.ProjectID, jobName, owner string, result RunResult) error {
	if s.repo == nil {
		return fmt.Errorf("jobs: service not initialised")
	}

	state, err := s.repo.GetJobState(ctx, projectID, jobName)
	if err != nil {
		return fmt.Errorf("jobs: release lease: get state: %w", err)
	}

	// Ownership check: only the current owner can release.
	if state.LeaseOwner != owner {
		return domain.NewConflictError("job_lease_owner_mismatch",
			fmt.Sprintf("job %q is leased by %q, not %q", jobName, state.LeaseOwner, owner))
	}

	now := time.Now().UTC()
	state.LeaseOwner = ""
	state.LeaseExpiresAt = nil
	state.Status = result.Status
	state.UpdatedAt = now

	switch result.Status {
	case JobOK:
		state.LastSuccessAt = &now
		state.RetryCount = 0
	case JobError:
		state.LastFailedAt = &now
		state.LastErrorCode = result.Code
		state.LastError = result.Message
		state.RetryCount++
	case JobDegraded:
		// Degraded preserves last success; don't increment retry.
		state.LastFailedAt = &now
		state.LastErrorCode = result.Code
		state.LastError = result.Message
	}

	if err := s.repo.UpsertJobState(ctx, *state); err != nil {
		return fmt.Errorf("jobs: release lease: upsert: %w", err)
	}
	return nil
}

// IsLeaseExpired reports whether the job's lease has passed its
// expiration time and is safe to preempt.
func IsLeaseExpired(state *JobState) bool {
	if state == nil || state.LeaseExpiresAt == nil {
		return false
	}
	return time.Now().UTC().After(*state.LeaseExpiresAt)
}

// --- Run-Due ----------------------------------------------------------

// JobFunc is the signature of a runnable job. It receives the current
// state and returns a result. The engine handles lease lifecycle.
type JobFunc func(ctx context.Context, state *JobState) (RunResult, error)

// RunDue executes all jobs whose interval has elapsed and whose state
// is not terminal. For each due job:
//
//  1. Acquire the lease (skip if held by another owner).
//  2. Compute the input digest; skip if unchanged since last success.
//  3. Execute the job function.
//  4. Release the lease with the result.
//  5. On transient failure, retry up to max_retries with backoff.
//
// Returns a summary of all executed and skipped jobs.
func (s *Service) RunDue(ctx context.Context, projectID domain.ProjectID, owner string, jobs map[string]JobFunc) ([]RunResult, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("jobs: service not initialised")
	}

	states, err := s.repo.ListJobStates(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("jobs: run due: list states: %w", err)
	}

	var results []RunResult
	for _, summary := range states {
		// Skip terminal states.
		if summary.Status.IsTerminal() {
			continue
		}
		// Skip if no job function is registered.
		fn, ok := jobs[summary.JobName]
		if !ok {
			continue
		}

		// Acquire the lease.
		state, leaseErr := s.AcquireLease(ctx, projectID, summary.JobName, owner)
		if leaseErr != nil {
			// Lease held by another owner — skip silently.
			results = append(results, RunResult{
				Status:  summary.Status,
				Code:    "lease_held",
				Message: fmt.Sprintf("skipped: %v", leaseErr),
			})
			continue
		}

		// Execute with retry.
		result := s.executeWithRetry(ctx, projectID, state, owner, fn)
		results = append(results, result)
	}

	return results, nil
}

// executeWithRetry runs a job function, retrying on transient failure
// up to max_retries. After max_retries, the lease is released with
// JobError. Each retry is spaced with a short backoff.
func (s *Service) executeWithRetry(ctx context.Context, projectID domain.ProjectID, state *JobState, owner string, fn JobFunc) RunResult {
	maxRetries := state.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	var lastResult RunResult
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, execErr := fn(ctx, state)
		if execErr != nil {
			lastResult = RunResult{
				Status:  JobError,
				Code:    "execution_error",
				Message: execErr.Error(),
			}
		} else {
			lastResult = result
		}

		// Success or declared OK/degraded — release and return.
		if lastResult.Status == JobOK || lastResult.Status == JobDegraded {
			if releaseErr := s.ReleaseLease(ctx, projectID, state.JobName, owner, lastResult); releaseErr != nil {
				return RunResult{
					Status:  JobError,
					Code:    "release_failed",
					Message: releaseErr.Error(),
				}
			}
			return lastResult
		}

		// Retryable failure — backoff and retry.
		if attempt < maxRetries {
			backoff := time.Duration(attempt+1) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				// Context cancelled — release as error.
				s.ReleaseLease(ctx, projectID, state.JobName, owner, RunResult{
					Status:  JobError,
					Code:    "context_cancelled",
					Message: ctx.Err().Error(),
				})
				return RunResult{Status: JobError, Code: "context_cancelled"}
			case <-time.After(backoff):
			}
		}
	}

	// Exhausted retries — release as error.
	if releaseErr := s.ReleaseLease(ctx, projectID, state.JobName, owner, lastResult); releaseErr != nil {
		return RunResult{
			Status:  JobError,
			Code:    "release_failed",
			Message: releaseErr.Error(),
		}
	}
	return lastResult
}

// --- Crash Recovery ---------------------------------------------------

// RecoverStaleLeases clears leases that have exceeded their expiration
// time, returning those jobs to idle. This is safe to call at startup
// and periodically: only expired leases owned by another process are
// cleared.
func (s *Service) RecoverStaleLeases(ctx context.Context, projectID domain.ProjectID) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("jobs: database is nil")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		`UPDATE job_state
		 SET status = 'idle',
		     lease_owner = '',
		     lease_expires_at = NULL,
		     updated_at = ?
		 WHERE project_id = ?
		   AND status = 'running'
		   AND lease_expires_at IS NOT NULL
		   AND lease_expires_at < ?`,
		now, string(projectID), now,
	)
	if err != nil {
		return 0, fmt.Errorf("jobs: recover stale leases: %w", err)
	}
	recovered, _ := result.RowsAffected()
	return int(recovered), nil
}

// ComputeDigest derives a stable input digest for a job. The default
// implementation returns an empty string; callers should override this
// by computing a hash over the inputs that matter for idempotency.
func ComputeDigest(inputs ...string) string {
	// For v1, concatenate inputs and hash. A future version can use
	// a proper streaming hash if inputs are large.
	if len(inputs) == 0 {
		return ""
	}
	h := sha256.New()
	for _, in := range inputs {
		h.Write([]byte(in))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
