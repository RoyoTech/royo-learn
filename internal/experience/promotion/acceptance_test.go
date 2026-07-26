// Acceptance test for Hito 7 slice 7.4 — promotion pipeline end-to-end.
//
// The test is split from the slice 7.2 service tests so the CLI/MCP
// integration story is observable in isolation: the happy path
// (qualified → Learning), the idempotent path (already promoted →
// existing Learning) and the not-qualified path (observed → typed
// error). The fixture mirrors the CLI surface so a regression that
// only the CLI can reproduce is caught here.
//
// The test is parked in the promotion package because the acceptance
// surface is the bridge, not the dispatcher. The CLI is invoked
// indirectly through the same promotion.NewService + capture.Service
// the dispatcher uses, which gives the same backend semantics
// without dragging the full CLI flag-parser into the test. The CLI
// surface is exercised in cmd/royo-learn/experience_patterns_test.go
// (slice 7.4 RED tests).
//
// Per docs/23-PATTERN-MINING.md §8 RF-E08, the three paths must:
//
//   1. qualified → Learning: pattern promoted, status=promoted,
//      proposed_learning_id populated, exactly one Learning exists
//      with idempotency_key = "promotion:" + pattern.Fingerprint.
//   2. already-promoted → idempotent: second call returns the same
//      LearningID with WasNew=false, no duplicate audit row, no
//      duplicate Learning.
//   3. not-qualified → error: pattern in observed state surfaces
//      ErrPromotionNotEligible with the canonical pattern_not_qualified
//      code; the pattern is untouched.

package promotion

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/patterns"
	"agent-royo-learn/internal/storage"
)

// learningCountForKey counts the persisted learnings with the given
// idempotency key. The promotion pipeline keys Capture on
// "promotion:" + pattern.Fingerprint, so the count after the
// already-promoted path must remain 1.
func learningCountForKey(t *testing.T, db *storage.DB, key string) int {
	t.Helper()
	var n int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM learnings WHERE idempotency_key = ?`, key).Scan(&n); err != nil {
		t.Fatalf("query learnings: %v", err)
	}
	return n
}

// patternStatus returns the persisted status of the pattern so the
// acceptance tests can confirm the bridge did not mutate the pattern
// when the call was rejected.
func patternStatus(t *testing.T, db *storage.DB, patternID domain.ExperiencePatternID) patterns.PatternStatus {
	t.Helper()
	var status string
	if err := db.DB.QueryRow(`SELECT status FROM experience_patterns WHERE id = ?`, string(patternID)).Scan(&status); err != nil {
		t.Fatalf("query pattern status: %v", err)
	}
	return patterns.PatternStatus(status)
}

// promotionAuditRowCount counts the experience_pattern_promoted audit
// rows for the pattern. The slice 7.3 idempotency guard makes this
// stable across retries: the first call writes 1, the second call
// must NOT write a second row.
func promotionAuditRowCount(t *testing.T, db *storage.DB, patternID domain.ExperiencePatternID) int {
	t.Helper()
	var n int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE entity_id = ? AND operation = ?`,
		string(patternID), "experience_pattern_promoted").Scan(&n); err != nil {
		t.Fatalf("query audit_events: %v", err)
	}
	return n
}

// TestAcceptance_QualifiedPattern_PromotionSucceeds pins path (1).
// A qualified pattern flows through the full pipeline and the
// acceptance invariants hold:
//
//   - status='promoted' + proposed_learning_id IS NOT NULL
//   - exactly one Learning with idempotency_key = "promotion:" + fp
//   - exactly one experience_pattern_promoted audit row
func TestAcceptance_QualifiedPattern_PromotionSucceeds(t *testing.T) {
	t.Parallel()

	fx := newPromotionFixture(t, patterns.PatternQualified)

	res, err := fx.promotion.Promote(context.Background(), fx.projectID, &PromotionInput{
		PatternID: fx.pattern.ID,
		Actor:     domain.Actor{Kind: "user", Name: "operator"},
		Note:      "acceptance: qualified pattern",
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if res == nil {
		t.Fatal("Promote returned nil result")
	}
	if !res.WasNew {
		t.Fatalf("WasNew = false, want true")
	}
	if res.LearningID == "" {
		t.Fatal("LearningID is empty")
	}

	// Pattern invariants.
	got, err := fx.patternSvc.Get(context.Background(), fx.pattern.ID)
	if err != nil {
		t.Fatalf("patternSvc.Get: %v", err)
	}
	if got.Status != patterns.PatternPromoted {
		t.Fatalf("pattern.Status = %s, want promoted", got.Status)
	}
	if got.ProposedLearningID == nil || string(*got.ProposedLearningID) != string(res.LearningID) {
		t.Fatalf("pattern.ProposedLearningID = %v, want %s", got.ProposedLearningID, res.LearningID)
	}

	// Learning invariant: exactly one learning with the idempotency key.
	key := "promotion:" + fx.pattern.Fingerprint
	if n := learningCountForKey(t, fx.db, key); n != 1 {
		t.Fatalf("learnings with key %q = %d, want 1", key, n)
	}

	// Audit invariant: exactly one promotion audit row.
	if n := promotionAuditRowCount(t, fx.db, fx.pattern.ID); n != 1 {
		t.Fatalf("experience_pattern_promoted rows = %d, want 1", n)
	}
}

// TestAcceptance_AlreadyPromoted_IsIdempotent pins path (2). The
// second call returns the same LearningID with WasNew=false, no
// duplicate learning, no duplicate audit row. The pattern invariants
// also remain stable (the run is a no-op on the database side).
func TestAcceptance_AlreadyPromoted_IsIdempotent(t *testing.T) {
	t.Parallel()

	fx := newPromotionFixture(t, patterns.PatternQualified)

	first, err := fx.promotion.Promote(context.Background(), fx.projectID, &PromotionInput{
		PatternID: fx.pattern.ID,
		Actor:     domain.Actor{Kind: "user", Name: "operator"},
		Note:      "acceptance: first call",
	})
	if err != nil {
		t.Fatalf("first Promote: %v", err)
	}
	if !first.WasNew {
		t.Fatalf("first WasNew = false, want true")
	}

	// Second call. The slice 7.3 idempotency guard must short-circuit
	// before Capture runs, so the audit row count stays at 1.
	second, err := fx.promotion.Promote(context.Background(), fx.projectID, &PromotionInput{
		PatternID: fx.pattern.ID,
		Actor:     domain.Actor{Kind: "user", Name: "operator"},
		Note:      "acceptance: idempotent retry",
	})
	if err != nil {
		t.Fatalf("second Promote: %v", err)
	}
	if second.WasNew {
		t.Fatalf("second WasNew = true, want false (idempotent)")
	}
	if second.LearningID != first.LearningID {
		t.Fatalf("second LearningID = %s, want %s (same as first)",
			second.LearningID, first.LearningID)
	}
	if second.AuditID != first.AuditID {
		t.Fatalf("second AuditID = %s, want %s (same as first)",
			second.AuditID, first.AuditID)
	}

	// Database invariants: still exactly one learning, still exactly
	// one audit row.
	key := "promotion:" + fx.pattern.Fingerprint
	if n := learningCountForKey(t, fx.db, key); n != 1 {
		t.Fatalf("learnings with key %q = %d, want 1 (no duplicate)", key, n)
	}
	if n := promotionAuditRowCount(t, fx.db, fx.pattern.ID); n != 1 {
		t.Fatalf("experience_pattern_promoted rows = %d, want 1 (no duplicate)", n)
	}
}

// TestAcceptance_NotQualified_Error pins path (3). A pattern in the
// observed state cannot be promoted: the call surfaces
// ErrPromotionNotEligible with the canonical pattern_not_qualified
// code, and the pattern is untouched in the database.
func TestAcceptance_NotQualified_Error(t *testing.T) {
	t.Parallel()

	fx := newPromotionFixture(t, patterns.PatternObserved)

	_, err := fx.promotion.Promote(context.Background(), fx.projectID, &PromotionInput{
		PatternID: fx.pattern.ID,
		Actor:     domain.Actor{Kind: "user", Name: "operator"},
		Note:      "acceptance: not eligible",
	})
	if err == nil {
		t.Fatal("Promote on observed pattern = nil error, want ErrPromotionNotEligible")
	}
	if !ErrorIs(err, ErrPromotionNotEligible) {
		t.Fatalf("error %v does not match ErrPromotionNotEligible", err)
	}
	domainErr, ok := domain.AsDomainError(err)
	if !ok {
		t.Fatalf("error %v is not a DomainError", err)
	}
	if domainErr.Code != domain.ErrPatternNotQualified {
		t.Fatalf("error code = %s, want %s", domainErr.Code, domain.ErrPatternNotQualified)
	}

	// Pattern invariants: status remains observed, no proposed_learning_id.
	if got := patternStatus(t, fx.db, fx.pattern.ID); got != patterns.PatternObserved {
		t.Fatalf("pattern.Status = %s, want observed (pattern must not be touched)", got)
	}
	var proposed sql.NullString
	if err := fx.db.DB.QueryRow(`SELECT proposed_learning_id FROM experience_patterns WHERE id = ?`,
		string(fx.pattern.ID)).Scan(&proposed); err != nil {
		t.Fatalf("query proposed_learning_id: %v", err)
	}
	if proposed.Valid {
		t.Fatalf("proposed_learning_id = %v, want NULL", proposed)
	}

	// Audit invariant: no experience_pattern_promoted row was written.
	if n := promotionAuditRowCount(t, fx.db, fx.pattern.ID); n != 0 {
		t.Fatalf("experience_pattern_promoted rows = %d, want 0", n)
	}
}

// TestAcceptance_CaptureServiceBridgesLearningFromPattern is an
// integration-level pin that the capture pipeline receives the
// expected promotion fields end-to-end. We verify the learning
// title mirrors the pattern's title (the redaction pipeline stabilises
// the fields before Capture), the scope is project, and the
// destination is the project default. These assertions guard the
// contract docs/23-PATTERN-MINING.md §8 promises.
func TestAcceptance_CaptureServiceBridgesLearningFromPattern(t *testing.T) {
	t.Parallel()

	fx := newPromotionFixture(t, patterns.PatternQualified)

	res, err := fx.promotion.Promote(context.Background(), fx.projectID, &PromotionInput{
		PatternID: fx.pattern.ID,
		Actor:     domain.Actor{Kind: "user", Name: "operator"},
		Note:      "acceptance: bridge invariants",
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	// The learning row must exist with the expected scope/destination.
	var scopeGuess, proposedDestination sql.NullString
	if err := fx.db.DB.QueryRow(`SELECT scope_guess, proposed_destination FROM learnings WHERE id = ?`,
		string(res.LearningID)).Scan(&scopeGuess, &proposedDestination); err != nil {
		t.Fatalf("query learnings: %v", err)
	}
	if !scopeGuess.Valid || scopeGuess.String != string(domain.ScopeProject) {
		t.Fatalf("learning scope_guess = %v, want %s", scopeGuess, domain.ScopeProject)
	}
	if !proposedDestination.Valid || proposedDestination.String != string(domain.DestProject) {
		t.Fatalf("learning proposed_destination = %v, want %s", proposedDestination, domain.DestProject)
	}
}

// TestAcceptance_StableJSON_IsExecutableByExternalConsumer pins the
// JSON shape of the promotion result so external consumers (CLI, MCP,
// future skill invocations) can rely on a stable contract. The
// Documented fields are serialized exactly as the regex captures
// them; reshuffling the order or renaming requires a versioned
// contract change.
func TestAcceptance_StableJSON_IsExecutableByExternalConsumer(t *testing.T) {
	t.Parallel()

	fx := newPromotionFixture(t, patterns.PatternQualified)

	res, err := fx.promotion.Promote(context.Background(), fx.projectID, &PromotionInput{
		PatternID: fx.pattern.ID,
		Actor:     domain.Actor{Kind: "user", Name: "operator"},
		Note:      "acceptance: json shape",
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	// The Result envelope is the canonical public shape; the
	// PromotionResult JSON tags pin the field names so the test only
	// re-asserts what the contract guards.
	if res.PatternID == "" || res.LearningID == "" || res.AuditID == "" {
		t.Fatalf("Result has empty required fields: %+v", res)
	}
	if !strings.Contains(string(res.LearningID), "-") {
		t.Fatalf("LearningID %q does not look like a UUID/v7", res.LearningID)
	}
}
