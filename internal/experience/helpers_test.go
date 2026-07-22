package experience

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	projectpath "agent-royo-learn/internal/project"
)

func TestServiceHelperFailureAndBoundaryPaths(t *testing.T) {
	svc := NewService(nil)
	if svc.cfg.MaxTurnBytes != DefaultMaxTurnBytes || svc.cfg.MaxCursorBytes != DefaultMaxCursorBytes || svc.cfg.MaxLocatorBytes != DefaultMaxLocatorBytes {
		t.Fatalf("defaults = turn:%d cursor:%d locator:%d", svc.cfg.MaxTurnBytes, svc.cfg.MaxCursorBytes, svc.cfg.MaxLocatorBytes)
	}
	if svc.now().IsZero() {
		t.Fatal("zero clock should fall back to a real UTC time")
	}
	zeroClock := NewService(nil, Config{Now: func() time.Time { return time.Time{} }})
	if zeroClock.now().IsZero() {
		t.Fatal("zero configured clock should fall back to a real UTC time")
	}

	if _, _, err := svc.redactArguments(map[string]any{"bad": math.NaN()}); err == nil {
		t.Fatal("redactArguments accepted an unsupported JSON value")
	}
	if arguments, changed, err := svc.redactArguments(nil); err != nil || arguments != nil || changed {
		t.Fatalf("redactArguments(nil) = %#v changed:%v err:%v, want nil unchanged", arguments, changed, err)
	}
	if _, changed, err := svc.redactJSONValue(float64(1)); err != nil || changed {
		t.Fatalf("redactJSONValue scalar = changed:%v err:%v, want unchanged", changed, err)
	}
	if _, err := svc.prepareCursor("", `{}`); domainCode(err) != domain.ErrInvalidArgument {
		t.Fatalf("empty cursor instance error = %v, want invalid_argument", err)
	}
	if _, err := svc.prepareCursor("instance", "not-json"); domainCode(err) != domain.ErrExperienceSchemaUnsupported {
		t.Fatalf("invalid cursor JSON error = %v, want schema unsupported", err)
	}

	redactionSvc := NewService(nil, Config{KnownSecrets: []string{"secret\r\nvalue"}})
	redactedChanged := false
	clean, err := redactionSvc.redactText("prefix secret\r\nvalue", &redactedChanged)
	if err != nil || !redactedChanged || clean != "prefix [REDACTED:known]" {
		t.Fatalf("redact before normalization = %q changed:%v err:%v", clean, redactedChanged, err)
	}

	boundedSvc := NewService(nil, Config{MaxCursorBytes: 32})
	if _, err := boundedSvc.prepareCursor("instance", `{"cursor":"012345678901234567890123456789"}`); domainCode(err) != domain.ErrExperiencePayloadTooLarge {
		t.Fatalf("oversized cursor error = %v, want payload too large", err)
	}

	preciseCursor, err := svc.prepareCursor("instance", `{"offset":9007199254740993}`)
	if err != nil {
		t.Fatalf("precise cursor: %v", err)
	}
	if preciseCursor.cursorJSON != `{"offset":9007199254740993}` {
		t.Fatalf("cursor JSON = %q, want exact integer representation", preciseCursor.cursorJSON)
	}

	collisionInput := map[string]any{
		"<private>one</private>": 1,
		"<private>two</private>": 2,
	}
	_, _, collisionErr := svc.redactJSONValue(collisionInput)
	if domainCode(collisionErr) != domain.ErrExperienceSchemaUnsupported {
		t.Fatalf("redacted key collision error = %v, want schema unsupported", collisionErr)
	}
	if strings.Contains(collisionErr.Error(), "one") || strings.Contains(collisionErr.Error(), "two") {
		t.Fatalf("redacted key collision error leaked an original key: %v", collisionErr)
	}

	badTurn := validEnvelope(t.TempDir(), "bad-turn")
	badTurn.Turn.ToolCalls = []SafeToolCall{{Name: "tool", Arguments: map[string]any{"bad": math.NaN()}}}
	if _, err := buildTurn("session", badTurn, false, time.Now().UTC()); err == nil {
		t.Fatal("buildTurn accepted an unsupported tool argument")
	}

	if err := locatorPathError(&projectpath.Error{Code: projectpath.ErrSymlinkEscape}); domainCode(err) != domain.ErrExperienceLocatorOutsideRoot {
		t.Fatalf("symlink path error = %v, want outside-root", err)
	}
	if err := locatorPathError(&projectpath.Error{Code: projectpath.ErrPathOutsideRoot}); domainCode(err) != domain.ErrExperienceLocatorOutsideRoot {
		t.Fatalf("outside path error = %v, want outside-root", err)
	}
	if err := locatorPathError(errors.New("invalid")); domainCode(err) != domain.ErrExperienceLocatorInvalid {
		t.Fatalf("generic path error = %v, want locator-invalid", err)
	}

	if !normalizeTime(time.Time{}).IsZero() {
		t.Fatal("normalizeTime changed the zero value")
	}
	if cloneTime(nil) != nil {
		t.Fatal("cloneTime(nil) returned a value")
	}
	value := time.Date(2026, 7, 21, 12, 0, 0, 0, time.FixedZone("test", 3600))
	cloned := cloneTime(&value)
	if cloned == nil || cloned.Equal(value) == false || cloned == &value {
		t.Fatalf("cloneTime = %#v, want equal independent value", cloned)
	}

	if err := validateProjectAndLocator(nil, value.Format(time.RFC3339), domain.TranscriptLocator{}); domainCode(err) != domain.ErrExperienceLocatorOutsideRoot {
		t.Fatalf("nil project validation = %v, want outside-root", err)
	}
	projectRoot := t.TempDir()
	project := &domain.Project{CanonicalPath: projectRoot}
	locator := domain.TranscriptLocator{Kind: "file", Path: projectRoot + "/source.txt"}
	if err := validateProjectAndLocator(project, "relative-root", locator); domainCode(err) != domain.ErrExperienceLocatorInvalid {
		t.Fatalf("relative project root validation = %v, want locator-invalid", err)
	}
	if err := validateProjectAndLocator(project, projectRoot, domain.TranscriptLocator{Kind: "file", Path: "relative-source.txt"}); domainCode(err) != domain.ErrExperienceLocatorInvalid {
		t.Fatalf("relative locator validation = %v, want locator-invalid", err)
	}

	envelope := validEnvelope(projectRoot, "helper-times")
	started := time.Date(2026, 7, 21, 11, 0, 0, 0, time.UTC)
	closed := started.Add(time.Hour)
	stable := started.Add(30 * time.Minute)
	envelope.Session.StartedAt = &started
	envelope.Session.ClosedAt = &closed
	envelope.Turn.StableSince = &stable
	session, err := buildSession("project", envelope, closed)
	if err != nil || session.StartedAt == nil || session.ClosedAt == nil {
		t.Fatalf("buildSession with timestamps = %#v, %v", session, err)
	}
	turn, err := buildTurn(session.ID, envelope, false, closed)
	if err != nil || turn.StableAt == nil {
		t.Fatalf("buildTurn with stable timestamp = %#v, %v", turn, err)
	}
	zeroTimes := validEnvelope(projectRoot, "zero-times")
	zeroTimes.Session.UpdatedAt = time.Time{}
	zeroTimes.Turn.OccurredAt = time.Time{}
	zeroTimes.Turn.StableSince = nil
	if _, err := buildSession("project", zeroTimes, closed); err != nil {
		t.Fatalf("buildSession with zero updated_at: %v", err)
	}
	if _, err := buildTurn("session", zeroTimes, false, closed); err != nil {
		t.Fatalf("buildTurn with zero occurred_at: %v", err)
	}
}

func TestIngestRejectsMissingServiceInputs(t *testing.T) {
	var nilService *Service
	if _, err := nilService.Ingest(context.Background(), "project", nil); err == nil {
		t.Fatal("nil service returned nil error")
	}
	if _, err := NewService(nil).Ingest(context.Background(), "project", nil); err == nil {
		t.Fatal("nil database returned nil error")
	}
}

func TestIngestValidatesInputBeforeDataAccess(t *testing.T) {
	db, project, projectRoot := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})
	if _, err := svc.Ingest(context.Background(), project.ID, nil); domainCode(err) != domain.ErrInvalidArgument {
		t.Fatalf("nil input error = %v, want invalid_argument", err)
	}
	if _, err := svc.Ingest(context.Background(), "", &IngestInput{Envelope: validEnvelope(projectRoot, "empty-project")}); domainCode(err) != domain.ErrInvalidArgument {
		t.Fatalf("empty project error = %v, want invalid_argument", err)
	}
	assertExperienceCounts(t, db, 0, 0, 0, 0)
}

func domainCode(err error) domain.ErrorCode {
	if domainErr, ok := domain.AsDomainError(err); ok {
		return domainErr.Code
	}
	return ""
}
