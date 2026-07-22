package experience

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/evidence"
	projectpath "agent-royo-learn/internal/project"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

// DefaultMaxTurnBytes is the contract default for one complete turn payload.
const DefaultMaxTurnBytes int64 = 262144

// DefaultMaxCursorBytes bounds one source checkpoint before it is parsed.
const DefaultMaxCursorBytes int64 = 65536

// DefaultMaxLocatorBytes bounds local transcript locator metadata.
const DefaultMaxLocatorBytes int64 = 16384

// DefaultMaxErrorDetailsBytes bounds failure details written to operational sinks.
const DefaultMaxErrorDetailsBytes = 1024

const (
	DefaultFailureRecordingTimeout = 2 * time.Second
	maxFailureRecordingTimeout     = 5 * time.Second
)

// Config contains low-risk ingestion controls. Trust roots are not accepted
// here: the stored project's canonical path remains the boundary for locators.
type Config struct {
	MaxTurnBytes    int64
	MaxCursorBytes  int64
	MaxLocatorBytes int64
	KnownSecrets    []string
	Now             func() time.Time
	FailureTimeout  time.Duration
}

// IngestInput carries one envelope and, optionally, the source checkpoint that
// should be advanced after the data transaction commits.
type IngestInput struct {
	Envelope       ExperienceEnvelope
	SourceInstance string
	CursorJSON     string
	// SourceOrder is the adapter-supplied monotonic position of CursorJSON.
	// Zero is the valid first position; later checkpoints must be greater.
	SourceOrder int64
}

// IngestRequest is an alias for callers that prefer request terminology.
type IngestRequest = IngestInput

// IngestResult contains only redacted, bounded operational data. It never
// includes transcript text, tool output, or private reasoning.
type IngestResult struct {
	Session    *domain.ExperienceSession
	Turn       *domain.ExperienceTurn
	Cursor     *domain.IngestionCursor
	Created    bool
	Updated    bool
	Idempotent bool
	Superseded bool
	Redacted   bool
}

// Service ingests neutral experience envelopes into SQLite.
type Service struct {
	db       *storage.DB
	cfg      Config
	metrics  ingestionMetrics
	commitTx func(*sql.Tx) error
}

// MetricsSnapshot is a local, dependency-free view of ingestion health.
type MetricsSnapshot struct {
	Attempts              uint64        `json:"attempts"`
	Successes             uint64        `json:"successes"`
	Errors                uint64        `json:"errors"`
	FailureRecordingError uint64        `json:"failure_recording_errors"`
	CommitUnknowns        uint64        `json:"commit_unknowns"`
	TotalDuration         time.Duration `json:"total_duration"`
	LastDuration          time.Duration `json:"last_duration"`
}

type ingestionMetrics struct {
	attempts              atomic.Uint64
	successes             atomic.Uint64
	errors                atomic.Uint64
	failureRecordingError atomic.Uint64
	commitUnknowns        atomic.Uint64
	totalDuration         atomic.Int64
	lastDuration          atomic.Int64
}

// NewService creates an ingestion service. A zero or negative byte limit uses
// the contract default.
func NewService(db *storage.DB, configs ...Config) *Service {
	cfg := Config{}
	if len(configs) > 0 {
		cfg = configs[0]
	}
	if cfg.MaxTurnBytes <= 0 {
		cfg.MaxTurnBytes = DefaultMaxTurnBytes
	}
	if cfg.MaxCursorBytes <= 0 {
		cfg.MaxCursorBytes = DefaultMaxCursorBytes
	}
	if cfg.MaxCursorBytes > domain.MaxExperienceCursorBytes {
		cfg.MaxCursorBytes = domain.MaxExperienceCursorBytes
	}
	if cfg.MaxLocatorBytes <= 0 {
		cfg.MaxLocatorBytes = DefaultMaxLocatorBytes
	}
	cfg.KnownSecrets = append([]string(nil), cfg.KnownSecrets...)
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		db:  db,
		cfg: cfg,
		commitTx: func(tx *sql.Tx) error {
			return tx.Commit()
		},
	}
}

// NewIngestionService is an explicit constructor name for integration code.
func NewIngestionService(db *storage.DB, configs ...Config) *Service {
	return NewService(db, configs...)
}

// Ingest validates, redacts, fingerprints, deduplicates and persists one
// envelope. The data, audit and successful cursor decision share one SQLite
// transaction. Failure observation is written separately only after rollback.
func (s *Service) Ingest(ctx context.Context, projectID domain.ProjectID, input *IngestInput) (result *IngestResult, ingestErr error) {
	started := time.Now()
	var failureRecordErr error
	if s != nil {
		defer func() {
			s.recordMetrics(time.Since(started), ingestErr, failureRecordErr)
		}()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.db == nil || s.db.DB == nil {
		return nil, fmt.Errorf("experience: database is required")
	}
	if input == nil {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "experience ingest input is nil")
	}
	if projectID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "experience project id is required")
	}
	if len(string(projectID)) > domain.MaxExperienceIDBytes {
		return nil, domain.NewValidationError(domain.ErrExperiencePayloadTooLarge, "experience project id exceeds the permitted byte limit")
	}

	var failure *ingestionFailure
	var failureNow time.Time
	defer func() {
		if ingestErr == nil || failure == nil {
			return
		}
		if failureNow.IsZero() {
			failureNow = s.now()
		}
		failureRecordErr = s.recordFailure(ctx, failure, ingestErr, failureNow)
		if failureRecordErr != nil {
			// Keep both errors inspectable. The ingestion error remains the
			// primary failure; recording failure state is a second signal.
			ingestErr = errors.Join(ingestErr, fmt.Errorf("experience: record failure: %w", failureRecordErr))
		}
	}()

	if err := ValidateEnvelope(&input.Envelope); err != nil {
		return nil, err
	}

	// Validate size before redaction as a memory guard, then repeat it on the
	// safe copy because replacement markers can expand a payload.
	if err := s.validateTurnBytes(input.Envelope); err != nil {
		return nil, err
	}
	if err := s.validateLocatorBytes(input.Envelope); err != nil {
		return nil, err
	}
	redactedEnvelope, redacted, unsafeIdentity, err := s.prepareEnvelope(input.Envelope)
	if err != nil {
		return nil, err
	}
	if unsafeIdentity {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			"experience identity contains redacted content")
	}
	if err := ValidateEnvelope(&redactedEnvelope); err != nil {
		return nil, err
	}
	if err := s.validateTurnBytes(redactedEnvelope); err != nil {
		return nil, err
	}
	if err := s.validateLocatorBytes(redactedEnvelope); err != nil {
		return nil, err
	}
	if err := validateCursorInput(input); err != nil {
		return nil, err
	}
	var cursor *preparedCursor
	if input.SourceInstance != "" {
		cursor, err = s.prepareCursorWithOrder(input.SourceInstance, input.CursorJSON, input.SourceOrder)
		if err != nil {
			return nil, err
		}
	}

	now := s.now()
	failureNow = now

	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("experience: begin data transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	project, err := storage.GetProject(ctx, tx, projectID)
	if err != nil {
		return nil, fmt.Errorf("experience: validate project: %w", err)
	}
	failure = &ingestionFailure{
		projectID:     projectID,
		source:        redactedEnvelope.Source,
		cursor:        cursor,
		actor:         redactedEnvelope.Actor,
		sessionID:     redactedEnvelope.Session.ExternalID,
		turnID:        redactedEnvelope.Turn.ExternalID,
		payloadDigest: digestSafeEnvelope(redactedEnvelope),
	}
	if err := validateProjectAndLocator(project, redactedEnvelope.ProjectRoot, redactedEnvelope.Session.Locator); err != nil {
		return nil, err
	}

	result, changed, err := s.ingestDataTx(ctx, tx, projectID, redactedEnvelope, redacted, now)
	if err != nil {
		return nil, err
	}
	var cursorChanged bool
	if cursor != nil {
		cursorResult, changedCursor, cursorErr := s.advanceCursorTx(ctx, tx, projectID, redactedEnvelope.Source, cursor, now)
		if cursorErr != nil {
			return nil, fmt.Errorf("experience: advance cursor: %w", cursorErr)
		}
		result.Redacted = result.Redacted || cursor.redacted
		result.Cursor = cursorResult
		cursorChanged = changedCursor
		if cursorChanged {
			if err := recordCursorAudit(ctx, tx, redactedEnvelope.Actor, now, cursorResult, redacted || cursor.redacted); err != nil {
				return nil, fmt.Errorf("experience: audit cursor: %w", err)
			}
		}
	}

	if changed || cursorChanged {
		commit := s.commitTx
		if commit == nil {
			commit = func(tx *sql.Tx) error { return tx.Commit() }
		}
		if err := commit(tx); err != nil {
			// Release the transaction without interpreting the result. A commit
			// error can mean either durable success or rollback.
			_ = tx.Rollback()
			commitUnknownErr := domain.NewExperienceCommitUnknownError(err)
			commitAuditErr := s.recordCommitUnknown(ctx, failure, now)
			failure = nil
			if commitAuditErr != nil {
				commitUnknownErr = &domain.ConflictError{DomainError: &domain.DomainError{
					Code:        domain.ErrExperienceCommitUnknown,
					Message:     "experience ingestion commit outcome is unknown",
					Recoverable: true,
					NextAction:  "inspect the commit-unknown audit outcome and retry idempotently",
					Cause:       errors.Join(err, fmt.Errorf("experience: record commit-unknown audit: %w", commitAuditErr)),
				}}
			}
			return nil, commitUnknownErr
		}
	} else {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			return nil, fmt.Errorf("experience: rollback idempotent transaction: %w", err)
		}
	}
	return result, nil
}

// IngestEnvelope is a convenience wrapper for callers without a checkpoint.
func (s *Service) IngestEnvelope(ctx context.Context, projectID domain.ProjectID, envelope ExperienceEnvelope) (*IngestResult, error) {
	return s.Ingest(ctx, projectID, &IngestInput{Envelope: envelope})
}

func (s *Service) ingestDataTx(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, envelope ExperienceEnvelope, redacted bool, now time.Time) (*IngestResult, bool, error) {
	session, err := buildSession(projectID, envelope, now)
	if err != nil {
		return nil, false, err
	}

	existingSession, err := storage.FindExperienceSession(ctx, tx, projectID, envelope.Source, envelope.Session.ExternalID)
	if err != nil {
		return nil, false, fmt.Errorf("experience: find session: %w", err)
	}
	if existingSession != nil {
		session.ID = existingSession.ID
		session.CreatedAt = existingSession.CreatedAt
	}

	turn, err := buildTurn(session.ID, envelope, redacted, now)
	if err != nil {
		return nil, false, err
	}

	if existingSession == nil {
		if err := storage.SaveExperienceSession(ctx, tx, session); err != nil {
			return nil, false, fmt.Errorf("experience: save session: %w", err)
		}
		if err := storage.SaveExperienceTurn(ctx, tx, turn); err != nil {
			return nil, false, fmt.Errorf("experience: save turn: %w", err)
		}
		if err := recordSessionAudit(ctx, tx, envelope.Actor, now, session, redacted); err != nil {
			return nil, false, err
		}
		if err := recordTurnAudit(ctx, tx, envelope.Actor, now, turn, "experience_turn_ingested", "", redacted); err != nil {
			return nil, false, err
		}
		return &IngestResult{
			Session:  session,
			Turn:     turn,
			Created:  true,
			Redacted: redacted,
		}, true, nil
	}

	existingTurn, err := storage.FindExperienceTurn(ctx, tx, existingSession.ID, envelope.Turn.ExternalID)
	if err != nil {
		return nil, false, fmt.Errorf("experience: find turn: %w", err)
	}
	if existingTurn != nil {
		if existingTurn.Fingerprint == turn.Fingerprint {
			refreshedSession, sessionChanged, err := refreshSession(ctx, tx, session, existingSession)
			if err != nil {
				return nil, false, fmt.Errorf("experience: refresh session: %w", err)
			}
			if sessionChanged {
				if err := recordSessionUpdateAudit(ctx, tx, envelope.Actor, now, existingSession, refreshedSession, redacted); err != nil {
					return nil, false, err
				}
			}
			return &IngestResult{
				Session:    refreshedSession,
				Turn:       existingTurn,
				Updated:    sessionChanged,
				Idempotent: true,
				Redacted:   existingTurn.Redacted,
			}, sessionChanged, nil
		}
		if existingTurn.Status == domain.TurnSuperseded {
			return nil, false, domain.NewConflictError(domain.ErrExperienceRevisionConflict,
				"experience turn revision is superseded by an originating pattern")
		}
		if turn.SourceRevision == existingTurn.SourceRevision {
			return nil, false, domain.NewConflictError(domain.ErrExperienceRevisionConflict,
				"experience turn revision is unchanged but its fingerprint differs")
		}

		turn.ID = existingTurn.ID
		turn.SessionID = existingTurn.SessionID
		refreshedSession, sessionChanged, err := refreshSession(ctx, tx, session, existingSession)
		if err != nil {
			return nil, false, fmt.Errorf("experience: refresh session for revision: %w", err)
		}
		if sessionChanged {
			if err := recordSessionUpdateAudit(ctx, tx, envelope.Actor, now, existingSession, refreshedSession, redacted); err != nil {
				return nil, false, err
			}
		}
		if err := storage.UpdateExperienceTurn(ctx, tx, turn, existingTurn.SourceRevision); err != nil {
			return nil, false, fmt.Errorf("experience: update turn revision: %w", err)
		}
		if err := recordTurnAudit(ctx, tx, envelope.Actor, now, turn, "experience_turn_revised", existingTurn.SourceRevision, redacted); err != nil {
			return nil, false, err
		}
		return &IngestResult{
			Session:  refreshedSession,
			Turn:     turn,
			Updated:  true,
			Redacted: redacted,
		}, true, nil
	}

	refreshedSession, sessionChanged, err := refreshSession(ctx, tx, session, existingSession)
	if err != nil {
		return nil, false, fmt.Errorf("experience: refresh session: %w", err)
	}
	if sessionChanged {
		if err := recordSessionUpdateAudit(ctx, tx, envelope.Actor, now, existingSession, refreshedSession, redacted); err != nil {
			return nil, false, err
		}
	}
	if err := storage.SaveExperienceTurn(ctx, tx, turn); err != nil {
		return nil, false, fmt.Errorf("experience: save turn: %w", err)
	}
	if err := recordTurnAudit(ctx, tx, envelope.Actor, now, turn, "experience_turn_ingested", "", redacted); err != nil {
		return nil, false, err
	}
	return &IngestResult{
		Session:  refreshedSession,
		Turn:     turn,
		Created:  true,
		Redacted: redacted,
	}, true, nil
}

func refreshSession(ctx context.Context, tx *sql.Tx, candidate, existing *domain.ExperienceSession) (*domain.ExperienceSession, bool, error) {
	if existing == nil {
		return candidate, true, nil
	}
	candidate.ID = existing.ID
	candidate.CreatedAt = existing.CreatedAt
	if !candidate.UpdatedAt.After(existing.UpdatedAt) {
		return existing, false, nil
	}
	if err := storage.UpdateExperienceSession(ctx, tx, candidate, existing.UpdatedAt); err != nil {
		return nil, false, err
	}
	return candidate, true, nil
}

func (s *Service) advanceCursorTx(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, source domain.ExperienceSource, prepared *preparedCursor, now time.Time) (*domain.IngestionCursor, bool, error) {
	existing, err := storage.FindIngestionCursor(ctx, tx, projectID, source, prepared.sourceInstance)
	if err != nil {
		return nil, false, fmt.Errorf("find cursor: %w", err)
	}
	if existing != nil {
		switch {
		case existing.LastSuccessfulAt != nil && prepared.sourceOrder < existing.SourceOrder:
			return nil, false, domain.NewConflictError(domain.ErrExperienceCursorConflict,
				"ingestion cursor source order is stale")
		case prepared.sourceOrder == existing.SourceOrder:
			if existing.CursorJSON != prepared.cursorJSON || existing.InputDigest != prepared.inputDigest {
				return nil, false, domain.NewConflictError(domain.ErrExperienceCursorConflict,
					"ingestion cursor source order is unchanged but checkpoint differs")
			}
			if existing.LastSuccessfulAt != nil && existing.LastErrorCode == "" {
				return existing, false, nil
			}
		}
	}

	completed := now
	cursor := &domain.IngestionCursor{
		ProjectID:        projectID,
		Source:           source,
		SourceInstance:   prepared.sourceInstance,
		CursorJSON:       prepared.cursorJSON,
		LastSuccessfulAt: &completed,
		LastAttemptAt:    &completed,
		InputDigest:      prepared.inputDigest,
		Revision:         1,
		SourceOrder:      prepared.sourceOrder,
	}
	if existing == nil {
		if err := storage.SaveIngestionCursor(ctx, tx, cursor); err != nil {
			return nil, false, fmt.Errorf("save cursor: %w", err)
		}
	} else {
		cursor.Revision = existing.Revision + 1
		if err := storage.UpdateIngestionCursor(ctx, tx, cursor, existing.Revision); err != nil {
			return nil, false, fmt.Errorf("update cursor: %w", err)
		}
	}
	return cursor, true, nil
}

// advanceCursor is retained as a small transaction wrapper for package callers
// that need to exercise cursor persistence independently of ingestion.
func (s *Service) advanceCursor(ctx context.Context, projectID domain.ProjectID, source domain.ExperienceSource, prepared *preparedCursor, now time.Time) (*domain.IngestionCursor, error) {
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin cursor transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	cursor, _, err := s.advanceCursorTx(ctx, tx, projectID, source, prepared, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit cursor transaction: %w", err)
	}
	return cursor, nil
}

func (s *Service) prepareEnvelope(input ExperienceEnvelope) (ExperienceEnvelope, bool, bool, error) {
	output := input
	redacted := false
	unsafeIdentity := false
	redactField := func(value string, identity bool) string {
		clean, changed := s.redactTextFlags(value)
		if changed {
			redacted = true
			if identity {
				unsafeIdentity = true
			}
		}
		return clean
	}

	output.ProjectRoot = redactField(input.ProjectRoot, false)
	output.Session.ExternalID = redactField(input.Session.ExternalID, true)
	output.Session.Locator.Kind = redactField(input.Session.Locator.Kind, false)
	output.Session.Locator.Path = redactField(input.Session.Locator.Path, false)
	output.Session.Locator.SessionID = redactField(input.Session.Locator.SessionID, true)
	output.Session.Locator.TurnID = redactField(input.Session.Locator.TurnID, true)
	output.Session.Locator.SourceHash = redactField(input.Session.Locator.SourceHash, false)
	output.Session.UpdatedAt = normalizeTime(input.Session.UpdatedAt)
	output.Session.StartedAt = cloneTime(input.Session.StartedAt)
	output.Session.ClosedAt = cloneTime(input.Session.ClosedAt)
	output.Turn.ExternalID = redactField(input.Turn.ExternalID, true)
	output.Turn.FinishReason = redactField(input.Turn.FinishReason, false)
	output.Turn.OccurredAt = normalizeTime(input.Turn.OccurredAt)
	output.Turn.StableSince = cloneTime(input.Turn.StableSince)
	output.Turn.UserText = redactField(input.Turn.UserText, false)
	output.Turn.AssistantText = redactField(input.Turn.AssistantText, false)
	output.Turn.SourceRevision = redactField(input.Turn.SourceRevision, true)
	output.Actor.Kind = redactField(input.Actor.Kind, false)
	output.Actor.Name = redactField(input.Actor.Name, false)
	output.Actor.Model = redactField(input.Actor.Model, false)
	output.Actor.SessionID = redactField(input.Actor.SessionID, false)

	if input.Turn.ToolCalls != nil {
		output.Turn.ToolCalls = make([]SafeToolCall, len(input.Turn.ToolCalls))
	}
	for i, call := range input.Turn.ToolCalls {
		output.Turn.ToolCalls[i] = call
		output.Turn.ToolCalls[i].Name = redactField(call.Name, false)
		output.Turn.ToolCalls[i].Outcome = redactField(call.Outcome, false)
		output.Turn.ToolCalls[i].OutputHash = redactField(call.OutputHash, false)
		output.Turn.ToolCalls[i].OutputHint = redactField(call.OutputHint, false)
		args, argsRedacted, err := s.redactArguments(call.Arguments)
		if err != nil {
			return ExperienceEnvelope{}, false, false, fmt.Errorf("experience: redact tool call %d arguments: %w", i, err)
		}
		output.Turn.ToolCalls[i].Arguments = args
		redacted = redacted || argsRedacted
	}
	return output, redacted, unsafeIdentity, nil
}

func (s *Service) redactText(value string, redacted *bool) (string, error) {
	clean, changed := s.redactTextFlags(value)
	if changed {
		*redacted = true
	}
	return clean, nil
}

func (s *Service) redactTextFlags(value string) (string, bool) {
	var redacted []byte
	if len(s.cfg.KnownSecrets) == 0 {
		redacted = evidence.Redact([]byte(value), nil)
	} else {
		redacted = evidence.Redact([]byte(value), s.cfg.KnownSecrets)
	}
	return normalizeText(string(redacted)), string(redacted) != value
}

func (s *Service) redactArguments(arguments map[string]any) (map[string]any, bool, error) {
	if arguments == nil {
		return nil, false, nil
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil, false, err
	}
	value, err := decodeJSONUseNumber(encoded)
	if err != nil {
		return nil, false, err
	}
	clean, redacted, err := s.redactJSONValue(value)
	if err != nil {
		return nil, false, err
	}
	cleanMap, ok := clean.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("experience: JSON arguments must remain an object")
	}
	return cleanMap, redacted, nil
}

func (s *Service) redactJSONValue(value any) (any, bool, error) {
	switch typed := value.(type) {
	case string:
		redacted := false
		clean, err := s.redactText(typed, &redacted)
		return clean, redacted, err
	case []any:
		out := make([]any, len(typed))
		redacted := false
		for i, item := range typed {
			clean, changed, err := s.redactJSONValue(item)
			if err != nil {
				return nil, false, err
			}
			out[i] = clean
			redacted = redacted || changed
		}
		return out, redacted, nil
	case map[string]any:
		out := make(map[string]any, len(typed))
		redacted := false
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			item := typed[key]
			keyChanged := false
			cleanKey, err := s.redactText(key, &keyChanged)
			if err != nil {
				return nil, false, err
			}
			clean, changed, err := s.redactJSONValue(item)
			if err != nil {
				return nil, false, err
			}
			if _, exists := out[cleanKey]; exists {
				return nil, false, domain.NewValidationError(domain.ErrExperienceSchemaUnsupported,
					"experience JSON object contains colliding redacted keys")
			}
			out[cleanKey] = clean
			redacted = redacted || keyChanged || changed
		}
		return out, redacted, nil
	default:
		return value, false, nil
	}
}

func (s *Service) prepareCursor(sourceInstance, cursorJSON string) (*preparedCursor, error) {
	return s.prepareCursorWithOrder(sourceInstance, cursorJSON, 0)
}

func (s *Service) prepareCursorWithOrder(sourceInstance, cursorJSON string, sourceOrder int64) (*preparedCursor, error) {
	if strings.TrimSpace(sourceInstance) == "" || strings.TrimSpace(cursorJSON) == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			"experience cursor requires source instance and cursor JSON")
	}
	if sourceOrder < 0 {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			"experience cursor source order must be non-negative")
	}
	if len(sourceInstance) > domain.MaxExperienceSourceInstanceBytes {
		return nil, domain.NewValidationError(domain.ErrExperiencePayloadTooLarge,
			"experience cursor source instance exceeds the permitted byte limit")
	}
	if int64(len(cursorJSON)) > s.cfg.MaxCursorBytes {
		return nil, domain.NewValidationError(domain.ErrExperiencePayloadTooLarge,
			"experience cursor JSON exceeds configured byte limit")
	}
	raw, err := decodeJSONUseNumber([]byte(cursorJSON))
	if err != nil {
		return nil, domain.NewValidationError(domain.ErrExperienceSchemaUnsupported,
			"experience cursor JSON is invalid")
	}
	clean, redacted, err := s.redactJSONValue(raw)
	if err != nil {
		return nil, domain.NewValidationError(domain.ErrExperienceSchemaUnsupported,
			"experience cursor JSON is not supported")
	}
	encoded, err := json.Marshal(clean)
	if err != nil {
		return nil, domain.NewValidationError(domain.ErrExperienceSchemaUnsupported,
			"experience cursor JSON is not supported")
	}
	if int64(len(encoded)) > s.cfg.MaxCursorBytes {
		return nil, domain.NewValidationError(domain.ErrExperiencePayloadTooLarge,
			"experience cursor JSON exceeds configured byte limit")
	}
	cleanInstance, instanceChanged := s.redactTextFlags(sourceInstance)
	if cleanInstance == "" || instanceChanged {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			"experience cursor source instance is invalid")
	}
	return &preparedCursor{
		sourceInstance: cleanInstance,
		cursorJSON:     string(encoded),
		inputDigest:    digestString(string(encoded)),
		redacted:       redacted || instanceChanged,
		sourceOrder:    sourceOrder,
	}, nil
}

func validateCursorInput(input *IngestInput) error {
	hasInstance := strings.TrimSpace(input.SourceInstance) != ""
	hasCursor := strings.TrimSpace(input.CursorJSON) != ""
	if hasInstance != hasCursor {
		return domain.NewValidationError(domain.ErrInvalidArgument,
			"experience cursor requires source instance and cursor JSON together")
	}
	return nil
}

func (s *Service) validateTurnBytes(envelope ExperienceEnvelope) error {
	encoded, err := json.Marshal(envelope.Turn)
	if err != nil {
		return domain.NewValidationError(domain.ErrExperienceSchemaUnsupported,
			"experience turn payload is not JSON serializable")
	}
	if int64(len(encoded)) > s.cfg.MaxTurnBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge,
			"experience turn payload exceeds configured byte limit")
	}
	return nil
}

func (s *Service) validateLocatorBytes(envelope ExperienceEnvelope) error {
	encoded, err := json.Marshal(envelope.Session.Locator)
	if err != nil {
		return domain.NewValidationError(domain.ErrExperienceSchemaUnsupported,
			"experience locator metadata is not JSON serializable")
	}
	if int64(len(encoded)) > s.cfg.MaxLocatorBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge,
			"experience locator metadata exceeds configured byte limit")
	}
	return nil
}

func decodeJSONUseNumber(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}

func validateProjectAndLocator(project *domain.Project, envelopeRoot string, locator domain.TranscriptLocator) error {
	if project == nil || project.CanonicalPath == "" || envelopeRoot == "" {
		return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot,
			"experience project root is not trusted")
	}
	if !filepath.IsAbs(project.CanonicalPath) || !filepath.IsAbs(envelopeRoot) || !filepath.IsAbs(locator.Path) {
		return domain.NewValidationError(domain.ErrExperienceLocatorInvalid,
			"experience project and locator paths must be absolute")
	}
	storedRoot, err := projectpath.Canonicalize(project.CanonicalPath)
	if err != nil {
		return locatorPathError(err)
	}
	claimedRoot, err := projectpath.Canonicalize(envelopeRoot)
	if err != nil || !projectpath.IsInsideRoot(storedRoot, claimedRoot) || !projectpath.IsInsideRoot(claimedRoot, storedRoot) {
		return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot,
			"experience envelope project root does not match the stored project")
	}
	canonicalLocator, err := projectpath.Canonicalize(locator.Path)
	if err != nil {
		return locatorPathError(err)
	}
	if !projectpath.IsInsideRoot(canonicalLocator, storedRoot) {
		return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot,
			"experience locator is outside the stored project root")
	}
	if projectpath.IsProtectedPath(canonicalLocator) {
		return domain.NewValidationError(domain.ErrProtectedPath,
			"experience locator points to a protected path")
	}
	return nil
}

func locatorPathError(err error) error {
	var pathErr *projectpath.Error
	if errors.As(err, &pathErr) {
		switch pathErr.Code {
		case projectpath.ErrSymlinkEscape:
			return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot,
				"experience locator symlink escapes the stored project root")
		case projectpath.ErrPathOutsideRoot:
			return domain.NewValidationError(domain.ErrExperienceLocatorOutsideRoot,
				"experience locator path is outside the trusted root")
		}
	}
	return domain.NewValidationError(domain.ErrExperienceLocatorInvalid,
		"experience locator path is invalid")
}

func buildSession(projectID domain.ProjectID, envelope ExperienceEnvelope, now time.Time) (*domain.ExperienceSession, error) {
	updatedAt := envelope.Session.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	session := &domain.ExperienceSession{
		ID:                domain.ExperienceSessionID(uuid.Must(uuid.NewV7()).String()),
		ProjectID:         projectID,
		Source:            envelope.Source,
		ExternalSessionID: envelope.Session.ExternalID,
		Locator:           envelope.Session.Locator,
		StartedAt:         cloneTime(envelope.Session.StartedAt),
		UpdatedAt:         normalizeTime(updatedAt),
		ClosedAt:          cloneTime(envelope.Session.ClosedAt),
		CreatedAt:         now,
	}
	metadata, err := json.Marshal(struct {
		Source     domain.ExperienceSource  `json:"source"`
		ExternalID string                   `json:"external_session_id"`
		Locator    domain.TranscriptLocator `json:"locator"`
	}{
		Source:     session.Source,
		ExternalID: session.ExternalSessionID,
		Locator:    session.Locator,
	})
	if err != nil {
		return nil, fmt.Errorf("experience: encode session metadata: %w", err)
	}
	session.MetadataSHA256 = digestBytes(metadata)
	return session, nil
}

func buildTurn(sessionID domain.ExperienceSessionID, envelope ExperienceEnvelope, redacted bool, now time.Time) (*domain.ExperienceTurn, error) {
	toolCalls := envelope.Turn.ToolCalls
	if toolCalls == nil {
		toolCalls = make([]SafeToolCall, 0)
	}
	toolJSON, err := json.Marshal(toolCalls)
	if err != nil {
		return nil, fmt.Errorf("experience: encode tool calls: %w", err)
	}
	input := fingerprintInput{
		Source:          string(envelope.Source),
		ExternalSession: envelope.Session.ExternalID,
		ExternalTurn:    envelope.Turn.ExternalID,
		Sequence:        envelope.Turn.Sequence,
		UserDigest:      digestString(envelope.Turn.UserText),
		AssistantDigest: digestString(envelope.Turn.AssistantText),
		ToolCallsDigest: digestBytes(toolJSON),
		FinishReason:    envelope.Turn.FinishReason,
		Complete:        envelope.Turn.Complete,
		SourceRevision:  envelope.Turn.SourceRevision,
	}
	if input.SourceRevision == "" {
		input.SourceRevision = revisionSeed(input)
	}
	occurredAt := envelope.Turn.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = now
	}
	stableAt := cloneTime(envelope.Turn.StableSince)
	if stableAt == nil {
		stable := now
		stableAt = &stable
	}
	return &domain.ExperienceTurn{
		ID:              domain.ExperienceTurnID(uuid.Must(uuid.NewV7()).String()),
		SessionID:       sessionID,
		ExternalTurnID:  envelope.Turn.ExternalID,
		Sequence:        envelope.Turn.Sequence,
		Status:          domain.TurnIngested,
		Fingerprint:     fingerprint(input),
		UserDigest:      input.UserDigest,
		AssistantDigest: input.AssistantDigest,
		ToolCallsDigest: input.ToolCallsDigest,
		OccurredAt:      normalizeTime(occurredAt),
		StableAt:        stableAt,
		IngestedAt:      now,
		SourceRevision:  input.SourceRevision,
		Redacted:        redacted,
	}, nil
}

func recordSessionAudit(ctx context.Context, tx *sql.Tx, actor domain.Actor, now time.Time, session *domain.ExperienceSession, redacted bool) error {
	event := &domain.AuditEvent{
		ID:            domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt:    now,
		Actor:         actor,
		Operation:     "experience_session_discovered",
		EntityType:    "experience_session",
		EntityID:      string(session.ID),
		PayloadSHA256: session.MetadataSHA256,
		Result:        "success",
		Details: map[string]any{
			"project_id": string(session.ProjectID),
			"source":     string(session.Source),
			"redacted":   redacted,
		},
	}
	if err := storage.RecordEventTx(ctx, tx, event); err != nil {
		return fmt.Errorf("experience: audit session: %w", err)
	}
	return nil
}

func recordTurnAudit(ctx context.Context, tx *sql.Tx, actor domain.Actor, now time.Time, turn *domain.ExperienceTurn, operation, previousRevision string, redacted bool) error {
	var previousState, newState *string
	if previousRevision != "" {
		previousBytes, err := json.Marshal(map[string]string{"source_revision": previousRevision})
		if err != nil {
			return fmt.Errorf("experience: encode previous turn state: %w", err)
		}
		currentBytes, err := json.Marshal(map[string]string{"source_revision": turn.SourceRevision})
		if err != nil {
			return fmt.Errorf("experience: encode current turn state: %w", err)
		}
		previous := string(previousBytes)
		current := string(currentBytes)
		previousState, newState = &previous, &current
	}
	event := &domain.AuditEvent{
		ID:            domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt:    now,
		Actor:         actor,
		Operation:     operation,
		EntityType:    "experience_turn",
		EntityID:      string(turn.ID),
		PreviousState: previousState,
		NewState:      newState,
		PayloadSHA256: turn.Fingerprint,
		Result:        "success",
		Details: map[string]any{
			"session_id":      string(turn.SessionID),
			"external_turn":   turn.ExternalTurnID,
			"source_revision": turn.SourceRevision,
			"redacted":        redacted,
		},
	}
	if err := storage.RecordEventTx(ctx, tx, event); err != nil {
		return fmt.Errorf("experience: audit turn: %w", err)
	}
	return nil
}

func recordSessionUpdateAudit(ctx context.Context, tx *sql.Tx, actor domain.Actor, now time.Time, previous, current *domain.ExperienceSession, redacted bool) error {
	previousBytes, err := json.Marshal(map[string]string{
		"updated_at":      formatAuditTime(previous.UpdatedAt),
		"metadata_sha256": previous.MetadataSHA256,
	})
	if err != nil {
		return fmt.Errorf("experience: encode previous session state: %w", err)
	}
	currentBytes, err := json.Marshal(map[string]string{
		"updated_at":      formatAuditTime(current.UpdatedAt),
		"metadata_sha256": current.MetadataSHA256,
	})
	if err != nil {
		return fmt.Errorf("experience: encode current session state: %w", err)
	}
	previousState, newState := string(previousBytes), string(currentBytes)
	event := &domain.AuditEvent{
		ID:            domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt:    now,
		Actor:         actor,
		Operation:     "experience_session_updated",
		EntityType:    "experience_session",
		EntityID:      string(current.ID),
		PreviousState: &previousState,
		NewState:      &newState,
		PayloadSHA256: current.MetadataSHA256,
		Result:        "success",
		Details: map[string]any{
			"project_id": string(current.ProjectID),
			"source":     string(current.Source),
			"redacted":   redacted,
		},
	}
	if err := storage.RecordEventTx(ctx, tx, event); err != nil {
		return fmt.Errorf("experience: audit session update: %w", err)
	}
	return nil
}

func recordCursorAudit(ctx context.Context, tx *sql.Tx, actor domain.Actor, now time.Time, cursor *domain.IngestionCursor, redacted bool) error {
	if cursor == nil {
		return domain.NewValidationError(domain.ErrInvalidArgument, "experience cursor audit requires a cursor")
	}
	operation := "experience_cursor_advanced"
	var previousState *string
	if cursor.Revision == 1 {
		operation = "experience_cursor_created"
	} else {
		previous := fmt.Sprintf(`{"revision":%d}`, cursor.Revision-1)
		previousState = &previous
	}
	newState := fmt.Sprintf(`{"revision":%d,"source_order":%d,"input_digest":%q}`, cursor.Revision, cursor.SourceOrder, cursor.InputDigest)
	entityID := digestString(string(cursor.ProjectID) + "\x00" + string(cursor.Source) + "\x00" + cursor.SourceInstance)
	event := &domain.AuditEvent{
		ID:            domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt:    now,
		Actor:         actor,
		Operation:     operation,
		EntityType:    "ingestion_cursor",
		EntityID:      entityID,
		PreviousState: previousState,
		NewState:      &newState,
		PayloadSHA256: cursor.InputDigest,
		Result:        "success",
		Details: map[string]any{
			"project_id":   string(cursor.ProjectID),
			"source":       string(cursor.Source),
			"revision":     cursor.Revision,
			"source_order": cursor.SourceOrder,
			"redacted":     redacted,
		},
	}
	if err := storage.RecordEventTx(ctx, tx, event); err != nil {
		return fmt.Errorf("experience: audit cursor change: %w", err)
	}
	return nil
}

func formatAuditTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

type preparedCursor struct {
	sourceInstance string
	cursorJSON     string
	inputDigest    string
	redacted       bool
	sourceOrder    int64
}

type ingestionFailure struct {
	projectID     domain.ProjectID
	source        domain.ExperienceSource
	cursor        *preparedCursor
	actor         domain.Actor
	sessionID     string
	turnID        string
	payloadDigest string
}

func digestSafeEnvelope(envelope ExperienceEnvelope) string {
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return ""
	}
	return digestBytes(encoded)
}

func (s *Service) recordFailure(ctx context.Context, failure *ingestionFailure, ingestErr error, now time.Time) error {
	if failure == nil || failure.projectID == "" || !domain.IsValidExperienceSource(failure.source) {
		return nil
	}
	code, message := s.safeFailure(ingestErr)
	errorCode := code
	background, cancel := s.failureContext(ctx)
	defer cancel()
	tx, err := s.db.DB.BeginTx(background, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var cursorErr error
	if failure.cursor != nil {
		cursorErr = storage.RecordIngestionCursorFailure(
			background,
			tx,
			failure.projectID,
			failure.source,
			failure.cursor.sourceInstance,
			failure.cursor.sourceOrder,
			failure.cursor.cursorJSON,
			failure.cursor.inputDigest,
			code,
			message,
			now,
		)
	}
	if cursorErr == nil && failure.cursor != nil {
		if err := recordCursorFailureAudit(background, tx, failure.actor, now, failure, code, message); err != nil {
			return err
		}
	}

	event := &domain.AuditEvent{
		ID:            domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt:    now,
		Actor:         failure.actor,
		Operation:     "experience_ingestion_failed",
		EntityType:    "experience_ingestion",
		EntityID:      digestString(failure.sessionID + "\x00" + failure.turnID),
		PayloadSHA256: failure.payloadDigest,
		Result:        "error",
		ErrorCode:     &errorCode,
		Details: map[string]any{
			"source":  string(failure.source),
			"message": message,
		},
	}
	if err := storage.RecordEventTx(background, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return cursorErr
}

func recordCursorFailureAudit(ctx context.Context, tx *sql.Tx, actor domain.Actor, now time.Time, failure *ingestionFailure, code, message string) error {
	if failure == nil || failure.cursor == nil {
		return nil
	}
	errorCode := code
	entityID := digestString(string(failure.projectID) + "\x00" + string(failure.source) + "\x00" + failure.cursor.sourceInstance)
	event := &domain.AuditEvent{
		ID:            domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt:    now,
		Actor:         actor,
		Operation:     "experience_cursor_failure_recorded",
		EntityType:    "ingestion_cursor",
		EntityID:      entityID,
		PayloadSHA256: failure.cursor.inputDigest,
		Result:        "error",
		ErrorCode:     &errorCode,
		Details: map[string]any{
			"source":       string(failure.source),
			"source_order": failure.cursor.sourceOrder,
			"message":      message,
		},
	}
	if err := storage.RecordEventTx(ctx, tx, event); err != nil {
		return fmt.Errorf("experience: audit cursor failure: %w", err)
	}
	return nil
}

func (s *Service) recordCommitUnknown(ctx context.Context, failure *ingestionFailure, now time.Time) error {
	if failure == nil || failure.projectID == "" || !domain.IsValidExperienceSource(failure.source) {
		return nil
	}
	background, cancel := s.failureContext(ctx)
	defer cancel()
	tx, err := s.db.DB.BeginTx(background, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	errorCode := string(domain.ErrExperienceCommitUnknown)
	event := &domain.AuditEvent{
		ID:            domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt:    now,
		Actor:         failure.actor,
		Operation:     "experience_ingestion_commit_unknown",
		EntityType:    "experience_ingestion",
		EntityID:      digestString(failure.sessionID + "\x00" + failure.turnID),
		PayloadSHA256: failure.payloadDigest,
		Result:        "commit_unknown",
		ErrorCode:     &errorCode,
		Details: map[string]any{
			"source":  string(failure.source),
			"message": "experience ingestion commit outcome is unknown",
		},
	}
	if err := storage.RecordEventTx(background, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) failureContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := s.cfg.FailureTimeout
	if timeout <= 0 {
		timeout = DefaultFailureRecordingTimeout
	}
	if timeout > maxFailureRecordingTimeout {
		timeout = maxFailureRecordingTimeout
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

func (s *Service) recordMetrics(duration time.Duration, ingestErr, failureRecordErr error) {
	s.metrics.attempts.Add(1)
	if ingestErr == nil {
		s.metrics.successes.Add(1)
	} else {
		s.metrics.errors.Add(1)
		if domainErr, ok := domain.AsDomainError(ingestErr); ok && domainErr.Code == domain.ErrExperienceCommitUnknown {
			s.metrics.commitUnknowns.Add(1)
		}
	}
	if failureRecordErr != nil {
		s.metrics.failureRecordingError.Add(1)
	}
	nanos := duration.Nanoseconds()
	s.metrics.totalDuration.Add(nanos)
	s.metrics.lastDuration.Store(nanos)
}

// Metrics returns a point-in-time local snapshot. It performs no I/O.
func (s *Service) Metrics() MetricsSnapshot {
	if s == nil {
		return MetricsSnapshot{}
	}
	return MetricsSnapshot{
		Attempts:              s.metrics.attempts.Load(),
		Successes:             s.metrics.successes.Load(),
		Errors:                s.metrics.errors.Load(),
		FailureRecordingError: s.metrics.failureRecordingError.Load(),
		CommitUnknowns:        s.metrics.commitUnknowns.Load(),
		TotalDuration:         time.Duration(s.metrics.totalDuration.Load()),
		LastDuration:          time.Duration(s.metrics.lastDuration.Load()),
	}
}

func (s *Service) safeFailure(err error) (string, string) {
	code := "internal_error"
	message := "experience ingestion failed"
	if domainErr, ok := domain.AsDomainError(err); ok {
		code = string(domainErr.Code)
		message = domainErr.Message
	}
	if len(code) > domain.MaxExperienceErrorCodeBytes {
		code = "internal_error"
	}
	redacted := evidence.Redact([]byte(message), s.cfg.KnownSecrets)
	return code, boundErrorDetails(normalizeText(string(redacted)))
}

func boundErrorDetails(value string) string {
	if len(value) <= DefaultMaxErrorDetailsBytes {
		return value
	}
	value = string([]rune(value))
	for len(value) > DefaultMaxErrorDetailsBytes-3 {
		value = value[:len(value)-1]
		value = strings.ToValidUTF8(value, "")
	}
	return value + "..."
}

func normalizeText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimSpace(value)
}

func normalizeTime(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return value.UTC()
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := normalizeTime(*value)
	return &copy
}

func (s *Service) now() time.Time {
	now := s.cfg.Now()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return now.UTC()
}
