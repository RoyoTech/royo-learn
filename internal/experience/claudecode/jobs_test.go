package claudecode

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/jobs"
	"agent-royo-learn/internal/experience/semantic"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/storage/storagetest"
)

func TestClaudecodeJob_AccessorReturnsTypedJob(t *testing.T) {
	if NewAdapter().Job() == nil {
		t.Fatal("Job() returned nil")
	}
}

func TestClaudecodeJob_SourceMatches(t *testing.T) {
	if got := NewAdapter().Job().Source; got != string(domain.SourceClaudeCode) {
		t.Fatalf("Source = %q, want %q", got, domain.SourceClaudeCode)
	}
}

func TestClaudecodeJob_DistinctPerCall(t *testing.T) {
	a := NewAdapter()
	if a.Job() == a.Job() {
		t.Fatal("Job() returned the same pointer twice")
	}
}

func TestClaudecodeJob_FuncScansRealFixture(t *testing.T) {
	path, err := filepath.Abs("testdata/fixtures/session-001.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(path)
	result, err := NewAdapter().Job().Func(context.Background(), semantic.Deps{SourceInstance: SourceInstance{
		Source: domain.SourceClaudeCode, ProjectRoot: root, JSONLPath: path,
	}})
	if err != nil {
		t.Fatalf("Func: %v", err)
	}
	if len(result.Envelopes) != 2 || result.SkippedMalformed != 1 || result.SkippedIncomplete != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestClaudecodeJob_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewAdapter().Job().Func(ctx, semantic.Deps{})
	if err != context.Canceled {
		t.Fatalf("Func error = %v, want context.Canceled", err)
	}
}

// TestJobRegistryEntry_Shape pins the static fields so the registration
// helper is the single source of truth for the job name and default config
// (per docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md §2 Hito 8 jobs row).
func TestJobRegistryEntry_Shape(t *testing.T) {
	entry := newIngestJobRegistryEntry()

	if entry.JobName != "experience_ingest:claude_code" {
		t.Errorf("JobName = %q, want %q", entry.JobName, "experience_ingest:claude_code")
	}
	if entry.Description == "" {
		t.Error("Description is empty, want a non-empty summary")
	}
	if entry.DefaultIntervalSec != 300 {
		t.Errorf("DefaultIntervalSec = %d, want 300 (5 min, matches opencode precedent)", entry.DefaultIntervalSec)
	}
	if entry.DefaultMaxRetries != 3 {
		t.Errorf("DefaultMaxRetries = %d, want 3", entry.DefaultMaxRetries)
	}
	if entry.Enabled {
		t.Error("Enabled = true, want false (Hito 3 flips this; this PR only registers)")
	}
}

// TestJobRegistration_Idempotent verifies that Register twice in a row
// produces exactly one registry row (the engine's
// ON CONFLICT(job_name) DO UPDATE makes this safe per
// docs/22-ADAPTER-CONTRACT.md and docs/25 §2).
func TestJobRegistration_Idempotent(t *testing.T) {
	_, svc := openRegistryTestHarness(t)
	ctx := context.Background()
	projID := domain.ProjectID("proj-claudecode")

	entry := newIngestJobRegistryEntry()

	// First register — must succeed and surface the entry.
	if err := svc.Register(ctx, projID, entry); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	list, err := svc.ListRegistry(ctx)
	if err != nil {
		t.Fatalf("ListRegistry after first Register: %v", err)
	}
	count := countJobName(list, entry.JobName)
	if count != 1 {
		t.Fatalf("after first Register: count for %q = %d, want 1", entry.JobName, count)
	}

	// Second register — must succeed and NOT duplicate.
	if err := svc.Register(ctx, projID, entry); err != nil {
		t.Fatalf("second Register (idempotent): %v", err)
	}
	list, err = svc.ListRegistry(ctx)
	if err != nil {
		t.Fatalf("ListRegistry after second Register: %v", err)
	}
	count = countJobName(list, entry.JobName)
	if count != 1 {
		t.Fatalf("after second Register: count for %q = %d, want 1 (idempotent)", entry.JobName, count)
	}

	// Sanity: registry is otherwise empty for this project (we created
	// it fresh above).
	if len(list) != 1 {
		t.Fatalf("len(registry) = %d, want exactly 1 (only the claude_code entry)", len(list))
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

func openRegistryTestHarness(t *testing.T) (*storage.DB, *jobs.Service) {
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
		 VALUES ('proj-claudecode', 'claudecode-reg', 'ClaudeCodeReg', '/tmp/claudecode-reg', 'fp', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return wrapper, svc
}
