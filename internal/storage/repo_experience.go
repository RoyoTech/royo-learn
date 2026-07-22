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
// The updated-at predicate prevents an older envelope from winning a refresh race.
func UpdateExperienceSession(ctx context.Context, tx *sql.Tx, session *domain.ExperienceSession, expectedUpdatedAt time.Time) error {
	if err := domain.ValidateExperienceSession(session); err != nil {
		return err
	}
	if expectedUpdatedAt.IsZero() || !session.UpdatedAt.After(expectedUpdatedAt) {
		return domain.NewConflictError(domain.ErrExperienceRevisionConflict,
			"experience session update timestamp is stale")
	}
	locatorJSON, err := json.Marshal(session.Locator)
	if err != nil {
		return fmt.Errorf("UpdateExperienceSession: marshal locator: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE experience_sessions
		SET locator_json = ?, started_at = ?, updated_at = ?, closed_at = ?, metadata_sha256 = ?
		WHERE id = ? AND project_id = ? AND source = ? AND external_session_id = ? AND updated_at = ?
	`, string(locatorJSON), nullableTime(session.StartedAt), formatTime(session.UpdatedAt),
		nullableTime(session.ClosedAt), session.MetadataSHA256, string(session.ID),
		string(session.ProjectID), string(session.Source), session.ExternalSessionID, formatTime(expectedUpdatedAt))
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

// UpdateExperienceTurn stores a new revision of an existing turn identity when
// the caller's expected source revision is still current.
func UpdateExperienceTurn(ctx context.Context, tx *sql.Tx, turn *domain.ExperienceTurn, expectedSourceRevision string) error {
	if err := domain.ValidateExperienceTurn(turn); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE experience_turns
		SET sequence = ?, status = ?, fingerprint = ?, user_digest = ?, assistant_digest = ?,
		    tool_calls_digest = ?, safe_summary = ?, occurred_at = ?, stable_at = ?, ingested_at = ?,
		    source_revision = ?, redacted = ?
		WHERE id = ? AND session_id = ? AND external_turn_id = ? AND source_revision = ?
	`, turn.Sequence, string(turn.Status), turn.Fingerprint, turn.UserDigest, turn.AssistantDigest,
		turn.ToolCallsDigest, turn.SafeSummary, formatTime(turn.OccurredAt), nullableTime(turn.StableAt),
		formatTime(turn.IngestedAt), turn.SourceRevision, boolToInt(turn.Redacted), string(turn.ID),
		string(turn.SessionID), turn.ExternalTurnID, expectedSourceRevision)
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
			 last_error_code, last_error_message, input_digest, revision, source_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, string(cursor.ProjectID), string(cursor.Source), cursor.SourceInstance, cursor.CursorJSON,
		nullableTime(cursor.LastSuccessfulAt), nullableTime(cursor.LastAttemptAt), cursor.LastErrorCode,
		cursor.LastErrorMessage, cursor.InputDigest, cursor.Revision, cursor.SourceOrder)
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
		       last_error_code, last_error_message, input_digest, revision, source_order
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
		    last_error_message = ?, input_digest = ?, revision = ?, source_order = ?
		WHERE project_id = ? AND source = ? AND source_instance = ? AND revision = ?
		  AND (last_successful_at IS NULL OR source_order < ? OR
		       (source_order = ? AND (last_successful_at IS NULL OR last_error_code <> '') AND input_digest = ?))
	`, cursor.CursorJSON, nullableTime(cursor.LastSuccessfulAt), nullableTime(cursor.LastAttemptAt),
		cursor.LastErrorCode, cursor.LastErrorMessage, cursor.InputDigest, cursor.Revision, cursor.SourceOrder,
		string(cursor.ProjectID), string(cursor.Source), cursor.SourceInstance, expectedRevision,
		cursor.SourceOrder, cursor.SourceOrder, cursor.InputDigest)
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

// RecordIngestionCursorFailure records the last failed attempt without moving
// the successful checkpoint. A source-order guard prevents stale failures from
// overwriting a newer cursor, while a first failure creates an unsucceeded row
// that can be completed by an exact retry.
func RecordIngestionCursorFailure(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, source domain.ExperienceSource, sourceInstance string, sourceOrder int64, cursorJSON, inputDigest, code, message string, attemptedAt time.Time) error {
	if projectID == "" || !domain.IsValidExperienceSource(source) || sourceInstance == "" || cursorJSON == "" || inputDigest == "" || sourceOrder < 0 {
		return domain.NewValidationError(domain.ErrInvalidArgument, "invalid ingestion cursor failure")
	}
	if len(string(projectID)) > domain.MaxExperienceIDBytes ||
		len(sourceInstance) > domain.MaxExperienceSourceInstanceBytes ||
		len(cursorJSON) > domain.MaxExperienceCursorBytes ||
		len(inputDigest) > domain.MaxExperienceDigestBytes ||
		len(code) > domain.MaxExperienceErrorCodeBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge, "ingestion cursor failure metadata is too large")
	}
	if len(message) > maxExperienceErrorMessageBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge, "ingestion cursor failure message is too large")
	}
	existing, err := FindIngestionCursor(ctx, tx, projectID, source, sourceInstance)
	if err != nil {
		return err
	}
	if existing == nil {
		attempt := attemptedAt
		return SaveIngestionCursor(ctx, tx, &domain.IngestionCursor{
			ProjectID:        projectID,
			Source:           source,
			SourceInstance:   sourceInstance,
			CursorJSON:       cursorJSON,
			LastAttemptAt:    &attempt,
			LastErrorCode:    code,
			LastErrorMessage: message,
			InputDigest:      inputDigest,
			Revision:         1,
			// A failed attempt is not a successful checkpoint. Retain its
			// source order only to reject older failure metadata; a later
			// successful attempt may still replace this unsucceeded row.
			SourceOrder: sourceOrder,
		})
	}
	if sourceOrder < existing.SourceOrder {
		return domain.NewConflictError(domain.ErrExperienceCursorConflict,
			"ingestion cursor failure source order is stale")
	}
	if sourceOrder == existing.SourceOrder &&
		(cursorJSON != existing.CursorJSON || inputDigest != existing.InputDigest) {
		return domain.NewConflictError(domain.ErrExperienceCursorConflict,
			"ingestion cursor failure metadata conflicts at source order")
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE ingestion_cursors
		SET cursor_json = CASE WHEN last_successful_at IS NULL THEN ? ELSE cursor_json END,
		    input_digest = CASE WHEN last_successful_at IS NULL THEN ? ELSE input_digest END,
		    last_attempt_at = ?, last_error_code = ?, last_error_message = ?,
		    source_order = CASE WHEN last_successful_at IS NULL THEN ? ELSE source_order END,
		    revision = revision + 1
		WHERE project_id = ? AND source = ? AND source_instance = ? AND revision = ?
		  AND (last_successful_at IS NULL OR source_order <= ?)
	`, cursorJSON, inputDigest, nullableTime(&attemptedAt), code, message, sourceOrder, string(projectID), string(source), sourceInstance, existing.Revision, sourceOrder)
	if err != nil {
		return fmt.Errorf("RecordIngestionCursorFailure: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("RecordIngestionCursorFailure: rows affected: %w", err)
	}
	if rows != 1 {
		return domain.NewConflictError(domain.ErrExperienceCursorConflict,
			"ingestion cursor failure target is stale")
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
		&cursor.LastErrorMessage, &cursor.InputDigest, &cursor.Revision, &cursor.SourceOrder); err != nil {
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
	if len(string(cursor.ProjectID)) > domain.MaxExperienceIDBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge, "ingestion cursor project_id is too large")
	}
	if !domain.IsValidExperienceSource(cursor.Source) {
		return domain.NewValidationError(domain.ErrInvalidArgument, "invalid experience source")
	}
	if cursor.SourceInstance == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "ingestion cursor source_instance is required")
	}
	if len(cursor.SourceInstance) > domain.MaxExperienceSourceInstanceBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge, "ingestion cursor source_instance is too large")
	}
	if cursor.CursorJSON == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "ingestion cursor cursor_json is required")
	}
	if len(cursor.CursorJSON) > domain.MaxExperienceCursorBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge, "ingestion cursor cursor_json is too large")
	}
	if len(cursor.LastErrorCode) > domain.MaxExperienceErrorCodeBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge, "ingestion cursor error code is too large")
	}
	if len(cursor.InputDigest) > domain.MaxExperienceDigestBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge, "ingestion cursor input digest is too large")
	}
	if cursor.Revision < 1 {
		return domain.NewValidationError(domain.ErrInvalidArgument, "ingestion cursor revision must be positive")
	}
	if cursor.SourceOrder < 0 {
		return domain.NewValidationError(domain.ErrInvalidArgument, "ingestion cursor source order must be non-negative")
	}
	if len(cursor.LastErrorMessage) > maxExperienceErrorMessageBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge, "ingestion cursor error message is too large")
	}
	return nil
}

const maxExperienceErrorMessageBytes = domain.MaxExperienceErrorMessageBytes

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
