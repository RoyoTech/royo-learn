package jobs

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/semantic"
	"agent-royo-learn/internal/storage"
)

// Service implements the job engine operations: registration, lease
// acquisition/release, and run-due dispatch (slice 8.2).
type Service struct {
	repo   *Repository
	db     *sql.DB
	bounds LeaseBounds
	// nowFn is the package-level injectable clock. Tests inject a
	// deterministic clock so audit_events.occurred_at and
	// job_run_log.started_at are reproducible. Nil falls back to
	// time.Now.UTC (the production default).
	nowFn func() time.Time
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

// NewServiceWithClock creates a service that uses the provided clock
// for audit-event timestamps. Tests inject a deterministic clock so
// the audit-event row's occurred_at is reproducible. A nil clock
// falls back to time.Now.UTC (the production default).
func NewServiceWithClock(db *sql.DB, now func() time.Time) *Service {
	s := NewService(db, DefaultLeaseBounds())
	s.nowFn = now
	return s
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

	state, err := s.AcquireLeaseTx(ctx, tx, projectID, jobName, owner)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("jobs: acquire lease: commit: %w", err)
	}
	return state, nil
}

// AcquireLeaseTx is the tx-aware sibling of AcquireLease. It performs
// the same lease-acquisition logic but inside the caller's *sql.Tx
// so the audit-event emission + lease-state update + run-log writes
// commit atomically (Hito 11 PR #14, design.md §"Decision:
// Lease-acquisition path inside RunOne"). The two helpers share the
// same conflict-detection rules: an active lease owned by a
// different owner is rejected with the same domain.NewConflictError
// code.
func (s *Service) AcquireLeaseTx(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, jobName, owner string) (*JobState, error) {
	if s.db == nil {
		return nil, fmt.Errorf("jobs: database is nil")
	}
	if tx == nil {
		return nil, fmt.Errorf("jobs: acquire lease tx: nil tx")
	}

	// Read the state row inside the caller's tx so we honour the
	// audit-finalisation ordering and avoid a cross-handle SELECT
	// (which can race with the audit-finalisation ctx on
	// modernc.org/sqlite when the parent ctx is cancelled).
	state, err := s.repo.GetJobStateTx(ctx, tx, projectID, jobName)
	if err != nil {
		return nil, fmt.Errorf("jobs: acquire lease tx: get state: %w", err)
	}

	// Same conflict-detection rules as AcquireLease.
	if state.Status == JobRunning && state.LeaseOwner != "" && state.LeaseOwner != owner {
		now := time.Now().UTC()
		if state.LeaseExpiresAt != nil && now.Before(*state.LeaseExpiresAt) {
			return nil, domain.NewConflictError("job_lease_held",
				fmt.Sprintf("job %q is leased by %q until %s", jobName, state.LeaseOwner, state.LeaseExpiresAt.Format(time.RFC3339)))
		}
	}

	// Update the state row inside the caller's tx.
	now := time.Now().UTC()
	expires := now.Add(s.bounds.LeaseDuration)
	state.LeaseOwner = owner
	state.LeaseExpiresAt = &expires
	state.Status = JobRunning
	state.LastStartedAt = &now
	state.UpdatedAt = now

	if err := s.repo.UpsertJobStateTx(ctx, tx, *state); err != nil {
		return nil, fmt.Errorf("jobs: acquire lease tx: upsert state: %w", err)
	}
	return state, nil
}

// ReleaseLeaseTx is the tx-aware sibling of ReleaseLease. It performs
// the same lease-release logic inside the caller's *sql.Tx so the
// audit-event emission + lease-state update + run-log writes commit
// atomically (Hito 11 PR #14). The two helpers share the same
// ownership-check rule: only the current owner can release.
func (s *Service) ReleaseLeaseTx(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, jobName, owner string, result RunResult) error {
	if s.repo == nil {
		return fmt.Errorf("jobs: service not initialised")
	}
	if tx == nil {
		return fmt.Errorf("jobs: release lease tx: nil tx")
	}

	state, err := s.repo.GetJobStateTx(ctx, tx, projectID, jobName)
	if err != nil {
		return fmt.Errorf("jobs: release lease tx: get state: %w", err)
	}

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
		state.LastFailedAt = &now
		state.LastErrorCode = result.Code
		state.LastError = result.Message
	}

	if err := s.repo.UpsertJobStateTx(ctx, tx, *state); err != nil {
		return fmt.Errorf("jobs: release lease tx: upsert: %w", err)
	}
	return nil
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

// --- RunOne (audit hook) ----------------------------------------------
//
// RunOne is the one-shot sibling of RunDue. It owns a single
// lease + audit + job_run_log transaction that emits exactly four
// audit_events rows (job_pending, job_running, job_succeeded OR
// job_failed) and writes one job_run_log row per execution. The four
// events share a single run_id so the operator can correlate them
// in audit_events by run_id. The payload allow-list (semantic.
// BuildJobPayload) enforces the Hito 10 SEVERE trace-leak invariant
// (hito10-codex-review-fixes.md, docs/24-TM §6): every payload key
// is engine-controlled, never an ExperienceEnvelope field.
//
// RunOne does NOT swallow the JobFunc's error: the caller (CLI
// dispatcher in PR #15) receives the returned error and decides
// whether to exit non-zero. The job_failed audit event is emitted
// inside RunOne before the error returns so the audit invariant
// is preserved even when the caller fails the process.
//
// RunOne is intentionally synchronous. The async / scheduled path
// continues to live in RunDue (the Hito 3 watch loop, PR #10).
// RunDue keeps its retry loop; RunOne is the one-shot path used
// by the unified CLI (experience scan --source=...).

// auditActorName is the Actor.Name stamped on every job_* audit event
// emitted by RunOne. The constant keeps the audit sink consistent
// across calls so the operator can filter by name.
const auditActorName = "jobs-service"

// RunOne runs one job end-to-end. The audit + lease + run-log
// writes are split into four short, independent transactions so a
// body-side ctx cancellation cannot replay the Hito 11 PR #14
// "transaction has already been committed or rolled back" race:
//
//  1. Tx-A — pending: insert job_run_log stub + emit job_pending.
//  2. Lease tx — acquire the lease via the existing AcquireLease.
//  3. Tx-B — running: bump job_run_log.attempt + emit job_running.
//  4. Body — execute the JobFunc with s.db (never the wrapping tx).
//  5. Tx-C — terminal: release lease + emit terminal audit event +
//     finish run-log. Uses context.Background() so a parent
//     cancellation cannot auto-rollback the finalisation tx.
//
// The audit invariant (4 events in order, all sharing the run_id)
// is preserved by minting the run_id once at entry and threading
// it through every phase. The main trade-off vs the original
// one-big-tx design is that the audit events and the lease state
// are no longer strictly atomic: each can succeed independently.
// This matches the existing RunDue pattern and eliminates the
// cancellation race that broke TestRunOne_CancellationHonoured.
//
// The method returns an error when the JobFunc fails, when the
// lease cannot be acquired (with error wrapping job_lease_held),
// when context cancellation interrupts, or when the audit hook
// itself fails. The four audit events are written in all but the
// lease-conflict path (in that case job_pending + job_failed are
// emitted and the JobFunc is skipped).
func (s *Service) RunOne(ctx context.Context, projectID domain.ProjectID, jobName, owner string, j *semantic.Job) (JobRunOutcome, error) {
	var outcome JobRunOutcome
	if s.db == nil {
		return outcome, fmt.Errorf("jobs: database is nil")
	}
	if s.repo == nil {
		return outcome, fmt.Errorf("jobs: service not initialised")
	}
	if j == nil {
		return outcome, fmt.Errorf("jobs: run one: nil Job argument")
	}

	// Ensure the registry row exists before any audit emission. The
	// CLI dispatcher (PR #15) calls Register before RunOne, but
	// the engine defends against a missing row so the audit
	// invariant is never violated by an unregistered job name.
	if _, err := s.repo.GetRegistryEntry(ctx, jobName); err != nil {
		if ErrorIs(err, ErrJobNotFound) {
			return outcome, fmt.Errorf("jobs: run one: registry entry not found for %q: %w", jobName, err)
		}
		return outcome, fmt.Errorf("jobs: run one: get registry: %w", err)
	}

	// Mint a deterministic UUIDv7 per run. The same run_id is
	// shared by all four audit_events rows + the single job_run_log
	// row emitted inside this RunOne invocation.
	runID := uuid.Must(uuid.NewV7()).String()
	outcome.RunID = runID
	outcome.JobName = jobName
	outcome.State = semantic.StatePending
	outcome.Attempt = 0
	outcome.StartedAt = time.Now().UTC()

	// Tx-A: pending — insert job_run_log stub + emit job_pending.
	if err := s.writePending(ctx, runID, jobName, j.Source, outcome.StartedAt); err != nil {
		return outcome, err
	}

	// Acquire the lease (separate tx managed by AcquireLease).
	if _, leaseErr := s.AcquireLease(ctx, projectID, jobName, owner); leaseErr != nil {
		// Lease held by another owner: emit job_failed with
		// error_code=job_lease_held. The pending row stays in
		// job_run_log (preserved by the audit invariant). The
		// finalisation ctx is detached from parent cancellation so
		// the audit invariant survives a body-side cancel race.
		finaliseCtx := context.WithoutCancel(ctx)
		finishedAt := time.Now().UTC()
		failEvent, fErr := s.buildJobAuditEvent(finaliseCtx, semantic.EventJobFailed, runID, jobName, string(j.Source), semantic.StateLeaseHeld, 0, "job_lease_held", leaseErr.Error(), finishedAt)
		if fErr != nil {
			return outcome, fmt.Errorf("jobs: run one: build lease-held event: %w", fErr)
		}
		if err := s.commitTerminalAudit(finaliseCtx, runID, jobName, j.Source, semantic.StateLeaseHeld, 0, "job_lease_held", leaseErr.Error(), finishedAt, failEvent); err != nil {
			return outcome, fmt.Errorf("jobs: run one: commit lease-held: %w", err)
		}
		outcome.State = semantic.StateLeaseHeld
		outcome.Attempt = 0
		outcome.ErrorCode = "job_lease_held"
		outcome.ErrorMessage = leaseErr.Error()
		outcome.FinishedAt = &finishedAt
		return outcome, fmt.Errorf("jobs: run one: lease held: %w", leaseErr)
	}

	// Tx-B: running — bump attempt + emit job_running.
	if err := s.writeRunning(ctx, runID, jobName, j.Source); err != nil {
		return outcome, err
	}
	outcome.Attempt = 1
	outcome.State = semantic.StateRunning

	// Body: execute the JobFunc. The body opens its own inner txs
	// (for adapter Ingest paths); the engine never hands a tx to
	// the body so the adapter code cannot commit or roll back the
	// audit state. A body-side ctx cancellation propagates to the
	// return value below; the finalisation tx uses a detached ctx
	// so the audit invariant survives the cancel race.
	deps := semantic.Deps{
		DB:             s.db,
		Now:            s.now,
		SourceInstance: j.Source, // narrowed to ExperienceSource string; PR #15 binds the typed value.
	}
	jobResult, jobErr := j.Func(ctx, deps)

	// Tx-C: terminal — release lease + emit terminal audit event +
	// finish run-log. Uses context.Background() so a parent
	// cancellation cannot auto-rollback the finalisation tx.
	finaliseCtx := context.Background()
	finalisedAt := time.Now().UTC()
	if jobErr != nil || jobResult.ErrorCode != "" {
		// Failure path: emit job_failed, release lease as JobError,
		// finish run-log with failed state.
		code := jobResult.ErrorCode
		if code == "" {
			code = "execution_error"
		}
		msg := jobResult.ErrorMessage
		if msg == "" && jobErr != nil {
			msg = jobErr.Error()
		}
		failEvent, fErr := s.buildJobAuditEvent(finaliseCtx, semantic.EventJobFailed, runID, jobName, string(j.Source), semantic.StateFailed, 1, code, msg, finalisedAt)
		if fErr != nil {
			return outcome, fmt.Errorf("jobs: run one: build failed event: %w", fErr)
		}
		if err := s.releaseLease(finaliseCtx, projectID, jobName, owner, RunResult{Status: JobError, Code: code, Message: msg}); err != nil {
			return outcome, fmt.Errorf("jobs: run one: release lease on failure: %w", err)
		}
		if err := s.commitTerminalAudit(finaliseCtx, runID, jobName, j.Source, semantic.StateFailed, 1, code, msg, finalisedAt, failEvent); err != nil {
			return outcome, fmt.Errorf("jobs: run one: commit failed: %w", err)
		}
		outcome.State = semantic.StateFailed
		outcome.ErrorCode = code
		outcome.ErrorMessage = msg
		outcome.FinishedAt = &finalisedAt
		if jobErr != nil {
			return outcome, fmt.Errorf("jobs: run one: %w", jobErr)
		}
		return outcome, nil
	}

	// Success path: emit job_succeeded, release lease as JobOK,
	// finish run-log with succeeded state.
	okEvent, err := s.buildJobAuditEvent(finaliseCtx, semantic.EventJobSucceeded, runID, jobName, string(j.Source), semantic.StateSucceeded, 1, "", "", finalisedAt)
	if err != nil {
		return outcome, fmt.Errorf("jobs: run one: build succeeded event: %w", err)
	}
	if err := s.releaseLease(finaliseCtx, projectID, jobName, owner, RunResult{Status: JobOK}); err != nil {
		return outcome, fmt.Errorf("jobs: run one: release lease on success: %w", err)
	}
	if err := s.commitTerminalAudit(finaliseCtx, runID, jobName, j.Source, semantic.StateSucceeded, 1, "", "", finalisedAt, okEvent); err != nil {
		return outcome, fmt.Errorf("jobs: run one: commit succeeded: %w", err)
	}
	outcome.State = semantic.StateSucceeded
	outcome.FinishedAt = &finalisedAt
	return outcome, nil
}

// writePending is the Tx-A helper: a short tx that records the
// initial job_pending audit event and inserts the job_run_log stub
// row. It commits independently so the audit invariant is preserved
// even when the subsequent lease acquisition fails.
func (s *Service) writePending(ctx context.Context, runID, jobName, source string, startedAt time.Time) error {
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("jobs: run one: begin pending tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback() //nolint: errcheck
		}
	}()
	pendingEvent, err := s.buildJobAuditEvent(ctx, semantic.EventJobPending, runID, jobName, source, semantic.StatePending, 0, "", "", startedAt)
	if err != nil {
		return fmt.Errorf("jobs: run one: build pending event: %w", err)
	}
	if err := storage.RecordEventTx(ctx, tx, pendingEvent); err != nil {
		return fmt.Errorf("jobs: run one: record pending: %w", err)
	}
	if err := s.repo.RecordRunLog(ctx, tx, jobRunLog{
		RunID:     runID,
		JobName:   jobName,
		State:     semantic.StatePending,
		StartedAt: startedAt,
		Attempt:   0,
	}); err != nil {
		return fmt.Errorf("jobs: run one: record run log pending: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("jobs: run one: commit pending: %w", err)
	}
	committed = true
	return nil
}

// writeRunning is the Tx-B helper: a short tx that bumps the
// job_run_log attempt counter and emits the job_running audit event.
// It commits independently so a body-side cancel cannot replay the
// Hito 11 PR #14 "transaction committed or rolled back" race.
func (s *Service) writeRunning(ctx context.Context, runID, jobName, source string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("jobs: run one: begin running tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback() //nolint: errcheck
		}
	}()
	runningEvent, err := s.buildJobAuditEvent(ctx, semantic.EventJobRunning, runID, jobName, source, semantic.StateRunning, 1, "", "", time.Now().UTC())
	if err != nil {
		return fmt.Errorf("jobs: run one: build running event: %w", err)
	}
	if err := storage.RecordEventTx(ctx, tx, runningEvent); err != nil {
		return fmt.Errorf("jobs: run one: record running: %w", err)
	}
	if err := s.repo.UpdateRunLogAttempt(ctx, tx, runID, 1); err != nil {
		return fmt.Errorf("jobs: run one: update attempt: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("jobs: run one: commit running: %w", err)
	}
	committed = true
	return nil
}

// commitTerminalAudit is the Tx-C helper: a short tx that writes
// the terminal audit event and stamps the run-log row. The lease
// release is performed separately (releaseLease) so the audit
// invariant is preserved even when the lease row was already cleaned
// up by another process.
func (s *Service) commitTerminalAudit(ctx context.Context, runID, jobName, source, state string, attempt int, errorCode, errorMessage string, occurredAt time.Time, event *domain.AuditEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("jobs: run one: begin terminal tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback() //nolint: errcheck
		}
	}()
	if err := storage.RecordEventTx(ctx, tx, event); err != nil {
		return fmt.Errorf("jobs: run one: record terminal: %w", err)
	}
	if err := s.repo.FinishRunLog(ctx, tx, runID, state, errorCode, errorMessage); err != nil {
		return fmt.Errorf("jobs: run one: finish run log: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("jobs: run one: commit terminal: %w", err)
	}
	committed = true
	return nil
}

// releaseLease is the wrapping helper that opens a short tx for
// the lease-release write. It is the non-tx sibling of
// ReleaseLeaseTx used by the Tx-C finalisation path. The ctx is
// passed straight to the underlying SQL driver so a detached
// finaliseCtx (context.Background()) survives a parent cancel.
func (s *Service) releaseLease(ctx context.Context, projectID domain.ProjectID, jobName, owner string, result RunResult) error {
	if s.db == nil {
		return fmt.Errorf("jobs: database is nil")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("jobs: release lease: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback() //nolint: errcheck
		}
	}()
	if err := s.ReleaseLeaseTx(ctx, tx, projectID, jobName, owner, result); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("jobs: release lease: commit: %w", err)
	}
	committed = true
	return nil
}

// now returns the package-level clock used to stamp audit events.
// The clock is injectable for tests via NewServiceWithClock (the
// default falls back to time.Now). The function is intentionally
// tiny; it exists so RunOne can call it without depending on the
// Service.bounds field (which is lease-config, not clock config).
func (s *Service) now() time.Time {
	if s == nil {
		return time.Now().UTC()
	}
	if s.nowFn != nil {
		return s.nowFn().UTC()
	}
	return time.Now().UTC()
}

// buildJobAuditEvent builds a *domain.AuditEvent for a job_*
// transition. The payload is constructed via semantic.BuildJobPayload
// (the 7-key allow-list, no transcript content — see
// hito10-codex-review-fixes.md for the Hito 10 SEVERE invariant).
// The function returns an error when the transition or state is not
// one of the documented constants; it never returns an event with
// forbidden payload keys.
func (s *Service) buildJobAuditEvent(ctx context.Context, transition, runID, jobName, source, state string, attempt int, errorCode, errorMessage string, occurredAt time.Time) (*domain.AuditEvent, error) {
	if !isValidTransition(transition) {
		return nil, fmt.Errorf("jobs: invalid audit transition %q", transition)
	}
	if !isValidJobState(state) {
		return nil, fmt.Errorf("jobs: invalid audit state %q", state)
	}
	payload := semantic.BuildJobPayload(jobName, runID, source, state, transition, fmt.Sprintf("%d", attempt), errorCode, errorMessage)
	details := make(map[string]any, len(payload))
	for k, v := range payload {
		details[k] = v
	}
	event := &domain.AuditEvent{
		ID:         domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt: occurredAt,
		Actor: domain.Actor{
			Kind: "system",
			Name: auditActorName,
		},
		Operation:  transition,
		EntityType: "job_run",
		EntityID:   runID,
		Result:     "success",
		Details:    details,
	}
	if errorCode != "" {
		ec := errorCode
		event.ErrorCode = &ec
	}
	if errorMessage != "" {
		event.Result = "error"
	}
	return event, nil
}

// isValidTransition reports whether the transition argument is one
// of the four documented EventJob* constants. The check is the
// runtime counterpart to the static allow-list in
// semantic/events.go: a free-form transition string cannot reach
// the audit sink.
func isValidTransition(t string) bool {
	switch t {
	case semantic.EventJobPending, semantic.EventJobRunning, semantic.EventJobSucceeded, semantic.EventJobFailed:
		return true
	}
	return false
}

// isValidJobState reports whether the state argument is one of the
// five documented State* constants. The check mirrors the
// job_run_log CHECK constraint from migration 008.
func isValidJobState(s string) bool {
	switch s {
	case semantic.StatePending, semantic.StateRunning, semantic.StateSucceeded, semantic.StateFailed, semantic.StateLeaseHeld:
		return true
	}
	return false
}
