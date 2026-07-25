package opencode

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
)

// maxMessageBytes caps the size of any single transcript payload the adapter
// forwards verbatim. The core service re-bounds the envelope before
// persistence; this cap is a defense against a runaway upstream DB.
const maxMessageBytes = 1 << 20 // 1 MB

// maxScanMessageBytes caps the human-readable ScanResult.Message field so a
// flood of validation errors cannot grow the result unboundedly.
const maxScanMessageBytes = 1024

// Scan reads sessions and turns from the OpenCode SQLite store and produces
// neutral ExperienceEnvelopes. The source database is opened in read-only mode
// and never written to; tests in scan_test.go pin this invariant via mtime and
// SHA-256 assertions.
//
// Status mapping:
//
//   - "ok"        — DB readable, schema matches, envelopes produced (zero or
//     more), no validation failures.
//   - "degraded"  — DB readable but at least one envelope failed validation
//     (partial success); OR the DB is unreadable / schema-mismatched.
//   - "error"     — caller mistake (wrong source, empty DBPath).
//   - ctx.Err()   — context cancellation is returned directly, never wrapped
//     into ScanResult.Status.
func (a *Adapter) Scan(ctx context.Context, req ScanRequest) (ScanResult, error) {
	now := a.now()
	if err := ctx.Err(); err != nil {
		return ScanResult{}, err
	}

	instance := req.Instance
	if instance.Source != domain.SourceOpenCode {
		return ScanResult{
			Instance:  instance,
			Status:    "error",
			Code:      string(domain.ErrInvalidArgument),
			Message:   "opencode scan: instance source is not opencode",
			ScannedAt: now,
		}, nil
	}
	if strings.TrimSpace(instance.DBPath) == "" {
		return ScanResult{
			Instance:  instance,
			Status:    "error",
			Code:      string(domain.ErrInvalidArgument),
			Message:   "opencode scan: instance DBPath is required",
			ScannedAt: now,
		}, nil
	}

	fileHash, hashErr := hashFileContents(instance.DBPath)
	if hashErr != nil {
		if errors.Is(hashErr, os.ErrNotExist) {
			return ScanResult{
				Instance:  instance,
				Status:    "degraded",
				Code:      string(domain.ErrExperienceSourceNotFound),
				Message:   "opencode scan: source database file does not exist",
				ScannedAt: now,
			}, nil
		}
		return ScanResult{
			Instance:  instance,
			Status:    "degraded",
			Code:      string(domain.ErrExperienceSourceNotFound),
			Message:   fmt.Sprintf("opencode scan: cannot hash source: %v", hashErr),
			ScannedAt: now,
		}, nil
	}

	db, openErr := sql.Open("sqlite", "file:"+instance.DBPath+"?mode=ro")
	if openErr != nil {
		return ScanResult{
			Instance:  instance,
			Status:    "degraded",
			Code:      string(domain.ErrExperienceSourceNotFound),
			Message:   fmt.Sprintf("opencode scan: cannot open source read-only: %v", openErr),
			ScannedAt: now,
		}, nil
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ScanResult{}, ctx.Err()
		}
		return ScanResult{
			Instance:  instance,
			Status:    "degraded",
			Code:      string(domain.ErrExperienceSourceNotFound),
			Message:   fmt.Sprintf("opencode scan: cannot ping source: %v", err),
			ScannedAt: now,
		}, nil
	}

	if !verifyOpenCodeSchema(ctx, db) {
		return ScanResult{
			Instance:  instance,
			Status:    "degraded",
			Code:      string(domain.ErrExperienceSchemaUnsupported),
			Message:   "opencode scan: source database does not match the expected OpenCode schema",
			ScannedAt: now,
		}, nil
	}

	envelopes, validationMessages, skippedIncomplete, err := a.collectEnvelopes(ctx, db, instance, fileHash, req.Cursor)
	if err != nil {
		// Context cancellation reaches here as a normal scan error; bubble it
		// up so the caller can distinguish cancellation from source failure.
		return ScanResult{}, err
	}

	sort.Slice(envelopes, func(i, j int) bool {
		if envelopes[i].Session.ExternalID != envelopes[j].Session.ExternalID {
			return envelopes[i].Session.ExternalID < envelopes[j].Session.ExternalID
		}
		return envelopes[i].Turn.Sequence < envelopes[j].Turn.Sequence
	})

	var nextCursor map[string]any
	if len(envelopes) > 0 {
		last := envelopes[len(envelopes)-1]
		nextCursor = map[string]any{
			"last_session_id": last.Session.ExternalID,
			"last_sequence":   last.Turn.Sequence,
		}
	} else if req.Cursor != nil {
		nextCursor = req.Cursor
	}

	status := "ok"
	degraded := false
	if len(envelopes) > 0 && len(validationMessages) > 0 {
		status = "degraded"
		degraded = true
	}

	message := boundedMessage(validationMessages)

	return ScanResult{
		Instance:          instance,
		Envelopes:         envelopes,
		NextCursor:        nextCursor,
		Status:            status,
		Code:              "",
		Message:           message,
		Degraded:          degraded,
		SkippedIncomplete: skippedIncomplete,
		ScannedAt:         now,
	}, nil
}

// collectEnvelopes reads sessions and messages, skipping incomplete turns and
// any envelope that fails validation. The returned validationMessages is the
// bounded list of validation errors; the caller decides whether to surface
// them as a degraded status. The fourth return value is the count of turns
// dropped because complete=0, surfaced to callers so they can report the gap.
func (a *Adapter) collectEnvelopes(
	ctx context.Context,
	db *sql.DB,
	instance SourceInstance,
	fileHash string,
	cursor map[string]any,
) ([]experience.ExperienceEnvelope, []string, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, 0, err
	}

	cursorSession, cursorSeq, hasCursor := cursorCheckpoint(cursor)

	rows, err := db.QueryContext(ctx,
		"SELECT id, started_at, updated_at, closed_at FROM sessions ORDER BY started_at ASC, id ASC")
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, nil, 0, ctx.Err()
		}
		return nil, nil, 0, err
	}
	defer rows.Close()

	type sessionRow struct {
		id        string
		startedAt int64
		updatedAt int64
		closedAt  sql.NullInt64
	}
	var sessions []sessionRow
	for rows.Next() {
		var s sessionRow
		if err := rows.Scan(&s.id, &s.startedAt, &s.updatedAt, &s.closedAt); err != nil {
			return nil, nil, 0, err
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, nil, 0, ctx.Err()
		}
		return nil, nil, 0, err
	}
	rows.Close()

	var envelopes []experience.ExperienceEnvelope
	var validation []string
	var skippedIncomplete int

	for _, session := range sessions {
		if err := ctx.Err(); err != nil {
			return nil, nil, 0, err
		}

		msgRows, err := db.QueryContext(ctx,
			"SELECT id, sequence, role, content, finish, created_at, complete, revision FROM messages WHERE session_id = ? ORDER BY sequence ASC, id ASC",
			session.id)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, nil, 0, ctx.Err()
			}
			return nil, nil, 0, err
		}

		for msgRows.Next() {
			if err := ctx.Err(); err != nil {
				_ = msgRows.Close()
				return nil, nil, 0, err
			}
			var (
				id        string
				sequence  int64
				role      string
				content   sql.NullString
				finish    sql.NullString
				createdAt int64
				complete  int
				revision  sql.NullString
			)
			if err := msgRows.Scan(&id, &sequence, &role, &content, &finish, &createdAt, &complete, &revision); err != nil {
				_ = msgRows.Close()
				return nil, nil, 0, err
			}

			// Skip incomplete turns. The contract rule is "no incomplete
			// turns" — a turn with complete=0 has not yet been emitted by the
			// source and would force the service into an unstable state. The
			// counter is surfaced through ScanResult.SkippedIncomplete so the
			// caller can report the gap.
			if complete == 0 {
				skippedIncomplete++
				continue
			}

			// Cursor checkpoint: skip turns at or before the last delivered
			// (session, sequence) so repeated scans with the same cursor are
			// stable input for the idempotency layer.
			if hasCursor && cursorAtOrBefore(session.id, sequence, cursorSession, cursorSeq) {
				continue
			}

			envelope := buildEnvelope(instance, session, id, sequence, role, content.String, finish.String, createdAt, revision.String, fileHash)
			if err := experience.ValidateEnvelope(&envelope); err != nil {
				validation = append(validation, err.Error())
				continue
			}
			envelopes = append(envelopes, envelope)
		}
		if err := msgRows.Close(); err != nil {
			return nil, nil, 0, err
		}
		if err := msgRows.Err(); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, nil, 0, ctx.Err()
			}
			return nil, nil, 0, err
		}
	}

	return envelopes, validation, skippedIncomplete, nil
}

// buildEnvelope converts one (session, message) pair into an ExperienceEnvelope.
// The SourceRevision is seeded deterministically when the message row does not
// supply one, so the core service can detect duplicate turns via Fingerprint.
func buildEnvelope(
	instance SourceInstance,
	session struct {
		id        string
		startedAt int64
		updatedAt int64
		closedAt  sql.NullInt64
	},
	msgID string,
	sequence int64,
	role string,
	content string,
	finish string,
	createdAt int64,
	revision string,
	fileHash string,
) experience.ExperienceEnvelope {
	text := content
	if len(text) > maxMessageBytes {
		text = text[:maxMessageBytes]
	}

	startedAt := time.UnixMilli(session.startedAt).UTC()
	updatedAt := time.UnixMilli(session.updatedAt).UTC()

	envelope := experience.ExperienceEnvelope{
		SchemaVersion: experience.ExperienceEnvelopeSchemaVersion,
		Source:        domain.SourceOpenCode,
		ProjectRoot:   instance.ProjectRoot,
	}
	envelope.Session.ExternalID = session.id
	envelope.Session.StartedAt = &startedAt
	envelope.Session.UpdatedAt = updatedAt
	if session.closedAt.Valid && session.closedAt.Int64 != 0 {
		closedAt := time.UnixMilli(session.closedAt.Int64).UTC()
		envelope.Session.ClosedAt = &closedAt
	}
	envelope.Session.Locator = domain.TranscriptLocator{
		Kind:       "sqlite",
		Path:       instance.DBPath,
		SessionID:  session.id,
		TurnID:     msgID,
		SourceHash: fileHash,
	}

	envelope.Turn.ExternalID = msgID
	envelope.Turn.Sequence = sequence
	envelope.Turn.Complete = true
	envelope.Turn.FinishReason = finish
	envelope.Turn.OccurredAt = time.UnixMilli(createdAt).UTC()

	switch role {
	case "user":
		envelope.Turn.UserText = text
	case "assistant":
		envelope.Turn.AssistantText = text
	}

	if revision != "" {
		envelope.Turn.SourceRevision = revision
	} else {
		envelope.Turn.SourceRevision = experience.DigestString(
			msgID + ":" + revision + ":" + strconv.FormatInt(sequence, 10))
	}

	envelope.Actor = domain.Actor{
		Kind:      "agent",
		Name:      "opencode",
		Model:     "",
		SessionID: session.id,
	}

	return envelope
}

// cursorCheckpoint extracts the (session_id, sequence) pair from a cursor map.
// The map may carry either int64 or float64 values depending on whether the
// caller constructed it natively or decoded it from JSON.
func cursorCheckpoint(cursor map[string]any) (string, int64, bool) {
	if cursor == nil {
		return "", 0, false
	}
	sid, _ := cursor["last_session_id"].(string)
	if sid == "" {
		return "", 0, false
	}
	switch v := cursor["last_sequence"].(type) {
	case int64:
		return sid, v, true
	case int:
		return sid, int64(v), true
	case float64:
		return sid, int64(v), true
	case int32:
		return sid, int64(v), true
	}
	return "", 0, false
}

// cursorAtOrBefore reports whether (sessionID, sequence) is at or before the
// cursor checkpoint. Lexicographic on session id (stable), then numeric on
// sequence within the same session.
func cursorAtOrBefore(sessionID string, sequence int64, cursorSession string, cursorSeq int64) bool {
	if sessionID < cursorSession {
		return true
	}
	if sessionID == cursorSession && sequence <= cursorSeq {
		return true
	}
	return false
}

// hashFileContents returns the hex-encoded SHA-256 of the file at path.
func hashFileContents(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// boundedMessage concatenates the validation error strings, separated by "; ",
// and truncates the result to maxScanMessageBytes. Empty input returns "".
func boundedMessage(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	joined := strings.Join(parts, "; ")
	if len(joined) <= maxScanMessageBytes {
		return joined
	}
	return joined[:maxScanMessageBytes]
}
