package codex

import (
	"context"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/jobs"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/storage/storagetest"
)

// TestJobRegistryEntry_Shape pins the static fields so the registration
// helper is the single source of truth for the job name and default config
// (per docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md §2 Hito 10 jobs row).
func TestJobRegistryEntry_Shape(t *testing.T) {
	entry := JobRegistryEntry()

	if entry.JobName != "experience_ingest:codex" {
		t.Errorf("JobName = %q, want %q", entry.JobName, "experience_ingest:codex")
	}
	if entry.Description == "" {
		t.Error("Description is empty, want a non-empty summary")
	}
	if entry.DefaultIntervalSec != 300 {
		t.Errorf("DefaultIntervalSec = %d, want 300 (5 min, matches opencode and claudecode precedent)", entry.DefaultIntervalSec)
	}
	if entry.DefaultMaxRetries != 3 {
		t.Errorf("DefaultMaxRetries = %d, want 3", entry.DefaultMaxRetries)
	}
	if entry.Enabled {
		t.Error("Enabled = true, want false (Ola 2 flips this; this slice only registers)")
	}
	if entry.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want a non-zero UTC timestamp")
	}
	if entry.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt location = %v, want UTC", entry.CreatedAt.Location())
	}
}

// TestJobRegistration_Idempotent verifies that Register twice in a row
// produces exactly one registry row.
func TestJobRegistration_Idempotent(t *testing.T) {
	_, svc := openCodexRegistryTestHarness(t)
	ctx := context.Background()
	projID := domain.ProjectID("proj-codex")

	entry := JobRegistryEntry()

	if err := svc.Register(ctx, projID, entry); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	list, err := svc.ListRegistry(ctx)
	if err != nil {
		t.Fatalf("ListRegistry after first Register: %v", err)
	}
	if count := countJobName(list, entry.JobName); count != 1 {
		t.Fatalf("after first Register: count for %q = %d, want 1", entry.JobName, count)
	}

	if err := svc.Register(ctx, projID, entry); err != nil {
		t.Fatalf("second Register (idempotent): %v", err)
	}
	list, err = svc.ListRegistry(ctx)
	if err != nil {
		t.Fatalf("ListRegistry after second Register: %v", err)
	}
	if count := countJobName(list, entry.JobName); count != 1 {
		t.Fatalf("after second Register: count for %q = %d, want 1 (idempotent)", entry.JobName, count)
	}
	if len(list) != 1 {
		t.Fatalf("len(registry) = %d, want exactly 1 (only the codex entry)", len(list))
	}
}

func countJobName(list []jobs.JobRegistryEntry, name string) int {
	n := 0
	for _, e := range list {
		if e.JobName == name {
			n++
		}
	}
	return n
}

func openCodexRegistryTestHarness(t *testing.T) (*storage.DB, *jobs.Service) {
	t.Helper()
	wrapper := storagetest.OpenTemp(t)
	if err := storage.Migrate(wrapper); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := jobs.NewServiceWithDefaults(wrapper.DB)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := wrapper.DB.ExecContext(ctx,
		`INSERT OR IGNORE INTO projects (id, project_key, display_name, canonical_path, fingerprint, created_at, updated_at)
		 VALUES ('proj-codex', 'codex-reg', 'CodexReg', '/tmp/codex-reg', 'fp', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return wrapper, svc
}
