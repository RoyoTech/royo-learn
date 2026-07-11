package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"agent-royo-learn/internal/domain"
)

// SaveLearning inserts a new learning. Returns a ConflictError on duplicate ID.
func SaveLearning(ctx context.Context, tx *sql.Tx, l *domain.Learning) error {
	recProcJSON := marshalStringSlice(l.RecommendedProcedure)
	retrievalJSON := marshalStringSlice(l.RetrievalTerms)
	approvedDestJSON := "{}"
	if l.ApprovedDestination != nil {
		approvedDestJSON = marshalAny(l.ApprovedDestination)
	}
	approvedScope := (*string)(nil)
	if l.ApprovedScope != nil {
		s := string(*l.ApprovedScope)
		approvedScope = &s
	}
	idempotencyKey := (*string)(nil)
	if l.IdempotencyKey != nil {
		idempotencyKey = l.IdempotencyKey
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO learnings (
			id, project_id, status, type, title, context, observation,
			reusable_lesson, recommended_procedure_json, limits_text,
			scope_guess, approved_scope, confidence, evidence_level,
			proposed_destination, approved_destination_json, retrieval_terms_text,
			fingerprint, normalized_hash, idempotency_key,
			actor_json, revision, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		string(l.ID),
		string(l.ProjectID),
		string(l.Status),
		string(l.Type),
		l.Title,
		l.Context,
		l.Observation,
		l.ReusableLesson,
		recProcJSON,
		l.Limits,
		string(l.ScopeGuess),
		approvedScope,
		string(l.Confidence),
		string(l.EvidenceLevel),
		string(l.ProposedDestination),
		approvedDestJSON,
		retrievalJSON,
		l.Fingerprint,
		l.NormalizedHash,
		idempotencyKey,
		l.Actor.ActorJSON(),
		l.Revision,
		l.CreatedAt.Format(time.RFC3339),
		l.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.NewConflictError(domain.ErrDuplicateLearning, "learning already exists: "+string(l.ID))
		}
		return fmt.Errorf("SaveLearning: %w", err)
	}
	return nil
}

// GetLearning retrieves a learning by ID.
func GetLearning(ctx context.Context, tx *sql.Tx, id domain.LearningID) (*domain.Learning, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT
			id, project_id, status, type, title, context, observation,
			reusable_lesson, recommended_procedure_json, limits_text,
			scope_guess, approved_scope, confidence, evidence_level,
			proposed_destination, approved_destination_json, retrieval_terms_text,
			fingerprint, normalized_hash, idempotency_key,
			actor_json, revision, created_at, updated_at
		FROM learnings WHERE id = ?
	`, string(id))
	return scanLearning(row)
}

// ListLearnings returns learnings for a project, with optional filters.
func ListLearnings(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, filter domain.LearningFilter) ([]*domain.Learning, error) {
	var clauses []string
	var args []interface{}

	clauses = append(clauses, "project_id = ?")
	args = append(args, string(projectID))

	if len(filter.Status) > 0 {
		placeholders := make([]string, len(filter.Status))
		for i, s := range filter.Status {
			placeholders[i] = "?"
			args = append(args, string(s))
		}
		clauses = append(clauses, fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.Type) > 0 {
		placeholders := make([]string, len(filter.Type))
		for i, t := range filter.Type {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		clauses = append(clauses, fmt.Sprintf("type IN (%s)", strings.Join(placeholders, ",")))
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	query := fmt.Sprintf(`
		SELECT
			id, project_id, status, type, title, context, observation,
			reusable_lesson, recommended_procedure_json, limits_text,
			scope_guess, approved_scope, confidence, evidence_level,
			proposed_destination, approved_destination_json, retrieval_terms_text,
			fingerprint, normalized_hash, idempotency_key,
			actor_json, revision, created_at, updated_at
		FROM learnings
		WHERE %s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, strings.Join(clauses, " AND "))

	args = append(args, limit, filter.Offset)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListLearnings: %w", err)
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

// UpdateLearning updates an existing learning.
func UpdateLearning(ctx context.Context, tx *sql.Tx, l *domain.Learning) error {
	recProcJSON := marshalStringSlice(l.RecommendedProcedure)
	retrievalJSON := marshalStringSlice(l.RetrievalTerms)
	approvedDestJSON := "{}"
	if l.ApprovedDestination != nil {
		approvedDestJSON = marshalAny(l.ApprovedDestination)
	}
	approvedScope := (*string)(nil)
	if l.ApprovedScope != nil {
		s := string(*l.ApprovedScope)
		approvedScope = &s
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE learnings SET
			status = ?, type = ?, title = ?, context = ?, observation = ?,
			reusable_lesson = ?, recommended_procedure_json = ?, limits_text = ?,
			scope_guess = ?, approved_scope = ?, confidence = ?, evidence_level = ?,
			proposed_destination = ?, approved_destination_json = ?, retrieval_terms_text = ?,
			fingerprint = ?, normalized_hash = ?, idempotency_key = ?,
			actor_json = ?, revision = ?, updated_at = ?
		WHERE id = ?
	`,
		string(l.Status), string(l.Type), l.Title, l.Context, l.Observation,
		l.ReusableLesson, recProcJSON, l.Limits,
		string(l.ScopeGuess), approvedScope, string(l.Confidence), string(l.EvidenceLevel),
		string(l.ProposedDestination), approvedDestJSON, retrievalJSON,
		l.Fingerprint, l.NormalizedHash, l.IdempotencyKey,
		l.Actor.ActorJSON(), l.Revision, l.UpdatedAt.Format(time.RFC3339),
		string(l.ID),
	)
	if err != nil {
		return fmt.Errorf("UpdateLearning: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return domain.NewNotFoundError(domain.ErrLearningNotFound, fmt.Sprintf("learning %q", l.ID))
	}
	return nil
}

// FindByHash looks up a learning by project_id + normalized_hash.
func FindByHash(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, hash string) (*domain.Learning, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT
			id, project_id, status, type, title, context, observation,
			reusable_lesson, recommended_procedure_json, limits_text,
			scope_guess, approved_scope, confidence, evidence_level,
			proposed_destination, approved_destination_json, retrieval_terms_text,
			fingerprint, normalized_hash, idempotency_key,
			actor_json, revision, created_at, updated_at
		FROM learnings WHERE project_id = ? AND normalized_hash = ?
	`, string(projectID), hash)
	l, err := scanLearning(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // not found is not an error for this method
		}
		return nil, fmt.Errorf("FindByHash: %w", err)
	}
	return l, nil
}

// SaveRevision inserts a point-in-time snapshot of a learning payload.
func SaveRevision(ctx context.Context, db *sql.DB, learningID domain.LearningID, revision int, payload domain.Learning, sha256 string) error {
	payloadJSON := marshalAny(payload)
	_, err := db.ExecContext(ctx, `
		INSERT INTO learning_revisions (id, learning_id, revision, payload_json, payload_sha256, created_at, created_by_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		domain.LearningID("rev-"+string(learningID)+"-"+fmt.Sprintf("%d", revision)),
		string(learningID),
		revision,
		payloadJSON,
		sha256,
		payload.CreatedAt.Format(time.RFC3339),
		payload.Actor.ActorJSON(),
	)
	if err != nil {
		return fmt.Errorf("SaveRevision: %w", err)
	}
	return nil
}

// GetRevisions returns all revisions for a learning, ordered by revision.
func GetRevisions(ctx context.Context, db *sql.DB, learningID domain.LearningID) ([]*domain.Revision, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, learning_id, revision, payload_json, payload_sha256, created_at, created_by_json
		FROM learning_revisions
		WHERE learning_id = ?
		ORDER BY revision ASC
	`, string(learningID))
	if err != nil {
		return nil, fmt.Errorf("GetRevisions: %w", err)
	}
	defer rows.Close()

	var out []*domain.Revision
	for rows.Next() {
		r := &domain.Revision{}
		var createdAt, createdByJSON string
		if err := rows.Scan(
			&r.ID,
			(*string)(&r.LearningID),
			&r.Revision,
			&createdAt, // placeholder for payload_json
			&r.PayloadSHA256,
			&createdAt,
			&createdByJSON,
		); err != nil {
			return nil, fmt.Errorf("GetRevisions scan: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		out = append(out, r)
	}
	return out, rows.Err()
}

// scanLearning scans a single row into a *Learning.
func scanLearning(row interface{ Scan(...interface{}) error }) (*domain.Learning, error) {
	l := &domain.Learning{}
	var (
		createdAt, updatedAt, actorJSON string
		approvedScope, idempotencyKey   *string
		recProcJSON, retrievalJSON      string
		approvedDestJSON                string
	)

	err := row.Scan(
		(*string)(&l.ID),
		(*string)(&l.ProjectID),
		(*string)(&l.Status),
		(*string)(&l.Type),
		&l.Title,
		&l.Context,
		&l.Observation,
		&l.ReusableLesson,
		&recProcJSON,
		&l.Limits,
		(*string)(&l.ScopeGuess),
		&approvedScope,
		(*string)(&l.Confidence),
		(*string)(&l.EvidenceLevel),
		(*string)(&l.ProposedDestination),
		&approvedDestJSON,
		&retrievalJSON,
		&l.Fingerprint,
		&l.NormalizedHash,
		&idempotencyKey,
		&actorJSON,
		&l.Revision,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	l.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	l.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	l.Actor = parseActor(actorJSON)
	l.IdempotencyKey = idempotencyKey
	l.RecommendedProcedure = unmarshalStringSlice(recProcJSON)
	l.RetrievalTerms = unmarshalStringSlice(retrievalJSON)

	if approvedScope != nil {
		s := domain.Scope(*approvedScope)
		l.ApprovedScope = &s
	}

	return l, nil
}

// scanLearningFromRows scans a learning from *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanLearningFromRows(rows scanner) (*domain.Learning, error) {
	l := &domain.Learning{}
	var (
		createdAt, updatedAt, actorJSON string
		approvedScope, idempotencyKey   *string
		recProcJSON, retrievalJSON      string
		approvedDestJSON                string
	)

	err := rows.Scan(
		(*string)(&l.ID),
		(*string)(&l.ProjectID),
		(*string)(&l.Status),
		(*string)(&l.Type),
		&l.Title,
		&l.Context,
		&l.Observation,
		&l.ReusableLesson,
		&recProcJSON,
		&l.Limits,
		(*string)(&l.ScopeGuess),
		&approvedScope,
		(*string)(&l.Confidence),
		(*string)(&l.EvidenceLevel),
		(*string)(&l.ProposedDestination),
		&approvedDestJSON,
		&retrievalJSON,
		&l.Fingerprint,
		&l.NormalizedHash,
		&idempotencyKey,
		&actorJSON,
		&l.Revision,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scanLearningFromRows: %w", err)
	}

	l.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	l.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	l.Actor = parseActor(actorJSON)
	l.IdempotencyKey = idempotencyKey
	l.RecommendedProcedure = unmarshalStringSlice(recProcJSON)
	l.RetrievalTerms = unmarshalStringSlice(retrievalJSON)

	if approvedScope != nil {
		s := domain.Scope(*approvedScope)
		l.ApprovedScope = &s
	}

	return l, nil
}

// parseActor parses an Actor from its JSON representation.
func parseActor(raw string) domain.Actor {
	a := domain.Actor{}
	if raw == "" || raw == "{}" {
		return a
	}
	json.Unmarshal([]byte(raw), &a)
	return a
}

// unmarshalStringSlice parses a JSON array of strings.
func unmarshalStringSlice(raw string) []string {
	if raw == "" || raw == "[]" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
