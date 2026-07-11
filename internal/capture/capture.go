package capture

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

// CaptureInput is the input for the Capture operation.
type CaptureInput struct {
	Title          string
	Context        string
	Observation    string
	Lesson         string
	Type           domain.LearningType
	Scope          domain.Scope
	Destination    domain.DestinationType
	Confidence     domain.Confidence
	EvidenceLevel  domain.EvidenceLevel
	Recommended    []string
	Limits         string
	RetrievalTerms []string
	Actor          domain.Actor
}

// CaptureResult is the output of a Capture operation.
type CaptureResult struct {
	LearningID domain.LearningID
	Status     domain.LearningStatus
	New        bool // false if deduplicated
}

// Service provides the capture operation.
type Service struct {
	db         *storage.DB
	recordsDir string
}

// NewService creates a new capture Service.
func NewService(db *storage.DB, recordsDir string) *Service {
	return &Service{db: db, recordsDir: recordsDir}
}

// Capture captures a new learning or returns an existing one by normalized hash.
func (s *Service) Capture(ctx context.Context, projectID domain.ProjectID, input *CaptureInput) (*CaptureResult, error) {
	if input == nil {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "capture input is nil")
	}

	if input.Title == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "title is required")
	}
	if input.Context == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "context is required")
	}
	if input.Observation == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "observation is required")
	}
	if input.Lesson == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "lesson is required")
	}

	// Apply defaults.
	learningType := input.Type
	if learningType == "" {
		learningType = domain.TypeProcedure
	}
	scope := input.Scope
	if scope == "" {
		scope = domain.ScopeProject
	}
	destination := input.Destination
	if destination == "" {
		destination = domain.DestProject
	}
	confidence := input.Confidence
	if confidence == "" {
		confidence = domain.ConfidenceMedium
	}
	evidenceLevel := input.EvidenceLevel
	if evidenceLevel == "" {
		evidenceLevel = domain.EvidenceInsufficient
	}

	now := time.Now().UTC()
	learning := &domain.Learning{
		ProjectID:            projectID,
		Status:               domain.StatusCaptured,
		Type:                 learningType,
		Title:                input.Title,
		Context:              input.Context,
		Observation:          input.Observation,
		ReusableLesson:       input.Lesson,
		RecommendedProcedure: input.Recommended,
		Limits:               input.Limits,
		ScopeGuess:           scope,
		Confidence:           confidence,
		EvidenceLevel:        evidenceLevel,
		ProposedDestination:  destination,
		RetrievalTerms:       input.RetrievalTerms,
		Fingerprint:          "", // set below after hash computation
		Actor:                input.Actor,
		Revision:             1,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	// Compute normalized hash.
	hash, err := domain.ComputeHash(learning)
	if err != nil {
		return nil, fmt.Errorf("capture: compute hash: %w", err)
	}
	learning.NormalizedHash = hash
	learning.Fingerprint = Fingerprint(learning)

	// Check for duplicate by normalized hash.
	existing, err := s.findExisting(ctx, projectID, hash)
	if err != nil {
		return nil, fmt.Errorf("capture: check existing: %w", err)
	}
	if existing != nil {
		return &CaptureResult{
			LearningID: existing.ID,
			Status:     existing.Status,
			New:        false,
		}, nil
	}

	// Assign ID.
	learning.ID = domain.LearningID(uuid.Must(uuid.NewV7()).String())

	// Validate.
	if err := domain.Validate(learning); err != nil {
		return nil, fmt.Errorf("capture: validate: %w", err)
	}

	// Save in transaction.
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("capture: begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	if err := storage.SaveLearning(ctx, tx, learning); err != nil {
		return nil, fmt.Errorf("capture: save learning: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("capture: commit: %w", err)
	}
	tx = nil

	// Record audit event (append-only, outside transaction is acceptable).
	auditEvt := &domain.AuditEvent{
		ID:            domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt:    now,
		Actor:         input.Actor,
		Operation:     "capture",
		EntityType:    "learning",
		EntityID:      string(learning.ID),
		PayloadSHA256: hash,
		Result:        "success",
	}
	if err := storage.RecordEvent(ctx, s.db.DB, auditEvt); err != nil {
		return nil, fmt.Errorf("capture: record audit: %w", err)
	}

	// Write Markdown record.
	if err := WriteRecord(s.recordsDir, learning); err != nil {
		return nil, fmt.Errorf("capture: write record: %w", err)
	}

	return &CaptureResult{
		LearningID: learning.ID,
		Status:     learning.Status,
		New:        true,
	}, nil
}

func (s *Service) findExisting(ctx context.Context, projectID domain.ProjectID, hash string) (*domain.Learning, error) {
	tx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	return storage.FindByHash(ctx, tx, projectID, hash)
}
