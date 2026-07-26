// MCP tool tests for Hito 6 slice 6.4 (pattern-mining tools).
//
// The three new tools (learning_list_patterns, learning_get_pattern,
// learning_dismiss_pattern) follow the same pattern as the existing
// experience_detect_events tool. The tests verify:
//
//   - list returns the typed envelope with the seeded patterns.
//   - get returns the pattern + membership rows.
//   - dismiss is idempotent on the same reason.
//   - dismiss rejects a different reason on an already-dismissed
//     pattern.

package mcpserver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/patterns"
	"agent-royo-learn/internal/storage"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// seedPattern seeds a project + a synthetic qualified pattern the
// MCP pattern tests operate on. The MCP testServer already creates a
// project + DB; this helper only needs to persist the pattern.
func seedPattern(t *testing.T, ts *testServer) domain.ExperiencePatternID {
	t.Helper()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	memberIDs := []domain.ExperienceEventID{"evt-1", "evt-2", "evt-3", "evt-4", "evt-5"}
	if err := storage.WithTx(context.Background(), ts.db, func(tx *sql.Tx) error {
		turnIDs := make([]domain.ExperienceTurnID, 3)
		for i := range turnIDs {
			sessionID := domain.ExperienceSessionID(fmt.Sprintf("mcp-pattern-session-%d", i+1))
			turnID := domain.ExperienceTurnID(fmt.Sprintf("mcp-pattern-turn-%d", i+1))
			turnIDs[i] = turnID
			occurredAt := now.AddDate(0, 0, i)
			session := &domain.ExperienceSession{
				ID: sessionID, ProjectID: ts.projectID, Source: domain.SourceOpenCode,
				ExternalSessionID: fmt.Sprintf("mcp-pattern-external-session-%d", i+1),
				Locator:           domain.TranscriptLocator{Kind: "sqlite", Path: "C:/safe/mcp-pattern-sessions.db", SessionID: string(sessionID)},
				StartedAt:         &occurredAt, UpdatedAt: occurredAt, ClosedAt: &occurredAt,
				MetadataSHA256: fmt.Sprintf("mcp-pattern-session-digest-%d", i+1), CreatedAt: occurredAt,
			}
			if err := storage.SaveExperienceSession(context.Background(), tx, session); err != nil {
				return err
			}
			turn := &domain.ExperienceTurn{
				ID: turnID, SessionID: sessionID, ExternalTurnID: fmt.Sprintf("mcp-pattern-external-turn-%d", i+1),
				Sequence: int64(i + 1), Status: domain.TurnIngested, Fingerprint: fmt.Sprintf("mcp-pattern-turn-fingerprint-%d", i+1),
				UserDigest: "user-digest", AssistantDigest: "assistant-digest", ToolCallsDigest: "tool-digest",
				SafeSummary: "Synthetic pattern fixture.", OccurredAt: occurredAt, StableAt: &occurredAt,
				IngestedAt: occurredAt, SourceRevision: "revision-1", Redacted: true,
			}
			if err := storage.SaveExperienceTurn(context.Background(), tx, turn); err != nil {
				return err
			}
		}
		for i, eventID := range memberIDs {
			event := &domain.ExperienceEvent{
				ID: eventID, ProjectID: ts.projectID, TurnID: turnIDs[i%len(turnIDs)], Kind: domain.EventTestFailure,
				Summary: "Synthetic test failure.", Observation: "A deterministic test failed.", Outcome: "success",
				Fingerprint: fmt.Sprintf("mcp-pattern-event-fingerprint-%d", i+1), EvidenceJSON: `[{"kind":"test"}]`,
				Detector:   domain.DetectorIdentity{Kind: "deterministic", Name: "mcp-pattern-fixture", Version: "1.0.0"},
				Confidence: domain.ConfidenceHigh, CreatedAt: now.Add(time.Duration(i) * time.Minute),
			}
			if err := storage.SaveExperienceEvent(context.Background(), tx, event); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed MCP pattern events: %v", err)
	}
	svc := patterns.NewService(ts.db)
	cluster := patterns.ClusterRecord{
		Fingerprint:        "fp-mcp-1",
		Kind:               domain.EventTestFailure,
		Members:            memberIDs,
		Sessions:           map[string]struct{}{"sess-1": {}, "sess-2": {}, "sess-3": {}},
		Days:               map[string]struct{}{"2026-07-25": {}, "2026-07-26": {}, "2026-07-27": {}},
		DistinctSessions:   3,
		DistinctDays:       3,
		OccurrenceCount:    5,
		SuccessfulOutcomes: 3,
		FirstSeenAt:        now,
		LastSeenAt:         now,
		RetrievalTerms:     []string{"compile", "missing", "header"},
	}
	saved, err := svc.IngestCluster(context.Background(), ts.projectID, cluster, patterns.QualificationDecision{Status: patterns.PatternQualified})
	if err != nil {
		t.Fatalf("IngestCluster: %v", err)
	}
	return saved.ID
}

// TestCallTool_LearningListPatterns verifies the list tool returns
// the seeded pattern with the documented status filter.
func TestCallTool_LearningListPatterns(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "agent")
	patternID := seedPattern(t, ts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.callTool(ctx, "learning_list_patterns", map[string]any{
		"status": "qualified",
	})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false, got true; result=%+v", result)
	}

	data := mustDecodeMap(t, result)
	if got := data["status"]; got != "ok" {
		t.Fatalf("status = %v, want ok", got)
	}
	if got, _ := data["total"].(float64); got != 1 {
		t.Fatalf("total = %v, want 1", data["total"])
	}
	patternsList, _ := data["patterns"].([]any)
	if len(patternsList) != 1 {
		t.Fatalf("patterns len = %d, want 1", len(patternsList))
	}
	first, _ := patternsList[0].(map[string]any)
	if first["id"] != string(patternID) {
		t.Fatalf("patterns[0].id = %v, want %s", first["id"], patternID)
	}
}

// TestCallTool_LearningGetPattern verifies the get tool returns the
// pattern + its membership rows.
func TestCallTool_LearningGetPattern(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "agent")
	patternID := seedPattern(t, ts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.callTool(ctx, "learning_get_pattern", map[string]any{
		"pattern_id":   string(patternID),
		"with_members": true,
	})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false, got true; result=%+v", result)
	}

	data := mustDecodeMap(t, result)
	pattern, _ := data["pattern"].(map[string]any)
	if pattern == nil {
		t.Fatal("pattern is nil")
	}
	if pattern["fingerprint"] != "fp-mcp-1" {
		t.Fatalf("fingerprint = %v, want fp-mcp-1", pattern["fingerprint"])
	}
	members, _ := data["members"].([]any)
	if len(members) == 0 {
		t.Fatal("members is empty, want ≥ 1")
	}
}

// TestCallTool_LearningDismissPattern_Idempotent verifies the
// dismiss tool is idempotent on the same reason.
func TestCallTool_LearningDismissPattern_Idempotent(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "admin")
	patternID := seedPattern(t, ts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < 2; i++ {
		result, err := ts.callTool(ctx, "learning_dismiss_pattern", map[string]any{
			"pattern_id": string(patternID),
			"reason":     "not_reusable",
			"note":       "first note",
		})
		if err != nil {
			t.Fatalf("transport error (call %d): %v", i, err)
		}
		if result.IsError {
			t.Fatalf("call %d: expected IsError=false, got true; result=%+v", i, result)
		}
	}

	// Verify the stored reason matches by re-fetching the pattern.
	result, err := ts.callTool(ctx, "learning_get_pattern", map[string]any{
		"pattern_id": string(patternID),
	})
	if err != nil {
		t.Fatalf("transport error (get): %v", err)
	}
	data := mustDecodeMap(t, result)
	pattern, _ := data["pattern"].(map[string]any)
	if pattern["status"] != "dismissed" {
		t.Fatalf("status = %v, want dismissed", pattern["status"])
	}
	if pattern["dismissal_reason"] != "not_reusable" {
		t.Fatalf("dismissal_reason = %v, want not_reusable", pattern["dismissal_reason"])
	}
}

// TestCallTool_LearningDismissPattern_RejectsDifferentReason
// verifies the (pattern_id, reason) idempotence rule rejects a
// different reason.
func TestCallTool_LearningDismissPattern_RejectsDifferentReason(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "admin")
	patternID := seedPattern(t, ts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.callTool(ctx, "learning_dismiss_pattern", map[string]any{
		"pattern_id": string(patternID),
		"reason":     "one_off",
	})
	if err != nil {
		t.Fatalf("first dismiss transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("first dismiss: IsError=true, want false; result=%+v", result)
	}

	result, err = ts.callTool(ctx, "learning_dismiss_pattern", map[string]any{
		"pattern_id": string(patternID),
		"reason":     "false_cluster",
	})
	if err != nil {
		t.Fatalf("second dismiss transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("dismiss with different reason: IsError=false, want true; result=%+v", result)
	}
	if !strings.Contains(errorText(t, result), "pattern_insufficient_sources") {
		t.Fatalf("error text = %v, want pattern_insufficient_sources", result.Content)
	}
}

// errorText extracts the text content from an MCP tool result so the
// tests can assert on the surfaced error code.
func errorText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		return ""
	}
	for _, c := range result.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			return text.Text
		}
	}
	return ""
}

// =========================================================================
// Slice 7.4 — learning_promote_pattern (Hito 7).
//
// The promote tool is the MCP surface over promotion.Service.Promote.
// It must:
//
//   - be available on the admin profile only (D2: nothing destructive
//     in read or agent);
//   - reject promoted patterns with pattern_not_qualified only when
//     the pattern is in the observed state;
//   - return the promote result with status=ok + pattern (status=promoted
//     + proposed_learning_id) + result envelope;
//   - be idempotent on a second call: WasNew=false, same LearningID,
//     same AuditID;
//   - reject missing pattern_id with invalid_argument.
// =========================================================================

// TestCallTool_LearningPromotePattern_Success verifies the happy path:
// an admin call on a qualified pattern returns the promotion result
// with the pattern re-fetched in the promoted state.
func TestCallTool_LearningPromotePattern_Success(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "admin")
	patternID := seedPattern(t, ts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.callTool(ctx, "learning_promote_pattern", map[string]any{
		"pattern_id": string(patternID),
		"note":       "ready for production",
	})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false, got true; result=%+v", result)
	}
	data := mustDecodeMap(t, result)
	if got := data["status"]; got != "ok" {
		t.Fatalf("status = %v, want ok", got)
	}
	pattern, _ := data["pattern"].(map[string]any)
	if pattern == nil {
		t.Fatalf("pattern is nil; data=%+v", data)
	}
	if pattern["status"] != "promoted" {
		t.Fatalf("pattern.status = %v, want promoted", pattern["status"])
	}
	if pattern["proposed_learning_id"] == "" || pattern["proposed_learning_id"] == nil {
		t.Fatalf("pattern.proposed_learning_id = %v, want populated", pattern["proposed_learning_id"])
	}
	res, ok := data["result"].(map[string]any)
	if !ok {
		t.Fatalf("result is not a map: %T", data["result"])
	}
	if got, _ := res["was_new"].(bool); !got {
		t.Fatalf("result.was_new = %v, want true", res["was_new"])
	}
	if res["learning_id"] == "" || res["learning_id"] == nil {
		t.Fatalf("result.learning_id is empty")
	}
	if res["audit_id"] == "" || res["audit_id"] == nil {
		t.Fatalf("result.audit_id is empty")
	}
	if res["learning_id"] != pattern["proposed_learning_id"] {
		t.Fatalf("result.learning_id = %v, pattern.proposed_learning_id = %v (must match)",
			res["learning_id"], pattern["proposed_learning_id"])
	}
}

// TestCallTool_LearningPromotePattern_NotAdmin verifies the tool is
// guarded by the admin profile. Calling it from the read profile must
// surface an unknown-tool or access-denied error envelope.
func TestCallTool_LearningPromotePattern_NotAdmin(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "read")
	patternID := seedPattern(t, ts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.callTool(ctx, "learning_promote_pattern", map[string]any{
		"pattern_id": string(patternID),
	})
	if err != nil {
		// Transport-level "tool not found" surfaces as a Go error from
		// the SDK; that's the expected guard for read-only profiles.
		return
	}
	if !result.IsError {
		t.Fatalf("promote on read profile: IsError=false, want true; result=%+v", result)
	}
	text := errorText(t, result)
	if !strings.Contains(text, "access_denied") &&
		!strings.Contains(text, "unknown_tool") &&
		!strings.Contains(text, "tool_not_found") &&
		!strings.Contains(text, "method_not_found") {
		t.Fatalf("error text = %q, want access_denied|unknown_tool|tool_not_found|method_not_found", text)
	}
}

// TestCallTool_LearningPromotePattern_Idempotent verifies the second
// call against an already-promoted pattern returns WasNew=false with
// the same LearningID and AuditID.
func TestCallTool_LearningPromotePattern_Idempotent(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "admin")
	patternID := seedPattern(t, ts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	first, err := ts.callTool(ctx, "learning_promote_pattern", map[string]any{
		"pattern_id": string(patternID),
		"note":       "first call",
	})
	if err != nil {
		t.Fatalf("first call transport error: %v", err)
	}
	if first.IsError {
		t.Fatalf("first call: IsError=true, want false; result=%+v", first)
	}
	firstData := mustDecodeMap(t, first)
	firstResult, _ := firstData["result"].(map[string]any)
	firstLearningID := firstResult["learning_id"]
	firstAuditID := firstResult["audit_id"]

	second, err := ts.callTool(ctx, "learning_promote_pattern", map[string]any{
		"pattern_id": string(patternID),
		"note":       "second call",
	})
	if err != nil {
		t.Fatalf("second call transport error: %v", err)
	}
	if second.IsError {
		t.Fatalf("second call: IsError=true, want false; result=%+v", second)
	}
	secondData := mustDecodeMap(t, second)
	secondResult, _ := secondData["result"].(map[string]any)
	if got, _ := secondResult["was_new"].(bool); got {
		t.Fatalf("second call was_new = true, want false (idempotent)")
	}
	if secondResult["learning_id"] != firstLearningID {
		t.Fatalf("second learning_id = %v, want %v (same as first)",
			secondResult["learning_id"], firstLearningID)
	}
	if secondResult["audit_id"] != firstAuditID {
		t.Fatalf("second audit_id = %v, want %v (same as first)",
			secondResult["audit_id"], firstAuditID)
	}
}

// TestCallTool_LearningPromotePattern_NotQualified verifies the tool
// refuses a pattern that is still in the observed state. The error
// envelope must carry the canonical pattern_not_qualified code.
func TestCallTool_LearningPromotePattern_NotQualified(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "admin")
	// Seed an observed (not qualified) pattern.
	patternID := seedObservedPattern(t, ts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.callTool(ctx, "learning_promote_pattern", map[string]any{
		"pattern_id": string(patternID),
	})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("promote on observed pattern: IsError=false, want true; result=%+v", result)
	}
	if !strings.Contains(errorText(t, result), "pattern_not_qualified") {
		t.Fatalf("error text = %q, want pattern_not_qualified", errorText(t, result))
	}
}

// TestCallTool_LearningPromotePattern_InvalidInput verifies the input
// validation guard: an empty pattern_id surfaces an invalid_argument
// error envelope. The schema-level `required` validation rejects a
// missing key entirely, so we exercise the handler-level guard by
// sending an empty string. Both paths must surface invalid_argument.
func TestCallTool_LearningPromotePattern_InvalidInput(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "admin")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.callTool(ctx, "learning_promote_pattern", map[string]any{
		"pattern_id": "",
	})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("promote with empty pattern_id: IsError=false, want true; result=%+v", result)
	}
	if !strings.Contains(errorText(t, result), "invalid_argument") {
		t.Fatalf("error text = %q, want invalid_argument", errorText(t, result))
	}
}

// seedObservedPattern seeds a pattern in the observed state so the
// promote-not-qualified tests can target a pattern that is not
// eligible for promotion. The fixture mirrors seedPattern but
// overrides the qualification decision so the persisted pattern stays
// in PatternObserved.
func seedObservedPattern(t *testing.T, ts *testServer) domain.ExperiencePatternID {
	t.Helper()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	memberIDs := []domain.ExperienceEventID{"obs-evt-1", "obs-evt-2", "obs-evt-3"}
	if err := storage.WithTx(context.Background(), ts.db, func(tx *sql.Tx) error {
		turnID := domain.ExperienceTurnID("obs-pattern-turn-1")
		sessionID := domain.ExperienceSessionID("obs-pattern-session-1")
		occurredAt := now
		session := &domain.ExperienceSession{
			ID: sessionID, ProjectID: ts.projectID, Source: domain.SourceOpenCode,
			ExternalSessionID: "obs-pattern-external-session",
			Locator:           domain.TranscriptLocator{Kind: "sqlite", Path: "C:/safe/obs-pattern-sessions.db", SessionID: string(sessionID)},
			StartedAt:         &occurredAt, UpdatedAt: occurredAt, ClosedAt: &occurredAt,
			MetadataSHA256: "obs-pattern-session-digest", CreatedAt: occurredAt,
		}
		if err := storage.SaveExperienceSession(context.Background(), tx, session); err != nil {
			return err
		}
		turn := &domain.ExperienceTurn{
			ID: turnID, SessionID: sessionID, ExternalTurnID: "obs-pattern-external-turn",
			Sequence: 1, Status: domain.TurnIngested, Fingerprint: "obs-pattern-turn-fingerprint",
			UserDigest: "user-digest", AssistantDigest: "assistant-digest", ToolCallsDigest: "tool-digest",
			SafeSummary: "Synthetic observed pattern fixture.", OccurredAt: occurredAt, StableAt: &occurredAt,
			IngestedAt: occurredAt, SourceRevision: "revision-1", Redacted: true,
		}
		if err := storage.SaveExperienceTurn(context.Background(), tx, turn); err != nil {
			return err
		}
		for i, eventID := range memberIDs {
			event := &domain.ExperienceEvent{
				ID: eventID, ProjectID: ts.projectID, TurnID: turnID, Kind: domain.EventTestFailure,
				Summary: "Synthetic observed pattern event.", Observation: "1-session, 1-day cluster.",
				Outcome: "success", Fingerprint: fmt.Sprintf("obs-pattern-event-fingerprint-%d", i+1),
				EvidenceJSON: `[{"kind":"test"}]`,
				Detector:     domain.DetectorIdentity{Kind: "deterministic", Name: "obs-pattern-fixture", Version: "1.0.0"},
				Confidence:   domain.ConfidenceMedium, CreatedAt: now.Add(time.Duration(i) * time.Minute),
			}
			if err := storage.SaveExperienceEvent(context.Background(), tx, event); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed observed pattern events: %v", err)
	}
	svc := patterns.NewService(ts.db)
	cluster := patterns.ClusterRecord{
		Fingerprint:        "fp-mcp-obs-1",
		Kind:               domain.EventTestFailure,
		Members:            memberIDs,
		Sessions:           map[string]struct{}{"sess-1": {}},
		Days:               map[string]struct{}{"2026-07-25": {}},
		DistinctSessions:   1,
		DistinctDays:       1,
		OccurrenceCount:    1,
		SuccessfulOutcomes: 0,
		FirstSeenAt:        now,
		LastSeenAt:         now,
		RetrievalTerms:     []string{"observed"},
	}
	saved, err := svc.IngestCluster(context.Background(), ts.projectID, cluster, patterns.QualificationDecision{Status: patterns.PatternObserved})
	if err != nil {
		t.Fatalf("IngestCluster(observed): %v", err)
	}
	if saved.Status != patterns.PatternObserved {
		t.Fatalf("seeded pattern status = %s, want observed", saved.Status)
	}
	return saved.ID
}
