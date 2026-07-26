// Package patterns implements the pattern-mining layer of the
// experience-discovery pipeline (Hito 6). Patterns group recurring
// ExperienceEvents into auditable candidates that can be qualified or
// dismissed by a reviewer; promotion remains out of scope here and
// ships in Hito 7 via capture.Service.
//
// Slices 6.0–6.4:
//
//   - 6.0 ships the contract: status enums, dismissal reasons,
//     Membership validation, QualificationCriteria defaults, Config
//     defaults, typed errors and the public interfaces (Pattern,
//     Cluster, Qualifier, Dismissal, Lister, Getter). No mining
//     logic yet.
//   - 6.1 adds the deterministic pattern fingerprint and the
//     retrieval-term normalizer.
//   - 6.2 adds the pure v1 clustering algorithm (exact fingerprint +
//     conservative Jaccard over retrieval terms).
//   - 6.3 adds the qualification rules and the anti-pattern
//     detectors (3 retries in one session, false cluster, etc.).
//   - 6.4 adds the SQLite migration 005, the repository/service
//     surface (Dismiss, List, Get), the CLI and MCP tools, and the
//     synthetic-fixture acceptance test.
//
// The package never writes a Learning. Promotion is the bridge, not a
// side effect. See docs/23-PATTERN-MINING.md and
// docs/24-EXPERIENCE-THREAT-MODEL.md.

package patterns

import (
	"context"
	"errors"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/evidence"
)

// PatternStatus is the closed enum that governs the lifecycle of an
// ExperiencePattern. See docs/21-EXPERIENCE-DOMAIN.md §6.
type PatternStatus string

const (
	PatternObserved  PatternStatus = "observed"
	PatternQualified PatternStatus = "qualified"
	PatternDismissed PatternStatus = "dismissed"
	PatternPromoted  PatternStatus = "promoted"
	PatternStale     PatternStatus = "stale"
)

// ToPromotionFields converts an ExperiencePattern into the structural
// PromotionFields bag the promotion pipeline redacts and hashes
// before persisting a Learning. The mapping is deterministic so the
// same pattern always produces the same redacted bag and therefore
// the same promotion fingerprint:
//
//	Title          <- pattern.Title
//	Context        <- pattern.Summary
//	Observation    <- "<pattern_kind>:<fingerprint_prefix_16>"
//	ReusableLesson <- ""
//	Limits         <- ""
//	Recommended    <- nil
//	RetrievalTerms <- []string{string(pattern.Kind), pattern.Fingerprint}
//
// The Observation is derived rather than copied verbatim so a secret
// that happens to live in the raw pattern title cannot leak through
// the persisted Learning; the prefix length (16) matches the
// safePrefix helper the patterns.Service.IngestCluster uses so the
// two derivations stay aligned.
func (p ExperiencePattern) ToPromotionFields() evidence.PromotionFields {
	fingerprintPrefix := safePrefix(p.Fingerprint, 16)
	return evidence.PromotionFields{
		Title:          p.Title,
		Context:        p.Summary,
		Observation:    string(p.Kind) + ":" + fingerprintPrefix,
		ReusableLesson: "",
		Limits:         "",
		Recommended:    nil,
		RetrievalTerms: []string{string(p.Kind), p.Fingerprint},
	}
}

// DismissalReason is the closed enum of typed reasons used by
// Dismissal. Adding a new reason requires editing this table and
// docs/23-PATTERN-MINING.md §7.
type DismissalReason string

const (
	DismissalOneOff               DismissalReason = "one_off"
	DismissalNotReusable          DismissalReason = "not_reusable"
	DismissalAlreadyCovered       DismissalReason = "already_covered"
	DismissalContradicted         DismissalReason = "contradicted"
	DismissalInsufficientEvidence DismissalReason = "insufficient_evidence"
	DismissalPrivateOrSensitive   DismissalReason = "private_or_sensitive"
	DismissalFalseCluster         DismissalReason = "false_cluster"
)

// QualificationMode is the named policy that governs conservative
// defaults. v1 ships only the conservative mode; permissive and
// exploratory modes are explicitly out of scope.
type QualificationMode string

const (
	ModeConservative QualificationMode = "conservative"
)

// PolicyVersion is the audit-bound identity of the qualification
// rules in use. Bumping it forces downstream consumers to re-evaluate
// the qualified patterns, mirroring the detector Version() contract
// from internal/experience/detectors/detectors.go.
const PolicyVersion = "v1.0.0"

// Membership captures the relationship between a Pattern and one of
// its member events. SimilarityKind is a stable identifier ("exact_fingerprint",
// "jaccard_retrieval_terms", etc.); SimilarityScore is a float in
// [0, 1] where 1.0 means "identical" by that metric.
type Membership struct {
	EventID         domain.ExperienceEventID `json:"event_id"`
	SimilarityKind  string                   `json:"similarity_kind"`
	SimilarityScore float64                  `json:"similarity_score"`
	AddedAt         time.Time                `json:"added_at"`
}

// Validate enforces the documented invariant: similarity score must
// be inside [0, 1], event id must be present, similarity kind must be
// non-empty.
func (m Membership) Validate() error {
	if m.EventID == "" {
		return fmt.Errorf("patterns: membership event id is required")
	}
	if m.SimilarityKind == "" {
		return fmt.Errorf("patterns: membership similarity kind is required")
	}
	if m.SimilarityScore < 0 || m.SimilarityScore > 1 {
		return fmt.Errorf("patterns: membership similarity score %v is outside [0,1]", m.SimilarityScore)
	}
	return nil
}

// QualificationCriteria captures the conservative thresholds the
// Qualifier uses. The defaults mirror docs/23-PATTERN-MINING.md §5
// (≥ 3 sessions OR ≥ 3 days; ≥ 2 successful occurrences; etc.). The
// field set is intentionally small and named; nothing here is a
// silent magic number.
type QualificationCriteria struct {
	MinDistinctSessions      int               `json:"min_distinct_sessions"`
	MinDistinctDays          int               `json:"min_distinct_days"`
	MinSuccessfulOccurrences int               `json:"min_successful_occurrences"`
	MinRetrievalJaccard      float64           `json:"min_retrieval_jaccard"`
	MaxClusterMembers        int               `json:"max_cluster_members"`
	QualificationMode        QualificationMode `json:"qualification_mode"`
	PolicyVersion            string            `json:"policy_version"`
}

// DefaultQualificationCriteria returns the conservative defaults from
// docs/23 §5. MaxClusterMembers is the documented 100-cap so a single
// cluster cannot starve review attention.
func DefaultQualificationCriteria() QualificationCriteria {
	return QualificationCriteria{
		MinDistinctSessions:      3,
		MinDistinctDays:          2,
		MinSuccessfulOccurrences: 2,
		MinRetrievalJaccard:      0.5,
		MaxClusterMembers:        100,
		QualificationMode:        ModeConservative,
		PolicyVersion:            PolicyVersion,
	}
}

// Validate enforces the documented invariants: every threshold
// non-zero/non-negative, retrieval Jaccard inside (0, 1], max
// cluster size positive, policy version and qualification mode
// populated. Failing fast at construction matches the detector
// constructor convention from internal/experience/detectors/retry.go.
func (c QualificationCriteria) Validate() error {
	if c.MinDistinctSessions <= 0 {
		return domain.NewValidationError(domain.ErrInvalidConfig, fmt.Sprintf("patterns: min_distinct_sessions must be > 0, got %d", c.MinDistinctSessions))
	}
	if c.MinDistinctDays < 0 {
		return domain.NewValidationError(domain.ErrInvalidConfig, fmt.Sprintf("patterns: min_distinct_days must be >= 0, got %d", c.MinDistinctDays))
	}
	if c.MinSuccessfulOccurrences <= 0 {
		return domain.NewValidationError(domain.ErrInvalidConfig, fmt.Sprintf("patterns: min_successful_occurrences must be > 0, got %d", c.MinSuccessfulOccurrences))
	}
	if c.MinRetrievalJaccard <= 0 || c.MinRetrievalJaccard > 1 {
		return domain.NewValidationError(domain.ErrInvalidConfig, fmt.Sprintf("patterns: min_retrieval_jaccard must be in (0,1], got %v", c.MinRetrievalJaccard))
	}
	if c.MaxClusterMembers <= 0 {
		return domain.NewValidationError(domain.ErrInvalidConfig, fmt.Sprintf("patterns: max_cluster_members must be > 0, got %d", c.MaxClusterMembers))
	}
	if c.QualificationMode == "" {
		return domain.NewValidationError(domain.ErrInvalidConfig, "patterns: qualification_mode is required")
	}
	if c.PolicyVersion == "" {
		return domain.NewValidationError(domain.ErrInvalidConfig, "patterns: policy_version is required")
	}
	return nil
}

// Config bundles the low-risk knobs that govern clustering and
// qualification. Higher-risk knobs (trusted roots, endpoints,
// credentials) are intentionally absent — those live in the user
// configuration, never in the miner. See docs/24-EXPERIENCE-THREAT-MODEL.md §5.
type Config struct {
	// MinRetrievalJaccard is the conservative Jaccard threshold used
	// when clustering retrieval terms. The default (0.5) is documented
	// as the v1 conservative choice; it is a NAMED, configurable
	// default so the project can change it with a single edit and a
	// documentation note (per parent task brief). It is fully
	// reversible: a tighter threshold produces smaller, purer clusters.
	MinRetrievalJaccard float64 `json:"min_retrieval_jaccard"`

	// MaxClusterMembers caps the size of a single cluster to keep
	// review attention focused (docs/23 §5 default 100). It is
	// configurable and documented; the slice 6.4 acceptance test uses
	// the default. Clusters beyond the cap are split deterministically
	// (slice 6.2 owns the splitting rule).
	MaxClusterMembers int `json:"max_cluster_members"`

	// Qualification mirrors the criteria used by the Qualifier. It is
	// duplicated here (not aliased) so a future miner revision can
	// decouple clustering thresholds from qualification thresholds
	// without breaking the field shape.
	Qualification QualificationCriteria `json:"qualification"`
}

// DefaultConfig returns the conservative default configuration used
// when the operator does not override anything. MinRetrievalJaccard
// is 0.5 (named default; reversible per slice 6.2 docs). MaxClusterMembers
// is 100 (per docs/23 §5).
func DefaultConfig() Config {
	return Config{
		MinRetrievalJaccard: 0.5,
		MaxClusterMembers:   100,
		Qualification:       DefaultQualificationCriteria(),
	}
}

// Validate enforces the documented invariants so the orchestrator
// fails fast at startup rather than at the first observation.
func (c Config) Validate() error {
	if c.MinRetrievalJaccard <= 0 || c.MinRetrievalJaccard > 1 {
		return domain.NewValidationError(domain.ErrInvalidConfig, fmt.Sprintf("patterns: config min_retrieval_jaccard must be in (0,1], got %v", c.MinRetrievalJaccard))
	}
	if c.MaxClusterMembers <= 0 {
		return domain.NewValidationError(domain.ErrInvalidConfig, fmt.Sprintf("patterns: config max_cluster_members must be > 0, got %d", c.MaxClusterMembers))
	}
	if err := c.Qualification.Validate(); err != nil {
		return err
	}
	return nil
}

// ExperiencePattern is the persistent pattern entity. It mirrors the
// domain type declared in docs/21-EXPERIENCE-DOMAIN.md §6. The slice
// 6.0 contract pins the field set; slice 6.4 owns the persistence
// surface.
type ExperiencePattern struct {
	ID                 domain.ExperiencePatternID `json:"id"`
	ProjectID          domain.ProjectID           `json:"project_id"`
	Status             PatternStatus              `json:"status"`
	Kind               domain.ExperienceEventKind `json:"kind"`
	Fingerprint        string                     `json:"fingerprint"`
	Title              string                     `json:"title"`
	Summary            string                     `json:"summary"`
	DistinctSessions   int                        `json:"distinct_sessions"`
	DistinctDays       int                        `json:"distinct_days"`
	OccurrenceCount    int                        `json:"occurrence_count"`
	FirstSeenAt        time.Time                  `json:"first_seen_at"`
	LastSeenAt         time.Time                  `json:"last_seen_at"`
	ProposedLearningID *domain.LearningID         `json:"proposed_learning_id,omitempty"`
	DetectorVersion    string                     `json:"detector_version"`
	InputDigest        string                     `json:"input_digest"`
	CreatedAt          time.Time                  `json:"created_at"`
	UpdatedAt          time.Time                  `json:"updated_at"`
	// Revision is the optimistic-locking counter the repository
	// maintains. The v1 algorithm bumps Revision on every Status
	// transition; future migrations can add columns without touching
	// this field.
	Revision int `json:"revision"`
	// DismissalReason records the typed reason the operator used the
	// last time the pattern was dismissed. Empty when the pattern is
	// not in the dismissed state. The Dismiss service is idempotent
	// on (pattern_id, reason): re-dismissing with the same reason is a
	// no-op, while a different reason is rejected with
	// ErrPatternInsufficientSources.
	DismissalReason DismissalReason `json:"dismissal_reason,omitempty"`
}

// Pattern is the read-only view the package exposes to other layers
// (CLI, MCP). It is intentionally small; mutation lives behind the
// Service and repository surfaces (slice 6.4).
type Pattern interface {
	ID() string
	Status() PatternStatus
	Memberships() []Membership
	Fingerprint() string
	Kind() domain.ExperienceEventKind
}

// Cluster is the in-memory grouping produced by the clustering
// algorithm (slice 6.2). It is NOT a domain entity; it is the input
// to the Qualifier. The fingerprint is the cluster's identity; two
// clusters share an id only when their fingerprints match exactly.
type Cluster interface {
	ID() string
	Fingerprint() string
	Status() PatternStatus
	Members() []domain.ExperienceEventID
	DistinctSessions() int
	DistinctDays() int
	OccurrenceCount() int
	Kind() domain.ExperienceEventKind
}

// QualificationDecision is the result of Qualifier.Qualify. It is a
// pure value: status, the cluster it refers to, and the reasons that
// drove the decision. The Qualifier must NOT mutate the cluster.
type QualificationDecision struct {
	Cluster Cluster       `json:"-"`
	Status  PatternStatus `json:"status"`
	Reasons []string      `json:"reasons,omitempty"`
}

// Qualifier evaluates a cluster against QualificationCriteria and
// returns the decision. Slice 6.3 implements the conservative
// ruleset; slice 6.0 ships the contract only.
type Qualifier interface {
	Qualify(ctx context.Context, cluster Cluster) (QualificationDecision, error)
}

// DismissalDetails carries the structured metadata required to make
// a dismissal auditable. Reason is the typed enum; Note is the
// optional reviewer comment (bounded to MaxDismissalNoteBytes to
// keep it audit-safe).
type DismissalDetails struct {
	Reason DismissalReason `json:"reason"`
	Note   string          `json:"note,omitempty"`
	Actor  domain.Actor    `json:"actor"`
}

// MaxDismissalNoteBytes bounds the reviewer note so the dismissal
// trail cannot grow unbounded through the audit sink.
const MaxDismissalNoteBytes = 1024

// Dismissal marks a Pattern as Dismissed with a typed reason. The
// call must be idempotent: dismissing the same pattern twice with the
// same reason returns nil and records no extra audit event. The
// rationale lives in docs/23-PATTERN-MINING.md §7 ("Un patrón
// descartado no reaparece con los mismos miembros y detector_version
// salvo evidencia nueva suficiente.").
type Dismissal interface {
	Dismiss(ctx context.Context, patternID domain.ExperiencePatternID, reason DismissalReason, details DismissalDetails) error
}

// ListerFilter narrows the result set of Lister.List. Status is the
// required projection; Kind and Project narrow further. Limit is
// optional (zero means "all").
type ListerFilter struct {
	Status  PatternStatus
	Kind    domain.ExperienceEventKind
	Project domain.ProjectID
	Limit   int
}

// Lister returns the patterns that match the supplied filter, in
// stable order (last_seen_at DESC, then id ASC).
type Lister interface {
	List(ctx context.Context, filter ListerFilter) ([]ExperiencePattern, error)
}

// Getter returns a single pattern by id, or ErrPatternNotFound when
// none exists.
type Getter interface {
	Get(ctx context.Context, id domain.ExperiencePatternID) (*ExperiencePattern, error)
}

// --- Typed errors -------------------------------------------------
//
// Per docs/23 §8 the package surfaces these typed errors. They are
// exposed as package-level variables so callers can compare with
// errors.Is; they also carry the canonical domain code so the CLI
// and MCP layers can render stable error envelopes without
// re-classifying them.

var (
	// ErrPatternNotFound is returned when Getter.Get receives an id
	// that does not match any persisted pattern.
	ErrPatternNotFound = domain.NewNotFoundError(domain.ErrPatternNotFound, "experience pattern")

	// ErrPatternNotQualified is returned when an action (currently:
	// dismissal by dismiss-on-not-qualified workflows) requires the
	// pattern to be qualified and the current status forbids it.
	ErrPatternNotQualified = domain.NewValidationError(domain.ErrPatternNotQualified,
		"experience pattern is not qualified")

	// ErrPatternAlreadyPromoted is returned when an action requires
	// the pattern to be dismissable but the current status is already
	// promoted. Promotion is the only path that overrides dismissal;
	// once a pattern has been promoted it must not be dismissed.
	ErrPatternAlreadyPromoted = domain.NewConflictError(domain.ErrPatternAlreadyPromoted,
		"experience pattern is already promoted and cannot be dismissed")

	// ErrPatternFalseCluster is returned when the operator attempts
	// to dismiss with reason=false_cluster on a cluster that the
	// stored metrics describe as non-false (e.g. ≥ 3 sessions).
	ErrPatternFalseCluster = domain.NewConflictError(domain.ErrPatternFalseCluster,
		"experience pattern cluster is not flagged as false")

	// ErrPatternInsufficientSources is returned when the operator
	// attempts an action that requires traceable member sources but
	// the pattern has fewer than the documented minimum (≤ 2 distinct
	// sources).
	ErrPatternInsufficientSources = domain.NewValidationError(domain.ErrPatternInsufficientSources,
		"experience pattern has fewer than the required distinct sources")
)

// ErrorIs exposes the package-level typed errors so callers and
// tests can compare with errors.Is without depending on the
// variable identity. Kept as a function (not a constant) so adding
// new errors in slices 6.1–6.4 only requires extending this single
// source of truth.
func ErrorIs(err, target error) bool {
	return errors.Is(err, target)
}
