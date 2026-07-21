package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// SaveExperienceSession inserts a validated experience session.
func SaveExperienceSession(ctx context.Context, tx *sql.Tx, session *domain.ExperienceSession) error {
	if err := domain.ValidateExperienceSession(session); err != nil {
		return err
	}
	locatorJSON, err := json.Marshal(session.Locator)
	if err != nil {
		return fmt.Errorf("SaveExperienceSession: marshal locator: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO experience_sessions
			(id, project_id, source, external_session_id, locator_json, started_at,
			 updated_at, closed_at, metadata_sha256, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, string(session.ID), string(session.ProjectID), string(session.Source), session.ExternalSessionID,
		string(locatorJSON), nullableTime(session.StartedAt), formatTime(session.UpdatedAt),
		nullableTime(session.ClosedAt), session.MetadataSHA256, formatTime(session.CreatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return domain.NewConflictError(domain.ErrExperienceRevisionConflict, "experience session identity already exists")
		}
		return fmt.Errorf("SaveExperienceSession: %w", err)
	}
	return nil
}

// FindExperienceSession returns the session with the external identity, or nil.
func FindExperienceSession(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, source domain.ExperienceSource, externalID string) (*domain.ExperienceSession, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, project_id, source, external_session_id, locator_json, started_at,
		       updated_at, closed_at, metadata_sha256, created_at
		FROM experience_sessions
		WHERE project_id = ? AND source = ? AND external_session_id = ?
	`, string(projectID), string(source), externalID)
	session, err := scanExperienceSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("FindExperienceSession: %w", err)
	}
	return session, nil
}

// UpdateExperienceSession refreshes mutable session metadata without changing identity.
func UpdateExperienceSession(ctx context.Context, tx *sql.Tx, session *domain.ExperienceSession) error {
	if err := domain.ValidateExperienceSession(session); err != nil {
		return err
	}
	locatorJSON, err := json.Marshal(session.Locator)
	if err != nil {
		return fmt.Errorf("UpdateExperienceSession: marshal locator: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE experience_sessions
		SET locator_json = ?, started_at = ?, updated_at = ?, closed_at = ?, metadata_sha256 = ?
		WHERE id = ?
	`, string(locatorJSON), nullableTime(session.StartedAt), formatTime(session.UpdatedAt),
		nullableTime(session.ClosedAt), session.MetadataSHA256, string(session.ID))
	if err != nil {
		return fmt.Errorf("UpdateExperienceSession: %w", err)
	}
	return requireExperienceUpdate(result, "experience session")
}

// SaveExperienceTurn inserts a validated experience turn.
func SaveExperienceTurn(ctx context.Context, tx *sql.Tx, turn *domain.ExperienceTurn) error {
	if err := domain.ValidateExperienceTurn(turn); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO experience_turns
			(id, session_id, external_turn_id, sequence, status, fingerprint, user_digest,
			 assistant_digest, tool_calls_digest, safe_summary, occurred_at, stable_at,
			 ingested_at, source_revision, redacted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, string(turn.ID), string(turn.SessionID), turn.ExternalTurnID, turn.Sequence, string(turn.Status),
		turn.Fingerprint, turn.UserDigest, turn.AssistantDigest, turn.ToolCallsDigest, turn.SafeSummary,
		formatTime(turn.OccurredAt), nullableTime(turn.StableAt), formatTime(turn.IngestedAt),
		turn.SourceRevision, boolToInt(turn.Redacted))
	if err != nil {
		if isUniqueViolation(err) {
			return domain.NewConflictError(domain.ErrExperienceRevisionConflict, "experience turn identity already exists")
		}
		return fmt.Errorf("SaveExperienceTurn: %w", err)
	}
	return nil
}

// FindExperienceTurn returns the turn with the external identity, or nil.
func FindExperienceTurn(ctx context.Context, tx *sql.Tx, sessionID domain.ExperienceSessionID, externalID string) (*domain.ExperienceTurn, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, session_id, external_turn_id, sequence, status, fingerprint, user_digest,
		       assistant_digest, tool_calls_digest, safe_summary, occurred_at, stable_at,
		       ingested_at, source_revision, redacted
		FROM experience_turns WHERE session_id = ? AND external_turn_id = ?
	`, string(sessionID), externalID)
	turn, err := scanExperienceTurn(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("FindExperienceTurn: %w", err)
	}
	return turn, nil
}

// UpdateExperienceTurn stores a new revision of an existing turn identity.
func UpdateExperienceTurn(ctx context.Context, tx *sql.Tx, turn *domain.ExperienceTurn) error {
	if err := domain.ValidateExperienceTurn(turn); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE experience_turns
		SET sequence = ?, status = ?, fingerprint = ?, user_digest = ?, assistant_digest = ?,
		    tool_calls_digest = ?, safe_summary = ?, occurred_at = ?, stable_at = ?, ingested_at = ?,
		    source_revision = ?, redacted = ?
		WHERE id = ?
	`, turn.Sequence, string(turn.Status), turn.Fingerprint, turn.UserDigest, turn.AssistantDigest,
		turn.ToolCallsDigest, turn.SafeSummary, formatTime(turn.OccurredAt), nullableTime(turn.StableAt),
		formatTime(turn.IngestedAt), turn.SourceRevision, boolToInt(turn.Redacted), string(turn.ID))
	if err != nil {
		return fmt.Errorf("UpdateExperienceTurn: %w", err)
	}
	return requireExperienceUpdate(result, "experience turn")
}

// SaveExperienceEvent inserts a validated event and its detector identity.
func SaveExperienceEvent(ctx context.Context, tx *sql.Tx, event *domain.ExperienceEvent) error {
	if err := domain.ValidateExperienceEvent(event); err != nil {
		return err
	}
	var turnProjectID string
	err := tx.QueryRowContext(ctx, `
		SELECT s.project_id
		FROM experience_turns t
		JOIN experience_sessions s ON s.id = t.session_id
		WHERE t.id = ?
	`, string(event.TurnID)).Scan(&turnProjectID)
	if err == nil && turnProjectID != string(event.ProjectID) {
		return domain.NewConflictError(domain.ErrExperienceRevisionConflict, "experience event project does not match turn project")
	}
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("SaveExperienceEvent: check turn provenance: %w", err)
	}
	detectorJSON, err := json.Marshal(event.Detector)
	if err != nil {
		return fmt.Errorf("SaveExperienceEvent: marshal detector: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO experience_events
			(id, project_id, turn_id, kind, summary, observation, outcome, fingerprint,
			 evidence_json, detector_json, confidence, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, string(event.ID), string(event.ProjectID), string(event.TurnID), string(event.Kind), event.Summary,
		event.Observation, event.Outcome, event.Fingerprint, event.EvidenceJSON, string(detectorJSON),
		string(event.Confidence), formatTime(event.CreatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return domain.NewConflictError(domain.ErrExperienceRevisionConflict, "experience event already exists")
		}
		return fmt.Errorf("SaveExperienceEvent: %w", err)
	}
	return nil
}

// FindExperienceEvent returns an event by ID, or nil.
func FindExperienceEvent(ctx context.Context, tx *sql.Tx, id domain.ExperienceEventID) (*domain.ExperienceEvent, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, project_id, turn_id, kind, summary, observation, outcome, fingerprint,
		       evidence_json, detector_json, confidence, created_at
		FROM experience_events WHERE id = ?
	`, string(id))
	event, err := scanExperienceEvent(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("FindExperienceEvent: %w", err)
	}
	return event, nil
}

// SaveIngestionCursor inserts the initial cursor for a source identity.
func SaveIngestionCursor(ctx context.Context, tx *sql.Tx, cursor *domain.IngestionCursor) error {
	if err := validateIngestionCursor(cursor); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO ingestion_cursors
			(project_id, source, source_instance, cursor_json, last_successful_at, last_attempt_at,
			 last_error_code, last_error_message, input_digest, revision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, string(cursor.ProjectID), string(cursor.Source), cursor.SourceInstance, cursor.CursorJSON,
		nullableTime(cursor.LastSuccessfulAt), nullableTime(cursor.LastAttemptAt), cursor.LastErrorCode,
		cursor.LastErrorMessage, cursor.InputDigest, cursor.Revision)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.NewConflictError(domain.ErrExperienceRevisionConflict, "ingestion cursor identity already exists")
		}
		return fmt.Errorf("SaveIngestionCursor: %w", err)
	}
	return nil
}

// FindIngestionCursor returns the cursor for a source identity, or nil.
func FindIngestionCursor(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, source domain.ExperienceSource, sourceInstance string) (*domain.IngestionCursor, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT project_id, source, source_instance, cursor_json, last_successful_at, last_attempt_at,
		       last_error_code, last_error_message, input_digest, revision
		FROM ingestion_cursors WHERE project_id = ? AND source = ? AND source_instance = ?
	`, string(projectID), string(source), sourceInstance)
	cursor, err := scanIngestionCursor(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("FindIngestionCursor: %w", err)
	}
	return cursor, nil
}

// UpdateIngestionCursor atomically replaces a cursor when its revision matches.
func UpdateIngestionCursor(ctx context.Context, tx *sql.Tx, cursor *domain.IngestionCursor, expectedRevision int) error {
	if err := validateIngestionCursor(cursor); err != nil {
		return err
	}
	if cursor.Revision != expectedRevision+1 {
		return domain.NewConflictError(domain.ErrExperienceCursorConflict, "ingestion cursor revision must advance by one")
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE ingestion_cursors
		SET cursor_json = ?, last_successful_at = ?, last_attempt_at = ?, last_error_code = ?,
		    last_error_message = ?, input_digest = ?, revision = ?
		WHERE project_id = ? AND source = ? AND source_instance = ? AND revision = ?
	`, cursor.CursorJSON, nullableTime(cursor.LastSuccessfulAt), nullableTime(cursor.LastAttemptAt),
		cursor.LastErrorCode, cursor.LastErrorMessage, cursor.InputDigest, cursor.Revision,
		string(cursor.ProjectID), string(cursor.Source), cursor.SourceInstance, expectedRevision)
	if err != nil {
		return fmt.Errorf("UpdateIngestionCursor: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("UpdateIngestionCursor: rows affected: %w", err)
	}
	if rows != 1 {
		return domain.NewConflictError(domain.ErrExperienceCursorConflict, "ingestion cursor revision is stale")
	}
	return nil
}

func scanExperienceSession(row scanner) (*domain.ExperienceSession, error) {
	session := &domain.ExperienceSession{}
	var locatorJSON, updatedAt, createdAt string
	var startedAt, closedAt sql.NullString
	if err := row.Scan((*string)(&session.ID), (*string)(&session.ProjectID), (*string)(&session.Source),
		&session.ExternalSessionID, &locatorJSON, &startedAt, &updatedAt, &closedAt,
		&session.MetadataSHA256, &createdAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(locatorJSON), &session.Locator); err != nil {
		return nil, fmt.Errorf("decode locator_json: %w", err)
	}
	var err error
	if session.StartedAt, err = parseNullableTime(startedAt); err != nil {
		return nil, err
	}
	if session.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	if session.ClosedAt, err = parseNullableTime(closedAt); err != nil {
		return nil, err
	}
	if session.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	return session, nil
}

func scanExperienceTurn(row scanner) (*domain.ExperienceTurn, error) {
	turn := &domain.ExperienceTurn{}
	var occurredAt, ingestedAt string
	var stableAt sql.NullString
	var redacted int
	if err := row.Scan((*string)(&turn.ID), (*string)(&turn.SessionID), &turn.ExternalTurnID,
		&turn.Sequence, (*string)(&turn.Status), &turn.Fingerprint, &turn.UserDigest,
		&turn.AssistantDigest, &turn.ToolCallsDigest, &turn.SafeSummary, &occurredAt, &stableAt,
		&ingestedAt, &turn.SourceRevision, &redacted); err != nil {
		return nil, err
	}
	var err error
	if turn.OccurredAt, err = parseTime(occurredAt); err != nil {
		return nil, err
	}
	if turn.StableAt, err = parseNullableTime(stableAt); err != nil {
		return nil, err
	}
	if turn.IngestedAt, err = parseTime(ingestedAt); err != nil {
		return nil, err
	}
	turn.Redacted = redacted != 0
	return turn, nil
}

func scanExperienceEvent(row scanner) (*domain.ExperienceEvent, error) {
	event := &domain.ExperienceEvent{}
	var detectorJSON, createdAt string
	if err := row.Scan((*string)(&event.ID), (*string)(&event.ProjectID), (*string)(&event.TurnID),
		(*string)(&event.Kind), &event.Summary, &event.Observation, &event.Outcome, &event.Fingerprint,
		&event.EvidenceJSON, &detectorJSON, (*string)(&event.Confidence), &createdAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(detectorJSON), &event.Detector); err != nil {
		return nil, fmt.Errorf("decode detector_json: %w", err)
	}
	var err error
	event.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	return event, nil
}

func scanIngestionCursor(row scanner) (*domain.IngestionCursor, error) {
	cursor := &domain.IngestionCursor{}
	var lastSuccessfulAt, lastAttemptAt sql.NullString
	if err := row.Scan((*string)(&cursor.ProjectID), (*string)(&cursor.Source), &cursor.SourceInstance,
		&cursor.CursorJSON, &lastSuccessfulAt, &lastAttemptAt, &cursor.LastErrorCode,
		&cursor.LastErrorMessage, &cursor.InputDigest, &cursor.Revision); err != nil {
		return nil, err
	}
	var err error
	if cursor.LastSuccessfulAt, err = parseNullableTime(lastSuccessfulAt); err != nil {
		return nil, err
	}
	if cursor.LastAttemptAt, err = parseNullableTime(lastAttemptAt); err != nil {
		return nil, err
	}
	return cursor, nil
}

func validateIngestionCursor(cursor *domain.IngestionCursor) error {
	if cursor == nil {
		return domain.NewValidationError(domain.ErrInvalidArgument, "ingestion cursor is nil")
	}
	if cursor.ProjectID == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "ingestion cursor project_id is required")
	}
	if !domain.IsValidExperienceSource(cursor.Source) {
		return domain.NewValidationError(domain.ErrInvalidArgument, fmt.Sprintf("invalid experience source: %q", cursor.Source))
	}
	if cursor.SourceInstance == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "ingestion cursor source_instance is required")
	}
	if cursor.CursorJSON == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "ingestion cursor cursor_json is required")
	}
	if cursor.Revision < 1 {
		return domain.NewValidationError(domain.ErrInvalidArgument, "ingestion cursor revision must be positive")
	}
	return nil
}

func requireExperienceUpdate(result sql.Result, entity string) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update %s rows affected: %w", entity, err)
	}
	if rows != 1 {
		return domain.NewConflictError(domain.ErrExperienceRevisionConflict, entity+" update target does not exist")
	}
	return nil
}

func formatTime(value time.Time) string { return value.Format(time.RFC3339Nano) }

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", value, err)
	}
	return parsed, nil
}

func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
