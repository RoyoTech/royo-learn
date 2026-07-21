package domain

import "testing"

func validSession() *ExperienceSession {
	return &ExperienceSession{
		ID:                "sess-1",
		ProjectID:         "proj-1",
		Source:            SourceOpenCode,
		ExternalSessionID: "ext-sess-1",
		Locator:           TranscriptLocator{Kind: "sqlite", Path: "/tmp/opencode.db"},
	}
}

func validTurn() *ExperienceTurn {
	return &ExperienceTurn{
		ID:             "turn-1",
		SessionID:      "sess-1",
		ExternalTurnID: "ext-turn-1",
		Sequence:       0,
		Status:         TurnStable,
	}
}

func validEvent() *ExperienceEvent {
	return &ExperienceEvent{
		ID:         "evt-1",
		ProjectID:  "proj-1",
		TurnID:     "turn-1",
		Kind:       EventUserCorrection,
		Summary:    "user corrected the migration order",
		Detector:   DetectorIdentity{Kind: "deterministic", Name: "test-outcome", Version: "1.0.0"},
		Confidence: ConfidenceMedium,
	}
}

// wantCode asserts err is a DomainError carrying the expected code.
func wantCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %q, got nil", code)
	}
	de, ok := AsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if de.Code != code {
		t.Fatalf("expected code %q, got %q", code, de.Code)
	}
}

func TestValidateExperienceSession(t *testing.T) {
	if err := ValidateExperienceSession(validSession()); err != nil {
		t.Fatalf("valid session rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ExperienceSession)
		code   ErrorCode
	}{
		{"missing id", func(s *ExperienceSession) { s.ID = "" }, ErrInvalidArgument},
		{"missing project", func(s *ExperienceSession) { s.ProjectID = "" }, ErrInvalidArgument},
		{"missing external", func(s *ExperienceSession) { s.ExternalSessionID = "" }, ErrInvalidArgument},
		{"invalid source", func(s *ExperienceSession) { s.Source = "nope" }, ErrInvalidArgument},
		{"locator invalid kind", func(s *ExperienceSession) { s.Locator.Kind = "ftp" }, ErrExperienceLocatorInvalid},
		{"locator empty path", func(s *ExperienceSession) { s.Locator.Path = "" }, ErrExperienceLocatorInvalid},
		{"locator remote api", func(s *ExperienceSession) { s.Locator.Kind = "api" }, ErrExperienceSchemaUnsupported},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := validSession()
			tc.mutate(s)
			wantCode(t, ValidateExperienceSession(s), tc.code)
		})
	}

	if err := ValidateExperienceSession(nil); err == nil {
		t.Fatal("nil session accepted")
	}
}

func TestValidateTranscriptLocator(t *testing.T) {
	for _, kind := range []string{"sqlite", "jsonl", "rollout", "file"} {
		if err := ValidateTranscriptLocator(TranscriptLocator{Kind: kind, Path: "/x"}); err != nil {
			t.Errorf("local kind %q rejected: %v", kind, err)
		}
	}
}

func TestValidateExperienceTurn(t *testing.T) {
	if err := ValidateExperienceTurn(validTurn()); err != nil {
		t.Fatalf("valid turn rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ExperienceTurn)
		code   ErrorCode
	}{
		{"missing id", func(x *ExperienceTurn) { x.ID = "" }, ErrInvalidArgument},
		{"missing session", func(x *ExperienceTurn) { x.SessionID = "" }, ErrInvalidArgument},
		{"missing external", func(x *ExperienceTurn) { x.ExternalTurnID = "" }, ErrInvalidArgument},
		{"negative sequence", func(x *ExperienceTurn) { x.Sequence = -1 }, ErrInvalidArgument},
		{"invalid status", func(x *ExperienceTurn) { x.Status = "weird" }, ErrInvalidArgument},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x := validTurn()
			tc.mutate(x)
			wantCode(t, ValidateExperienceTurn(x), tc.code)
		})
	}

	// Every declared status must validate.
	for _, s := range []TurnStatus{TurnObserved, TurnIncomplete, TurnStable, TurnIngested, TurnSuperseded, TurnFailed} {
		x := validTurn()
		x.Status = s
		if err := ValidateExperienceTurn(x); err != nil {
			t.Errorf("status %q rejected: %v", s, err)
		}
	}
}

func TestValidateExperienceEvent(t *testing.T) {
	if err := ValidateExperienceEvent(validEvent()); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ExperienceEvent)
		code   ErrorCode
	}{
		{"missing id", func(e *ExperienceEvent) { e.ID = "" }, ErrInvalidArgument},
		{"missing project", func(e *ExperienceEvent) { e.ProjectID = "" }, ErrInvalidArgument},
		{"missing turn provenance", func(e *ExperienceEvent) { e.TurnID = "" }, ErrInvalidArgument},
		{"invalid kind", func(e *ExperienceEvent) { e.Kind = "chatter" }, ErrInvalidArgument},
		{"missing summary", func(e *ExperienceEvent) { e.Summary = "" }, ErrInvalidArgument},
		{"missing detector kind", func(e *ExperienceEvent) { e.Detector = DetectorIdentity{} }, ErrInvalidArgument},
		{"unknown detector kind", func(e *ExperienceEvent) { e.Detector.Kind = "heuristic" }, ErrInvalidArgument},
		{"invalid confidence", func(e *ExperienceEvent) { e.Confidence = "certainish" }, ErrInvalidArgument},
		{"host llm high confidence", func(e *ExperienceEvent) {
			e.Detector.Kind = "host_llm"
			e.Confidence = ConfidenceHigh
		}, ErrInvalidArgument},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := validEvent()
			tc.mutate(e)
			wantCode(t, ValidateExperienceEvent(e), tc.code)
		})
	}

	t.Run("deterministic detector may use high confidence", func(t *testing.T) {
		e := validEvent()
		e.Confidence = ConfidenceHigh
		if err := ValidateExperienceEvent(e); err != nil {
			t.Fatalf("deterministic high-confidence event rejected: %v", err)
		}
	})
}

func TestExperienceErrorCodesRegistered(t *testing.T) {
	want := []ErrorCode{
		ErrExperienceSourceNotFound, ErrExperienceSchemaUnsupported,
		ErrExperienceTurnIncomplete, ErrExperienceLocatorInvalid,
		ErrExperienceLocatorOutsideRoot, ErrExperiencePayloadTooLarge,
		ErrExperienceRevisionConflict, ErrExperienceCursorConflict,
	}
	all := make(map[ErrorCode]bool)
	for _, c := range AllErrorCodes() {
		all[c] = true
	}
	for _, c := range want {
		if !all[c] {
			t.Errorf("error code %q missing from AllErrorCodes()", c)
		}
		if ExitCode := c.ExitCode(); ExitCode == 1 {
			t.Errorf("error code %q has no explicit exit code mapping", c)
		}
	}
}
