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

// SuggestSimilar returns learnings that share salient terms with the query,
// OR-matched for recall and ranked by FTS5 relevance. It is the read side of the
// plan 4.5 similar-candidate suggestion during capture: broad recall so a human
// can judge, never a decision. excludeID drops the just-captured learning.
func SuggestSimilar(ctx context.Context, db *DB, projectID domain.ProjectID, query string, excludeID domain.LearningID, limit int) ([]*domain.Learning, error) {
	if query == "" {
		return nil, nil
	}
	match := orMatchExpr(query)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
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
		WHERE learnings_fts MATCH ? AND l.project_id = ? AND l.id <> ?
		ORDER BY rank
		LIMIT ?
	`, match, string(projectID), string(excludeID), limit)
	if err != nil {
		return nil, fmt.Errorf("SuggestSimilar: %w", err)
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

// orMatchExpr builds an FTS5 OR expression from a free-text query. It drops FTS5
// operators and short tokens (length < 4) to keep the suggestion focused on
// salient words, and caps the term count so the MATCH stays bounded.
func orMatchExpr(query string) string {
	replacer := strings.NewReplacer(
		"*", "", "^", "", `"`, "", "(", "", ")", "", ":", "", "~", "",
		"AND", "", "OR", "", "NOT", "", "NEAR", "",
	)
	cleaned := replacer.Replace(query)

	seen := make(map[string]bool)
	var terms []string
	for _, term := range strings.Fields(cleaned) {
		t := strings.ToLower(strings.TrimSpace(term))
		if len(t) < 4 || seen[t] {
			continue
		}
		seen[t] = true
		terms = append(terms, `"`+t+`"`)
		if len(terms) >= 16 {
			break
		}
	}
	if len(terms) == 0 {
		return ""
	}
	return strings.Join(terms, " OR ")
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
