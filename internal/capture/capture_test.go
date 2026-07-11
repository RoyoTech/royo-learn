package capture

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

// setupCaptureDB creates a temporary DB with migrations and a test project.
func setupCaptureDB(t *testing.T) (*storage.DB, *domain.Project) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	proj := &domain.Project{
		ID:            domain.ProjectID(uuid.Must(uuid.NewV7()).String()),
		ProjectKey:    "capture-test",
		DisplayName:   "Capture Test",
		CanonicalPath: t.TempDir(),
		GitRemote:     "",
		Fingerprint:   "fp-cap-proj",
	}

	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := storage.SaveProject(ctx, tx, proj); err != nil {
		tx.Rollback()
		t.Fatalf("SaveProject: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	return db, proj
}

func TestCaptureNewLearning(t *testing.T) {
	t.Parallel()

	db, proj := setupCaptureDB(t)
	recordsDir := t.TempDir()
	svc := NewService(db, recordsDir)

	input := &CaptureInput{
		Title:       "New Test Learning",
		Context:     "Testing basic capture",
		Observation: "Basic capture works",
		Lesson:      "Test everything",
		Type:        domain.TypeProcedure,
		Scope:       domain.ScopeProject,
		Actor:       domain.Actor{Kind: "agent", Name: "test-agent", Model: "test-model", SessionID: "sess-001"},
	}

	result, err := svc.Capture(context.Background(), proj.ID, input)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if result.LearningID == "" {
		t.Fatal("Capture returned empty LearningID")
	}
	if result.Status != domain.StatusCaptured {
		t.Errorf("Status = %q, want %q", result.Status, domain.StatusCaptured)
	}
	if !result.New {
		t.Error("Expected New = true for first capture")
	}
}

func TestCaptureDedupExact(t *testing.T) {
	t.Parallel()

	db, proj := setupCaptureDB(t)
	recordsDir := t.TempDir()
	svc := NewService(db, recordsDir)

	input := &CaptureInput{
		Title:       "Dedup Test",
		Context:     "Same content twice",
		Observation: "This should deduplicate",
		Lesson:      "Dedup works",
		Type:        domain.TypeProcedure,
		Scope:       domain.ScopeProject,
		Actor:       domain.Actor{Kind: "agent", Name: "test-agent", Model: "test-model", SessionID: "sess-001"},
	}

	result1, err := svc.Capture(context.Background(), proj.ID, input)
	if err != nil {
		t.Fatalf("Capture 1: %v", err)
	}
	if !result1.New {
		t.Error("First capture should be New = true")
	}

	result2, err := svc.Capture(context.Background(), proj.ID, input)
	if err != nil {
		t.Fatalf("Capture 2: %v", err)
	}
	if result2.New {
		t.Error("Second capture with same input should be New = false (dedup)")
	}
	if result1.LearningID != result2.LearningID {
		t.Errorf("Dedup should return same ID: %q vs %q", result1.LearningID, result2.LearningID)
	}
}

func TestCaptureWithDifferentInputDifferentID(t *testing.T) {
	t.Parallel()

	db, proj := setupCaptureDB(t)
	recordsDir := t.TempDir()
	svc := NewService(db, recordsDir)

	input1 := &CaptureInput{
		Title:       "Learning A",
		Context:     "Context A",
		Observation: "Observation A",
		Lesson:      "Lesson A",
		Type:        domain.TypeProcedure,
		Scope:       domain.ScopeProject,
		Actor:       domain.Actor{Kind: "agent", Name: "test-agent", Model: "test-model", SessionID: "sess-001"},
	}

	input2 := &CaptureInput{
		Title:       "Learning B",
		Context:     "Context B",
		Observation: "Observation B",
		Lesson:      "Lesson B",
		Type:        domain.TypeDiagnostic,
		Scope:       domain.ScopeProject,
		Actor:       domain.Actor{Kind: "agent", Name: "test-agent", Model: "test-model", SessionID: "sess-002"},
	}

	result1, _ := svc.Capture(context.Background(), proj.ID, input1)
	result2, _ := svc.Capture(context.Background(), proj.ID, input2)

	if result1.LearningID == result2.LearningID {
		t.Error("Different inputs should produce different IDs")
	}
}

func TestCaptureMissingTitle(t *testing.T) {
	t.Parallel()

	db, proj := setupCaptureDB(t)
	recordsDir := t.TempDir()
	svc := NewService(db, recordsDir)

	input := &CaptureInput{
		Title:       "",
		Context:     "context",
		Observation: "observation",
		Lesson:      "lesson",
		Type:        domain.TypeProcedure,
		Scope:       domain.ScopeProject,
		Actor:       domain.Actor{Kind: "agent", Name: "test-agent", Model: "test-model", SessionID: "sess-001"},
	}

	_, err := svc.Capture(context.Background(), proj.ID, input)
	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestCaptureNilInput(t *testing.T) {
	t.Parallel()

	db, proj := setupCaptureDB(t)
	recordsDir := t.TempDir()
	svc := NewService(db, recordsDir)

	_, err := svc.Capture(context.Background(), proj.ID, nil)
	if err == nil {
		t.Fatal("expected error for nil input")
	}
}

func TestCaptureRecordWritten(t *testing.T) {
	t.Parallel()

	db, proj := setupCaptureDB(t)
	recordsDir := t.TempDir()
	svc := NewService(db, recordsDir)

	input := &CaptureInput{
		Title:       "Record Test",
		Context:     "Context for record test",
		Observation: "Record observation",
		Lesson:      "Write records on capture",
		Type:        domain.TypeProcedure,
		Scope:       domain.ScopeProject,
		Actor:       domain.Actor{Kind: "agent", Name: "test-agent", Model: "test-model", SessionID: "sess-001"},
	}

	result, err := svc.Capture(context.Background(), proj.ID, input)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	recordPath := filepath.Join(recordsDir, string(result.LearningID)+".md")
	if _, err := os.Stat(recordPath); err != nil {
		t.Fatalf("record file not found at %s: %v", recordPath, err)
	}
}
