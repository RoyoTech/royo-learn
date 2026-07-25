// Persistence layer for Hito 5 slice 5.4.
//
// Persist ingests a CandidateEvent into the canonical experience
// store by mapping it to an ExperienceEnvelope and delegating to the
// existing ingestion pipeline. The pipeline already handles
// redaction, fingerprint computation, idempotency, audit and
// persistence; the detector must not duplicate any of that.
//
// The envelope uses deterministic identifiers derived from the event
// content so re-runs with the same event are idempotent -
// same CandidateEvent twice yields the same Turn.ExternalID, the
// same fingerprint, and the same persisted row.
//
// Mapping:
//
//   Source               = domain.SourceDetector  ("detector")
//   Session.ExternalID   = "detector:" + event.Kind
//   Session.Locator.Kind = "detector"
//   Turn.ExternalID      = EventFingerprint(event)   (sha256 hex)
//   Turn.UserText        = event.Problem
//   Turn.ToolCalls[0]    = { Name=event.Tool, Outcome=event.Result,
//                             Arguments={kind, result, paths,
//                             retrieval_terms, extra} }
//
// The detector's structured fields land in a single SafeToolCall so
// downstream pattern miners can read them without the envelope
// shape changing. The event's own fingerprint is the turn id, which
// gives the ingestion service a deterministic key on (session_id,
// external_turn_id).

package detectors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
)

// Persist ingests a CandidateEvent via the canonical experience
// pipeline. The event is converted into an ExperienceEnvelope with
// deterministic IDs and forwarded to Service.IngestEnvelope, which
// runs the standard redaction + fingerprint + idempotency + audit +
// persistence chain.
func Persist(
	ctx context.Context,
	svc *experience.Service,
	projectID domain.ProjectID,
	projectRoot string,
	ev CandidateEvent,
	now time.Time,
) (*experience.IngestResult, error) {
	if svc == nil {
		return nil, fmt.Errorf("detectors: persist: service is nil")
	}
	envelope, err := BuildDetectorEnvelope(projectID, projectRoot, ev, now)
	if err != nil {
		return nil, err
	}
	return svc.IngestEnvelope(ctx, projectID, envelope)
}

// BuildDetectorEnvelope constructs the synthetic ExperienceEnvelope
// that Persist hands to the ingestion pipeline. The function is
// pure (no DB, no IO) so it can be unit-tested without the
// experience.Service machinery; the CLI acceptance test covers
// the ingest path end-to-end.
func BuildDetectorEnvelope(
	projectID domain.ProjectID,
	projectRoot string,
	ev CandidateEvent,
	now time.Time,
) (experience.ExperienceEnvelope, error) {
	if projectID == "" {
		return experience.ExperienceEnvelope{}, fmt.Errorf("detectors: persist: project id is required")
	}
	if projectRoot == "" {
		return experience.ExperienceEnvelope{}, fmt.Errorf("detectors: persist: project root is required")
	}
	if ev.Kind == "" {
		return experience.ExperienceEnvelope{}, fmt.Errorf("detectors: persist: event has empty Kind")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	turnExternalID := EventFingerprint(ev)

	var envelope experience.ExperienceEnvelope
	envelope.SchemaVersion = experience.ExperienceEnvelopeSchemaVersion
	envelope.Source = domain.SourceDetector
	envelope.ProjectRoot = projectRoot
	envelope.Session.ExternalID = "detector:" + ev.Kind
	envelope.Session.UpdatedAt = now
	envelope.Session.Locator = domain.TranscriptLocator{
		Kind:       "detector",
		Path:       projectRoot,
		SessionID:  "detector:" + ev.Kind,
		TurnID:     turnExternalID,
		SourceHash: turnExternalID[:32],
	}
	envelope.Turn.ExternalID = turnExternalID
	envelope.Turn.Sequence = 0
	envelope.Turn.Complete = true
	envelope.Turn.FinishReason = "detector:" + ev.Kind
	envelope.Turn.OccurredAt = now
	envelope.Turn.UserText = ev.Problem
	envelope.Turn.ToolCalls = []experience.SafeToolCall{{
		Name:    ev.Tool,
		Outcome: ev.Result,
		Arguments: map[string]any{
			"kind":            ev.Kind,
			"result":          ev.Result,
			"paths":           ev.Paths,
			"retrieval_terms": ev.RetrievalTerms,
			"extra":           ev.Extra,
		},
	}}
	envelope.Turn.SourceRevision = turnExternalID[:16]
	envelope.Actor = domain.Actor{
		Kind: "system",
		Name: "detector:" + ev.Kind,
	}

	return envelope, nil
}

// EventFingerprint computes a stable sha256 fingerprint for a
// CandidateEvent. Two events with identical canonical content
// produce the same fingerprint; identical Kind + Problem + Tool +
// Result + sorted Paths + sorted RetrievalTerms + sorted Extra
// produce the same fingerprint regardless of map iteration order or
// slice ordering.
//
// The fingerprint is the Turn.ExternalID used by Persist, so the
// idempotency contract of the ingestion pipeline applies directly:
// running the same event twice produces one persisted row.
//
// The output is 64 lowercase hex characters. The first 16 and 32
// characters are reused by Persist for SourceRevision and
// SourceHash respectively; both must remain stable across runs.
func EventFingerprint(ev CandidateEvent) string {
	h := sha256.New()
	write := func(s string) {
		_, _ = h.Write([]byte(s))
		_, _ = h.Write([]byte{0})
	}
	write(ev.Kind)
	write(ev.Problem)
	write(ev.Tool)
	write(ev.Result)

	sortedPaths := append([]string(nil), ev.Paths...)
	sort.Strings(sortedPaths)
	for _, p := range sortedPaths {
		write(p)
	}

	sortedTerms := append([]string(nil), ev.RetrievalTerms...)
	sort.Strings(sortedTerms)
	for _, t := range sortedTerms {
		write(t)
	}

	if len(ev.Extra) > 0 {
		keys := make([]string, 0, len(ev.Extra))
		for k := range ev.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			write(k)
			b, _ := json.Marshal(ev.Extra[k])
			_, _ = h.Write(b)
			_, _ = h.Write([]byte{0})
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}
