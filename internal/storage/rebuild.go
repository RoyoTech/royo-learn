package storage

import (
	"context"
	"fmt"
)

// RebuildSearchIndex replaces FTS rows transactionally from canonical tables.
func RebuildSearchIndex(ctx context.Context, db *DB) (int64, error) {
	if db == nil || db.DB == nil {
		return 0, fmt.Errorf("storage: rebuild search index: nil database")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("storage: rebuild search index: begin: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM learnings_fts"); err != nil {
		return 0, fmt.Errorf("storage: rebuild search index: clear: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO learnings_fts(
			learning_id, project_key, title, context, observation, reusable_lesson, retrieval_terms
		)
		SELECT
			l.id, p.project_key, l.title, l.context, l.observation,
			l.reusable_lesson, l.retrieval_terms_text
		FROM learnings l
		JOIN projects p ON p.id = l.project_id
	`)
	if err != nil {
		return 0, fmt.Errorf("storage: rebuild search index: populate: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("storage: rebuild search index: count: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("storage: rebuild search index: commit: %w", err)
	}
	return count, nil
}
