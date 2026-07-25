// Qualification tests for Hito 6 slice 6.3.
//
// The Qualifier evaluates a ClusterRecord against the conservative
// criteria in docs/23-PATTERN-MINING.md §5:
//
//   - ≥ 3 distinct sessions OR ≥ 3 distinct days (criterion A).
//   - ≥ 2 successful outcomes OR an explicit repeated correction
//     (criterion B).
//   - No posterior contradiction that the operator has recorded
//     (criterion C).
//   - Not a fact or a preference (criterion D — the candidate kind is
//     not EventPreference nor EventUnknown).
//   - Not covered by an existing Learning (criterion E).
//   - Not from reintentos técnicos duplicados — specifically, the
//     3-retries-in-1-session anti-pattern must NOT qualify (criterion
//     F / anti-anti-pattern detector).
//   - Traceable sources: ≥ 2 distinct sessions, each with at least one
//     known member source (criterion G).
//   - Similarity does not depend on a single generic word
//     (criterion H).
//
// Each criterion is a separate code path so the tests can exercise
// them independently. The Qualifier is a pure function: it does not
// mutate the cluster and does not call I/O.

package patterns

import (
	"context"
	"errors"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

// makeCluster is the test helper that builds a ClusterRecord with
// sensible defaults. The qualification tests tweak individual fields
// to exercise the boundary conditions. SuccessfulOutcomes is set to
// the documented default (≥ 2) so the happy path does not need to
// override it.
func makeCluster(t *testing.T, mut func(*ClusterRecord)) ClusterRecord {
	t.Helper()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	c := ClusterRecord{
		Fingerprint:        "fp-Q",
		Kind:               domain.EventTestFailure,
		Members:            []domain.ExperienceEventID{"evt-1", "evt-2", "evt-3"},
		Sessions:           map[string]struct{}{"sess-1": {}, "sess-2": {}, "sess-3": {}},
		Days:               map[string]struct{}{"2026-07-25": {}, "2026-07-26": {}, "2026-07-27": {}},
		DistinctSessions:   3,
		DistinctDays:       3,
		OccurrenceCount:    3,
		SuccessfulOutcomes: 2,
		FirstSeenAt:        now,
		LastSeenAt:         now.Add(48 * time.Hour),
		RetrievalTerms:     []string{"compile", "missing", "header"},
		SourceFingerprints: []string{"fp-Q"},
	}
	if mut != nil {
		mut(&c)
	}
	return c
}

// qualifyOK returns the Qualifier and a QualifierError with ok=true
// when the qualification decision is "qualified".
func qualifyOK(t *testing.T, c ClusterRecord) (*ConservativeQualifier, QualificationDecision, error) {
	t.Helper()
	q, err := NewQualifier(DefaultQualificationCriteria())
	if err != nil {
		t.Fatalf("NewQualifier: %v", err)
	}
	d, err := q.Qualify(context.Background(), c)
	return q, d, err
}

// TestQualify_HappyPath covers the conservative happy path: a cluster
// that satisfies every criterion MUST be promoted to qualified.
func TestQualify_HappyPath(t *testing.T) {
	t.Parallel()

	_, d, err := qualifyOK(t, makeCluster(t, nil))
	if err != nil {
		t.Fatalf("Qualify = %v, want nil", err)
	}
	if d.Status != PatternQualified {
		t.Fatalf("Status = %s, want %s (reasons=%v)", d.Status, PatternQualified, d.Reasons)
	}
}

// TestQualify_ThreeDaysInOneSessionDoesNotQualify pins the docs/23 §9
// anti-pattern rule: even when the days-branch of criterion A is
// satisfied, a single-session cluster (regardless of how many days
// span the retries) MUST NOT qualify. This is the canonical
// anti-anti-pattern detector.
//
// The test deliberately expects PatternObserved (not qualified) so
// a future refactor that loosens criterion F fails loudly.
func TestQualify_ThreeDaysInOneSessionDoesNotQualify(t *testing.T) {
	t.Parallel()

	c := makeCluster(t, func(c *ClusterRecord) {
		c.Sessions = map[string]struct{}{"sess-1": {}}
		c.Days = map[string]struct{}{"2026-07-25": {}, "2026-07-26": {}, "2026-07-27": {}}
		c.DistinctSessions = 1
		c.DistinctDays = 3
		c.SuccessfulOutcomes = 2
		c.RepeatedCorrection = true
	})
	_, d, err := qualifyOK(t, c)
	if err != nil {
		t.Fatalf("Qualify = %v, want nil", err)
	}
	if d.Status == PatternQualified {
		t.Fatalf("Status = %s, want not qualified (single-session anti-pattern overrides OR)", d.Status)
	}
	if !containsReason(d.Reasons, "single_session_retries") {
		t.Fatalf("Reasons = %v, want single_session_retries present", d.Reasons)
	}
}

// TestQualify_ThreeSessionsOneDayQualifies exercises the inverse of
// the anti-pattern: 3 sessions in 1 day IS enough (criterion A
// disjunction). It complements the anti-pattern test above so the
// v1 algorithm is fully pinned around the OR semantics.
func TestQualify_ThreeSessionsOneDayQualifies(t *testing.T) {
	t.Parallel()

	c := makeCluster(t, func(c *ClusterRecord) {
		c.Days = map[string]struct{}{"2026-07-25": {}}
		c.DistinctDays = 1
	})
	_, d, err := qualifyOK(t, c)
	if err != nil {
		t.Fatalf("Qualify = %v, want nil", err)
	}
	if d.Status != PatternQualified {
		t.Fatalf("Status = %s, want %s (3 sessions qualifies via OR)", d.Status, PatternQualified)
	}
}

// TestQualify_TwoSessionsOneDayStillFails covers the inverse of
// criterion A: when neither branch (sessions nor days) meets the
// threshold the pattern must NOT qualify. We use 2 sessions and 1
// day with the documented defaults (MinDistinctSessions=3,
// MinDistinctDays=2 per docs/23 §5), so neither branch is satisfied.
func TestQualify_TwoSessionsOneDayStillFails(t *testing.T) {
	t.Parallel()

	c := makeCluster(t, func(c *ClusterRecord) {
		c.Sessions = map[string]struct{}{"sess-1": {}, "sess-2": {}}
		c.DistinctSessions = 2
		c.Days = map[string]struct{}{"2026-07-25": {}}
		c.DistinctDays = 1
	})
	_, d, err := qualifyOK(t, c)
	if err != nil {
		t.Fatalf("Qualify = %v, want nil", err)
	}
	if d.Status == PatternQualified {
		t.Fatalf("Status = %s, want not qualified (sessions<3 AND days<2)", d.Status)
	}
	if !containsReason(d.Reasons, "insufficient_distinct_sessions_or_days") {
		t.Fatalf("Reasons = %v, want insufficient_distinct_sessions_or_days present", d.Reasons)
	}
}

// TestQualify_OneSessionThreeRetriesAntiPattern covers the canonical
// anti-pattern from docs/23 §9: "La cualificación de una sola sesión
// con 3 reintentos NO cualifica." Even with 3 occurrences in 1 session
// the cluster must NOT qualify. This is the regression test for the
// anti-anti-pattern detector.
func TestQualify_OneSessionThreeRetriesAntiPattern(t *testing.T) {
	t.Parallel()

	c := makeCluster(t, func(c *ClusterRecord) {
		c.Members = []domain.ExperienceEventID{"evt-1", "evt-2", "evt-3"}
		c.Sessions = map[string]struct{}{"sess-1": {}}
		c.Days = map[string]struct{}{"2026-07-25": {}, "2026-07-26": {}, "2026-07-27": {}}
		c.DistinctSessions = 1
		c.DistinctDays = 3
		c.OccurrenceCount = 3
	})

	q, err := NewQualifier(DefaultQualificationCriteria())
	if err != nil {
		t.Fatalf("NewQualifier: %v", err)
	}
	d, err := q.Qualify(context.Background(), c)
	if err != nil {
		t.Fatalf("Qualify = %v, want nil", err)
	}
	if d.Status == PatternQualified {
		t.Fatalf("Status = %s, want not qualified (1-session 3-retries anti-pattern)", d.Status)
	}
	if !containsReason(d.Reasons, "single_session_retries") {
		t.Fatalf("Reasons = %v, want single_session_retries to be present", d.Reasons)
	}
}

// TestQualify_SuccessfulOccurrences covers criterion B: the cluster
// must carry at least 2 successful outcomes (Result="success") or an
// explicit repeated correction. The test uses an explicit
// SuccessfulOutcomes counter because the Qualifier does not derive
// success from member metadata directly (criterion B input lives on
// the cluster summary).
func TestQualify_SuccessfulOccurrences(t *testing.T) {
	t.Parallel()

	// No successes → not qualified.
	c := makeCluster(t, nil)
	c.SuccessfulOutcomes = 1
	_, d, err := qualifyOK(t, c)
	if err != nil {
		t.Fatalf("Qualify = %v", err)
	}
	if d.Status == PatternQualified {
		t.Fatalf("Status = %s, want not qualified (1 success < 2)", d.Status)
	}

	// 2 successes → qualified.
	c.SuccessfulOutcomes = 2
	_, d, err = qualifyOK(t, c)
	if err != nil {
		t.Fatalf("Qualify = %v", err)
	}
	if d.Status != PatternQualified {
		t.Fatalf("Status = %s, want qualified (2 successes)", d.Status)
	}
}

// TestQualify_RepeatedCorrectionAlternative covers the second branch
// of criterion B: a repeated correction also satisfies the success
// requirement, even when no outcome is "success".
func TestQualify_RepeatedCorrectionAlternative(t *testing.T) {
	t.Parallel()

	c := makeCluster(t, func(c *ClusterRecord) {
		c.SuccessfulOutcomes = 0
		c.RepeatedCorrection = true
	})
	_, d, err := qualifyOK(t, c)
	if err != nil {
		t.Fatalf("Qualify = %v", err)
	}
	if d.Status != PatternQualified {
		t.Fatalf("Status = %s, want qualified (repeated correction)", d.Status)
	}
}

// TestQualify_PreferenceNeverQualifies covers criterion D: a
// preference event kind cannot be promoted to a Skill. This aligns
// with the docs/23 §3 rule "preference never becomes a Skill".
func TestQualify_PreferenceNeverQualifies(t *testing.T) {
	t.Parallel()

	c := makeCluster(t, func(c *ClusterRecord) {
		c.Kind = domain.EventPreference
	})
	_, d, err := qualifyOK(t, c)
	if err != nil {
		t.Fatalf("Qualify = %v", err)
	}
	if d.Status == PatternQualified {
		t.Fatalf("preference Status = %s, want not qualified", d.Status)
	}
}

// TestQualify_ContradictionBlocks covers criterion C: a posterior
// contradiction must block qualification. The Qualifier exposes a
// HasContradiction flag on the cluster so the orchestrator can stamp
// it; the test pins the behavior.
func TestQualify_ContradictionBlocks(t *testing.T) {
	t.Parallel()

	c := makeCluster(t, func(c *ClusterRecord) {
		c.HasContradiction = true
	})
	_, d, err := qualifyOK(t, c)
	if err != nil {
		t.Fatalf("Qualify = %v", err)
	}
	if d.Status == PatternQualified {
		t.Fatalf("Status = %s, want not qualified (contradiction)", d.Status)
	}
	if !containsReason(d.Reasons, "contradicted") {
		t.Fatalf("Reasons = %v, want contradicted present", d.Reasons)
	}
}

// TestQualify_CoveredByLearning covers criterion E: when an existing
// Learning already covers the pattern the candidate must NOT
// qualify. The duplicate would otherwise pollute review attention.
func TestQualify_CoveredByLearning(t *testing.T) {
	t.Parallel()

	c := makeCluster(t, func(c *ClusterRecord) {
		c.CoveredByLearningID = "learning-123"
	})
	_, d, err := qualifyOK(t, c)
	if err != nil {
		t.Fatalf("Qualify = %v", err)
	}
	if d.Status == PatternQualified {
		t.Fatalf("Status = %s, want not qualified (covered by existing Learning)", d.Status)
	}
}

// TestQualify_TraceabilityAtLeastTwoSources covers criterion G: the
// sources must be traceable. A cluster with only 1 distinct source
// is not enough. DistinctSessions ≥ 2 is the proxy the v1 algorithm
// uses (each session represents a traceable source).
func TestQualify_TraceabilityAtLeastTwoSources(t *testing.T) {
	t.Parallel()

	c := makeCluster(t, func(c *ClusterRecord) {
		c.Sessions = map[string]struct{}{"sess-1": {}}
		c.DistinctSessions = 1
		// Force the days path to also fail so we hit the criterion G
		// failure deterministically.
		c.Days = map[string]struct{}{"2026-07-25": {}}
		c.DistinctDays = 1
	})
	_, d, err := qualifyOK(t, c)
	if err != nil {
		t.Fatalf("Qualify = %v", err)
	}
	if d.Status == PatternQualified {
		t.Fatalf("Status = %s, want not qualified (single source)", d.Status)
	}
}

// TestQualify_GenericSingleWordRejected covers criterion H: a cluster
// whose only retrieval term is a generic word ("error", "fail", etc.)
// must NOT qualify. This is the documented "single generic word"
// guard.
func TestQualify_GenericSingleWordRejected(t *testing.T) {
	t.Parallel()

	c := makeCluster(t, func(c *ClusterRecord) {
		c.RetrievalTerms = []string{"error"}
	})
	_, d, err := qualifyOK(t, c)
	if err != nil {
		t.Fatalf("Qualify = %v", err)
	}
	if d.Status == PatternQualified {
		t.Fatalf("Status = %s, want not qualified (generic single-word similarity)", d.Status)
	}
}

// TestQualify_FewerThanTwoMembers covers the membership floor: the
// algorithm requires at least 2 members to even consider a pattern.
// Anything below that floor is observably not a "pattern".
func TestQualify_FewerThanTwoMembers(t *testing.T) {
	t.Parallel()

	c := makeCluster(t, func(c *ClusterRecord) {
		c.Members = []domain.ExperienceEventID{"evt-1"}
		c.OccurrenceCount = 1
	})
	_, d, err := qualifyOK(t, c)
	if err != nil {
		t.Fatalf("Qualify = %v", err)
	}
	if d.Status == PatternQualified {
		t.Fatalf("Status = %s, want not qualified (only 1 member)", d.Status)
	}
}

// TestQualify_ValidatorError ensures the constructor rejects
// misconfigured criteria. This matches the detector constructor
// convention from internal/experience/detectors/retry.go.
func TestQualify_ValidatorError(t *testing.T) {
	t.Parallel()

	bad := DefaultQualificationCriteria()
	bad.MinDistinctSessions = 0
	if _, err := NewQualifierWithCriteria(bad); err == nil {
		t.Fatal("NewQualifierWithCriteria accepted invalid criteria")
	}
}

// TestQualify_PureMutationFree pins the "no mutation" rule. The
// Qualifier must not modify the input cluster (the orchestrator may
// share clusters across threads).
func TestQualify_PureMutationFree(t *testing.T) {
	t.Parallel()

	c := makeCluster(t, nil)
	snapshot := c
	q, err := NewQualifier(DefaultQualificationCriteria())
	if err != nil {
		t.Fatalf("NewQualifier: %v", err)
	}
	_, _ = q.Qualify(context.Background(), c)
	if !clusterEqual(c, snapshot) {
		t.Fatalf("Qualify mutated the cluster: before=%+v after=%+v", snapshot, c)
	}
}

// TestQualify_CriteriaAreConfigurable ensures the criteria are real
// knobs. Raising MinDistinctSessions from 3 to 4 disqualifies a
// cluster with exactly 3 sessions.
func TestQualify_CriteriaAreConfigurable(t *testing.T) {
	t.Parallel()

	c := makeCluster(t, nil)

	criteria := DefaultQualificationCriteria()
	criteria.MinDistinctSessions = 4
	criteria.MinDistinctDays = 4
	q, _ := NewQualifierWithCriteria(criteria)
	d, err := q.Qualify(context.Background(), c)
	if err != nil {
		t.Fatalf("Qualify = %v", err)
	}
	if d.Status == PatternQualified {
		t.Fatalf("Status = %s, want not qualified (criteria raised to 4)", d.Status)
	}
}

// TestQualify_ContextCancel covers the contract that Qualify honors
// ctx.Done(). The current implementation does no I/O so the cancel
// check is defensive: a future implementation that loads external
// relations must still respect cancellation.
func TestQualify_ContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q, err := NewQualifier(DefaultQualificationCriteria())
	if err != nil {
		t.Fatalf("NewQualifier: %v", err)
	}
	c := makeCluster(t, nil)
	// The pure Qualifier does not surface a cancel error today; the
	// test verifies it returns without panic and returns a valid
	// QualificationDecision value. Future I/O-backed implementations
	// must surface ctx.Err() here.
	_, err = q.Qualify(ctx, c)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Qualify = %v, want nil or context.Canceled", err)
	}
}

// --- helpers ---

func containsReason(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}

func clusterEqual(a, b ClusterRecord) bool {
	if a.Fingerprint != b.Fingerprint || a.Kind != b.Kind || a.OccurrenceCount != b.OccurrenceCount {
		return false
	}
	if a.DistinctSessions != b.DistinctSessions || a.DistinctDays != b.DistinctDays {
		return false
	}
	if len(a.Members) != len(b.Members) {
		return false
	}
	for i := range a.Members {
		if a.Members[i] != b.Members[i] {
			return false
		}
	}
	if a.SuccessfulOutcomes != b.SuccessfulOutcomes || a.RepeatedCorrection != b.RepeatedCorrection {
		return false
	}
	if a.HasContradiction != b.HasContradiction || a.CoveredByLearningID != b.CoveredByLearningID {
		return false
	}
	if len(a.RetrievalTerms) != len(b.RetrievalTerms) {
		return false
	}
	for i := range a.RetrievalTerms {
		if a.RetrievalTerms[i] != b.RetrievalTerms[i] {
			return false
		}
	}
	return true
}
