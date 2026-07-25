// Contract tests for the patterns package (Hito 6 slice 6.0).
//
// These tests live BEFORE the production types they exercise. In the RED
// phase the file must not compile (PatternStatus, DismissalReason,
// Membership, QualificationCriteria, ExperiencePattern, Pattern,
// Cluster, Qualifier, Dismissal, Lister, Getter, Config, and the
// constructors do not exist yet). In the GREEN phase patterns.go
// introduces the minimum surface required to make every test in this
// file pass with zero per-slice logic. Slice 6.0 ships only the
// contract; fingerprint, clustering, qualification and persistence
// arrive in slices 6.1–6.4.
//
// The contract verifies the structural invariants documented in
// docs/21-EXPERIENCE-DOMAIN.md §6 and docs/23-PATTERN-MINING.md §5–8:
//
//   - PatternStatus is a closed enum with the five canonical values.
//   - DismissalReason is a closed enum with the seven typed reasons.
//   - Membership carries similarity_kind/similarity_score as bounded
//     numeric values with stable JSON encoding.
//   - QualificationCriteria uses named/configurable conservative
//     defaults and records the policy version that produced it.
//   - ExperiencePattern exposes the documented domain fields.
//   - Pattern, Cluster, Qualifier, Dismissal, Lister, Getter have the
//     small surface that the slice contract promises, and each
//     reports errors via typed domain errors only.
//   - Config rejects thresholds out of the allowed range at
//     construction time so the orchestrator fails fast at startup.
//   - Required typed errors: pattern_not_found,
//     pattern_not_qualified, pattern_already_promoted,
//     pattern_false_cluster, pattern_insufficient_sources — each
//     maps to a stable domain error code.
//
// Slice 6.0 ships only the contract. Behavioural tests for fingerprint
// determinism, Jaccard clustering, qualification and dismissal land in
// slices 6.1–6.4 alongside their implementation files.

package patterns

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

// TestPatternStatus_Enum enumerates the five canonical PatternStatus
// values. Adding a new status requires editing this table and the
// companion type, so an accidental rename is caught at compile time
// before it reaches runtime.
func TestPatternStatus_Enum(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status PatternStatus
		want   string
	}{
		{"observed", PatternObserved, "observed"},
		{"qualified", PatternQualified, "qualified"},
		{"dismissed", PatternDismissed, "dismissed"},
		{"promoted", PatternPromoted, "promoted"},
		{"stale", PatternStale, "stale"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if string(tc.status) != tc.want {
				t.Fatalf("PatternStatus(%s) = %q, want %q", tc.name, string(tc.status), tc.want)
			}
		})
	}
}

// TestPatternStatus_StableJSON pins the JSON shape so external
// consumers (CLI, MCP, audit logs) can rely on it. The keys are
// pinned to camelCase by the package-level constant table.
func TestPatternStatus_StableJSON(t *testing.T) {
	t.Parallel()

	enc, err := json.Marshal(struct {
		Status PatternStatus `json:"status"`
	}{Status: PatternQualified})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"status":"qualified"}`
	if string(enc) != want {
		t.Fatalf("Marshal = %s, want %s", enc, want)
	}
}

// TestDismissalReason_Enum covers the seven typed reasons from
// docs/23-PATTERN-MINING.md §7. They are required so callers cannot
// silently invent reasons.
func TestDismissalReason_Enum(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		reason DismissalReason
		want   string
	}{
		{"one_off", DismissalOneOff, "one_off"},
		{"not_reusable", DismissalNotReusable, "not_reusable"},
		{"already_covered", DismissalAlreadyCovered, "already_covered"},
		{"contradicted", DismissalContradicted, "contradicted"},
		{"insufficient_evidence", DismissalInsufficientEvidence, "insufficient_evidence"},
		{"private_or_sensitive", DismissalPrivateOrSensitive, "private_or_sensitive"},
		{"false_cluster", DismissalFalseCluster, "false_cluster"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if string(tc.reason) != tc.want {
				t.Fatalf("DismissalReason(%s) = %q, want %q", tc.name, string(tc.reason), tc.want)
			}
		})
	}
}

// TestMembership_BoundedFields ensures the numeric similarity fields
// stay inside the documented [0, 1] range. Construction accepts any
// float, but Validation must reject values outside the interval and
// reject empty similarity_kind.
func TestMembership_BoundedFields(t *testing.T) {
	t.Parallel()

	m := Membership{
		EventID:         domain.ExperienceEventID("evt-1"),
		SimilarityKind:  "exact_fingerprint",
		SimilarityScore: 0.5,
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil for valid membership", err)
	}

	bad := []Membership{
		{EventID: domain.ExperienceEventID("evt-1"), SimilarityKind: "exact_fingerprint", SimilarityScore: -0.01},
		{EventID: domain.ExperienceEventID("evt-1"), SimilarityKind: "exact_fingerprint", SimilarityScore: 1.01},
		{EventID: domain.ExperienceEventID("evt-1"), SimilarityKind: "", SimilarityScore: 0.5},
		{EventID: "", SimilarityKind: "exact_fingerprint", SimilarityScore: 0.5},
	}
	for i, m := range bad {
		if err := m.Validate(); err == nil {
			t.Fatalf("Validate(membership %d) = nil, want error", i)
		}
	}
}

// TestMembership_StableJSON locks down the JSON shape so downstream
// tools (CLI, MCP) can parse it predictably. Numbers stay float64
// per docs/lessons.md (JSON round-trip int → float64).
func TestMembership_StableJSON(t *testing.T) {
	t.Parallel()

	enc, err := json.Marshal(Membership{
		EventID:         domain.ExperienceEventID("evt-1"),
		SimilarityKind:  "exact_fingerprint",
		SimilarityScore: 0.75,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var roundtrip map[string]any
	if err := json.Unmarshal(enc, &roundtrip); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if roundtrip["event_id"] != "evt-1" {
		t.Fatalf("event_id = %v, want evt-1", roundtrip["event_id"])
	}
	if roundtrip["similarity_kind"] != "exact_fingerprint" {
		t.Fatalf("similarity_kind = %v, want exact_fingerprint", roundtrip["similarity_kind"])
	}
	if v, ok := roundtrip["similarity_score"].(float64); !ok || v != 0.75 {
		t.Fatalf("similarity_score = %v (%T), want float64 0.75", roundtrip["similarity_score"], roundtrip["similarity_score"])
	}
}

// TestQualificationCriteria_Defaults pins the conservative defaults
// from docs/23-PATTERN-MINING.md §5. They are intentionally NAMED so
// the project can change them with one edit and one documentation
// note. The MaxClusterMembers default is documented as 100.
func TestQualificationCriteria_Defaults(t *testing.T) {
	t.Parallel()

	c := DefaultQualificationCriteria()
	if c.MinDistinctSessions != 3 {
		t.Fatalf("MinDistinctSessions = %d, want 3", c.MinDistinctSessions)
	}
	if c.MinDistinctDays != 2 {
		t.Fatalf("MinDistinctDays = %d, want 2", c.MinDistinctDays)
	}
	if c.MinSuccessfulOccurrences != 2 {
		t.Fatalf("MinSuccessfulOccurrences = %d, want 2", c.MinSuccessfulOccurrences)
	}
	if c.MaxClusterMembers != 100 {
		t.Fatalf("MaxClusterMembers = %d, want 100", c.MaxClusterMembers)
	}
	if c.PolicyVersion == "" {
		t.Fatal("PolicyVersion must be populated so audits can identify the rule set")
	}
	if c.QualificationMode != ModeConservative {
		t.Fatalf("QualificationMode = %q, want %q", c.QualificationMode, ModeConservative)
	}
}

// TestQualificationCriteria_Validate enforces the [min_sessions ≤
// max_cluster_members, etc.] invariants so a misconfigured caller
// fails fast. The contract rejects any non-positive threshold or any
// session-count below the day-count.
func TestQualificationCriteria_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*QualificationCriteria)
		wantErr bool
	}{
		{"valid", func(c *QualificationCriteria) {}, false},
		{"zero sessions", func(c *QualificationCriteria) { c.MinDistinctSessions = 0 }, true},
		{"negative days", func(c *QualificationCriteria) { c.MinDistinctDays = -1 }, true},
		{"zero successes", func(c *QualificationCriteria) { c.MinSuccessfulOccurrences = 0 }, true},
		{"empty policy version", func(c *QualificationCriteria) { c.PolicyVersion = "" }, true},
		{"empty qualification mode", func(c *QualificationCriteria) { c.QualificationMode = "" }, true},
		{"negative jaccard", func(c *QualificationCriteria) { c.MinRetrievalJaccard = -0.1 }, true},
		{"oversized jaccard", func(c *QualificationCriteria) { c.MinRetrievalJaccard = 1.01 }, true},
		{"zero max cluster members", func(c *QualificationCriteria) { c.MaxClusterMembers = 0 }, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := DefaultQualificationCriteria()
			tc.mutate(&c)
			err := c.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("Validate = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate = %v, want nil", err)
			}
		})
	}
}

// TestExperiencePattern_FieldSurface pins the field names exported by
// the ExperiencePattern struct. The handoff mandates that the
// canonical JSON schema stays stable across slices.
func TestExperiencePattern_FieldSurface(t *testing.T) {
	t.Parallel()

	p := ExperiencePattern{
		ID:               domain.ExperiencePatternID("pat-1"),
		ProjectID:        domain.ProjectID("proj-1"),
		Status:           PatternObserved,
		Kind:             domain.EventTestFailure,
		Fingerprint:      "fp",
		Title:            "title",
		Summary:          "summary",
		DistinctSessions: 3,
		DistinctDays:     2,
		OccurrenceCount:  5,
		FirstSeenAt:      time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		LastSeenAt:       time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		DetectorVersion:  "0.1.0",
		InputDigest:      "digest",
		CreatedAt:        time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		UpdatedAt:        time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	enc, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(enc)
	for _, key := range []string{
		`"id":"pat-1"`,
		`"project_id":"proj-1"`,
		`"status":"observed"`,
		`"kind":"test_failure"`,
		`"fingerprint":"fp"`,
		`"title":"title"`,
		`"summary":"summary"`,
		`"distinct_sessions":3`,
		`"distinct_days":2`,
		`"occurrence_count":5`,
		`"detector_version":"0.1.0"`,
		`"input_digest":"digest"`,
	} {
		if !strings.Contains(got, key) {
			t.Fatalf("Marshal missing %s in %s", key, got)
		}
	}
}

// TestConfig_Defaults exposes the conservative config defaults. The
// slice 6.0 contract freezes them so slices 6.1–6.4 can rely on the
// invariants without re-validating.
func TestConfig_Defaults(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultConfig().Validate = %v, want nil", err)
	}
	if cfg.MinRetrievalJaccard <= 0 || cfg.MinRetrievalJaccard > 1 {
		t.Fatalf("MinRetrievalJaccard = %f, want in (0,1]", cfg.MinRetrievalJaccard)
	}
	if cfg.MaxClusterMembers <= 0 {
		t.Fatalf("MaxClusterMembers = %d, want > 0", cfg.MaxClusterMembers)
	}
	if cfg.Qualification == (QualificationCriteria{}) {
		t.Fatal("DefaultConfig must populate Qualification")
	}
}

// TestConfig_Validate covers the boundaries the orchestrator depends
// on. Misconfigured thresholds are rejected at construction so the
// orchestrator cannot put the miner into a zombie state.
func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default Validate = %v, want nil", err)
	}

	bad := []struct {
		name   string
		mutate func(*Config)
	}{
		{"negative jaccard", func(c *Config) { c.MinRetrievalJaccard = -0.1 }},
		{"oversized jaccard", func(c *Config) { c.MinRetrievalJaccard = 1.01 }},
		{"zero max cluster members", func(c *Config) { c.MaxClusterMembers = 0 }},
		{"empty qualification", func(c *Config) { c.Qualification = QualificationCriteria{} }},
	}
	for _, tc := range bad {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := cfg
			tc.mutate(&c)
			if err := c.Validate(); err == nil {
				t.Fatalf("Validate(%s) = nil, want error", tc.name)
			}
		})
	}
}

// TestTypedErrors asserts that the documented typed errors are real
// values exposed by the package and that each carries its stable
// domain error code.
func TestTypedErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		code domain.ErrorCode
	}{
		{"pattern_not_found", ErrPatternNotFound, domain.ErrPatternNotFound},
		{"pattern_not_qualified", ErrPatternNotQualified, domain.ErrPatternNotQualified},
		{"pattern_already_promoted", ErrPatternAlreadyPromoted, domain.ErrPatternAlreadyPromoted},
		{"pattern_false_cluster", ErrPatternFalseCluster, domain.ErrPatternFalseCluster},
		{"pattern_insufficient_sources", ErrPatternInsufficientSources, domain.ErrPatternInsufficientSources},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.err == nil {
				t.Fatalf("%s is nil", tc.name)
			}
			domainErr, ok := domain.AsDomainError(tc.err)
			if !ok {
				t.Fatalf("%s is not a domain error", tc.name)
			}
			if domainErr.Code != tc.code {
				t.Fatalf("%s code = %q, want %q", tc.name, domainErr.Code, tc.code)
			}
		})
	}
}

// TestInterfaces_Compile guards the public surface of the package.
// Removing or renaming an interface or one of its methods breaks the
// build before any runtime test runs.
func TestInterfaces_Compile(t *testing.T) {
	t.Parallel()

	var _ Pattern = (*stubPattern)(nil)
	var _ Cluster = (*stubCluster)(nil)
	var _ Qualifier = (*stubQualifier)(nil)
	var _ Dismissal = (*stubDismissal)(nil)
	var _ Lister = (*stubLister)(nil)
	var _ Getter = (*stubGetter)(nil)
}

// --- Test stubs (consumed only by the compile guards above) ----

type stubPattern struct{}

func (stubPattern) ID() string                       { return "" }
func (stubPattern) Status() PatternStatus            { return PatternObserved }
func (stubPattern) Memberships() []Membership        { return nil }
func (stubPattern) Fingerprint() string              { return "" }
func (stubPattern) Kind() domain.ExperienceEventKind { return domain.EventUnknown }

type stubCluster struct{}

func (stubCluster) ID() string                          { return "" }
func (stubCluster) Fingerprint() string                 { return "" }
func (stubCluster) Status() PatternStatus               { return PatternObserved }
func (stubCluster) Members() []domain.ExperienceEventID { return nil }
func (stubCluster) DistinctSessions() int               { return 0 }
func (stubCluster) DistinctDays() int                   { return 0 }
func (stubCluster) OccurrenceCount() int                { return 0 }
func (stubCluster) Kind() domain.ExperienceEventKind    { return domain.EventUnknown }

type stubQualifier struct{}

func (stubQualifier) Qualify(_ context.Context, _ Cluster) (QualificationDecision, error) {
	return QualificationDecision{}, errors.New("not implemented")
}

type stubDismissal struct{}

func (stubDismissal) Dismiss(_ context.Context, _ domain.ExperiencePatternID, _ DismissalReason, _ DismissalDetails) error {
	return errors.New("not implemented")
}

type stubLister struct{}

func (stubLister) List(_ context.Context, _ ListerFilter) ([]ExperiencePattern, error) {
	return nil, errors.New("not implemented")
}

type stubGetter struct{}

func (stubGetter) Get(_ context.Context, _ domain.ExperiencePatternID) (*ExperiencePattern, error) {
	return nil, ErrPatternNotFound
}
