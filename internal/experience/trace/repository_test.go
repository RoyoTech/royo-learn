package trace

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/storage/storagetest"
)

func openTraceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	wrapper := storagetest.OpenTemp(t)
	if err := storage.Migrate(wrapper); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return wrapper.DB
}

func setupTraceTest(t *testing.T, db *sql.DB) (domain.LearningID, domain.ExperienceEventID) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	// Disable FK checks so we can insert fixture rows without satisfying
	// the full FK chain (experience_events -> experience_turns -> experience_sessions).
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable FK: %v", err)
	}
	t.Cleanup(func() {
		db.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
	})

	projectID := "proj-trace-test"
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO projects (id, project_key, display_name, canonical_path, fingerprint, created_at, updated_at)
		VALUES (?, 'trace-test', 'Trace', '/tmp/trace', 'fp', ?, ?)`, projectID, now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}

	learningID := domain.LearningID("learn-001")
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO learnings (id, project_id, title, context, observation, reusable_lesson, type, scope_guess, proposed_destination, confidence, evidence_level, idempotency_key, normalized_hash, status, fingerprint, actor_json, created_at, updated_at)
		VALUES (?, ?, 'title', 'ctx', 'obs', 'lesson', 'procedure', 'project', 'none', 'medium', 'insufficient', 'key', 'nh', 'draft', 'fp', '{}', ?, ?)`,
		string(learningID), projectID, now, now); err != nil {
		t.Fatalf("insert learning: %v", err)
	}

	eventID := domain.ExperienceEventID("ev-001")
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO experience_events (id, project_id, turn_id, kind, summary, observation, outcome, fingerprint, evidence_json, detector_json, confidence, created_at)
		VALUES (?, ?, 'turn-1', 'test_failure', 'summary', 'obs', 'fail', 'fp-ev', '{}', '{}', 'low', ?)`,
		string(eventID), projectID, now); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	return learningID, eventID
}

func TestSaveAndFindLearningEvent(t *testing.T) {
	db := openTraceTestDB(t)
	learningID, eventID := setupTraceTest(t, db)

	r := NewRepository(db)
	ctx := context.Background()

	// Save
	le := LearningEvent{
		LearningID:       learningID,
		EventID:          eventID,
		RelationshipType: RelationSource,
	}
	if err := r.SaveLearningEvent(ctx, le); err != nil {
		t.Fatalf("SaveLearningEvent: %v", err)
	}

	// Idempotent re-save
	if err := r.SaveLearningEvent(ctx, le); err != nil {
		t.Fatalf("SaveLearningEvent (idempotent): %v", err)
	}

	// Find by learning
	events, err := r.FindEventsByLearning(ctx, learningID)
	if err != nil {
		t.Fatalf("FindEventsByLearning: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("FindEventsByLearning returned %d events, want 1", len(events))
	}
	if events[0].EventID != eventID {
		t.Errorf("EventID = %q, want %q", events[0].EventID, eventID)
	}
	if events[0].RelationshipType != RelationSource {
		t.Errorf("RelationshipType = %q, want %q", events[0].RelationshipType, RelationSource)
	}

	// Find by event
	learnings, err := r.FindLearningsByEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("FindLearningsByEvent: %v", err)
	}
	if len(learnings) != 1 {
		t.Fatalf("FindLearningsByEvent returned %d learnings, want 1", len(learnings))
	}
}

func TestFindEventsByLearning_Empty(t *testing.T) {
	db := openTraceTestDB(t)
	r := NewRepository(db)
	ctx := context.Background()

	events, err := r.FindEventsByLearning(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("FindEventsByLearning: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestService_Trace_Integration(t *testing.T) {
	db := openTraceTestDB(t)
	learningID, eventID := setupTraceTest(t, db)

	// Link learning to event
	r := NewRepository(db)
	if err := r.SaveLearningEvent(context.Background(), LearningEvent{
		LearningID:       learningID,
		EventID:          eventID,
		RelationshipType: RelationSource,
	}); err != nil {
		t.Fatalf("SaveLearningEvent: %v", err)
	}

	// Trace
	svc := NewService(db)
	result, err := svc.Trace(context.Background(), learningID, TraceBounds{})
	if err != nil {
		t.Fatalf("Trace: %v", err)
	}
	if result.LearningID != learningID {
		t.Errorf("LearningID = %q, want %q", result.LearningID, learningID)
	}
	if len(result.Events) != 1 {
		t.Fatalf("Trace returned %d events, want 1", len(result.Events))
	}
	if result.Events[0].Event.ID != eventID {
		t.Errorf("Event.ID = %q, want %q", result.Events[0].Event.ID, eventID)
	}
	if result.Summary.TotalEvents != 1 {
		t.Errorf("Summary.TotalEvents = %d, want 1", result.Summary.TotalEvents)
	}
}

func TestService_Trace_WithExcerpt(t *testing.T) {
	db := openTraceTestDB(t)
	learningID, eventID := setupTraceTest(t, db)

	r := NewRepository(db)
	if err := r.SaveLearningEvent(context.Background(), LearningEvent{
		LearningID:       learningID,
		EventID:          eventID,
		RelationshipType: RelationSource,
	}); err != nil {
		t.Fatalf("SaveLearningEvent: %v", err)
	}

	svc := NewService(db)
	result, err := svc.Trace(context.Background(), learningID, TraceBounds{
		IncludeExcerpt:  true,
		MaxExcerptBytes: 50,
	})
	if err != nil {
		t.Fatalf("Trace with excerpt: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("Trace returned %d events, want 1", len(result.Events))
	}
	if result.Events[0].Excerpt == "" {
		t.Error("expected excerpt when IncludeExcerpt is true")
	}
	if result.Summary.WithExcerpts != 1 {
		t.Errorf("Summary.WithExcerpts = %d, want 1", result.Summary.WithExcerpts)
	}
}
