package capture

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/evidence"
	"agent-royo-learn/internal/record"
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

	// IdempotencyKey implements D5. The same key on a retry is a technical
	// retry: it creates neither a second learning nor a second copy of the
	// evidence.
	IdempotencyKey string

	// Evidence is persisted together with the learning, in one coherent
	// operation. Without it no learning captured through a public interface can
	// ever satisfy the D3 approval threshold.
	Evidence []evidence.Item
}

// CaptureResult is the output of a Capture operation.
type CaptureResult struct {
	LearningID  domain.LearningID
	Status      domain.LearningStatus
	New         bool // false if deduplicated or an idempotent retry
	EvidenceIDs []domain.EvidenceID
	Redacted    bool
}

// Service provides the capture operation.
type Service struct {
	db         *storage.DB
	recordsDir string
	evidence   *evidence.Service
}

// NewService creates a capture Service that cannot accept evidence.
//
// Prefer NewServiceWithEvidence on every public interface. A Service built here
// rejects any capture that carries evidence, rather than dropping it silently.
func NewService(db *storage.DB, recordsDir string) *Service {
	return &Service{db: db, recordsDir: recordsDir}
}

// NewServiceWithEvidence creates a capture Service wired to the evidence layer.
// This is the constructor the CLI and the MCP server use.
func NewServiceWithEvidence(db *storage.DB, recordsDir string, ev *evidence.Service) *Service {
	return &Service{db: db, recordsDir: recordsDir, evidence: ev}
}

// Capture captures a new learning, together with any evidence supplied with it.
//
// Order of operations matters and is load-bearing:
//
//	redact -> hash -> idempotency check -> dedup check -> prepare evidence -> persist
//
// Redaction runs first so that no sink — SQLite, the blob store, the Markdown
// record, the audit log or the returned result — can ever observe a secret, and
// so that the normalized hash is computed over redacted content and stays
// deterministic across retries.
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
	if len(input.Evidence) > 0 && s.evidence == nil {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			"capture: this service was built without an evidence store and cannot accept evidence")
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
		RecommendedProcedure: append([]string(nil), input.Recommended...),
		Limits:               input.Limits,
		ScopeGuess:           scope,
		Confidence:           confidence,
		EvidenceLevel:        evidenceLevel,
		ProposedDestination:  destination,
		RetrievalTerms:       append([]string(nil), input.RetrievalTerms...),
		Fingerprint:          "", // set below after hash computation
		Actor:                input.Actor,
		Revision:             1,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if input.IdempotencyKey != "" {
		key := input.IdempotencyKey
		learning.IdempotencyKey = &key
	}

	// Redaction BEFORE the hash, so deduplication is computed over redacted
	// content and a secret can never reach a sink through a learning field.
	evidence.RedactLearning(learning)

	// D5: the same idempotency key is a technical retry. Return the existing
	// learning and attach NO further evidence.
	if input.IdempotencyKey != "" {
		existing, err := s.findByIdempotencyKey(ctx, projectID, input.IdempotencyKey)
		if err != nil {
			return nil, fmt.Errorf("capture: check idempotency key: %w", err)
		}
		if existing != nil {
			return &CaptureResult{
				LearningID: existing.ID,
				Status:     existing.Status,
				New:        false,
			}, nil
		}
	}

	// Compute normalized hash over the redacted learning.
	hash, err := domain.ComputeHash(learning)
	if err != nil {
		return nil, fmt.Errorf("capture: compute hash: %w", err)
	}
	learning.NormalizedHash = hash
	learning.Fingerprint = Fingerprint(learning)

	// Conservative deduplication by content hash (D5, third rule): reuse the
	// learning and do NOT record a recurrence.
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

	// Prepare evidence: redaction and blob writes happen here, before SQLite.
	var records []*domain.Evidence
	if len(input.Evidence) > 0 {
		records, err = s.evidence.Prepare(learning.ID, input.Evidence, now)
		if err != nil {
			return nil, fmt.Errorf("capture: prepare evidence: %w", err)
		}
	}

	// Learning and evidence land in ONE transaction.
	if err := storage.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := storage.SaveLearning(ctx, tx, learning); err != nil {
			return fmt.Errorf("capture: save learning: %w", err)
		}
		if err := evidence.PersistTx(ctx, tx, records); err != nil {
			return fmt.Errorf("capture: save evidence: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Record audit event (append-only).
	auditEvt := &domain.AuditEvent{
		ID:            domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt:    now,
		Actor:         input.Actor,
		Operation:     "capture",
		EntityType:    "learning",
		EntityID:      string(learning.ID),
		PayloadSHA256: hash,
		Result:        "success",
		Details: map[string]any{
			"evidence_count": len(records),
		},
	}
	if err := storage.RecordEvent(ctx, s.db.DB, auditEvt); err != nil {
		return nil, fmt.Errorf("capture: record audit: %w", err)
	}

	// Write Markdown record.
	if err := record.WriteRecord(s.recordsDir, learning); err != nil {
		return nil, fmt.Errorf("capture: write record: %w", err)
	}

	return &CaptureResult{
		LearningID:  learning.ID,
		Status:      learning.Status,
		New:         true,
		EvidenceIDs: evidence.IDs(records),
		Redacted:    evidence.AnyRedacted(records),
	}, nil
}

// ---------------------------------------------------------------------------
// AddEvidence
// ---------------------------------------------------------------------------

// AddEvidenceInput is the input for attaching evidence to an existing learning.
type AddEvidenceInput struct {
	LearningID domain.LearningID
	Items      []evidence.Item
	Actor      domain.Actor

	// EvidenceLevel, when set, updates the learning's declared level in the same
	// operation. Without it a learning captured at the default `insufficient`
	// would stay unapprovable no matter how much real evidence it carries,
	// because the D3 threshold requires BOTH conditions.
	EvidenceLevel domain.EvidenceLevel
}

// AddEvidenceResult is the output of AddEvidence.
type AddEvidenceResult struct {
	LearningID    domain.LearningID
	EvidenceIDs   []domain.EvidenceID
	Count         int
	EvidenceLevel domain.EvidenceLevel
	Redacted      bool
}

// AddEvidence attaches evidence to an already-captured learning.
//
// This is the operation that makes the needs_evidence state usable. Without it a
// learning sent back for evidence could never return to approved through any
// public interface, which is precisely the defect this recorrido repairs.
func (s *Service) AddEvidence(ctx context.Context, projectID domain.ProjectID, input *AddEvidenceInput) (*AddEvidenceResult, error) {
	if input == nil {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "add evidence: input is nil")
	}
	if input.LearningID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "add evidence: learning_id is required")
	}
	if len(input.Items) == 0 {
		// Never report success for an attachment that attached nothing: the
		// caller would believe it had satisfied the approval threshold.
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			"add evidence: at least one evidence record is required")
	}
	if s.evidence == nil {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			"add evidence: this service was built without an evidence store")
	}
	if input.EvidenceLevel != "" && !domain.IsValidEvidenceLevel(input.EvidenceLevel) {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			fmt.Sprintf("add evidence: invalid evidence_level: %q", input.EvidenceLevel))
	}

	// Load the learning and verify ownership.
	tx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("add evidence: begin read tx: %w", err)
	}
	learning, err := storage.GetLearning(ctx, tx, input.LearningID)
	_ = tx.Rollback()
	if err != nil {
		return nil, fmt.Errorf("add evidence: get learning: %w", err)
	}
	if learning == nil {
		return nil, domain.NewNotFoundError(domain.ErrLearningNotFound, "learning: "+string(input.LearningID))
	}
	if learning.ProjectID != projectID {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			fmt.Sprintf("add evidence: learning %q belongs to project %q, not %q",
				input.LearningID, learning.ProjectID, projectID))
	}

	now := time.Now().UTC()

	// Redaction and blob writes, before SQLite.
	records, err := s.evidence.Prepare(learning.ID, input.Items, now)
	if err != nil {
		return nil, fmt.Errorf("add evidence: prepare: %w", err)
	}

	levelChanged := input.EvidenceLevel != "" && input.EvidenceLevel != learning.EvidenceLevel
	if levelChanged {
		learning.EvidenceLevel = input.EvidenceLevel
		learning.UpdatedAt = now
	}

	if err := storage.WithTx(ctx, s.db, func(wtx *sql.Tx) error {
		if err := evidence.PersistTx(ctx, wtx, records); err != nil {
			return fmt.Errorf("add evidence: save: %w", err)
		}
		if levelChanged {
			if err := storage.UpdateLearning(ctx, wtx, learning); err != nil {
				return fmt.Errorf("add evidence: update learning: %w", err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	auditEvt := &domain.AuditEvent{
		ID:         domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt: now,
		Actor:      input.Actor,
		Operation:  "add_evidence",
		EntityType: "learning",
		EntityID:   string(learning.ID),
		Result:     "success",
		Details: map[string]any{
			"evidence_count": len(records),
			"evidence_level": string(learning.EvidenceLevel),
		},
	}
	if err := storage.RecordEvent(ctx, s.db.DB, auditEvt); err != nil {
		return nil, fmt.Errorf("add evidence: record audit: %w", err)
	}

	// Keep the derived Markdown record in step with SQLite (D6).
	if levelChanged {
		if err := record.WriteRecord(s.recordsDir, learning); err != nil {
			return nil, fmt.Errorf("add evidence: write record: %w", err)
		}
	}

	return &AddEvidenceResult{
		LearningID:    learning.ID,
		EvidenceIDs:   evidence.IDs(records),
		Count:         len(records),
		EvidenceLevel: learning.EvidenceLevel,
		Redacted:      evidence.AnyRedacted(records),
	}, nil
}

// ListEvidence returns the evidence attached to a learning.
func (s *Service) ListEvidence(ctx context.Context, learningID domain.LearningID) ([]*domain.Evidence, error) {
	tx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("list evidence: begin tx: %w", err)
	}
	defer tx.Rollback()

	return storage.ListEvidenceByLearning(ctx, tx, learningID)
}

func (s *Service) findExisting(ctx context.Context, projectID domain.ProjectID, hash string) (*domain.Learning, error) {
	tx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	return storage.FindByHash(ctx, tx, projectID, hash)
}

func (s *Service) findByIdempotencyKey(ctx context.Context, projectID domain.ProjectID, key string) (*domain.Learning, error) {
	tx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	return storage.FindByIdempotencyKey(ctx, tx, projectID, key)
}
