package storage

import (
	"context"
	"fmt"
	"strings"

	"agent-royo-learn/internal/domain"
)

// Search performs a full-text search on learnings using FTS5.
// The query is sanitized to prevent FTS5 syntax errors.
func Search(ctx context.Context, db *DB, projectID domain.ProjectID, query string) ([]*domain.Learning, error) {
	if query == "" {
		return nil, nil
	}

	sanitized := sanitizeFTS(query)
	if sanitized == "" {
		return nil, nil
	}

	rows, err := db.DB.QueryContext(ctx, `
		SELECT
			l.id, l.project_id, l.status, l.type, l.title, l.context, l.observation,
			l.reusable_lesson, l.recommended_procedure_json, l.limits_text,
			l.scope_guess, l.approved_scope, l.confidence, l.evidence_level,
			l.proposed_destination, l.approved_destination_json, l.retrieval_terms_text,
			l.fingerprint, l.normalized_hash, l.idempotency_key,
			l.actor_json, l.revision, l.created_at, l.updated_at
		FROM learnings l
		JOIN learnings_fts ON learnings_fts.learning_id = l.id
		WHERE learnings_fts MATCH ? AND l.project_id = ?
		ORDER BY rank
		LIMIT 20
	`, sanitized, string(projectID))
	if err != nil {
		return nil, fmt.Errorf("Search: %w", err)
	}
	defer rows.Close()

	var out []*domain.Learning
	for rows.Next() {
		l, err := scanLearningFromRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// sanitizeFTS escapes special FTS5 characters and quotes each term.
// This prevents SQL injection via FTS5 syntax and ensures safe queries.
func sanitizeFTS(query string) string {
	// Remove or escape FTS5 special characters: * ^ " ( ) : ~
	replacer := strings.NewReplacer(
		"*", "",
		"^", "",
		`"`, "",
		"(", "",
		")", "",
		":", "",
		"~", "",
		"AND", "",
		"OR", "",
		"NOT", "",
		"NEAR", "",
	)
	cleaned := replacer.Replace(query)

	// Split into terms and quote each one.
	terms := strings.Fields(cleaned)
	if len(terms) == 0 {
		return ""
	}

	quoted := make([]string, len(terms))
	for i, term := range terms {
		quoted[i] = `"` + strings.TrimSpace(term) + `"`
	}
	return strings.Join(quoted, " ")
}
