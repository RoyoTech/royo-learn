package capture

import (
	"context"
	"database/sql"
	"testing"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/testutil"
)

// TestCapture_SuggestsSimilarButNeverDecides proves plan 4.5: capture uses FTS5
// to SUGGEST similar existing learnings, but never autonomously decides two
// learnings are equivalent (no relation is created automatically).
func TestCapture_SuggestsSimilarButNeverDecides(t *testing.T) {
	db, proj := setupCaptureDB(t)
	svc := NewService(db, testutil.TempDir(t))
	ctx := context.Background()

	// Seed an existing learning about connection pool exhaustion.
	first, err := svc.Capture(ctx, proj.ID, &CaptureInput{
		Title:       "Database connection pool exhausted under load",
		Context:     "High-traffic endpoint",
		Observation: "The service ran out of database connections during a spike",
		Lesson:      "Bound the connection pool and add backpressure",
		Type:        domain.TypeProcedure,
		Scope:       domain.ScopeProject,
		Actor:       domain.Actor{Kind: "agent", Name: "bot"},
	})
	if err != nil {
		t.Fatalf("seed capture: %v", err)
	}

	// Capture a distinct-but-related learning; the wording overlaps so FTS5 can
	// surface the first as a candidate.
	res, err := svc.Capture(ctx, proj.ID, &CaptureInput{
		Title:       "Connection pool tuning for the database",
		Context:     "Another endpoint",
		Observation: "Database connections were exhausted again during load",
		Lesson:      "Set a max pool size and fail fast",
		Type:        domain.TypeProcedure,
		Scope:       domain.ScopeProject,
		Actor:       domain.Actor{Kind: "agent", Name: "bot"},
	})
	if err != nil {
		t.Fatalf("second capture: %v", err)
	}
	if !res.New {
		t.Fatal("second capture is distinct content and should be New")
	}

	// It must SUGGEST the earlier learning as a similar candidate.
	found := false
	for _, c := range res.SimilarCandidates {
		if c.LearningID == first.LearningID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the earlier learning %q among similar candidates, got %+v", first.LearningID, res.SimilarCandidates)
	}

	// It must NOT autonomously decide they are equivalent: no relation exists.
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()
	rels, err := storage.ListRelationsBySource(ctx, tx, res.LearningID)
	if err != nil {
		t.Fatalf("ListRelationsBySource: %v", err)
	}
	if len(rels) != 0 {
		t.Fatalf("capture created %d relation(s) autonomously; it must only suggest", len(rels))
	}
}
