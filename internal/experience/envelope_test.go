package experience

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

func TestValidateEnvelopeRejectsUnsupportedSchema(t *testing.T) {
	envelope := validEnvelope(t.TempDir(), "schema")
	envelope.SchemaVersion = ExperienceEnvelopeSchemaVersion + 1

	err := ValidateEnvelope(&envelope)
	assertExperienceCode(t, err, domain.ErrExperienceSchemaUnsupported)
}

func TestValidateEnvelopeRejectsIncompleteTurn(t *testing.T) {
	envelope := validEnvelope(t.TempDir(), "incomplete")
	envelope.Turn.Complete = false

	err := ValidateEnvelope(&envelope)
	assertExperienceCode(t, err, domain.ErrExperienceTurnIncomplete)
}

func TestValidateEnvelopeRejectsInvalidShape(t *testing.T) {
	tests := []struct {
		name string
		make func() *ExperienceEnvelope
		code domain.ErrorCode
	}{
		{name: "nil", make: func() *ExperienceEnvelope { return nil }, code: domain.ErrInvalidArgument},
		{name: "source", make: func() *ExperienceEnvelope {
			envelope := validEnvelope(t.TempDir(), "invalid-source")
			envelope.Source = "unknown"
			return &envelope
		}, code: domain.ErrInvalidArgument},
		{name: "project root", make: func() *ExperienceEnvelope {
			envelope := validEnvelope(t.TempDir(), "missing-root")
			envelope.ProjectRoot = ""
			return &envelope
		}, code: domain.ErrInvalidArgument},
		{name: "session id", make: func() *ExperienceEnvelope {
			envelope := validEnvelope(t.TempDir(), "missing-session")
			envelope.Session.ExternalID = ""
			return &envelope
		}, code: domain.ErrInvalidArgument},
		{name: "locator kind", make: func() *ExperienceEnvelope {
			envelope := validEnvelope(t.TempDir(), "invalid-locator")
			envelope.Session.Locator.Kind = "http"
			return &envelope
		}, code: domain.ErrExperienceLocatorInvalid},
		{name: "remote locator", make: func() *ExperienceEnvelope {
			envelope := validEnvelope(t.TempDir(), "remote-locator")
			envelope.Session.Locator.Kind = "api"
			return &envelope
		}, code: domain.ErrExperienceSchemaUnsupported},
		{name: "turn id", make: func() *ExperienceEnvelope {
			envelope := validEnvelope(t.TempDir(), "missing-turn")
			envelope.Turn.ExternalID = ""
			return &envelope
		}, code: domain.ErrInvalidArgument},
		{name: "negative sequence", make: func() *ExperienceEnvelope {
			envelope := validEnvelope(t.TempDir(), "negative-sequence")
			envelope.Turn.Sequence = -1
			return &envelope
		}, code: domain.ErrInvalidArgument},
		{name: "tool name", make: func() *ExperienceEnvelope {
			envelope := validEnvelope(t.TempDir(), "missing-tool-name")
			envelope.Turn.ToolCalls = []SafeToolCall{{}}
			return &envelope
		}, code: domain.ErrInvalidArgument},
		{name: "tool arguments", make: func() *ExperienceEnvelope {
			envelope := validEnvelope(t.TempDir(), "invalid-tool-arguments")
			envelope.Turn.ToolCalls = []SafeToolCall{{Name: "tool", Arguments: map[string]any{"bad": func() {}}}}
			return &envelope
		}, code: domain.ErrExperienceSchemaUnsupported},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertExperienceCode(t, ValidateEnvelope(tt.make()), tt.code)
		})
	}
}

func TestValidateEnvelopeDoesNotEchoUntrustedSourceOrLocatorKind(t *testing.T) {
	secret := "sk-proj-error-leak-1234567890"

	source := validEnvelope(t.TempDir(), "unsafe-source")
	source.Source = domain.ExperienceSource("invalid-" + secret)
	err := ValidateEnvelope(&source)
	assertExperienceCode(t, err, domain.ErrInvalidArgument)
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("source validation leaked raw input: %v", err)
	}

	locator := validEnvelope(t.TempDir(), "unsafe-locator")
	locator.Session.Locator.Kind = "invalid-" + secret
	err = ValidateEnvelope(&locator)
	assertExperienceCode(t, err, domain.ErrExperienceLocatorInvalid)
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("locator validation leaked raw input: %v", err)
	}
}

func TestExperienceEnvelopeUsesStableJSONKeys(t *testing.T) {
	envelope := validEnvelope(t.TempDir(), "json")
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	for _, key := range []string{`"schema_version"`, `"project_root"`, `"session"`, `"turn"`} {
		if !strings.Contains(string(encoded), key) {
			t.Fatalf("encoded envelope = %s, missing key %s", encoded, key)
		}
	}
}

func TestExperienceEnvelopeNestedFieldsRoundTripSnakeCase(t *testing.T) {
	projectRoot := filepath.ToSlash(t.TempDir())
	sourcePath := filepath.ToSlash(filepath.Join(projectRoot, "source.db"))
	raw := `{"schema_version":1,"source":"opencode","project_root":"` + projectRoot + `","session":{"external_id":"session-json","started_at":"2026-07-21T10:00:00Z","updated_at":"2026-07-21T10:01:00Z","closed_at":"2026-07-21T10:02:00Z","locator":{"kind":"sqlite","path":"` + sourcePath + `","session_id":"native-session","turn_id":"native-turn","offset":9007199254740993,"source_hash":"source-hash"}},"turn":{"external_id":"turn-json","sequence":7,"complete":true,"finish_reason":"stop","occurred_at":"2026-07-21T10:03:00Z","stable_since":"2026-07-21T10:04:00Z","user_text":"request","assistant_text":"response","tool_calls":[{"name":"read","arguments":{"offset":9007199254740993},"exit_code":0,"outcome":"ok","output_hash":"output-hash","output_hint":"safe hint"}],"source_revision":"revision-1"},"actor":{"kind":"agent","name":"test-agent","model":"test-model","session_id":"actor-session"}}`

	var envelope ExperienceEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.Session.ExternalID != "session-json" || envelope.Session.Locator.SessionID != "native-session" {
		t.Fatalf("session fields = %#v, want snake_case values", envelope.Session)
	}
	if envelope.Turn.ExternalID != "turn-json" || envelope.Turn.FinishReason != "stop" || envelope.Turn.SourceRevision != "revision-1" {
		t.Fatalf("turn fields = %#v, want snake_case values", envelope.Turn)
	}
	if len(envelope.Turn.ToolCalls) != 1 || envelope.Turn.ToolCalls[0].OutputHint != "safe hint" {
		t.Fatalf("tool calls = %#v, want nested fields populated", envelope.Turn.ToolCalls)
	}
	if number, ok := envelope.Turn.ToolCalls[0].Arguments["offset"].(json.Number); !ok || number.String() != "9007199254740993" {
		t.Fatalf("tool offset = %#v, want exact JSON number", envelope.Turn.ToolCalls[0].Arguments["offset"])
	}

	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	for _, key := range []string{`"external_id"`, `"finish_reason"`, `"source_revision"`, `"session_id"`, `"source_hash"`} {
		if !strings.Contains(string(encoded), key) {
			t.Fatalf("encoded envelope = %s, missing key %s", encoded, key)
		}
	}
	for _, key := range []string{`"ExternalID"`, `"FinishReason"`, `"SourceRevision"`} {
		if strings.Contains(string(encoded), key) {
			t.Fatalf("encoded envelope = %s, contains unstable key %s", encoded, key)
		}
	}
}

func TestValidateEnvelopeBoundsMetadataBeforePersistence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ExperienceEnvelope)
	}{
		{name: "project root", mutate: func(e *ExperienceEnvelope) { e.ProjectRoot = strings.Repeat("x", 5000) }},
		{name: "session id", mutate: func(e *ExperienceEnvelope) { e.Session.ExternalID = strings.Repeat("x", 1024) }},
		{name: "locator path", mutate: func(e *ExperienceEnvelope) { e.Session.Locator.Path = strings.Repeat("x", 5000) }},
		{name: "actor", mutate: func(e *ExperienceEnvelope) { e.Actor.Name = strings.Repeat("x", 1024) }},
		{name: "source revision", mutate: func(e *ExperienceEnvelope) { e.Turn.SourceRevision = strings.Repeat("x", 1024) }},
		{name: "tool name", mutate: func(e *ExperienceEnvelope) { e.Turn.ToolCalls = []SafeToolCall{{Name: strings.Repeat("x", 1024)}} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := validEnvelope(t.TempDir(), tt.name)
			tt.mutate(&envelope)
			assertExperienceCode(t, ValidateEnvelope(&envelope), domain.ErrExperiencePayloadTooLarge)
		})
	}
}

func TestSafeToolCallJSONPreservesLargeNumbers(t *testing.T) {
	var call SafeToolCall
	if err := json.Unmarshal([]byte(`{"name":"read","arguments":{"offset":9007199254740993}}`), &call); err != nil {
		t.Fatalf("unmarshal tool call: %v", err)
	}
	number, ok := call.Arguments["offset"].(json.Number)
	if !ok || number.String() != "9007199254740993" {
		t.Fatalf("offset = %#v, want exact json.Number", call.Arguments["offset"])
	}
}

func validEnvelope(projectRoot, suffix string) ExperienceEnvelope {
	return ExperienceEnvelope{
		SchemaVersion: ExperienceEnvelopeSchemaVersion,
		Source:        domain.SourceOpenCode,
		ProjectRoot:   projectRoot,
		Session: struct {
			ExternalID string                   `json:"external_id"`
			StartedAt  *time.Time               `json:"started_at,omitempty"`
			UpdatedAt  time.Time                `json:"updated_at"`
			ClosedAt   *time.Time               `json:"closed_at,omitempty"`
			Locator    domain.TranscriptLocator `json:"locator"`
		}{
			ExternalID: "session-" + suffix,
			UpdatedAt:  time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC),
			Locator: domain.TranscriptLocator{
				Kind:      "sqlite",
				Path:      filepath.Join(projectRoot, "source.db"),
				SessionID: "native-session-" + suffix,
				TurnID:    "native-turn-" + suffix,
			},
		},
		Turn: struct {
			ExternalID     string         `json:"external_id"`
			Sequence       int64          `json:"sequence"`
			Complete       bool           `json:"complete"`
			FinishReason   string         `json:"finish_reason"`
			OccurredAt     time.Time      `json:"occurred_at"`
			StableSince    *time.Time     `json:"stable_since,omitempty"`
			UserText       string         `json:"user_text"`
			AssistantText  string         `json:"assistant_text"`
			ToolCalls      []SafeToolCall `json:"tool_calls,omitempty"`
			SourceRevision string         `json:"source_revision"`
		}{
			ExternalID:     "turn-" + suffix,
			Sequence:       1,
			Complete:       true,
			FinishReason:   "stop",
			OccurredAt:     time.Date(2026, 7, 21, 10, 1, 0, 0, time.UTC),
			SourceRevision: "revision-1",
			UserText:       "Please fix the failing test.",
			AssistantText:  "The test now passes.",
		},
		Actor: domain.Actor{
			Kind:      "agent",
			Name:      "test-agent",
			Model:     "test-model",
			SessionID: "actor-session-" + suffix,
		},
	}
}
