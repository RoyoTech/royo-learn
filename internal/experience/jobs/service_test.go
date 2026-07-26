package jobs

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/storage/storagetest"
)

func openServiceTestDB(t *testing.T) (*sql.DB, *Service) {
	t.Helper()
	wrapper := storagetest.OpenTemp(t)
	if err := storage.Migrate(wrapper); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewServiceWithDefaults(wrapper.DB)
	// Insert a project row so foreign keys don't fail.
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := wrapper.DB.ExecContext(ctx,
		`INSERT OR IGNORE INTO projects (id, project_key, display_name, canonical_path, fingerprint, created_at, updated_at)
		 VALUES ('proj-svc', 'svc-test', 'SvcTest', '/tmp/svc', 'fp', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return wrapper.DB, svc
}

// TestRegisterAndList verifies registry round-trip.
func TestRegisterAndList(t *testing.T) {
	_, svc := openServiceTestDB(t)
	ctx := context.Background()
	projID := domain.ProjectID("proj-svc")

	entry := JobRegistryEntry{
		JobName:            "test-job",
		Description:        "A test job",
		DefaultIntervalSec: 3600,
		DefaultMaxRetries:  3,
		Enabled:            true,
	}
	if err := svc.Register(ctx, projID, entry); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Idempotent re-register.
	if err := svc.Register(ctx, projID, entry); err != nil {
		t.Fatalf("Register (idempotent): %v", err)
	}

	list, err := svc.ListRegistry(ctx)
	if err != nil {
		t.Fatalf("ListRegistry: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	if list[0].JobName != "test-job" {
		t.Errorf("JobName = %q, want %q", list[0].JobName, "test-job")
	}
}

// TestRegister_EmptyName verifies validation.
func TestRegister_EmptyName(t *testing.T) {
	_, svc := openServiceTestDB(t)
	ctx := context.Background()

	err := svc.Register(ctx, domain.ProjectID("proj-svc"), JobRegistryEntry{JobName: ""})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestAcquireReleaseLease verifies lease lifecycle.
func TestAcquireReleaseLease(t *testing.T) {
	db, svc := openServiceTestDB(t)
	ctx := context.Background()

	projectID := domain.ProjectID("proj-svc")

	// Register a job.
	if err := svc.Register(ctx, projectID, JobRegistryEntry{
		JobName:            "lease-job",
		DefaultIntervalSec: 3600,
		DefaultMaxRetries:  3,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	owner := "runner-1"

	// Acquire the lease.
	state, err := svc.AcquireLease(ctx, projectID, "lease-job", owner)
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if state.Status != JobRunning {
		t.Errorf("Status = %q, want running", state.Status)
	}
	if state.LeaseOwner != owner {
		t.Errorf("LeaseOwner = %q, want %q", state.LeaseOwner, owner)
	}
	if state.LeaseExpiresAt == nil {
		t.Fatal("LeaseExpiresAt is nil")
	}

	// Release with success.
	if err := svc.ReleaseLease(ctx, projectID, "lease-job", owner, RunResult{
		Status: JobOK,
	}); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}

	// Verify state after release.
	repo := NewRepository(db)
	got, err := repo.GetJobState(ctx, projectID, "lease-job")
	if err != nil {
		t.Fatalf("GetJobState: %v", err)
	}
	if got.Status != JobOK {
		t.Errorf("Status = %q, want ok", got.Status)
	}
	if got.LeaseOwner != "" {
		t.Errorf("LeaseOwner = %q, want empty", got.LeaseOwner)
	}
	if got.LastSuccessAt == nil {
		t.Error("LastSuccessAt is nil")
	}
}

// TestAcquireLease_DeniedWhenHeld verifies two runners can't share a lease.
func TestAcquireLease_DeniedWhenHeld(t *testing.T) {
	_, svc := openServiceTestDB(t)
	ctx := context.Background()

	projectID := domain.ProjectID("proj-svc")
	if err := svc.Register(ctx, projectID, JobRegistryEntry{
		JobName:            "locked-job",
		DefaultIntervalSec: 3600,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Runner 1 acquires.
	_, err := svc.AcquireLease(ctx, projectID, "locked-job", "runner-1")
	if err != nil {
		t.Fatalf("AcquireLease (r1): %v", err)
	}

	// Runner 2 tries and MUST fail.
	_, err = svc.AcquireLease(ctx, projectID, "locked-job", "runner-2")
	if err == nil {
		t.Fatal("expected lease conflict, got nil")
	}
}

// TestReleaseLease_WrongOwner verifies ownership check.
func TestReleaseLease_WrongOwner(t *testing.T) {
	_, svc := openServiceTestDB(t)
	ctx := context.Background()

	projectID := domain.ProjectID("proj-svc")
	if err := svc.Register(ctx, projectID, JobRegistryEntry{
		JobName:            "owned-job",
		DefaultIntervalSec: 3600,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, err := svc.AcquireLease(ctx, projectID, "owned-job", "runner-1")
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}

	// Runner 2 tries to release — must fail.
	err = svc.ReleaseLease(ctx, projectID, "owned-job", "runner-2", RunResult{Status: JobOK})
	if err == nil {
		t.Fatal("expected owner mismatch error, got nil")
	}
}

// TestLeaseExpiry verifies IsLeaseExpired helper.
func TestLeaseExpiry(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	if !IsLeaseExpired(&JobState{LeaseExpiresAt: &past}) {
		t.Error("past lease should be expired")
	}
	if IsLeaseExpired(&JobState{LeaseExpiresAt: &future}) {
		t.Error("future lease should not be expired")
	}
	if IsLeaseExpired(nil) {
		t.Error("nil state should not be expired")
	}
	if IsLeaseExpired(&JobState{}) {
		t.Error("nil LeaseExpiresAt should not be expired")
	}
}

// TestReleaseLease_ErrorState verifies error transitions.
func TestReleaseLease_ErrorState(t *testing.T) {
	db, svc := openServiceTestDB(t)
	ctx := context.Background()

	projectID := domain.ProjectID("proj-svc")
	if err := svc.Register(ctx, projectID, JobRegistryEntry{
		JobName:           "err-job",
		DefaultMaxRetries: 3,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, err := svc.AcquireLease(ctx, projectID, "err-job", "runner-1")
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}

	if err := svc.ReleaseLease(ctx, projectID, "err-job", "runner-1", RunResult{
		Status:  JobError,
		Code:    "test_error",
		Message: "something went wrong",
	}); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}

	repo := NewRepository(db)
	got, err := repo.GetJobState(ctx, projectID, "err-job")
	if err != nil {
		t.Fatalf("GetJobState: %v", err)
	}
	if got.Status != JobError {
		t.Errorf("Status = %q, want error", got.Status)
	}
	if got.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", got.RetryCount)
	}
	if got.LastErrorCode != "test_error" {
		t.Errorf("LastErrorCode = %q, want test_error", got.LastErrorCode)
	}
}

// TestRunDue_ExecutesDueJob verifies that a registered job with an
// associated function gets executed.
func TestRunDue_ExecutesDueJob(t *testing.T) {
	_, svc := openServiceTestDB(t)
	ctx := context.Background()
	projID := domain.ProjectID("proj-svc")

	if err := svc.Register(ctx, projID, JobRegistryEntry{
		JobName:            "simple-job",
		DefaultIntervalSec: 0, // always due
		DefaultMaxRetries:  1,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	executed := false
	jobs := map[string]JobFunc{
		"simple-job": func(ctx context.Context, state *JobState) (RunResult, error) {
			executed = true
			return RunResult{Status: JobOK}, nil
		},
	}

	results, err := svc.RunDue(ctx, projID, "runner-1", jobs)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if !executed {
		t.Error("job was not executed")
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Status != JobOK {
		t.Errorf("Status = %q, want ok", results[0].Status)
	}
}

// TestRunDue_SkipsTerminalJobs verifies that jobs in OK or error state
// are not re-executed.
func TestRunDue_SkipsTerminalJobs(t *testing.T) {
	db, svc := openServiceTestDB(t)
	ctx := context.Background()
	projID := domain.ProjectID("proj-svc")

	// Register and manually set state to OK.
	if err := svc.Register(ctx, projID, JobRegistryEntry{
		JobName:            "done-job",
		DefaultIntervalSec: 0,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	repo := NewRepository(db)
	now := time.Now().UTC()
	if err := repo.UpsertJobState(ctx, JobState{
		ProjectID:  projID,
		JobName:    "done-job",
		Status:     JobOK,
		MaxRetries: 1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("UpsertJobState: %v", err)
	}

	executed := false
	jobs := map[string]JobFunc{
		"done-job": func(ctx context.Context, state *JobState) (RunResult, error) {
			executed = true
			return RunResult{Status: JobOK}, nil
		},
	}

	_, err := svc.RunDue(ctx, projID, "runner-1", jobs)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if executed {
		t.Error("terminal job should not have been executed")
	}
}

// TestRunDue_SkipsWhenLeaseHeld verifies that RunDue skips a job whose
// lease is held by another owner.
func TestRunDue_SkipsWhenLeaseHeld(t *testing.T) {
	_, svc := openServiceTestDB(t)
	ctx := context.Background()
	projID := domain.ProjectID("proj-svc")

	if err := svc.Register(ctx, projID, JobRegistryEntry{
		JobName:            "busy-job",
		DefaultIntervalSec: 0,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Another owner acquires the lease first.
	_, err := svc.AcquireLease(ctx, projID, "busy-job", "other-runner")
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}

	executed := false
	jobs := map[string]JobFunc{
		"busy-job": func(ctx context.Context, state *JobState) (RunResult, error) {
			executed = true
			return RunResult{Status: JobOK}, nil
		},
	}

	results, err := svc.RunDue(ctx, projID, "runner-1", jobs)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if executed {
		t.Error("job held by another owner should not have been executed")
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result (skipped)")
	}
	if results[0].Code != "lease_held" {
		t.Errorf("Code = %q, want lease_held", results[0].Code)
	}
}

// TestRunDue_RetryOnError verifies that a failing job is retried and
// eventually released as error.
func TestRunDue_RetryOnError(t *testing.T) {
	db, svc := openServiceTestDB(t)
	ctx := context.Background()
	projID := domain.ProjectID("proj-svc")

	if err := svc.Register(ctx, projID, JobRegistryEntry{
		JobName:            "flaky-job",
		DefaultIntervalSec: 0,
		DefaultMaxRetries:  2,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	attempts := 0
	jobs := map[string]JobFunc{
		"flaky-job": func(ctx context.Context, state *JobState) (RunResult, error) {
			attempts++
			return RunResult{Status: JobError, Code: "fail", Message: "always fails"}, nil
		},
	}

	results, err := svc.RunDue(ctx, projID, "runner-1", jobs)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	// max_retries=2 → 3 attempts (initial + 2 retries)
	if attempts < 3 {
		t.Errorf("attempts = %d, want at least 3", attempts)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Status != JobError {
		t.Errorf("Status = %q, want error", results[0].Status)
	}

	// Verify retry_count was incremented.
	repo := NewRepository(db)
	got, err := repo.GetJobState(ctx, projID, "flaky-job")
	if err != nil {
		t.Fatalf("GetJobState: %v", err)
	}
	if got.RetryCount < 1 {
		t.Errorf("RetryCount = %d, want >= 1", got.RetryCount)
	}
}

// TestRecoverStaleLeases verifies that expired leases are cleared.
func TestRecoverStaleLeases(t *testing.T) {
	db, svc := openServiceTestDB(t)
	ctx := context.Background()
	projID := domain.ProjectID("proj-svc")

	// Manually insert a stale lease.
	repo := NewRepository(db)
	now := time.Now().UTC()
	past := now.Add(-1 * time.Hour)
	err := repo.UpsertJobState(ctx, JobState{
		ProjectID:      projID,
		JobName:        "stale-job",
		Status:         JobRunning,
		LeaseOwner:     "dead-runner",
		LeaseExpiresAt: &past,
		MaxRetries:     3,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("UpsertJobState: %v", err)
	}

	recovered, err := svc.RecoverStaleLeases(ctx, projID)
	if err != nil {
		t.Fatalf("RecoverStaleLeases: %v", err)
	}
	if recovered != 1 {
		t.Errorf("recovered = %d, want 1", recovered)
	}

	// Verify state is back to idle.
	got, err := repo.GetJobState(ctx, projID, "stale-job")
	if err != nil {
		t.Fatalf("GetJobState: %v", err)
	}
	if got.Status != JobIdle {
		t.Errorf("Status = %q, want idle", got.Status)
	}
	if got.LeaseOwner != "" {
		t.Errorf("LeaseOwner = %q, want empty", got.LeaseOwner)
	}
}

// TestComputeDigest verifies the deterministic digest function.
func TestComputeDigest(t *testing.T) {
	d1 := ComputeDigest("hello", "world")
	d2 := ComputeDigest("hello", "world")
	d3 := ComputeDigest("hello", "different")

	if d1 != d2 {
		t.Errorf("same inputs produce different digests: %q vs %q", d1, d2)
	}
	if d1 == d3 {
		t.Errorf("different inputs produce same digest: %q", d1)
	}
	if d4 := ComputeDigest(); d4 != "" {
		t.Errorf("empty input = %q, want empty", d4)
	}
}

// TestDB_ReturnsHandle verifies DB() returns the underlying handle.
func TestDB_ReturnsHandle(t *testing.T) {
	db, _ := openServiceTestDB(t)
	repo := NewRepository(db)
	if repo.DB() == nil {
		t.Error("DB() returned nil")
	}
}

// TestRegister_NilRepo verifies error on nil service.
func TestRegister_NilRepo(t *testing.T) {
	svc := &Service{}
	err := svc.Register(context.Background(), "proj", JobRegistryEntry{JobName: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestListRegistry_NilRepo verifies error on nil service.
func TestListRegistry_NilRepo(t *testing.T) {
	svc := &Service{}
	_, err := svc.ListRegistry(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestReleaseLease_NilRepo verifies error on nil service.
func TestReleaseLease_NilRepo(t *testing.T) {
	svc := &Service{}
	err := svc.ReleaseLease(context.Background(), "proj", "job", "owner", RunResult{Status: JobOK})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestRecoverStaleLeases_NilDB verifies error on nil DB.
func TestRecoverStaleLeases_NilDB(t *testing.T) {
	svc := &Service{}
	_, err := svc.RecoverStaleLeases(context.Background(), "proj")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestAcquireLease_SameOwnerReacquire verifies same owner can re-acquire.
func TestAcquireLease_SameOwnerReacquire(t *testing.T) {
	_, svc := openServiceTestDB(t)
	ctx := context.Background()
	projID := domain.ProjectID("proj-svc")

	if err := svc.Register(ctx, projID, JobRegistryEntry{
		JobName: "reacquire-job",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// First acquire.
	_, err := svc.AcquireLease(ctx, projID, "reacquire-job", "runner-1")
	if err != nil {
		t.Fatalf("first AcquireLease: %v", err)
	}

	// Same owner re-acquires — allowed (it's the same owner).
	_, err = svc.AcquireLease(ctx, projID, "reacquire-job", "runner-1")
	if err != nil {
		t.Fatalf("second AcquireLease (same owner): %v", err)
	}
}

// TestRunDue_WithNoRegisteredFuncs skips jobs without a function.
func TestRunDue_WithNoRegisteredFuncs(t *testing.T) {
	_, svc := openServiceTestDB(t)
	ctx := context.Background()
	projID := domain.ProjectID("proj-svc")

	if err := svc.Register(ctx, projID, JobRegistryEntry{
		JobName: "no-fn-job",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// No job function mapped — should skip with no error.
	results, err := svc.RunDue(ctx, projID, "runner-1", nil)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestReleaseLease_DegradedState verifies degraded transitions.
func TestReleaseLease_DegradedState(t *testing.T) {
	db, svc := openServiceTestDB(t)
	ctx := context.Background()
	projID := domain.ProjectID("proj-svc")

	if err := svc.Register(ctx, projID, JobRegistryEntry{
		JobName: "degraded-job",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err := svc.AcquireLease(ctx, projID, "degraded-job", "runner-1")
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}

	if err := svc.ReleaseLease(ctx, projID, "degraded-job", "runner-1", RunResult{
		Status:  JobDegraded,
		Code:    "partial_error",
		Message: "some items failed",
	}); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}

	repo := NewRepository(db)
	got, err := repo.GetJobState(ctx, projID, "degraded-job")
	if err != nil {
		t.Fatalf("GetJobState: %v", err)
	}
	if got.Status != JobDegraded {
		t.Errorf("Status = %q, want degraded", got.Status)
	}
	// Degraded preserves last success (none yet, so nil).
	// RetryCount should NOT be incremented for degraded.
	if got.RetryCount != 0 {
		t.Errorf("RetryCount = %d, want 0 (degraded should not increment)", got.RetryCount)
	}
}

// TestAcquireLease_ExpiredLeaseTakeover verifies a different owner can
// take over an expired lease.
func TestAcquireLease_ExpiredLeaseTakeover(t *testing.T) {
	db, svc := openServiceTestDB(t)
	ctx := context.Background()
	projID := domain.ProjectID("proj-svc")

	// Manually insert an expired lease.
	repo := NewRepository(db)
	now := time.Now().UTC()
	past := now.Add(-1 * time.Hour)
	if err := repo.UpsertJobState(ctx, JobState{
		ProjectID:      projID,
		JobName:        "expired-job",
		Status:         JobRunning,
		LeaseOwner:     "old-runner",
		LeaseExpiresAt: &past,
		MaxRetries:     3,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertJobState: %v", err)
	}

	// New owner should be able to acquire the expired lease.
	state, err := svc.AcquireLease(ctx, projID, "expired-job", "new-runner")
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if state.LeaseOwner != "new-runner" {
		t.Errorf("LeaseOwner = %q, want new-runner", state.LeaseOwner)
	}
	if state.Status != JobRunning {
		t.Errorf("Status = %q, want running", state.Status)
	}
}

// TestAcquireLease_NilDB verifies error on nil DB.
func TestAcquireLease_NilDB(t *testing.T) {
	svc := &Service{}
	_, err := svc.AcquireLease(context.Background(), "proj", "job", "owner")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestListStates verifies the service ListStates method.
func TestListStates(t *testing.T) {
	_, svc := openServiceTestDB(t)
	ctx := context.Background()
	projID := domain.ProjectID("proj-svc")

	if err := svc.Register(ctx, projID, JobRegistryEntry{
		JobName:            "list-job",
		DefaultIntervalSec: 0,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	states, err := svc.ListStates(ctx, projID)
	if err != nil {
		t.Fatalf("ListStates: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("expected at least one state")
	}
	found := false
	for _, s := range states {
		if s.JobName == "list-job" {
			found = true
			break
		}
	}
	if !found {
		t.Error("list-job not found in ListStates result")
	}
}

// TestListStates_NilRepo verifies error on nil service.
func TestListStates_NilRepo(t *testing.T) {
	svc := &Service{}
	_, err := svc.ListStates(context.Background(), "proj")
	if err == nil {
		t.Fatal("expected error")
	}
}
