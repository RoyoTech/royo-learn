package patterns_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/patterns"
)

func TestConfigValidationReturnsTypedInvalidConfig(t *testing.T) {
	bad := patterns.DefaultConfig()
	bad.MaxClusterMembers = 0
	err := bad.Validate()
	if domainErr, ok := domain.AsDomainError(err); !ok || domainErr.Code != domain.ErrInvalidConfig {
		t.Fatalf("Config.Validate() = %v, want typed %s", err, domain.ErrInvalidConfig)
	}

	criteria := patterns.DefaultQualificationCriteria()
	criteria.MinDistinctSessions = 0
	if _, err := patterns.NewQualifier(criteria); func() bool {
		domainErr, ok := domain.AsDomainError(err)
		return !ok || domainErr.Code != domain.ErrInvalidConfig
	}() {
		t.Fatalf("NewQualifier() = %v, want typed %s", err, domain.ErrInvalidConfig)
	}
}

func TestPatternFingerprintStripsVolatileToolValues(t *testing.T) {
	base := patterns.PatternInput{Kind: "test_failure", Tool: `go test --token=alpha --port=8080 C:/Users/alice/repo`, Result: "failed", RetrievalTerms: []string{"go", "test"}}
	changed := base
	changed.Tool = `go test --token=beta --port=9090 C:/Users/bob/repo`
	if got, want := patterns.PatternFingerprint(base), patterns.PatternFingerprint(changed); got != want {
		t.Fatalf("fingerprints differ after only volatile/secret tool values changed: %s != %s", got, want)
	}
}

func TestGroupDeterministicUnderPermutationIncludingCap(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	candidates := []patterns.PatternCandidate{
		{EventID: "e3", Kind: domain.EventTestFailure, Fingerprint: "fp-b", RetrievalTerms: []string{"compile", "go"}, SessionID: "s3", OccurredAt: now.Add(2 * time.Hour)},
		{EventID: "e1", Kind: domain.EventTestFailure, Fingerprint: "fp-a", RetrievalTerms: []string{"compile", "go"}, SessionID: "s1", OccurredAt: now},
		{EventID: "e4", Kind: domain.EventTestFailure, Fingerprint: "fp-b", RetrievalTerms: []string{"compile", "go"}, SessionID: "s4", OccurredAt: now.Add(3 * time.Hour)},
		{EventID: "e2", Kind: domain.EventTestFailure, Fingerprint: "fp-a", RetrievalTerms: []string{"compile", "go"}, SessionID: "s2", OccurredAt: now.Add(time.Hour)},
	}
	permuted := []patterns.PatternCandidate{candidates[3], candidates[2], candidates[1], candidates[0]}
	cfg := patterns.DefaultConfig()
	cfg.MaxClusterMembers = 3
	cfg.Qualification.MaxClusterMembers = 3
	if got, want := patterns.Group(candidates, cfg), patterns.Group(permuted, cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("Group output depends on candidate order:\nfirst=%#v\nsecond=%#v", got, want)
	}
}

func TestIngestClusterRollsBackPatternWhenMemberIsUntraceable(t *testing.T) {
	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "atomic-ingest")
	svc := patterns.NewService(db)
	cluster := patterns.ClusterRecord{
		Fingerprint: "fp-atomic", Kind: domain.EventTestFailure,
		Members:  []domain.ExperienceEventID{"missing-event"},
		Sessions: map[string]struct{}{"s1": {}}, Days: map[string]struct{}{"2026-07-25": {}},
		DistinctSessions: 1, DistinctDays: 1, OccurrenceCount: 1,
		FirstSeenAt: time.Now().UTC(), LastSeenAt: time.Now().UTC(),
	}
	if _, err := svc.IngestCluster(context.Background(), fixture.ProjectID, cluster, patterns.QualificationDecision{}); err == nil {
		t.Fatal("IngestCluster() = nil error, want untraceable member failure")
	}
	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM experience_patterns WHERE project_id = ? AND fingerprint = ?`, fixture.ProjectID, cluster.Fingerprint).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("pattern rows after failed member insert = %d, want 0", count)
	}
}

func TestDismissedPatternDoesNotReopenForIdenticalEvidence(t *testing.T) {
	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "dismiss-suppress")
	svc := patterns.NewService(db)
	saved := dismissalFixture(t, svc, fixture.ProjectID, "fp-dismiss-suppress")
	if err := svc.Dismiss(context.Background(), saved.ID, patterns.DismissalNotReusable, patterns.DismissalDetails{}); err != nil {
		t.Fatal(err)
	}
	members, err := patterns.NewRepository(db).Members(context.Background(), saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]domain.ExperienceEventID, len(members))
	for i := range members {
		ids[i] = members[i].EventID
	}
	cluster := patterns.ClusterRecord{
		Fingerprint: saved.Fingerprint, Kind: saved.Kind, Members: ids,
		Sessions: map[string]struct{}{"s1": {}, "s2": {}, "s3": {}}, Days: map[string]struct{}{"d1": {}, "d2": {}},
		DistinctSessions: 3, DistinctDays: 2, OccurrenceCount: len(ids),
		FirstSeenAt: saved.FirstSeenAt, LastSeenAt: saved.LastSeenAt,
	}
	got, err := svc.IngestCluster(context.Background(), fixture.ProjectID, cluster, patterns.QualificationDecision{Status: patterns.PatternQualified})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != patterns.PatternDismissed || got.DismissalReason != patterns.DismissalNotReusable {
		t.Fatalf("identical evidence reopened dismissed pattern: status=%s reason=%s", got.Status, got.DismissalReason)
	}
}

func TestDismissedPatternReopensOnlyWithNewQualifiedEvidence(t *testing.T) {
	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "dismiss-reopen")
	svc := patterns.NewService(db)
	saved := dismissalFixture(t, svc, fixture.ProjectID, "fp-dismiss-reopen")
	if err := svc.Dismiss(context.Background(), saved.ID, patterns.DismissalInsufficientEvidence, patterns.DismissalDetails{}); err != nil {
		t.Fatal(err)
	}
	members, err := patterns.NewRepository(db).Members(context.Background(), saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	newID := domain.ExperienceEventID("evt-new-qualified")
	seedExperienceEvents(t, db, fixture.ProjectID, []domain.ExperienceEventID{newID})
	ids := []domain.ExperienceEventID{members[0].EventID, newID}
	cluster := patterns.ClusterRecord{
		Fingerprint: saved.Fingerprint, Kind: saved.Kind, Members: ids,
		Sessions: map[string]struct{}{"s1": {}, "s2": {}, "s3": {}}, Days: map[string]struct{}{"d1": {}, "d2": {}},
		DistinctSessions: 3, DistinctDays: 2, OccurrenceCount: len(ids),
		FirstSeenAt: saved.FirstSeenAt, LastSeenAt: saved.LastSeenAt.Add(time.Hour),
	}
	got, err := svc.IngestCluster(context.Background(), fixture.ProjectID, cluster, patterns.QualificationDecision{Status: patterns.PatternQualified})
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := svc.Get(context.Background(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != patterns.PatternQualified || persisted.DismissalReason != "" {
		t.Fatalf("new qualified evidence did not cleanly reopen: status=%s reason=%s", persisted.Status, persisted.DismissalReason)
	}
}

func TestInputDigestIsIndependentOfClusterSliceOrder(t *testing.T) {
	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "digest-order")
	svc := patterns.NewService(db)
	ids := []domain.ExperienceEventID{"evt-digest-a", "evt-digest-b"}
	seedExperienceEvents(t, db, fixture.ProjectID, ids)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	base := patterns.ClusterRecord{
		Fingerprint: "fp-digest-order", Kind: domain.EventTestFailure, Members: ids,
		Sessions: map[string]struct{}{"s1": {}, "s2": {}}, Days: map[string]struct{}{"d1": {}, "d2": {}},
		DistinctSessions: 2, DistinctDays: 2, OccurrenceCount: 2,
		FirstSeenAt: now, LastSeenAt: now, RetrievalTerms: []string{"go", "compile"},
		SourceFingerprints: []string{"source-b", "source-a"},
	}
	first, err := svc.IngestCluster(context.Background(), fixture.ProjectID, base, patterns.QualificationDecision{})
	if err != nil {
		t.Fatal(err)
	}
	base.Members = []domain.ExperienceEventID{ids[1], ids[0]}
	base.RetrievalTerms = []string{"compile", "go"}
	base.SourceFingerprints = []string{"source-a", "source-b"}
	second, err := svc.IngestCluster(context.Background(), fixture.ProjectID, base, patterns.QualificationDecision{})
	if err != nil {
		t.Fatal(err)
	}
	if first.InputDigest != second.InputDigest {
		t.Fatalf("input digest depends on slice order: %s != %s", first.InputDigest, second.InputDigest)
	}
}

func TestDismissalThroughRawRepositoryStillWritesAudit(t *testing.T) {
	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "raw-audit")
	seedExperienceEvents(t, db, fixture.ProjectID, []domain.ExperienceEventID{"evt-raw-audit"})
	svc := patterns.NewServiceFromRaw(db.DB)
	now := time.Now().UTC()
	saved, err := svc.IngestCluster(context.Background(), fixture.ProjectID, patterns.ClusterRecord{
		Fingerprint: "fp-raw-audit", Kind: domain.EventTestFailure, Members: []domain.ExperienceEventID{"evt-raw-audit"},
		Sessions: map[string]struct{}{"s1": {}}, Days: map[string]struct{}{"d1": {}}, DistinctSessions: 1, DistinctDays: 1, OccurrenceCount: 1,
		FirstSeenAt: now, LastSeenAt: now,
	}, patterns.QualificationDecision{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Dismiss(context.Background(), saved.ID, patterns.DismissalOneOff, patterns.DismissalDetails{Actor: domain.Actor{Kind: "user", Name: "tester"}}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = 'experience_pattern_dismissed' AND entity_id = ?`, saved.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("raw dismissal audit rows = %d, want 1", count)
	}
}

func TestGroupTalliesSuccessfulOutcomesForQualification(t *testing.T) {
	now := time.Now().UTC()
	cluster := patterns.Group([]patterns.PatternCandidate{
		{EventID: "e1", Kind: domain.EventTestFailure, Fingerprint: "fp", Result: "success", RetrievalTerms: []string{"compile"}, SessionID: "s1", OccurredAt: now},
		{EventID: "e2", Kind: domain.EventTestFailure, Fingerprint: "fp", Result: "corrected", RetrievalTerms: []string{"compile"}, SessionID: "s2", OccurredAt: now},
		{EventID: "e3", Kind: domain.EventTestFailure, Fingerprint: "fp", Result: "failed", RetrievalTerms: []string{"compile"}, SessionID: "s3", OccurredAt: now},
	}, patterns.DefaultConfig())
	if len(cluster) != 1 || cluster[0].SuccessfulOutcomes != 2 {
		t.Fatalf("successful outcomes = %#v, want one cluster with 2", cluster)
	}
}
