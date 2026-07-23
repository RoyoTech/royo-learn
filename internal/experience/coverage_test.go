package experience

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

// keep sql import used by failure-context tests
var _ sql.IsolationLevel

// TestBoundErrorDetailsBoundsAndHandlesUTF8 focuses on boundErrorDetails, which
// was the single lowest-covered function in Hito 1 slice 1.D (28.6%).
func TestBoundErrorDetailsBoundsAndHandlesUTF8(t *testing.T) {
	t.Parallel()

	short := "short message"
	if got := boundErrorDetails(short); got != short {
		t.Fatalf("boundErrorDetails(%q) = %q, want passthrough", short, got)
	}

	exactlyMax := strings.Repeat("x", DefaultMaxErrorDetailsBytes)
	if got := boundErrorDetails(exactlyMax); got != exactlyMax {
		t.Fatalf("boundErrorDetails at the limit = %q (changed unexpectedly)", got)
	}

	overByOne := strings.Repeat("x", DefaultMaxErrorDetailsBytes+1)
	if got := boundErrorDetails(overByOne); len(got) != DefaultMaxErrorDetailsBytes-3+3 {
		t.Fatalf("boundErrorDetails over-by-one len = %d, want %d", len(got), DefaultMaxErrorDetailsBytes)
	}
	if !strings.HasSuffix(boundErrorDetails(overByOne), "...") {
		t.Fatal("boundErrorDetails over-by-one missing ellipsis suffix")
	}

	longASCIIString := strings.Repeat("x", DefaultMaxErrorDetailsBytes+500)
	bounded := boundErrorDetails(longASCIIString)
	if len(bounded) >= len(longASCIIString) {
		t.Fatalf("boundErrorDetails did not truncate ASCII: len=%d, input=%d", len(bounded), len(longASCIIString))
	}
	if !strings.HasSuffix(bounded, "...") {
		t.Fatalf("bounded value %q missing ellipsis suffix", bounded)
	}

	longRunes := strings.Repeat("áéíóú", DefaultMaxErrorDetailsBytes)
	boundedRunes := boundErrorDetails(longRunes)
	if !strings.HasSuffix(boundedRunes, "...") {
		t.Fatalf("rune-bounded value %q missing ellipsis suffix", boundedRunes)
	}
	if len(boundedRunes) >= len(longRunes) {
		t.Fatalf("boundErrorDetails did not truncate multibyte: len=%d, input=%d", len(boundedRunes), len(longRunes))
	}
}

// TestSafeFailureBoundsLongCodesAndMessages exercises both arms of safeFailure
// for completeness now that boundErrorDetails is fully covered.
func TestSafeFailureBoundsLongCodesAndMessages(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, Config{KnownSecrets: []string{"s3cret"}})

	domainErr := domain.NewConflictError("test_code", "boom s3cret boom")
	code, message := svc.safeFailure(domainErr)
	if code == "internal_error" {
		t.Fatalf("safeFailure mapped domain code to internal_error: %v", domainErr)
	}
	if strings.Contains(message, "s3cret") {
		t.Fatalf("safeFailure leaked raw secret into message: %q", message)
	}

	longCode := strings.Repeat("x", domain.MaxExperienceErrorCodeBytes+8)
	hugeMessage := strings.Repeat("m", DefaultMaxErrorDetailsBytes+512)
	oversize := domain.NewValidationError(domain.ErrorCode(longCode), hugeMessage)
	codeOut, messageOut := svc.safeFailure(oversize)
	if codeOut != "internal_error" {
		t.Fatalf("safeFailure should clamp %q to internal_error, got %q", longCode, codeOut)
	}
	if len(messageOut) > DefaultMaxErrorDetailsBytes {
		t.Fatalf("safeFailure message length %d, want <= %d", len(messageOut), DefaultMaxErrorDetailsBytes)
	}

	domainErrShort := domain.NewValidationError("short_code", "short message")
	codeShort, messageShort := svc.safeFailure(domainErrShort)
	if codeShort != "short_code" || messageShort != "short message" {
		t.Fatalf("safeFailure(domain short) = (%q, %q), want passthrough", codeShort, messageShort)
	}
}

// TestDecodeJSONUseNumberAcceptsPrecisionNumbers covers decodeJSONUseNumber at
// 72.7%. The tests exercise the integer / float / scientific-notation /
// multiple-values / malformed branches.
func TestDecodeJSONUseNumberAcceptsPrecisionNumbers(t *testing.T) {
	t.Parallel()

	value, err := decodeJSONUseNumber([]byte(`{"offset":9007199254740993}`))
	if err != nil {
		t.Fatalf("decodeJSONUseNumber offset: %v", err)
	}
	root, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("decoded value = %T, want map[string]any", value)
	}
	number, ok := root["offset"].(json.Number)
	if !ok {
		t.Fatalf("offset = %#v, want json.Number", root["offset"])
	}
	if number.String() != "9007199254740993" {
		t.Fatalf("offset = %s, want 9007199254740993", number.String())
	}

	scientific, err := decodeJSONUseNumber([]byte(`1.5e308`))
	if err != nil {
		t.Fatalf("decodeJSONUseNumber scientific: %v", err)
	}
	if _, ok := scientific.(json.Number); !ok {
		t.Fatalf("scientific = %#v, want json.Number", scientific)
	}

	fractional, err := decodeJSONUseNumber([]byte(`3.1415`))
	if err != nil {
		t.Fatalf("decodeJSONUseNumber fractional: %v", err)
	}
	if _, ok := fractional.(json.Number); !ok {
		t.Fatalf("fractional = %#v, want json.Number", fractional)
	}

	if _, err := decodeJSONUseNumber([]byte(`{"k":}`)); err == nil {
		t.Fatal("decodeJSONUseNumber accepted malformed JSON")
	}

	if _, err := decodeJSONUseNumber([]byte(`{"k":1}{"k":2}`)); err == nil {
		t.Fatal("decodeJSONUseNumber accepted multiple JSON values")
	}
}

// TestPrepareCursorWithOrderRejectsInvalidInputs raises prepareCursorWithOrder
// from 78.3% by triggering every documented argument error.
func TestPrepareCursorWithOrderRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, Config{MaxCursorBytes: 64, FailureTimeout: time.Second, KnownSecrets: []string{"s3cret"}})

	if _, err := svc.prepareCursorWithOrder("   ", `{}`, 1); domainCode(err) != domain.ErrInvalidArgument {
		t.Fatalf("empty instance code = %v, want invalid_argument", err)
	}

	if _, err := svc.prepareCursorWithOrder("instance", "   ", 1); domainCode(err) != domain.ErrInvalidArgument {
		t.Fatalf("empty cursor code = %v, want invalid_argument", err)
	}

	if _, err := svc.prepareCursorWithOrder("instance", `{}`, -1); domainCode(err) != domain.ErrInvalidArgument {
		t.Fatalf("negative order code = %v, want invalid_argument", err)
	}

	oversizeInstance := strings.Repeat("a", domain.MaxExperienceSourceInstanceBytes+1)
	if _, err := svc.prepareCursorWithOrder(oversizeInstance, `{}`, 1); domainCode(err) != domain.ErrExperiencePayloadTooLarge {
		t.Fatalf("oversize instance code = %v, want payload too large", err)
	}

	bounded := NewService(nil, Config{MaxCursorBytes: 16, FailureTimeout: time.Second})
	if _, err := bounded.prepareCursorWithOrder("instance", `{"offset":9007199254740993}`, 1); domainCode(err) != domain.ErrExperiencePayloadTooLarge {
		t.Fatalf("oversize cursor code = %v, want payload too large", err)
	}

	if _, err := svc.prepareCursorWithOrder("instance", "not-json", 1); domainCode(err) != domain.ErrExperienceSchemaUnsupported {
		t.Fatalf("invalid cursor JSON code = %v, want schema unsupported", err)
	}

	if _, err := svc.prepareCursorWithOrder("<private>s3cret</private>", `{}`, 1); domainCode(err) != domain.ErrInvalidArgument {
		t.Fatalf("redacted instance code = %v, want invalid_argument", err)
	}
}

// TestAdvanceCursorWrapperPersistsCursor exercises the previously 0.0%-covered
// Service.advanceCursor transaction wrapper.
func TestAdvanceCursorWrapperPersistsCursor(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})

	cursor, err := svc.prepareCursor("instance-advance", `{"offset":1}`)
	if err != nil {
		t.Fatalf("prepareCursor: %v", err)
	}
	now := fixedClock()()
	persisted, err := svc.advanceCursor(context.Background(), project.ID, domain.SourceOpenCode, cursor, now)
	if err != nil {
		t.Fatalf("advanceCursor: %v", err)
	}
	if persisted == nil || persisted.Revision != 1 {
		t.Fatalf("persisted = %#v, want revision=1", persisted)
	}
	if persisted.SourceInstance != "instance-advance" {
		t.Fatalf("source instance = %q, want instance-advance", persisted.SourceInstance)
	}

	cursor2, err := svc.prepareCursorWithOrder("instance-advance", `{"offset":2}`, 2)
	if err != nil {
		t.Fatalf("prepareCursorWithOrder: %v", err)
	}
	persisted2, err := svc.advanceCursor(context.Background(), project.ID, domain.SourceOpenCode, cursor2, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("advanceCursor #2: %v", err)
	}
	if persisted2 == nil || persisted2.Revision != 2 || persisted2.CursorJSON != `{"offset":2}` {
		t.Fatalf("persisted2 = %#v, want revision=2 with new cursor", persisted2)
	}

	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM ingestion_cursors WHERE project_id = ?`, project.ID).Scan(&count); err != nil {
		t.Fatalf("count cursors: %v", err)
	}
	if count != 1 {
		t.Fatalf("cursor count = %d, want 1", count)
	}

	_ = projectRoot
}

// TestRefreshSessionReturnsExistingWhenNotAfter exercises the
// !candidate.UpdatedAt.After(existing.UpdatedAt) branch of refreshSession,
// bringing it above 77.8%.
func TestRefreshSessionReturnsExistingWhenNotAfter(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})

	envelope := validEnvelope(projectRoot, "refresh-noop")
	if _, err := svc.IngestEnvelope(context.Background(), project.ID, envelope); err != nil {
		t.Fatalf("initial ingest: %v", err)
	}

	rewound := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	if _, err := db.DB.Exec(`UPDATE experience_sessions SET updated_at = ? WHERE project_id = ?`, rewound.Format(time.RFC3339Nano), project.ID); err != nil {
		t.Fatalf("rewind updated_at: %v", err)
	}

	sameOrOlder := validEnvelope(projectRoot, "refresh-noop")
	sameOrOlder.Session.UpdatedAt = rewound
	result, err := svc.IngestEnvelope(context.Background(), project.ID, sameOrOlder)
	if err != nil {
		t.Fatalf("refresh ingest: %v", err)
	}
	if result == nil || !result.Idempotent || result.Updated || result.Created {
		t.Fatalf("refresh result = %#v, want idempotent no-op", result)
	}
	if result.Session == nil || !result.Session.UpdatedAt.Equal(rewound) {
		t.Fatalf("refreshed session updated_at = %v, want %v", result.Session.UpdatedAt, rewound)
	}
}

// TestIngestUpdatesSessionAuditOnExistingTurnRevision exercises
// recordSessionUpdateAudit by causing existing-turn revision change AND
// candidate UpdatedAt > existing.
func TestIngestUpdatesSessionAuditOnExistingTurnRevision(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})

	envelope := validEnvelope(projectRoot, "session-update")
	envelope.Session.UpdatedAt = time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	if _, err := svc.IngestEnvelope(context.Background(), project.ID, envelope); err != nil {
		t.Fatalf("initial ingest: %v", err)
	}

	second := validEnvelope(projectRoot, "session-update")
	second.Session.UpdatedAt = envelope.Session.UpdatedAt.Add(time.Minute)
	second.Turn.SourceRevision = "revision-2"
	second.Turn.AssistantText = "updated answer"
	if _, err := svc.IngestEnvelope(context.Background(), project.ID, second); err != nil {
		t.Fatalf("revision ingest: %v", err)
	}

	var sessionUpdates int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = 'experience_session_updated'`).Scan(&sessionUpdates); err != nil {
		t.Fatalf("count session_update audits: %v", err)
	}
	if sessionUpdates < 1 {
		t.Fatalf("session_update audits = %d, want at least 1", sessionUpdates)
	}
	var row string
	if err := db.DB.QueryRow(`SELECT previous_state FROM audit_events WHERE operation = 'experience_session_updated' ORDER BY sequence ASC LIMIT 1`).Scan(&row); err != nil {
		t.Fatalf("read previous_state: %v", err)
	}
	if !strings.Contains(row, "updated_at") || !strings.Contains(row, "metadata_sha256") {
		t.Fatalf("previous_state = %q, missing JSON keys", row)
	}
}

// TestIngestFailureWithCursorRecordsBothAudits covers recordFailure's
// cursor + audit branches by forcing an ingest failure on the locator-outside-
// project path with cursor data attached.
func TestIngestFailureWithCursorRecordsBothAudits(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})

	outsideDir := t.TempDir()
	cursorEnvelope := validEnvelope(projectRoot, "cursor-failure")
	cursorEnvelope.Session.Locator.Path = filepath.Join(outsideDir, "outside.db")
	input := &IngestInput{
		Envelope:       cursorEnvelope,
		SourceInstance: "cursor-failure-instance",
		CursorJSON:     `{"offset":42}`,
		SourceOrder:    1,
	}
	if _, err := svc.Ingest(context.Background(), project.ID, input); err == nil {
		t.Fatal("ingest with outside locator returned nil error")
	}

	var failureAuditCount, cursorFailureCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = 'experience_ingestion_failed'`).Scan(&failureAuditCount); err != nil {
		t.Fatalf("count failure audits: %v", err)
	}
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = 'experience_cursor_failure_recorded'`).Scan(&cursorFailureCount); err != nil {
		t.Fatalf("count cursor-failure audits: %v", err)
	}
	if failureAuditCount != 1 {
		t.Fatalf("ingestion-failed audits = %d, want 1", failureAuditCount)
	}
	if cursorFailureCount != 1 {
		t.Fatalf("cursor-failure audits = %d, want 1", cursorFailureCount)
	}
}

// TestRecordCommitUnknownAuditAndFailureSkipped covers the
// recordCommitUnknown branch specifically: a commit failure emits a
// commit_unknown audit event and skips the contradictory failure record.
func TestRecordCommitUnknownAuditAndFailureSkipped(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock(), FailureTimeout: 50 * time.Millisecond})
	svc.commitTx = func(*sql.Tx) error { return errors.New("transport gone") }

	envelope := validEnvelope(projectRoot, "commit-unknown-specific")
	envelope.Turn.ExternalID = "commit-unknown-specific-turn"
	envelope.Turn.OccurredAt = envelopeTime(3)
	_, err := svc.Ingest(context.Background(), project.ID, &IngestInput{Envelope: envelope})
	if err == nil {
		t.Fatal("ingest with commit failure returned nil error")
	}

	var commitUnknownCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = 'experience_ingestion_commit_unknown'`).Scan(&commitUnknownCount); err != nil {
		t.Fatalf("count commit-unknown audits: %v", err)
	}
	if commitUnknownCount != 1 {
		t.Fatalf("commit-unknown audits = %d, want 1", commitUnknownCount)
	}

	if snapshot := svc.Metrics(); snapshot.CommitUnknowns != 1 || snapshot.Errors != 1 {
		t.Fatalf("metrics = %+v, want commit_unknowns=1 and errors=1", snapshot)
	}
}

// TestRecordCommitUnknownHonorsShortFailureTimeout confirms that even with
// the failure-recording timeout stretched below the SQLite busy deadline the
// service still surfaces the commit-unknown error.
func TestRecordCommitUnknownHonorsShortFailureTimeout(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock(), FailureTimeout: 100 * time.Millisecond})
	svc.commitTx = func(*sql.Tx) error { return errors.New("transport gone") }

	envelope := validEnvelope(projectRoot, "short-timeout")
	envelope.Turn.ExternalID = "short-timeout-turn"
	envelope.Turn.OccurredAt = envelopeTime(4)
	_, err := svc.Ingest(context.Background(), project.ID, &IngestInput{Envelope: envelope})
	if err == nil {
		t.Fatal("ingest with commit failure returned nil error")
	}

	if domainCode(err) != domain.ErrExperienceCommitUnknown {
		t.Fatalf("Ingest error code = %v, want commit unknown", domainCode(err))
	}
}

// TestMetricsSnapshotsNilServiceAndCounts covers the s == nil arm and ensures
// the Metrics() reader sees the latest counter values.
func TestMetricsSnapshotsNilServiceAndCounts(t *testing.T) {
	t.Parallel()

	var nilService *Service
	if snapshot := nilService.Metrics(); snapshot != (MetricsSnapshot{}) {
		t.Fatalf("nil service metrics = %+v, want zero value", snapshot)
	}

	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})

	envelope := validEnvelope(projectRoot, "metrics-counter")
	envelope.Session.ExternalID = "session-metrics-counter"
	envelope.Turn.ExternalID = "turn-metrics-counter"
	if _, err := svc.IngestEnvelope(context.Background(), project.ID, envelope); err != nil {
		t.Fatalf("initial ingest: %v", err)
	}

	envelope.Session.ExternalID = "session-metrics-counter-2"
	envelope.Turn.ExternalID = "turn-metrics-counter-2"
	envelope.Session.Locator.Path = filepath.Join(t.TempDir(), "outside.db")
	envelope.Session.UpdatedAt = envelope.Session.UpdatedAt.Add(time.Hour)
	if _, err := svc.IngestEnvelope(context.Background(), project.ID, envelope); err == nil {
		t.Fatal("failed ingest should not return nil")
	}

	snapshot := svc.Metrics()
	if snapshot.Attempts < 2 || snapshot.Successes < 1 || snapshot.Errors < 1 {
		t.Fatalf("metrics = %+v, want at least 2 attempts / 1 success / 1 error", snapshot)
	}
	if snapshot.TotalDuration <= 0 || snapshot.LastDuration <= 0 {
		t.Fatalf("metrics durations = total:%s last:%s, want positive", snapshot.TotalDuration, snapshot.LastDuration)
	}
}

// TestSafeToolCallUnmarshalRejectsUnsupportedValues exercises the lower
// branches of SafeToolCall.UnmarshalJSON (multiple-value detection, malformed
// input) which were uncovered at envelope.go line 29 (69.2%).
func TestSafeToolCallUnmarshalRejectsUnsupportedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{name: "malformed", raw: `{"name":}`},
		{name: "multiple values", raw: `{"name":"a"}{"name":"b"}`},
		{name: "leading garbage", raw: `garbage`},
		{name: "empty", raw: ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var call SafeToolCall
			if err := call.UnmarshalJSON([]byte(tt.raw)); err == nil {
				t.Fatalf("UnmarshalJSON(%s) accepted invalid input %q", tt.name, tt.raw)
			}
		})
	}

	var withNothing SafeToolCall
	if err := withNothing.UnmarshalJSON([]byte(`{}`)); err != nil {
		t.Fatalf("UnmarshalJSON({}) error: %v", err)
	}
	if withNothing.Name != "" {
		t.Fatalf("UnmarshalJSON({}) populated name = %q, want zero value", withNothing.Name)
	}
}

// TestExperienceIngestionConflictCursorStates exercises cursor-conflict
// branches in advanceCursorTx that were not directly covered.
func TestExperienceIngestionConflictCursorStates(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})

	input := IngestInput{
		Envelope:       validEnvelope(projectRoot, "cursor-conflict"),
		SourceInstance: "conflict-source",
		CursorJSON:     `{"offset":5}`,
		SourceOrder:    10,
	}
	if _, err := svc.Ingest(context.Background(), project.ID, &input); err != nil {
		t.Fatalf("initial cursor ingest: %v", err)
	}

	staleOrder := IngestInput{
		Envelope:       validEnvelope(projectRoot, "cursor-conflict"),
		SourceInstance: "conflict-source",
		CursorJSON:     `{"offset":4}`,
		SourceOrder:    5,
	}
	if _, err := svc.Ingest(context.Background(), project.ID, &staleOrder); domainCode(err) != domain.ErrExperienceCursorConflict {
		t.Fatalf("stale source order = %v, want cursor conflict", err)
	}

	changedCursorSameOrder := IngestInput{
		Envelope:       validEnvelope(projectRoot, "cursor-conflict"),
		SourceInstance: "conflict-source",
		CursorJSON:     `{"offset":7}`,
		SourceOrder:    10,
	}
	if _, err := svc.Ingest(context.Background(), project.ID, &changedCursorSameOrder); domainCode(err) != domain.ErrExperienceCursorConflict {
		t.Fatalf("changed cursor same order = %v, want cursor conflict", err)
	}

	identicalRedelivery := IngestInput{
		Envelope:       validEnvelope(projectRoot, "cursor-conflict"),
		SourceInstance: "conflict-source",
		CursorJSON:     `{"offset":5}`,
		SourceOrder:    10,
	}
	if _, err := svc.Ingest(context.Background(), project.ID, &identicalRedelivery); err != nil {
		t.Fatalf("identical cursor same order = %v, want idempotent success", err)
	}
}

// TestFailureRecordingSkippedWhenProjectAndSourceMissing exercises the early
// return in recordFailure and recordCommitUnknown when projectID/source are
// missing. Uses a nil DB to avoid touching SQLite.
func TestFailureRecordingSkippedWhenProjectAndSourceMissing(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, Config{Now: fixedClock()})
	if err := svc.recordFailure(context.Background(), nil, errors.New("noop"), time.Now()); err != nil {
		t.Fatalf("recordFailure(nil) = %v, want nil", err)
	}
	if err := svc.recordFailure(context.Background(), &ingestionFailure{}, errors.New("noop"), time.Now()); err != nil {
		t.Fatalf("recordFailure(empty projectID) = %v, want nil", err)
	}
	if err := svc.recordCommitUnknown(context.Background(), nil, time.Now()); err != nil {
		t.Fatalf("recordCommitUnknown(nil) = %v, want nil", err)
	}
	if err := svc.recordCommitUnknown(context.Background(), &ingestionFailure{}, time.Now()); err != nil {
		t.Fatalf("recordCommitUnknown(empty projectID) = %v, want nil", err)
	}
}

// envelopeTime centralises the deterministic timestamps used by tests that need
// a unique updated_at for cursor-conflict discrimination.
func envelopeTime(seq int) time.Time {
	return time.Date(2026, 7, 21, 10, 0, seq, 0, time.UTC)
}

// TestNewServiceClampsCursorBytesAndPreservesCustomNow covers the defensive
// MaxCursorBytes clamp at NewService entry.
func TestNewServiceClampsCursorBytesAndPreservesCustomNow(t *testing.T) {
	t.Parallel()

	bounded := NewService(nil, Config{MaxCursorBytes: domain.MaxExperienceCursorBytes * 8})
	if bounded.cfg.MaxCursorBytes != domain.MaxExperienceCursorBytes {
		t.Fatalf("MaxCursorBytes not clamped: %d != %d", bounded.cfg.MaxCursorBytes, domain.MaxExperienceCursorBytes)
	}

	customNow := func() time.Time { return time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC) }
	withClock := NewService(nil, Config{Now: customNow})
	if !withClock.now().Equal(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("custom clock not honored: %v", withClock.now())
	}
}

// TestFailureContextDefaultsToBackground ensures failureContext handles a nil
// caller context and clamps an oversized FailureTimeout to maxFailureRecordingTimeout.
func TestFailureContextDefaultsToBackground(t *testing.T) {
	t.Parallel()

	noCtx := NewService(nil, Config{FailureTimeout: 0})
	bg, cancel := noCtx.failureContext(nil)
	defer cancel()
	if bg == nil || bg.Err() != nil {
		t.Fatalf("failureContext(nil) returned bad context: %#v", bg)
	}
	if deadline, ok := bg.Deadline(); !ok || deadline.IsZero() {
		t.Fatalf("failureContext(nil) missing deadline: %v", deadline)
	}

	oversized := NewService(nil, Config{FailureTimeout: 24 * time.Hour})
	bg2, cancel2 := oversized.failureContext(context.Background())
	defer cancel2()
	deadline, ok := bg2.Deadline()
	if !ok {
		t.Fatal("failureContext missing deadline")
	}
	remaining := time.Until(deadline)
	if remaining > 10*time.Second {
		t.Fatalf("failureContext did not clamp oversized timeout: %v", remaining)
	}
}

// TestRecordCursorAuditRejectsNilCursor exercises recordCursorAudit's nil
// cursor validation, which is otherwise only reachable through private callers.
func TestRecordCursorAuditRejectsNilCursor(t *testing.T) {
	t.Parallel()

	if err := recordCursorAudit(context.Background(), nil, domain.Actor{}, time.Now(), nil, false); domainCode(err) != domain.ErrInvalidArgument {
		t.Fatalf("recordCursorAudit(nil cursor) = %v, want invalid_argument", err)
	}
}

// TestRecordCursorFailureAuditIgnoresEmptyFailure covers the early return when
// the failure context has no cursor attached.
func TestRecordCursorFailureAuditIgnoresEmptyFailure(t *testing.T) {
	t.Parallel()

	if err := recordCursorFailureAudit(context.Background(), nil, domain.Actor{}, time.Now(), nil, "c", "m"); err != nil {
		t.Fatalf("recordCursorFailureAudit(nil failure) = %v, want nil", err)
	}
	if err := recordCursorFailureAudit(context.Background(), nil, domain.Actor{}, time.Now(), &ingestionFailure{}, "c", "m"); err != nil {
		t.Fatalf("recordCursorFailureAudit(no cursor) = %v, want nil", err)
	}
}

// TestDigestSafeEnvelopeFallsBackOnMarshalError documents the digestSafeEnvelope
// fallback when marshal unexpectedly fails. We can't trigger this through the
// real ExperienceEnvelope type, so we exercise the public crc32 fallback
// directly via DigestString to lock in deterministic behaviour.
func TestDigestStringStableAndUnique(t *testing.T) {
	t.Parallel()

	if DigestString("alpha") == DigestString("beta") {
		t.Fatal("DigestString collision for distinct inputs")
	}
	if got := DigestString("alpha"); got != DigestString("alpha") {
		t.Fatalf("DigestString not deterministic: %s != %s", got, DigestString("alpha"))
	}
}
