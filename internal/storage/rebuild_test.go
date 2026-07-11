package storage

import (
	"context"
	"testing"

	"agent-royo-learn/internal/domain"
)

func saveLearningForRebuild(t *testing.T, db *DB, projectID domain.ProjectID, title string) *domain.Learning {
	t.Helper()
	learning := newTestLearning(projectID)
	learning.Title = title
	tx, err := db.DB.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := SaveLearning(context.Background(), tx, learning); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return learning
}

func TestRebuildSearchIndexRestoresCanonicalRowsAndIsIdempotent(t *testing.T) {
	db, project := setupTestDB(t)
	ctx := context.Background()
	learning := saveLearningForRebuild(t, db, project.ID, "Unicode café security")
	if _, err := db.DB.ExecContext(ctx, `INSERT INTO audit_events(id, occurred_at, actor_json, operation, entity_type, entity_id, payload_sha256, result) VALUES('audit-1','2026-01-01T00:00:00Z','{}','test','learning',?,'hash','success')`, learning.ID); err != nil {
		t.Fatalf("insert audit fixture: %v", err)
	}
	if _, err := db.DB.ExecContext(ctx, "DELETE FROM learnings_fts"); err != nil {
		t.Fatalf("corrupt FTS fixture: %v", err)
	}
	for run := 1; run <= 2; run++ {
		count, err := RebuildSearchIndex(ctx, db)
		if err != nil {
			t.Fatalf("RebuildSearchIndex run %d: %v", run, err)
		}
		if count != 1 {
			t.Fatalf("RebuildSearchIndex run %d count = %d, want 1", run, count)
		}
	}
	results, err := Search(ctx, db, project.ID, "café")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].ID != learning.ID {
		t.Fatalf("Search results = %#v, want learning %s", results, learning.ID)
	}
	var indexed int
	if err := db.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM learnings_fts").Scan(&indexed); err != nil {
		t.Fatalf("count FTS rows: %v", err)
	}
	if indexed != 1 {
		t.Fatalf("FTS row count = %d after repeated rebuild, want 1", indexed)
	}
	if err := db.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events").Scan(&indexed); err != nil || indexed != 1 {
		t.Fatalf("audit history after rebuild = %d, %v; want 1, nil", indexed, err)
	}
}

func TestRebuildSearchIndexRollsBackOnInsertFailure(t *testing.T) {
	db, project := setupTestDB(t)
	ctx := context.Background()
	saveLearningForRebuild(t, db, project.ID, "blocked")
	fixtureSQL := []string{
		"DROP TRIGGER learnings_ai",
		"DROP TRIGGER learnings_au",
		"DROP TRIGGER learnings_ad",
		"DROP TABLE learnings_fts",
		`CREATE TABLE learnings_fts (
			learning_id TEXT, project_key TEXT, title TEXT CHECK(title <> 'blocked'),
			context TEXT, observation TEXT, reusable_lesson TEXT, retrieval_terms TEXT
		)`,
		`INSERT INTO learnings_fts VALUES ('sentinel', 'sentinel', 'safe', '', '', '', '')`,
	}
	for _, statement := range fixtureSQL {
		if _, err := db.DB.ExecContext(ctx, statement); err != nil {
			t.Fatalf("failure fixture %q: %v", statement, err)
		}
	}
	if _, err := RebuildSearchIndex(ctx, db); err == nil {
		t.Fatal("RebuildSearchIndex succeeded despite destination constraint")
	}
	var id string
	if err := db.DB.QueryRowContext(ctx, "SELECT learning_id FROM learnings_fts").Scan(&id); err != nil {
		t.Fatalf("read rolled-back FTS row: %v", err)
	}
	if id != "sentinel" {
		t.Fatalf("FTS row after failed rebuild = %q, want sentinel", id)
	}
}
