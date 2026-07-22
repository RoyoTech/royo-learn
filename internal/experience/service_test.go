package experience

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/evidence"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

func TestIngestCreatesSessionTurnCursorAndAudit(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	input := IngestInput{
		Envelope:       validEnvelope(projectRoot, "create"),
		SourceInstance: "opencode-db",
		CursorJSON:     `{"offset":1}`,
		SourceOrder:    1,
	}

	result, err := NewService(db, Config{Now: fixedClock()}).Ingest(context.Background(), project.ID, &input)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result == nil || result.Session == nil || result.Turn == nil || result.Cursor == nil {
		t.Fatalf("result = %#v, want session, turn, and cursor", result)
	}
	if !result.Created || result.Updated || result.Idempotent {
		t.Fatalf("result state = %#v, want created only", result)
	}
	if result.Turn.Status != domain.TurnIngested {
		t.Fatalf("turn status = %q, want %q", result.Turn.Status, domain.TurnIngested)
	}
	if result.Cursor.Revision != 1 {
		t.Fatalf("cursor revision = %d, want 1", result.Cursor.Revision)
	}

	assertExperienceCounts(t, db, 1, 1, 0, 1)
	var auditCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditCount); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditCount != 3 {
		t.Fatalf("audit count = %d, want 3", auditCount)
	}
	var cursorAuditCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = 'experience_cursor_created'`).Scan(&cursorAuditCount); err != nil {
		t.Fatalf("cursor audit count: %v", err)
	}
	if cursorAuditCount != 1 {
		t.Fatalf("cursor audit count = %d, want 1", cursorAuditCount)
	}
}

func TestIngestExactRetryIsIdempotent(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	input := IngestInput{
		Envelope:       validEnvelope(projectRoot, "retry"),
		SourceInstance: "opencode-db",
		CursorJSON:     `{"offset":2}`,
		SourceOrder:    1,
	}
	svc := NewService(db, Config{Now: fixedClock()})

	first, err := svc.Ingest(context.Background(), project.ID, &input)
	if err != nil {
		t.Fatalf("first Ingest: %v", err)
	}
	second, err := svc.Ingest(context.Background(), project.ID, &input)
	if err != nil {
		t.Fatalf("retry Ingest: %v", err)
	}
	if second == nil || !second.Idempotent || second.Created || second.Updated {
		t.Fatalf("retry result = %#v, want idempotent no-op", second)
	}
	if first.Session.ID != second.Session.ID || first.Turn.ID != second.Turn.ID {
		t.Fatalf("retry IDs changed: first=%#v second=%#v", first, second)
	}

	assertExperienceCounts(t, db, 1, 1, 0, 1)
	var auditCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditCount); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditCount != 3 {
		t.Fatalf("audit count after retry = %d, want 3", auditCount)
	}
}

func TestIngestRedactsBeforeDigestPersistenceAuditAndResponse(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	secret := "my-super-secret-token"
	envelope := validEnvelope(projectRoot, "redaction")
	envelope.Turn.UserText = "Use " + secret + " to fix the failing test."
	envelope.Turn.AssistantText = "The token is " + secret
	envelope.Actor.Name = "agent-" + secret
	envelope.Turn.ToolCalls = []SafeToolCall{{
		Name: "test",
		Arguments: map[string]any{
			"token":  secret,
			"nested": []any{secret, 1.0, map[string]any{"inner": secret}},
		},
		OutputHint: "output contains " + secret,
	}}
	input := IngestInput{
		Envelope:       envelope,
		SourceInstance: "opencode-db",
		CursorJSON:     `{"cursor":"` + secret + `"}`,
		SourceOrder:    1,
	}

	result, err := NewService(db, Config{
		KnownSecrets: []string{secret},
		Now:          fixedClock(),
	}).Ingest(context.Background(), project.ID, &input)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if !result.Redacted || !result.Turn.Redacted {
		t.Fatalf("redaction flags = result:%v turn:%v, want true", result.Redacted, result.Turn.Redacted)
	}
	if strings.Contains(fmt.Sprintf("%#v", result), secret) {
		t.Fatalf("secret reached response: %#v", result)
	}

	wantDigest := digestString(string(evidence.Redact([]byte(envelope.Turn.UserText), []string{secret})))
	if result.Turn.UserDigest != wantDigest {
		t.Fatalf("user digest = %q, want digest of redacted input %q", result.Turn.UserDigest, wantDigest)
	}
	wantFingerprint := fingerprint(fingerprintInput{
		Source:          string(result.Session.Source),
		ExternalSession: result.Session.ExternalSessionID,
		ExternalTurn:    result.Turn.ExternalTurnID,
		Sequence:        result.Turn.Sequence,
		UserDigest:      result.Turn.UserDigest,
		AssistantDigest: result.Turn.AssistantDigest,
		ToolCallsDigest: result.Turn.ToolCallsDigest,
		FinishReason:    "stop",
		Complete:        true,
		SourceRevision:  result.Turn.SourceRevision,
	})
	if result.Turn.Fingerprint != wantFingerprint {
		t.Fatalf("fingerprint = %q, want fingerprint built from persisted redacted digests %q", result.Turn.Fingerprint, wantFingerprint)
	}
	assertNoSecretInExperienceSinks(t, db, secret)
}

func TestIngestRejectsSecretBearingIdentityWithoutLeakingIt(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	secret := "identity-secret-value"
	envelope := validEnvelope(projectRoot, "unsafe-identity")
	envelope.Session.ExternalID = "session-" + secret

	_, err := NewService(db, Config{
		KnownSecrets: []string{secret},
		Now:          fixedClock(),
	}).Ingest(context.Background(), project.ID, &IngestInput{Envelope: envelope})
	assertExperienceCode(t, err, domain.ErrInvalidArgument)
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("identity validation leaked secret: %v", err)
	}
	assertExperienceCounts(t, db, 0, 0, 0, 0)
}

func TestIngestRedactsBeforeValidationErrors(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	secret := "sk-proj-validation-leak-1234567890"
	envelope := validEnvelope(projectRoot, "validation-redaction")
	envelope.Source = domain.ExperienceSource("invalid-" + secret)

	_, err := NewService(db, Config{
		KnownSecrets: []string{secret},
		Now:          fixedClock(),
	}).Ingest(context.Background(), project.ID, &IngestInput{Envelope: envelope})
	assertExperienceCode(t, err, domain.ErrInvalidArgument)
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("validation error leaked secret: %v", err)
	}
}

func TestIngestRejectsOversizedPayloadBeforePersistence(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	envelope := validEnvelope(projectRoot, "oversized")
	envelope.Turn.UserText = strings.Repeat("x", 128)

	_, err := NewService(db, Config{MaxTurnBytes: 32, Now: fixedClock()}).Ingest(
		context.Background(), project.ID, &IngestInput{Envelope: envelope},
	)
	assertExperienceCode(t, err, domain.ErrExperiencePayloadTooLarge)
	assertExperienceCounts(t, db, 0, 0, 0, 0)
	if strings.Contains(err.Error(), envelope.Turn.UserText) {
		t.Fatalf("oversized error leaked payload: %v", err)
	}
}

func TestIngestRejectsOversizedCursorBeforePersistence(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	envelope := validEnvelope(projectRoot, "oversized-cursor")
	_, err := NewService(db, Config{
		MaxCursorBytes: 32,
		Now:            fixedClock(),
	}).Ingest(context.Background(), project.ID, &IngestInput{
		Envelope:       envelope,
		SourceInstance: "opencode-db",
		CursorJSON:     `{"cursor":"012345678901234567890123456789"}`,
		SourceOrder:    1,
	})
	assertExperienceCode(t, err, domain.ErrExperiencePayloadTooLarge)
	assertExperienceCounts(t, db, 0, 0, 0, 0)
}

func TestIngestRejectsOversizedProjectAndSourceInstanceMetadata(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})

	longProjectID := domain.ProjectID(strings.Repeat("p", domain.MaxExperienceIDBytes+1))
	assertExperienceCode(t, mustIngestError(t, svc, longProjectID, &IngestInput{
		Envelope: validEnvelope(projectRoot, "long-project-id"),
	}), domain.ErrExperiencePayloadTooLarge)

	longSourceInstance := strings.Repeat("s", domain.MaxExperienceSourceInstanceBytes+1)
	assertExperienceCode(t, mustIngestError(t, svc, project.ID, &IngestInput{
		Envelope:       validEnvelope(projectRoot, "long-source-instance"),
		SourceInstance: longSourceInstance,
		CursorJSON:     `{}`,
	}), domain.ErrExperiencePayloadTooLarge)
	assertExperienceCounts(t, db, 0, 0, 0, 0)
}

func TestIngestRejectsOversizedLocatorMetadataBeforePersistence(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	envelope := validEnvelope(projectRoot, "oversized-locator")
	envelope.Session.Locator.SourceHash = strings.Repeat("a", 128)
	_, err := NewService(db, Config{
		MaxLocatorBytes: 64,
		Now:             fixedClock(),
	}).Ingest(context.Background(), project.ID, &IngestInput{Envelope: envelope})
	assertExperienceCode(t, err, domain.ErrExperiencePayloadTooLarge)
	assertExperienceCounts(t, db, 0, 0, 0, 0)
}

func TestIngestRejectsUnknownProjectAndOutsideLocator(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	unknown := validEnvelope(projectRoot, "unknown-project")
	_, err := NewService(db, Config{Now: fixedClock()}).Ingest(
		context.Background(), domain.ProjectID(uuid.Must(uuid.NewV7()).String()), &IngestInput{Envelope: unknown},
	)
	assertExperienceCode(t, err, domain.ErrProjectNotFound)
	assertExperienceCounts(t, db, 0, 0, 0, 0)

	outsideRoot := t.TempDir()
	outside := validEnvelope(projectRoot, "outside")
	outside.Session.Locator.Path = filepath.Join(outsideRoot, "source.db")
	_, err = NewService(db, Config{Now: fixedClock()}).Ingest(
		context.Background(), project.ID, &IngestInput{Envelope: outside},
	)
	assertExperienceCode(t, err, domain.ErrExperienceLocatorOutsideRoot)
	assertExperienceCounts(t, db, 0, 0, 0, 0)
}

func TestIngestRevisionUsesSourceRevisionCAS(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})
	input := IngestInput{Envelope: validEnvelope(projectRoot, "revision")}
	first, err := svc.Ingest(context.Background(), project.ID, &input)
	if err != nil {
		t.Fatalf("first Ingest: %v", err)
	}

	input.Envelope.Turn.UserText = "The corrected procedure also passes."
	input.Envelope.Turn.SourceRevision = "revision-2"
	updated, err := svc.Ingest(context.Background(), project.ID, &input)
	if err != nil {
		t.Fatalf("revision Ingest: %v", err)
	}
	if updated == nil || !updated.Updated || updated.Created || updated.Idempotent {
		t.Fatalf("revision result = %#v, want update", updated)
	}
	if updated.Turn.ID != first.Turn.ID || updated.Turn.SourceRevision != "revision-2" {
		t.Fatalf("turn revision = %#v, want same ID and revision-2", updated.Turn)
	}
	assertExperienceCounts(t, db, 1, 1, 0, 0)

	var revisedAudits int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = 'experience_turn_revised'`).Scan(&revisedAudits); err != nil {
		t.Fatalf("revised audit count: %v", err)
	}
	if revisedAudits != 1 {
		t.Fatalf("revised audit count = %d, want 1", revisedAudits)
	}

	superseded := *updated.Turn
	superseded.Status = domain.TurnSuperseded
	if err := storage.WithTx(context.Background(), db, func(tx *sql.Tx) error {
		return storage.UpdateExperienceTurn(context.Background(), tx, &superseded, updated.Turn.SourceRevision)
	}); err != nil {
		t.Fatalf("mark turn superseded: %v", err)
	}
	input.Envelope.Turn.UserText = "A later revision must not replace an originating turn."
	input.Envelope.Turn.SourceRevision = "revision-3"
	_, err = svc.Ingest(context.Background(), project.ID, &input)
	assertExperienceCode(t, err, domain.ErrExperienceRevisionConflict)

	var status, fingerprint, sourceRevision string
	if err := db.DB.QueryRow(`SELECT status, fingerprint, source_revision FROM experience_turns WHERE id = ?`, string(updated.Turn.ID)).Scan(&status, &fingerprint, &sourceRevision); err != nil {
		t.Fatalf("read superseded turn: %v", err)
	}
	if status != string(domain.TurnSuperseded) || sourceRevision != "revision-2" || fingerprint != updated.Turn.Fingerprint {
		t.Fatalf("superseded turn changed: status=%q revision=%q fingerprint=%q", status, sourceRevision, fingerprint)
	}
}

func TestIngestNewTurnUpdatesSessionAndCursorCAS(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewIngestionService(db, Config{Now: fixedClock()})
	firstInput := IngestInput{
		Envelope:       validEnvelope(projectRoot, "multiple-turns"),
		SourceInstance: "opencode-db",
		CursorJSON:     `{"offset":1}`,
		SourceOrder:    1,
	}
	first, err := svc.Ingest(context.Background(), project.ID, &firstInput)
	if err != nil {
		t.Fatalf("first Ingest: %v", err)
	}

	secondInput := firstInput
	secondInput.Envelope.Turn.ExternalID = "turn-multiple-turns-2"
	secondInput.Envelope.Turn.Sequence = 2
	secondInput.Envelope.Turn.SourceRevision = "revision-2"
	secondInput.Envelope.Turn.UserText = "The next turn is also captured."
	secondInput.CursorJSON = `{"offset":2}`
	secondInput.SourceOrder = 2
	second, err := svc.Ingest(context.Background(), project.ID, &secondInput)
	if err != nil {
		t.Fatalf("second Ingest: %v", err)
	}
	if second == nil || !second.Created || second.Turn.ID == first.Turn.ID || second.Cursor == nil || second.Cursor.Revision != 2 {
		t.Fatalf("second result = %#v, want new turn and cursor revision 2", second)
	}
	assertExperienceCounts(t, db, 1, 2, 0, 1)
}

func TestIngestDoesNotOverwriteSessionWithOlderEnvelope(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})
	initialTime := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	newerTime := initialTime.Add(time.Hour)

	firstInput := IngestInput{Envelope: validEnvelope(projectRoot, "session-refresh")}
	firstInput.Envelope.Session.UpdatedAt = initialTime
	if _, err := svc.Ingest(context.Background(), project.ID, &firstInput); err != nil {
		t.Fatalf("first Ingest: %v", err)
	}

	newer := firstInput
	newer.Envelope.Turn.ExternalID = "turn-session-refresh-2"
	newer.Envelope.Turn.Sequence = 2
	newer.Envelope.Turn.SourceRevision = "revision-2"
	newer.Envelope.Turn.UserText = "newer session metadata"
	newer.Envelope.Session.UpdatedAt = newerTime
	if _, err := svc.Ingest(context.Background(), project.ID, &newer); err != nil {
		t.Fatalf("newer Ingest: %v", err)
	}

	older := newer
	older.Envelope.Turn.ExternalID = "turn-session-refresh-3"
	older.Envelope.Turn.Sequence = 3
	older.Envelope.Turn.SourceRevision = "revision-3"
	older.Envelope.Turn.UserText = "older session metadata must not overwrite"
	older.Envelope.Session.UpdatedAt = initialTime.Add(30 * time.Minute)
	result, err := svc.Ingest(context.Background(), project.ID, &older)
	if err != nil {
		t.Fatalf("older Ingest: %v", err)
	}
	if result == nil || !result.Session.UpdatedAt.Equal(newerTime) {
		t.Fatalf("session updated_at = %#v, want %s", result, newerTime)
	}
	assertExperienceCounts(t, db, 1, 3, 0, 0)
}

func TestIngestRejectsInvalidCursorAndProjectBounds(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	base := validEnvelope(projectRoot, "invalid-input")
	svc := NewService(db, Config{Now: fixedClock()})

	tests := []struct {
		name  string
		input IngestInput
		code  domain.ErrorCode
	}{
		{
			name:  "cursor pair",
			input: IngestInput{Envelope: base, SourceInstance: "only-instance"},
			code:  domain.ErrInvalidArgument,
		},
		{
			name:  "cursor JSON",
			input: IngestInput{Envelope: base, SourceInstance: "instance", CursorJSON: "not-json"},
			code:  domain.ErrExperienceSchemaUnsupported,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertExperienceCode(t, mustIngestError(t, svc, project.ID, &tt.input), tt.code)
		})
	}

	mismatchedRoot := base
	mismatchedRoot.ProjectRoot = t.TempDir()
	assertExperienceCode(t, mustIngestError(t, svc, project.ID, &IngestInput{Envelope: mismatchedRoot}), domain.ErrExperienceLocatorOutsideRoot)

	protected := base
	protected.Session.Locator.Path = filepath.Join(projectRoot, ".env")
	assertExperienceCode(t, mustIngestError(t, svc, project.ID, &IngestInput{Envelope: protected}), domain.ErrProtectedPath)
	assertExperienceCounts(t, db, 0, 0, 0, 0)
}

func TestIngestRejectsChangedFingerprintWithSameSourceRevision(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})
	input := IngestInput{Envelope: validEnvelope(projectRoot, "same-revision")}
	if _, err := svc.Ingest(context.Background(), project.ID, &input); err != nil {
		t.Fatalf("first Ingest: %v", err)
	}
	input.Envelope.Turn.UserText = "changed content with an unchanged revision"
	_, err := svc.Ingest(context.Background(), project.ID, &input)
	assertExperienceCode(t, err, domain.ErrExperienceRevisionConflict)
}

func TestIngestRejectsLateCursorAndRollsBackTurn(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})
	firstInput := IngestInput{
		Envelope:       validEnvelope(projectRoot, "late-cursor"),
		SourceInstance: "opencode-db",
		CursorJSON:     `{"offset":3}`,
		SourceOrder:    3,
	}
	if _, err := svc.Ingest(context.Background(), project.ID, &firstInput); err != nil {
		t.Fatalf("first Ingest: %v", err)
	}

	late := firstInput
	late.Envelope.Turn.ExternalID = "turn-late-cursor-2"
	late.Envelope.Turn.Sequence = 2
	late.Envelope.Turn.SourceRevision = "revision-2"
	late.Envelope.Turn.UserText = "A late source checkpoint must not commit this turn."
	late.CursorJSON = `{"offset":2}`
	late.SourceOrder = 2
	_, err := svc.Ingest(context.Background(), project.ID, &late)
	assertExperienceCode(t, err, domain.ErrExperienceCursorConflict)
	assertExperienceCounts(t, db, 1, 1, 0, 1)

	var cursorJSON string
	var sourceOrder int64
	if err := db.DB.QueryRow(`SELECT cursor_json, source_order FROM ingestion_cursors`).Scan(&cursorJSON, &sourceOrder); err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	if cursorJSON != `{"offset":3}` || sourceOrder != 3 {
		t.Fatalf("cursor = %q order=%d, want offset 3/order 3", cursorJSON, sourceOrder)
	}
}

func TestIngestDefaultsSourceRevisionAndConvenienceWrapper(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	envelope := validEnvelope(projectRoot, "fallback-revision")
	envelope.Turn.SourceRevision = ""
	result, err := NewService(db, Config{Now: fixedClock()}).IngestEnvelope(context.Background(), project.ID, envelope)
	if err != nil {
		t.Fatalf("IngestEnvelope: %v", err)
	}
	if result == nil || result.Turn.SourceRevision == "" {
		t.Fatalf("result = %#v, want generated source revision", result)
	}
	if DefaultMaxTurnBytes != 262144 {
		t.Fatalf("DefaultMaxTurnBytes = %d, want 262144", DefaultMaxTurnBytes)
	}
}

func TestIngestAuditFailureRollsBackDataAndCursor(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	if _, err := db.DB.Exec(`
		CREATE TRIGGER fail_experience_audit
		BEFORE INSERT ON audit_events
		BEGIN
			SELECT RAISE(ABORT, 'test audit failure');
		END;
	`); err != nil {
		t.Fatalf("create audit trigger: %v", err)
	}

	input := IngestInput{
		Envelope:       validEnvelope(projectRoot, "rollback"),
		SourceInstance: "opencode-db",
		CursorJSON:     `{"offset":9}`,
	}
	_, err := NewService(db, Config{Now: fixedClock()}).Ingest(context.Background(), project.ID, &input)
	if err == nil || !strings.Contains(err.Error(), "audit") {
		t.Fatalf("Ingest error = %v, want audit failure", err)
	}
	assertExperienceCounts(t, db, 0, 0, 0, 0)
}

func TestIngestTurnWriteFailureRollsBackSession(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	if _, err := db.DB.Exec(`
		CREATE TRIGGER fail_experience_turn
		BEFORE INSERT ON experience_turns
		BEGIN
			SELECT RAISE(ABORT, 'test turn failure');
		END;
	`); err != nil {
		t.Fatalf("create turn trigger: %v", err)
	}

	_, err := NewService(db, Config{Now: fixedClock()}).Ingest(context.Background(), project.ID, &IngestInput{
		Envelope: validEnvelope(projectRoot, "turn-rollback"),
	})
	if err == nil || !strings.Contains(err.Error(), "turn") {
		t.Fatalf("Ingest error = %v, want turn failure", err)
	}
	assertExperienceCounts(t, db, 0, 0, 0, 0)
}

func TestIngestSessionWriteFailureLeavesNoRows(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	if _, err := db.DB.Exec(`
		CREATE TRIGGER fail_experience_session
		BEFORE INSERT ON experience_sessions
		BEGIN
			SELECT RAISE(ABORT, 'test session failure');
		END;
	`); err != nil {
		t.Fatalf("create session trigger: %v", err)
	}

	_, err := NewService(db, Config{Now: fixedClock()}).Ingest(context.Background(), project.ID, &IngestInput{
		Envelope: validEnvelope(projectRoot, "session-failure"),
	})
	if err == nil || !strings.Contains(err.Error(), "session") {
		t.Fatalf("Ingest error = %v, want session failure", err)
	}
	assertExperienceCounts(t, db, 0, 0, 0, 0)
}

func TestIngestRevisionAuditFailureRollsBackRevision(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})
	input := IngestInput{Envelope: validEnvelope(projectRoot, "revision-audit")}
	first, err := svc.Ingest(context.Background(), project.ID, &input)
	if err != nil {
		t.Fatalf("first Ingest: %v", err)
	}
	if _, err := db.DB.Exec(`
		CREATE TRIGGER fail_revision_audit
		BEFORE INSERT ON audit_events
		WHEN NEW.operation = 'experience_turn_revised'
		BEGIN
			SELECT RAISE(ABORT, 'test revision audit failure');
		END;
	`); err != nil {
		t.Fatalf("create revision audit trigger: %v", err)
	}

	input.Envelope.Turn.UserText = "revision audit must roll back"
	input.Envelope.Turn.SourceRevision = "revision-2"
	_, err = svc.Ingest(context.Background(), project.ID, &input)
	if err == nil || !strings.Contains(err.Error(), "audit") {
		t.Fatalf("revision Ingest error = %v, want audit failure", err)
	}
	assertExperienceCounts(t, db, 1, 1, 0, 0)
	var sourceRevision string
	if err := db.DB.QueryRow(`SELECT source_revision FROM experience_turns WHERE id = ?`, string(first.Turn.ID)).Scan(&sourceRevision); err != nil {
		t.Fatalf("read rolled back revision: %v", err)
	}
	if sourceRevision != first.Turn.SourceRevision {
		t.Fatalf("source revision = %q, want %q", sourceRevision, first.Turn.SourceRevision)
	}
	var failureAudits int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = 'experience_ingestion_failed'`).Scan(&failureAudits); err != nil {
		t.Fatalf("failure audit count: %v", err)
	}
	if failureAudits != 1 {
		t.Fatalf("failure audit count = %d, want 1", failureAudits)
	}
}

func TestIngestAdvancesCursorOnlyAfterDataCommit(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	if _, err := db.DB.Exec(`
		CREATE TRIGGER fail_experience_cursor
		BEFORE INSERT ON ingestion_cursors
		BEGIN
			SELECT RAISE(ABORT, 'test cursor failure');
		END;
	`); err != nil {
		t.Fatalf("create cursor trigger: %v", err)
	}

	input := IngestInput{
		Envelope:       validEnvelope(projectRoot, "cursor-after-commit"),
		SourceInstance: "opencode-db",
		CursorJSON:     `{"offset":10}`,
		SourceOrder:    1,
	}
	result, err := NewService(db, Config{Now: fixedClock()}).Ingest(context.Background(), project.ID, &input)
	if err == nil || !strings.Contains(err.Error(), "cursor") {
		t.Fatalf("Ingest error = %v, want cursor failure", err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want no result after atomic rollback", result)
	}
	assertExperienceCounts(t, db, 0, 0, 0, 0)
	var auditCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditCount); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("audit count = %d, want failure audit", auditCount)
	}
}

func TestIngestFailureUpdatesCursorAndAuditWithoutAdvancingIt(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	secret := "failure-audit-secret"
	svc := NewService(db, Config{KnownSecrets: []string{secret}, Now: fixedClock()})
	first := IngestInput{
		Envelope:       validEnvelope(projectRoot, "failure-cursor"),
		SourceInstance: "opencode-db",
		CursorJSON:     `{"offset":1}`,
		SourceOrder:    1,
	}
	if _, err := svc.Ingest(context.Background(), project.ID, &first); err != nil {
		t.Fatalf("first Ingest: %v", err)
	}

	failed := first
	failed.Envelope.Session.Locator.Path = filepath.Join(t.TempDir(), "outside-"+secret+".db")
	failed.CursorJSON = `{"offset":2}`
	failed.SourceOrder = 2
	_, err := svc.Ingest(context.Background(), project.ID, &failed)
	assertExperienceCode(t, err, domain.ErrExperienceLocatorOutsideRoot)
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("failure error leaked secret: %v", err)
	}

	var cursorJSON, errorCode, errorMessage string
	var sourceOrder int64
	var lastAttempt, lastSuccessful sql.NullString
	if err := db.DB.QueryRow(`SELECT cursor_json, source_order, last_error_code, last_error_message, last_attempt_at, last_successful_at FROM ingestion_cursors`).Scan(&cursorJSON, &sourceOrder, &errorCode, &errorMessage, &lastAttempt, &lastSuccessful); err != nil {
		t.Fatalf("read failed cursor: %v", err)
	}
	if cursorJSON != `{"offset":1}` || sourceOrder != 1 {
		t.Fatalf("failed cursor advanced: json=%q order=%d", cursorJSON, sourceOrder)
	}
	if errorCode != string(domain.ErrExperienceLocatorOutsideRoot) || errorMessage == "" || !lastAttempt.Valid || !lastSuccessful.Valid {
		t.Fatalf("failure cursor = code:%q message:%q attempt:%v success:%v", errorCode, errorMessage, lastAttempt.Valid, lastSuccessful.Valid)
	}

	var auditCode, auditDetails string
	if err := db.DB.QueryRow(`SELECT error_code, details_json FROM audit_events WHERE operation = 'experience_ingestion_failed'`).Scan(&auditCode, &auditDetails); err != nil {
		t.Fatalf("read failure audit: %v", err)
	}
	if auditCode != string(domain.ErrExperienceLocatorOutsideRoot) || strings.Contains(auditDetails, secret) || strings.Contains(auditDetails, "outside-") {
		t.Fatalf("failure audit = code:%q details:%q", auditCode, auditDetails)
	}
	var cursorFailureAudits int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = 'experience_cursor_failure_recorded'`).Scan(&cursorFailureAudits); err != nil {
		t.Fatalf("cursor failure audit count: %v", err)
	}
	if cursorFailureAudits != 1 {
		t.Fatalf("cursor failure audit count = %d, want 1", cursorFailureAudits)
	}
	assertNoSecretInExperienceSinks(t, db, secret)
}

func TestIngestFailedCursorDoesNotPoisonSuccessfulSourceOrder(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})

	failed := IngestInput{
		Envelope:       validEnvelope(projectRoot, "failure-order"),
		SourceInstance: "opencode-db",
		CursorJSON:     `{"offset":3}`,
		SourceOrder:    3,
	}
	failed.Envelope.Session.Locator.Path = filepath.Join(t.TempDir(), "outside.db")
	assertExperienceCode(t, mustIngestError(t, svc, project.ID, &failed), domain.ErrExperienceLocatorOutsideRoot)

	firstSuccess := IngestInput{
		Envelope:       validEnvelope(projectRoot, "successful-order"),
		SourceInstance: "opencode-db",
		CursorJSON:     `{"offset":2}`,
		SourceOrder:    2,
	}
	if _, err := svc.Ingest(context.Background(), project.ID, &firstSuccess); err != nil {
		t.Fatalf("successful lower source order after failure: %v", err)
	}

	failed.Envelope.ProjectRoot = projectRoot
	failed.Envelope.Session.Locator.Path = filepath.Join(projectRoot, "source.db")
	if _, err := svc.Ingest(context.Background(), project.ID, &failed); err != nil {
		t.Fatalf("retry failed source order: %v", err)
	}
	var sourceOrder int64
	if err := db.DB.QueryRow(`SELECT source_order FROM ingestion_cursors`).Scan(&sourceOrder); err != nil {
		t.Fatalf("read recovered cursor: %v", err)
	}
	if sourceOrder != 3 {
		t.Fatalf("recovered source order = %d, want 3", sourceOrder)
	}
	assertExperienceCounts(t, db, 2, 2, 0, 1)
}

func TestIngestFailureRecordingErrorIsObservableWithoutReplacingPrimaryError(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	if _, err := db.DB.Exec(`
		CREATE TRIGGER fail_failure_audit
		BEFORE INSERT ON audit_events
		WHEN NEW.operation = 'experience_ingestion_failed'
		BEGIN
			SELECT RAISE(ABORT, 'failure audit unavailable');
		END;
	`); err != nil {
		t.Fatalf("create failure audit trigger: %v", err)
	}

	envelope := validEnvelope(projectRoot, "failure-recording")
	envelope.Session.Locator.Path = filepath.Join(t.TempDir(), "outside.db")
	_, err := NewService(db, Config{Now: fixedClock()}).Ingest(context.Background(), project.ID, &IngestInput{
		Envelope:       envelope,
		SourceInstance: "opencode-db",
		CursorJSON:     `{"offset":1}`,
		SourceOrder:    1,
	})
	assertExperienceCode(t, err, domain.ErrExperienceLocatorOutsideRoot)
	if !strings.Contains(err.Error(), "experience: record failure") || !strings.Contains(err.Error(), "failure audit unavailable") {
		t.Fatalf("error = %v, want primary and failure-recording errors", err)
	}
}

func TestFailureRecordingContextHasBoundedDeadline(t *testing.T) {
	svc := NewService(nil, Config{FailureTimeout: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	failureCtx, release := svc.failureContext(ctx)
	defer release()
	deadline, ok := failureCtx.Deadline()
	if !ok {
		t.Fatal("failure context has no deadline")
	}
	if deadline.After(time.Now().Add(maxFailureRecordingTimeout + time.Second)) {
		t.Fatalf("failure deadline = %s, want bounded timeout", deadline)
	}
	if failureCtx.Err() != nil {
		t.Fatalf("failure context inherited cancellation: %v", failureCtx.Err())
	}
}

func TestIngestAuditsSessionRefreshAndCursorAdvanceWithRedactedPayloads(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	secret := "session-audit-secret"
	svc := NewService(db, Config{KnownSecrets: []string{secret}, Now: fixedClock()})
	first := IngestInput{
		Envelope:       validEnvelope(projectRoot, "audit-refresh"),
		SourceInstance: "opencode-db",
		CursorJSON:     `{"offset":1,"token":"` + secret + `"}`,
		SourceOrder:    1,
	}
	if _, err := svc.Ingest(context.Background(), project.ID, &first); err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	second := first
	second.Envelope.Turn.ExternalID = "turn-audit-refresh-2"
	second.Envelope.Turn.Sequence = 2
	second.Envelope.Turn.SourceRevision = "revision-2"
	second.Envelope.Turn.UserText = "newer metadata"
	second.Envelope.Session.UpdatedAt = second.Envelope.Session.UpdatedAt.Add(time.Hour)
	second.CursorJSON = `{"offset":2,"token":"` + secret + `"}`
	second.SourceOrder = 2
	if _, err := svc.Ingest(context.Background(), project.ID, &second); err != nil {
		t.Fatalf("second ingest: %v", err)
	}

	for _, operation := range []string{"experience_session_updated", "experience_cursor_created", "experience_cursor_advanced"} {
		var count int
		if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = ?`, operation).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", operation, err)
		}
		if operation == "experience_cursor_created" && count != 1 {
			t.Fatalf("%s count = %d, want 1", operation, count)
		}
		if operation != "experience_cursor_created" && count != 1 {
			t.Fatalf("%s count = %d, want 1", operation, count)
		}
	}
	assertNoSecretInExperienceSinks(t, db, secret)
}

func TestIngestMetricsExposeCountersAndDuration(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})
	if _, err := svc.Ingest(context.Background(), project.ID, &IngestInput{Envelope: validEnvelope(projectRoot, "metrics-success")}); err != nil {
		t.Fatalf("successful ingest: %v", err)
	}
	failed := validEnvelope(projectRoot, "metrics-error")
	failed.Session.Locator.Path = filepath.Join(t.TempDir(), "outside.db")
	if _, err := svc.Ingest(context.Background(), project.ID, &IngestInput{Envelope: failed}); err == nil {
		t.Fatal("failed ingest returned nil error")
	}
	snapshot := svc.Metrics()
	if snapshot.Attempts != 2 || snapshot.Successes != 1 || snapshot.Errors != 1 {
		t.Fatalf("metrics = %#v, want two attempts with one success and one error", snapshot)
	}
	if snapshot.TotalDuration <= 0 || snapshot.LastDuration <= 0 {
		t.Fatalf("metrics durations = total:%s last:%s, want positive durations", snapshot.TotalDuration, snapshot.LastDuration)
	}
}

func TestIngestCommitUnknownDoesNotRecordContradictoryFailureCursor(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})
	svc.commitTx = func(*sql.Tx) error { return errors.New("commit transport lost") }
	_, err := svc.Ingest(context.Background(), project.ID, &IngestInput{
		Envelope:       validEnvelope(projectRoot, "commit-unknown"),
		SourceInstance: "opencode-db",
		CursorJSON:     `{"offset":1}`,
		SourceOrder:    1,
	})
	assertExperienceCode(t, err, domain.ErrExperienceCommitUnknown)
	assertExperienceCounts(t, db, 0, 0, 0, 0)

	var unknownAudits, failureAudits int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = 'experience_ingestion_commit_unknown' AND result = 'commit_unknown'`).Scan(&unknownAudits); err != nil {
		t.Fatalf("unknown audit count: %v", err)
	}
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = 'experience_ingestion_failed'`).Scan(&failureAudits); err != nil {
		t.Fatalf("failure audit count: %v", err)
	}
	if unknownAudits != 1 || failureAudits != 0 {
		t.Fatalf("unknown audits = %d, failure audits = %d, want 1 and 0", unknownAudits, failureAudits)
	}
	if snapshot := svc.Metrics(); snapshot.CommitUnknowns != 1 {
		t.Fatalf("metrics = %#v, want one commit unknown", snapshot)
	}
}

func TestIngestCancellationLeavesStateUnchanged(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewService(db, Config{Now: fixedClock()}).Ingest(
		ctx, project.ID, &IngestInput{Envelope: validEnvelope(projectRoot, "cancelled")},
	)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Ingest error = %v, want context.Canceled", err)
	}
	assertExperienceCounts(t, db, 0, 0, 0, 0)
}

func TestIngestNilContextUsesBackground(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	result, err := NewService(db, Config{Now: fixedClock()}).Ingest(nil, project.ID, &IngestInput{
		Envelope: validEnvelope(projectRoot, "nil-context"),
	})
	if err != nil || result == nil || !result.Created {
		t.Fatalf("Ingest(nil context) = %#v, %v", result, err)
	}
}

func newExperienceTestDB(t *testing.T) (*storage.DB, *domain.Project, string) {
	t.Helper()
	root := t.TempDir()
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("create project root: %v", err)
	}
	db, err := storage.Open(filepath.Join(root, "royo.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := storage.Migrate(db); err != nil {
		db.Close()
		t.Fatalf("migrate database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	now := fixedClock()()
	project := &domain.Project{
		ID:            domain.ProjectID(uuid.Must(uuid.NewV7()).String()),
		ProjectKey:    "experience-test",
		DisplayName:   "Experience Test",
		CanonicalPath: projectRoot,
		Fingerprint:   "project-fingerprint",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := storage.WithTx(context.Background(), db, func(tx *sql.Tx) error {
		return storage.SaveProject(context.Background(), tx, project)
	}); err != nil {
		t.Fatalf("save project: %v", err)
	}
	return db, project, projectRoot
}

func fixedClock() func() time.Time {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return now }
}

func assertExperienceCounts(t *testing.T, db *storage.DB, sessions, turns, events, cursors int) {
	t.Helper()
	got := make([]int, 4)
	for i, table := range []string{"experience_sessions", "experience_turns", "experience_events", "ingestion_cursors"} {
		if err := db.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got[i]); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
	}
	want := []int{sessions, turns, events, cursors}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("experience counts = %v, want %v", got, want)
	}
}

func assertNoSecretInExperienceSinks(t *testing.T, db *storage.DB, secret string) {
	t.Helper()
	queries := []string{
		`SELECT external_session_id, locator_json, metadata_sha256 FROM experience_sessions`,
		`SELECT external_turn_id, fingerprint, user_digest, assistant_digest, tool_calls_digest, source_revision FROM experience_turns`,
		`SELECT actor_json, previous_state, new_state, details_json FROM audit_events`,
		`SELECT source_instance, cursor_json, input_digest, last_error_message FROM ingestion_cursors`,
	}
	for _, query := range queries {
		rows, err := db.DB.Query(query)
		if err != nil {
			t.Fatalf("query sink: %v", err)
		}
		for rows.Next() {
			values := make([]any, strings.Count(query, ",")+1)
			pointers := make([]any, len(values))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				rows.Close()
				t.Fatalf("scan sink: %v", err)
			}
			if strings.Contains(fmt.Sprint(values), secret) {
				rows.Close()
				t.Fatalf("secret reached sink for query %q: %v", query, values)
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close sink rows: %v", err)
		}
	}
}

func assertExperienceCode(t *testing.T, err error, want domain.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	got, ok := domain.AsDomainError(err)
	if !ok || got.Code != want {
		t.Fatalf("error = %T %v, code = %q; want %q", err, err, experienceCode(got), want)
	}
}

func mustIngestError(t *testing.T, svc *Service, projectID domain.ProjectID, input *IngestInput) error {
	t.Helper()
	_, err := svc.Ingest(context.Background(), projectID, input)
	return err
}

func experienceCode(err *domain.DomainError) domain.ErrorCode {
	if err == nil {
		return ""
	}
	return err.Code
}
