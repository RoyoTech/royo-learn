// Conservative Qualifier for Hito 6 slice 6.3.
//
// The Qualifier evaluates a ClusterRecord against the criteria
// documented in docs/23-PATTERN-MINING.md §5 and §9. The eight
// criteria are:
//
//   A. ≥ 3 distinct sessions OR ≥ 3 distinct days (with
//      disjunction; anti-pattern §9 makes "1 session with 3 retries"
//      explicit).
//   B. ≥ 2 successful outcomes OR an explicit repeated correction.
//   C. No posterior contradiction that the operator has recorded.
//   D. Not a fact or a preference (kind != EventPreference, != EventUnknown).
//   E. Not covered by an existing Learning.
//   F. Not from reintentos técnicos duplicados — the canonical
//      anti-pattern from §9 ("3 retries in 1 session" must not
//      qualify even when the days-branch of A is satisfied).
//   G. ≥ 2 distinct sources (DistinctSessions proxy).
//   H. The retrieval-term set is not a single generic word.
//
// The Qualifier is pure (no I/O, no DB, no clock) and never mutates
// the cluster. It honors ctx.Done() defensively so a future
// I/O-backed implementation can still satisfy the contract.

package patterns

import (
	"context"
	"strings"

	"agent-royo-learn/internal/domain"
)

// genericSingleWord is the closed list of terms that, alone, do not
// distinguish a pattern. It is intentionally short and conservative:
// if the project needs more entries, add them with a documentation
// note in docs/23 §5.
var genericSingleWord = map[string]struct{}{
	"error":     {},
	"fail":      {},
	"failed":    {},
	"failure":   {},
	"warning":   {},
	"info":      {},
	"unknown":   {},
	"misc":      {},
	"other":     {},
	"general":   {},
	"default":   {},
	"debug":     {},
	"trace":     {},
	"log":       {},
	"message":   {},
	"exception": {},
	"crash":     {},
	"halt":      {},
	"issue":     {},
	"problem":   {},
	"task":      {},
}

// Qualifier is the conservative pattern qualifier. It is constructed
// once and reused; criteria are immutable.
type ConservativeQualifier struct {
	criteria QualificationCriteria
}

// NewQualifier constructs a Qualifier with the supplied criteria and
// validates them. The function returns a typed error (never a panic)
// on invalid input so call-sites can surface the misconfiguration
// through the project's error envelope without crashing the
// orchestrator. The contract test in contract_test.go pins this
// invariant.
func NewQualifier(criteria QualificationCriteria) (*ConservativeQualifier, error) {
	return NewQualifierWithCriteria(criteria)
}

// NewQualifierWithCriteria returns a Qualifier and validates the
// supplied criteria at construction. The same fail-fast convention
// as the detector constructors (internal/experience/detectors/retry.go).
func NewQualifierWithCriteria(criteria QualificationCriteria) (*ConservativeQualifier, error) {
	if err := criteria.Validate(); err != nil {
		return nil, err
	}
	return &ConservativeQualifier{criteria: criteria}, nil
}

// Criteria returns the immutable criteria the Qualifier uses.
func (q *ConservativeQualifier) Criteria() QualificationCriteria {
	if q == nil {
		return QualificationCriteria{}
	}
	return q.criteria
}

// Qualify evaluates the cluster against every criterion. The result
// is either PatternQualified (when every criterion is satisfied) or
// PatternObserved (when at least one criterion fails). The decision
// carries the human-readable reasons that drove the verdict so the
// CLI / MCP can surface them to the reviewer.
//
// The function is pure: it never mutates the cluster, never calls
// I/O and never reads from a clock. It honors ctx.Done() defensively.
func (q *ConservativeQualifier) Qualify(ctx context.Context, c ClusterRecord) (QualificationDecision, error) {
	if q == nil {
		return QualificationDecision{}, domain.NewValidationError(domain.ErrInvalidArgument,
			"patterns: qualifier is nil")
	}
	if err := ctx.Err(); err != nil {
		return QualificationDecision{}, err
	}

	reasons := []string{}

	// Membership floor.
	if c.OccurrenceCount < 2 || len(c.Members) < 2 {
		reasons = append(reasons, "fewer_than_two_members")
	}

	// Criterion F: anti-pattern from docs/23 §9. The cluster must
	// have MORE than 1 distinct session. Three retries in one
	// session are exactly what we want to reject.
	if c.DistinctSessions < 2 {
		reasons = append(reasons, "single_session_retries")
	}

	// Criterion A (with disjunction): ≥ 3 sessions OR ≥ 3 days.
	if !(c.DistinctSessions >= q.criteria.MinDistinctSessions || c.DistinctDays >= q.criteria.MinDistinctDays) {
		reasons = append(reasons, "insufficient_distinct_sessions_or_days")
	}

	// Criterion B: ≥ 2 successful outcomes OR repeated correction.
	if c.SuccessfulOutcomes < q.criteria.MinSuccessfulOccurrences && !c.RepeatedCorrection {
		reasons = append(reasons, "insufficient_successful_occurrences")
	}

	// Criterion C: posterior contradiction blocks qualification.
	if c.HasContradiction {
		reasons = append(reasons, "contradicted")
	}

	// Criterion D: kind filter.
	if c.Kind == domain.EventPreference || c.Kind == domain.EventUnknown {
		reasons = append(reasons, "fact_or_preference")
	}

	// Criterion E: covered by an existing Learning.
	if strings.TrimSpace(c.CoveredByLearningID) != "" {
		reasons = append(reasons, "already_covered_by_learning")
	}

	// Criterion G: ≥ 2 distinct sources. The DistinctSessions proxy
	// is the v1 source; the cluster Sessions map carries the same
	// information.
	if len(c.Sessions) < 2 {
		reasons = append(reasons, "fewer_than_two_distinct_sources")
	}

	// Criterion H: similarity must not depend on a single generic
	// word.
	if isGenericSingleWord(c.RetrievalTerms) {
		reasons = append(reasons, "generic_single_word_similarity")
	}

	if len(reasons) > 0 {
		return QualificationDecision{
			Cluster: clusterAdapter{c},
			Status:  PatternObserved,
			Reasons: reasons,
		}, nil
	}
	return QualificationDecision{
		Cluster: clusterAdapter{c},
		Status:  PatternQualified,
	}, nil
}

// isGenericSingleWord reports whether the retrieval-term set is a
// single generic word (criterion H).
func isGenericSingleWord(terms []string) bool {
	if len(terms) != 1 {
		return false
	}
	_, ok := genericSingleWord[strings.ToLower(strings.TrimSpace(terms[0]))]
	return ok
}

// clusterAdapter lets the pure ClusterRecord satisfy the Cluster
// interface exposed by the contract (patterns.go) without copying
// fields. It is internal: callers consume QualificationDecision, not
// the adapter directly.
type clusterAdapter struct {
	ClusterRecord
}

func (c clusterAdapter) ID() string            { return c.ClusterRecord.Fingerprint }
func (c clusterAdapter) Fingerprint() string   { return c.ClusterRecord.Fingerprint }
func (c clusterAdapter) Status() PatternStatus { return PatternObserved }

func (c clusterAdapter) Members() []domain.ExperienceEventID {
	out := make([]domain.ExperienceEventID, len(c.ClusterRecord.Members))
	copy(out, c.ClusterRecord.Members)
	return out
}

func (c clusterAdapter) DistinctSessions() int { return c.ClusterRecord.DistinctSessions }

func (c clusterAdapter) DistinctDays() int { return c.ClusterRecord.DistinctDays }

func (c clusterAdapter) OccurrenceCount() int { return c.ClusterRecord.OccurrenceCount }

func (c clusterAdapter) Kind() domain.ExperienceEventKind { return c.ClusterRecord.Kind }
