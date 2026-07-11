package curate

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

// RelateInput is the input for creating a relation between two learnings.
type RelateInput struct {
	SourceLearningID domain.LearningID
	TargetLearningID domain.LearningID
	RelationType     domain.RelationType
	Confidence       *float64
	Rationale        string
	Actor            domain.Actor
}

// RelateResult is the output of a relation creation.
type RelateResult struct {
	RelationID domain.RelationID
}

// Relate creates a semantic relationship between two learnings.
func (s *Service) Relate(ctx context.Context, input *RelateInput) (*RelateResult, error) {
	if input == nil {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "relate: input is nil")
	}
	if input.SourceLearningID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "relate: source_learning_id is required")
	}
	if input.TargetLearningID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "relate: target_learning_id is required")
	}
	if input.RelationType == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "relate: relation_type is required")
	}

	// Prevent self-relations.
	if input.SourceLearningID == input.TargetLearningID {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			"relate: source and target must be different learnings (no self-relations)")
	}

	// Validate that both learnings exist.
	tx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("relate: begin read tx: %w", err)
	}

	source, err := storage.GetLearning(ctx, tx, input.SourceLearningID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("relate: source learning: %w", err)
	}
	if source == nil {
		tx.Rollback()
		return nil, domain.NewNotFoundError(domain.ErrLearningNotFound, "source learning: "+string(input.SourceLearningID))
	}

	target, err := storage.GetLearning(ctx, tx, input.TargetLearningID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("relate: target learning: %w", err)
	}
	if target == nil {
		tx.Rollback()
		return nil, domain.NewNotFoundError(domain.ErrLearningNotFound, "target learning: "+string(input.TargetLearningID))
	}
	tx.Rollback()

	// Check for duplicate relation.
	existingRelations, err := s.listExistingRelations(ctx, input.SourceLearningID, input.TargetLearningID)
	if err != nil {
		return nil, fmt.Errorf("relate: check existing: %w", err)
	}
	for _, rel := range existingRelations {
		if rel.Relation == input.RelationType {
			return nil, domain.NewConflictError(domain.ErrDuplicateLearning,
				fmt.Sprintf("relation of type %q already exists between %q and %q",
					input.RelationType, input.SourceLearningID, input.TargetLearningID))
		}
	}

	now := time.Now().UTC()
	relation := &domain.LearningRelation{
		ID:               domain.RelationID(uuid.Must(uuid.NewV7()).String()),
		SourceLearningID: input.SourceLearningID,
		TargetLearningID: input.TargetLearningID,
		Relation:         input.RelationType,
		Confidence:       input.Confidence,
		Rationale:        input.Rationale,
		Actor:            input.Actor,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// Save relation.
	if err := storage.WithTx(ctx, s.db, func(wtx *sql.Tx) error {
		if err := storage.SaveRelation(ctx, wtx, relation); err != nil {
			return fmt.Errorf("relate: save relation: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Record audit event.
	auditEvt := &domain.AuditEvent{
		ID:         domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt: now,
		Actor:      input.Actor,
		Operation:  "relate",
		EntityType: "learning_relation",
		EntityID:   string(relation.ID),
		Result:     "success",
		Details: map[string]any{
			"source_learning_id": string(input.SourceLearningID),
			"target_learning_id": string(input.TargetLearningID),
			"relation_type":      string(input.RelationType),
			"rationale":          input.Rationale,
		},
	}
	if err := storage.RecordEvent(ctx, s.db.DB, auditEvt); err != nil {
		return nil, fmt.Errorf("relate: record audit: %w", err)
	}

	return &RelateResult{
		RelationID: relation.ID,
	}, nil
}

// listExistingRelations checks for existing relations between two learnings
// in either direction.
func (s *Service) listExistingRelations(ctx context.Context, a, b domain.LearningID) ([]*domain.LearningRelation, error) {
	tx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	outbound, err := storage.ListRelationsBySource(ctx, tx, a)
	if err != nil {
		return nil, err
	}

	var matches []*domain.LearningRelation
	for _, r := range outbound {
		if r.TargetLearningID == b {
			matches = append(matches, r)
		}
	}
	return matches, nil
}
