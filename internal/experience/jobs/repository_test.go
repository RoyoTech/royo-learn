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

func openJobsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	wrapper := storagetest.OpenTemp(t)
	if err := storage.Migrate(wrapper); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return wrapper.DB
}

func setupJobFixture(t *testing.T, db *sql.DB) domain.ProjectID {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	projectID := domain.ProjectID("proj-jobs-test")
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO projects (id, project_key, display_name, canonical_path, fingerprint, created_at, updated_at)
		VALUES (?, 'jobs-test', 'JobsTest', '/tmp/jobs', 'fp', ?, ?)`, string(projectID), now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return projectID
}

// TestUpsertAndGetJobState verifies the basic CRUD round-trip.
func TestUpsertAndGetJobState(t *testing.T) {
	db := openJobsTestDB(t)
	projectID := setupJobFixture(t, db)
	repo := NewRepository(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	state := JobState{
		ProjectID:   projectID,
		JobName:     "test-job",
		Status:      JobIdle,
		InputDigest: "abc123",
		LeaseOwner:  "",
		MaxRetries:  3,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := repo.UpsertJobState(ctx, state); err != nil {
		t.Fatalf("UpsertJobState: %v", err)
	}

	got, err := repo.GetJobState(ctx, projectID, "test-job")
	if err != nil {
		t.Fatalf("GetJobState: %v", err)
	}
	if got.JobName != "test-job" {
		t.Errorf("JobName = %q, want %q", got.JobName, "test-job")
	}
	if got.Status != JobIdle {
		t.Errorf("Status = %q, want %q", got.Status, JobIdle)
	}
	if got.InputDigest != "abc123" {
		t.Errorf("InputDigest = %q, want %q", got.InputDigest, "abc123")
	}
}

// TestUpsertJobState_Idempotent verifies upsert updates existing rows.
func TestUpsertJobState_Idempotent(t *testing.T) {
	db := openJobsTestDB(t)
	projectID := setupJobFixture(t, db)
	repo := NewRepository(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	s1 := JobState{
		ProjectID:   projectID,
		JobName:     "idem-job",
		Status:      JobIdle,
		InputDigest: "first",
		MaxRetries:  3,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repo.UpsertJobState(ctx, s1); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	s2 := JobState{
		ProjectID:   projectID,
		JobName:     "idem-job",
		Status:      JobRunning,
		InputDigest: "second",
		LeaseOwner:  "owner-xyz",
		MaxRetries:  5,
		CreatedAt:   now,
		UpdatedAt:   now.Add(time.Minute),
	}
	if err := repo.UpsertJobState(ctx, s2); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := repo.GetJobState(ctx, projectID, "idem-job")
	if err != nil {
		t.Fatalf("GetJobState: %v", err)
	}
	if got.Status != JobRunning {
		t.Errorf("Status = %q, want %q", got.Status, JobRunning)
	}
	if got.InputDigest != "second" {
		t.Errorf("InputDigest = %q, want %q", got.InputDigest, "second")
	}
	if got.LeaseOwner != "owner-xyz" {
		t.Errorf("LeaseOwner = %q, want %q", got.LeaseOwner, "owner-xyz")
	}
	if got.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want %d", got.MaxRetries, 5)
	}
}

// TestGetJobState_NotFound verifies ErrJobNotFound is returned.
func TestGetJobState_NotFound(t *testing.T) {
	db := openJobsTestDB(t)
	projectID := setupJobFixture(t, db)
	repo := NewRepository(db)
	ctx := context.Background()

	_, err := repo.GetJobState(ctx, projectID, "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !ErrorIs(err, ErrJobNotFound) {
		t.Errorf("error does not wrap ErrJobNotFound: %v", err)
	}
}

// TestListJobStates verifies listing returns inserted states.
func TestListJobStates(t *testing.T) {
	db := openJobsTestDB(t)
	projectID := setupJobFixture(t, db)
	repo := NewRepository(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	for _, name := range []string{"job-a", "job-b", "job-c"} {
		if err := repo.UpsertJobState(ctx, JobState{
			ProjectID:  projectID,
			JobName:    name,
			Status:     JobIdle,
			MaxRetries: 3,
			CreatedAt:  now,
			UpdatedAt:  now,
		}); err != nil {
			t.Fatalf("upsert %s: %v", name, err)
		}
	}

	list, err := repo.ListJobStates(ctx, projectID)
	if err != nil {
		t.Fatalf("ListJobStates: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len(list) = %d, want 3", len(list))
	}
	names := make(map[string]bool)
	for _, s := range list {
		names[s.JobName] = true
	}
	for _, want := range []string{"job-a", "job-b", "job-c"} {
		if !names[want] {
			t.Errorf("missing job %q in list", want)
		}
	}
}

// TestUpsertAndGetRegistryEntry verifies registry CRUD.
func TestUpsertAndGetRegistryEntry(t *testing.T) {
	db := openJobsTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	entry := JobRegistryEntry{
		JobName:            "reg-test-job",
		Description:        "A test job",
		DefaultIntervalSec: 3600,
		DefaultMaxRetries:  3,
		Enabled:            true,
	}
	if err := repo.UpsertRegistryEntry(ctx, entry); err != nil {
		t.Fatalf("UpsertRegistryEntry: %v", err)
	}

	got, err := repo.GetRegistryEntry(ctx, "reg-test-job")
	if err != nil {
		t.Fatalf("GetRegistryEntry: %v", err)
	}
	if got.JobName != "reg-test-job" {
		t.Errorf("JobName = %q, want %q", got.JobName, "reg-test-job")
	}
	if got.DefaultIntervalSec != 3600 {
		t.Errorf("DefaultIntervalSec = %d, want 3600", got.DefaultIntervalSec)
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
}

// TestGetRegistryEntry_NotFound verifies error on missing entry.
func TestGetRegistryEntry_NotFound(t *testing.T) {
	db := openJobsTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	_, err := repo.GetRegistryEntry(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !ErrorIs(err, ErrJobNotFound) {
		t.Errorf("error does not wrap ErrJobNotFound: %v", err)
	}
}

// TestListRegistryEntries verifies listing.
func TestListRegistryEntries(t *testing.T) {
	db := openJobsTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	for _, name := range []string{"reg-a", "reg-b"} {
		if err := repo.UpsertRegistryEntry(ctx, JobRegistryEntry{
			JobName:            name,
			DefaultIntervalSec: 3600,
			DefaultMaxRetries:  3,
			Enabled:            true,
		}); err != nil {
			t.Fatalf("upsert %s: %v", name, err)
		}
	}

	list, err := repo.ListRegistryEntries(ctx)
	if err != nil {
		t.Fatalf("ListRegistryEntries: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
}
