package experience

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"agent-royo-learn/internal/domain"
)

// ExperienceEnvelopeSchemaVersion is the first neutral adapter-to-core schema.
const ExperienceEnvelopeSchemaVersion = 1

// SafeToolCall contains bounded metadata about a tool call. It is descriptive
// only: the ingestion service never executes its name or arguments.
type SafeToolCall struct {
	Name       string         `json:"name"`
	Arguments  map[string]any `json:"arguments,omitempty"`
	ExitCode   *int           `json:"exit_code,omitempty"`
	Outcome    string         `json:"outcome,omitempty"`
	OutputHash string         `json:"output_hash,omitempty"`
	OutputHint string         `json:"output_hint,omitempty"`
}

// UnmarshalJSON keeps arbitrary-precision JSON numbers as json.Number so a
// large source offset cannot silently change while the envelope is prepared.
func (c *SafeToolCall) UnmarshalJSON(data []byte) error {
	type safeToolCall SafeToolCall
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded safeToolCall
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	*c = SafeToolCall(decoded)
	return nil
}

// ExperienceEnvelope is the neutral input produced by a platform adapter.
// Full transcript content is accepted only in memory and is reduced to digests
// before persistence.
type ExperienceEnvelope struct {
	SchemaVersion int                     `json:"schema_version"`
	Source        domain.ExperienceSource `json:"source"`
	ProjectRoot   string                  `json:"project_root"`

	Session struct {
		ExternalID string                   `json:"external_id"`
		StartedAt  *time.Time               `json:"started_at,omitempty"`
		UpdatedAt  time.Time                `json:"updated_at"`
		ClosedAt   *time.Time               `json:"closed_at,omitempty"`
		Locator    domain.TranscriptLocator `json:"locator"`
	} `json:"session"`

	Turn struct {
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
	} `json:"turn"`

	Actor domain.Actor `json:"actor"`
}

// ValidateEnvelope validates structural and domain-level envelope invariants.
// Filesystem confinement is intentionally checked by the service after the
// project has been loaded from SQLite.
func ValidateEnvelope(envelope *ExperienceEnvelope) error {
	if envelope == nil {
		return domain.NewValidationError(domain.ErrInvalidArgument, "experience envelope is nil")
	}
	if envelope.SchemaVersion != ExperienceEnvelopeSchemaVersion {
		return domain.NewValidationError(domain.ErrExperienceSchemaUnsupported,
			fmt.Sprintf("unsupported experience envelope schema version: %d", envelope.SchemaVersion))
	}
	if !domain.IsValidExperienceSource(envelope.Source) {
		return domain.NewValidationError(domain.ErrInvalidArgument, "invalid experience source")
	}
	for _, bound := range []struct {
		field string
		value string
		limit int
	}{
		{field: "experience project root", value: envelope.ProjectRoot, limit: domain.MaxExperiencePathBytes},
		{field: "experience session external id", value: envelope.Session.ExternalID, limit: domain.MaxExperienceIDBytes},
		{field: "experience locator kind", value: envelope.Session.Locator.Kind, limit: domain.MaxExperienceMetadataBytes},
		{field: "experience locator path", value: envelope.Session.Locator.Path, limit: domain.MaxExperiencePathBytes},
		{field: "experience locator session id", value: envelope.Session.Locator.SessionID, limit: domain.MaxExperienceIDBytes},
		{field: "experience locator turn id", value: envelope.Session.Locator.TurnID, limit: domain.MaxExperienceIDBytes},
		{field: "experience locator source hash", value: envelope.Session.Locator.SourceHash, limit: domain.MaxExperienceDigestBytes},
		{field: "experience turn external id", value: envelope.Turn.ExternalID, limit: domain.MaxExperienceIDBytes},
		{field: "experience turn finish reason", value: envelope.Turn.FinishReason, limit: domain.MaxExperienceMetadataBytes},
		{field: "experience turn source revision", value: envelope.Turn.SourceRevision, limit: domain.MaxExperienceIDBytes},
	} {
		if len(bound.value) > bound.limit {
			return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge, bound.field+" exceeds the permitted byte limit")
		}
	}
	if err := domain.ValidateExperienceActor(envelope.Actor); err != nil {
		if domainErr, ok := domain.AsDomainError(err); ok && domainErr.Code == domain.ErrExperiencePayloadTooLarge {
			return err
		}
		return err
	}
	if envelope.ProjectRoot == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "experience project root is required")
	}
	if envelope.Session.ExternalID == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "experience session external id is required")
	}
	if err := domain.ValidateTranscriptLocator(envelope.Session.Locator); err != nil {
		return err
	}
	if envelope.Turn.ExternalID == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "experience turn external id is required")
	}
	if envelope.Turn.Sequence < 0 {
		return domain.NewValidationError(domain.ErrInvalidArgument, "experience turn sequence must be non-negative")
	}
	if !envelope.Turn.Complete {
		return domain.NewValidationError(domain.ErrExperienceTurnIncomplete,
			"experience turn is incomplete")
	}
	for i, call := range envelope.Turn.ToolCalls {
		for _, bound := range []struct {
			field string
			value string
			limit int
		}{
			{field: "tool call name", value: call.Name, limit: domain.MaxExperienceMetadataBytes},
			{field: "tool call outcome", value: call.Outcome, limit: domain.MaxExperienceMetadataBytes},
			{field: "tool call output hash", value: call.OutputHash, limit: domain.MaxExperienceDigestBytes},
			{field: "tool call output hint", value: call.OutputHint, limit: domain.MaxExperienceSummaryBytes},
		} {
			if len(bound.value) > bound.limit {
				return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge, bound.field+" exceeds the permitted byte limit")
			}
		}
		if call.Name == "" {
			return domain.NewValidationError(domain.ErrInvalidArgument,
				fmt.Sprintf("experience tool call %d name is required", i))
		}
		if _, err := json.Marshal(call.Arguments); err != nil {
			return domain.NewValidationError(domain.ErrExperienceSchemaUnsupported,
				fmt.Sprintf("experience tool call %d arguments are not JSON serializable", i))
		}
	}
	return nil
}
