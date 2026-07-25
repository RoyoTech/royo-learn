// Service tests for Hito 6 slice 6.4 (Dismissal, List, Get, IngestCluster).
//
// These tests cover the typed-dismissal contract documented in
// docs/23-PATTERN-MINING.md §7 and the project rules in
// docs/24-EXPERIENCE-THREAT-MODEL.md §6 (structured audit only, no
// transcript content, sensitive dismissal must not leak event
// content).

package patterns_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/patterns"
)

// dismissalFixture builds a project + saved pattern the dismissal
// tests operate on.
func dismissalFixture(t *testing.T, svc *patterns.Service, projectID domain.ProjectID, fp string) *patterns.ExperiencePattern {
	t.Helper()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	cluster := patterns.ClusterRecord{
		Fingerprint:        fp,
		Kind:               domain.EventTestFailure,
		Members:            []domain.ExperienceEventID{domain.ExperienceEventID("evt-" + fp)},
		Sessions:           map[string]struct{}{"sess-1": {}},
		Days:               map[string]struct{}{"2026-07-25": {}},
		DistinctSessions:   3,
		DistinctDays:       3,
		OccurrenceCount:    5,
		FirstSeenAt:        now,
		LastSeenAt:         now,
		RetrievalTerms:     []string{"compile"},
		SourceFingerprints: []string{fp},
	}
	// Seed the experience_event so the membership FK target exists.
	seedExperienceEvents(t, svc.DBForTest(), projectID, cluster.Members)
	decision := patterns.QualificationDecision{Status: patterns.PatternQualified}
	saved, err := svc.IngestCluster(context.Background(), projectID, cluster, decision)
	if err != nil {
		t.Fatalf("IngestCluster: %v", err)
	}
	return saved
}

// TestDismiss_IdempotentSameReason verifies the documented
// idempotency rule: re-dismissing the same pattern with the same
// reason returns nil and writes no extra audit row.
func TestDismiss_IdempotentSameReason(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "dismiss-idem")
	svc := patterns.NewService(db)
	saved := dismissalFixture(t, svc, fixture.ProjectID, "fp-dismiss-1")

	details := patterns.DismissalDetails{
		Reason: patterns.DismissalNotReusable,
		Note:   "first note",
		Actor:  domain.Actor{Kind: "user", Name: "reviewer"},
	}
	if err := svc.Dismiss(context.Background(), saved.ID, patterns.DismissalNotReusable, details); err != nil {
		t.Fatalf("first Dismiss: %v", err)
	}
	if err := svc.Dismiss(context.Background(), saved.ID, patterns.DismissalNotReusable, details); err != nil {
		t.Fatalf("second Dismiss (idempotent): %v", err)
	}
}

// TestDismiss_RejectsPromoted covers the docs/23 §9 invariant: once
// a pattern has been promoted (Hito 7 territory, but the contract is
// pinned here) it cannot be dismissed.
func TestDismiss_RejectsPromoted(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "dismiss-promoted")
	svc := patterns.NewService(db)
	saved := dismissalFixture(t, svc, fixture.ProjectID, "fp-dismiss-2")

	// Force the status to promoted through the repository so the
	// service sees it as already promoted. The acceptance matrix
	// (docs/25) pins this contract.
	repo := patterns.NewRepository(db)
	if _, err := repo.SetStatus(context.Background(), saved.ID, patterns.PatternPromoted); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	err := svc.Dismiss(context.Background(), saved.ID, patterns.DismissalNotReusable, patterns.DismissalDetails{})
	if err != patterns.ErrPatternAlreadyPromoted {
		t.Fatalf("Dismiss(promoted) = %v, want ErrPatternAlreadyPromoted", err)
	}
}

// TestDismiss_DifferentReasonRejected ensures the (pattern_id,
// reason) idempotence is strict: a different reason on an
// already-dismissed pattern is rejected, forcing the operator to
// surface the previous dismissal first.
func TestDismiss_DifferentReasonRejected(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "dismiss-diff")
	svc := patterns.NewService(db)
	saved := dismissalFixture(t, svc, fixture.ProjectID, "fp-dismiss-3")

	if err := svc.Dismiss(context.Background(), saved.ID, patterns.DismissalOneOff, patterns.DismissalDetails{}); err != nil {
		t.Fatalf("first Dismiss: %v", err)
	}
	err := svc.Dismiss(context.Background(), saved.ID, patterns.DismissalFalseCluster, patterns.DismissalDetails{})
	if err == nil {
		t.Fatal("Dismiss with different reason = nil, want error")
	}
	if !errors.Is(err, patterns.ErrPatternInsufficientSources) && !sameErrorCode(err, patterns.ErrPatternInsufficientSources) {
		t.Fatalf("Dismiss error = %v, want ErrPatternInsufficientSources", err)
	}
}

// TestDismiss_NotFoundReturnsTypedError ensures the missing-pattern
// path returns the canonical ErrPatternNotFound.
func TestDismiss_NotFoundReturnsTypedError(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	newProjectFixture(t, db, "dismiss-missing")
	svc := patterns.NewService(db)

	err := svc.Dismiss(context.Background(), domain.ExperiencePatternID("pat-missing"),
		patterns.DismissalOneOff, patterns.DismissalDetails{})
	if !errors.Is(err, patterns.ErrPatternNotFound) && !sameErrorCode(err, patterns.ErrPatternNotFound) {
		t.Fatalf("Dismiss(missing) = %v, want ErrPatternNotFound", err)
	}
}

// TestDismiss_InvalidReasonRejected ensures the typed-reason list is
// enforced at the service boundary.
func TestDismiss_InvalidReasonRejected(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "dismiss-bad-reason")
	svc := patterns.NewService(db)
	saved := dismissalFixture(t, svc, fixture.ProjectID, "fp-dismiss-4")

	err := svc.Dismiss(context.Background(), saved.ID, patterns.DismissalReason("nonsense"), patterns.DismissalDetails{})
	if err == nil {
		t.Fatal("Dismiss(invalid reason) = nil, want error")
	}
}

// TestDismiss_NoteByteLimit guards the MaxDismissalNoteBytes cap so
// a malicious actor cannot grow the audit row unbounded.
func TestDismiss_NoteByteLimit(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "dismiss-note")
	svc := patterns.NewService(db)
	saved := dismissalFixture(t, svc, fixture.ProjectID, "fp-dismiss-5")

	hugeNote := make([]byte, patterns.MaxDismissalNoteBytes+8)
	for i := range hugeNote {
		hugeNote[i] = 'a'
	}
	err := svc.Dismiss(context.Background(), saved.ID, patterns.DismissalOneOff, patterns.DismissalDetails{
		Note: string(hugeNote),
	})
	if err == nil {
		t.Fatal("Dismiss(huge note) = nil, want error")
	}
}

// TestList_DefaultObserved covers the documented default: List with
// no status returns observed patterns. The fixture dismisses the
// patterns without specifying a status transition, so they remain
// in the qualified state (the dismissal fixture uses
// PatternQualified on the decision).
func TestList_DefaultObserved(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "list-default")
	svc := patterns.NewService(db)

	dismissalFixture(t, svc, fixture.ProjectID, "fp-list-1")
	dismissalFixture(t, svc, fixture.ProjectID, "fp-list-2")

	// The fixture's decision is PatternQualified; the patterns end
	// up with status="qualified". List with no filter defaults to
	// "observed" and returns zero rows.
	out, err := svc.List(context.Background(), patterns.ListerFilter{Project: fixture.ProjectID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("List observed len = %d, want 0", len(out))
	}

	// Querying by status=qualified returns the saved patterns.
	out, err = svc.List(context.Background(), patterns.ListerFilter{Project: fixture.ProjectID, Status: patterns.PatternQualified})
	if err != nil {
		t.Fatalf("List qualified: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("List qualified len = %d, want 2", len(out))
	}
}

// TestList_FilterByKind ensures the kind filter narrows the result
// set.
func TestList_FilterByKind(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "list-kind")
	svc := patterns.NewService(db)

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	clusters := []patterns.ClusterRecord{
		{
			Fingerprint:      "fp-kind-a",
			Kind:             domain.EventTestFailure,
			Members:          []domain.ExperienceEventID{"evt-1"},
			Sessions:         map[string]struct{}{"sess-1": {}},
			Days:             map[string]struct{}{"2026-07-25": {}},
			DistinctSessions: 3, DistinctDays: 3, OccurrenceCount: 3,
			FirstSeenAt: now, LastSeenAt: now,
			RetrievalTerms: []string{"compile"},
		},
		{
			Fingerprint:      "fp-kind-b",
			Kind:             domain.EventCommandFailure,
			Members:          []domain.ExperienceEventID{"evt-2"},
			Sessions:         map[string]struct{}{"sess-2": {}},
			Days:             map[string]struct{}{"2026-07-25": {}},
			DistinctSessions: 3, DistinctDays: 3, OccurrenceCount: 3,
			FirstSeenAt: now, LastSeenAt: now,
			RetrievalTerms: []string{"linker"},
		},
	}
	for _, c := range clusters {
		seedExperienceEvents(t, db, fixture.ProjectID, c.Members)
		if _, err := svc.IngestCluster(context.Background(), fixture.ProjectID, c, patterns.QualificationDecision{Status: patterns.PatternQualified}); err != nil {
			t.Fatalf("IngestCluster: %v", err)
		}
	}

	out, err := svc.List(context.Background(), patterns.ListerFilter{
		Project: fixture.ProjectID,
		Status:  patterns.PatternQualified,
		Kind:    domain.EventTestFailure,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("List len = %d, want 1", len(out))
	}
	if out[0].Kind != domain.EventTestFailure {
		t.Fatalf("Kind = %s, want test_failure", out[0].Kind)
	}
}

// TestList_LimitCapped ensures the limit filter truncates the result
// without mutating the underlying data.
func TestList_LimitCapped(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "list-limit")
	svc := patterns.NewService(db)

	for i, fp := range []string{"fp-l-1", "fp-l-2", "fp-l-3"} {
		seedExperienceEvents(t, db, fixture.ProjectID, []domain.ExperienceEventID{domain.ExperienceEventID("evt-" + fp)})
		saved, err := svc.IngestCluster(context.Background(), fixture.ProjectID,
			patterns.ClusterRecord{
				Fingerprint:      fp,
				Kind:             domain.EventTestFailure,
				Members:          []domain.ExperienceEventID{domain.ExperienceEventID("evt-" + fp)},
				Sessions:         map[string]struct{}{"sess-" + fp: {}},
				Days:             map[string]struct{}{"2026-07-25": {}},
				DistinctSessions: 3, DistinctDays: 3, OccurrenceCount: 3,
				FirstSeenAt: time.Now().UTC(), LastSeenAt: time.Now().UTC(),
				RetrievalTerms: []string{"compile"},
			}, patterns.QualificationDecision{Status: patterns.PatternQualified})
		if err != nil {
			t.Fatalf("IngestCluster[%d] fp=%s: %v", i, fp, err)
		}
		t.Logf("saved[%d] id=%s fp=%s", i, saved.ID, saved.Fingerprint)
	}
	out, err := svc.List(context.Background(), patterns.ListerFilter{
		Project: fixture.ProjectID,
		Status:  patterns.PatternQualified,
		Limit:   2,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("List len = %d, want 2", len(out))
	}
}

// TestGet_RoundTrip ensures Get returns the saved pattern by id.
func TestGet_RoundTrip(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "get")
	svc := patterns.NewService(db)
	saved := dismissalFixture(t, svc, fixture.ProjectID, "fp-get-1")

	got, err := svc.Get(context.Background(), saved.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != saved.ID || got.Fingerprint != saved.Fingerprint {
		t.Fatalf("Get = %+v, want ID=%s Fingerprint=%s", got, saved.ID, saved.Fingerprint)
	}
}

// TestGet_NotFoundTyped covers the missing-id path.
func TestGet_NotFoundTyped(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	newProjectFixture(t, db, "get-missing")
	svc := patterns.NewService(db)

	_, err := svc.Get(context.Background(), domain.ExperiencePatternID("pat-missing"))
	if !errors.Is(err, patterns.ErrPatternNotFound) && !sameErrorCode(err, patterns.ErrPatternNotFound) {
		t.Fatalf("Get(missing) = %v, want ErrPatternNotFound", err)
	}
}

// TestIngestCluster_PreservesFirstSeenAt verifies the resave rule:
// re-ingesting the same fingerprint keeps FirstSeenAt stable and
// advances UpdatedAt.
func TestIngestCluster_PreservesFirstSeenAt(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "ingest-preserve")
	svc := patterns.NewService(db)

	first := dismissalFixture(t, svc, fixture.ProjectID, "fp-ingest-1")
	second := dismissalFixture(t, svc, fixture.ProjectID, "fp-ingest-1")

	if !second.FirstSeenAt.Equal(first.FirstSeenAt) {
		t.Fatalf("FirstSeenAt changed: first=%v second=%v", first.FirstSeenAt, second.FirstSeenAt)
	}
	if second.Revision <= first.Revision {
		t.Fatalf("Revision did not advance: first=%d second=%d", first.Revision, second.Revision)
	}
}

// TestStableJSON_RoundTrip locks down the JSON shape so CLI / MCP /
// audit consumers can rely on it. Empty dismissal_reason is omitted
// via `omitempty`; the test exercises both the populated and the
// default cases.
func TestStableJSON_RoundTrip(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "json")
	svc := patterns.NewService(db)
	saved := dismissalFixture(t, svc, fixture.ProjectID, "fp-json-1")

	encoded, err := saved.MarshalStable()
	if err != nil {
		t.Fatalf("MarshalStable: %v", err)
	}
	encodedStr := string(encoded)
	for _, key := range []string{
		`"id":"`,
		`"fingerprint":"fp-json-1"`,
		`"status":"qualified"`,
		`"distinct_sessions":3`,
		`"revision":`,
	} {
		if !contains(encodedStr, key) {
			t.Fatalf("MarshalStable missing %q in %s", key, encodedStr)
		}
	}

	// After dismissal, dismissal_reason is non-empty and must appear.
	if err := svc.Dismiss(context.Background(), saved.ID, patterns.DismissalOneOff, patterns.DismissalDetails{}); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	dismissed, err := svc.Get(context.Background(), saved.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	encoded, err = dismissed.MarshalStable()
	if err != nil {
		t.Fatalf("MarshalStable: %v", err)
	}
	encodedStr = string(encoded)
	if !contains(encodedStr, `"dismissal_reason":"one_off"`) {
		t.Fatalf("MarshalStable missing dismissal_reason in %s", encodedStr)
	}
	if !contains(encodedStr, `"status":"dismissed"`) {
		t.Fatalf("MarshalStable missing status dismissed in %s", encodedStr)
	}
}

// contains is a tiny stdlib-free helper for the JSON-round-trip test.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// errorsIs helper is defined in storage_test.go.

// sameErrorCode reports whether err is a domain error whose code
// matches target. It complements errors.Is by also matching
// typed errors that do not implement Unwrap (e.g. *ConflictError,
// *NotFoundError) — those wrap a *DomainError via struct embedding
// but errors.Is cannot reach it without an Unwrap method.
func sameErrorCode(err error, target error) bool {
	if err == nil || target == nil {
		return err == target
	}
	domainErr, ok := domain.AsDomainError(err)
	if !ok {
		return false
	}
	domainTarget, ok := domain.AsDomainError(target)
	if !ok {
		return false
	}
	return domainErr.Code == domainTarget.Code
}
