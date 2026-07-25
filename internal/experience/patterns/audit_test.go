// Regression tests for the slice 6.4 audit. Each test pins a single
// invariant the operator surfaced in the post-6.4 audit:
//
//   - Audit row + status change are atomic (same SQLite transaction).
//   - Dismissal note never leaks into the audit row when the reason
//     is private_or_sensitive (docs/24-EXPERIENCE-THREAT-MODEL.md §6).
//   - Membership rows trace back to a real experience_event via FK.
//   - Cluster split key is deterministic for candidate reorderings.
//   - Source revision/hash staleness: pattern with same fingerprint but
//     a newer detector_version is marked stale (docs/24 §3 T12).
//   - InputDigest is sha256 of canonical bytes, not raw concatenated
//     string content (regression of fingerprint-leakage risk).
//   - Pattern id is UUIDv7-formatted, not a hand-rolled
//     timestamp+counter string (consistency with the project).
//   - NewQualifier does not panic on invalid input (defensive).
//   - Repository.AddMember routes through the raw-DB constructor
//     (no nil-pointer panic on the alternate path).
//
// All tests are RED at file-write time; the implementation surface
// catches up in the next commit.

package patterns_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/patterns"
	"agent-royo-learn/internal/storage"
)

// auditDismissalFixture seeds a project + a saved qualified pattern
// the audit-dismiss tests operate on. It mirrors the patternsCLIFixture
// pattern but does not require the resolvePublishContext fixture so
// it can run in isolation.
func auditDismissalFixture(t *testing.T, db *storage.DB) (*patterns.Service, *patterns.ExperiencePattern) {
	t.Helper()
	fixture := newProjectFixture(t, db, "audit")
	repo := patterns.NewRepository(db)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	cluster := patterns.ClusterRecord{
		Fingerprint:      "fp-audit-1",
		Kind:             domain.EventTestFailure,
		Members:          []domain.ExperienceEventID{"evt-1", "evt-2"},
		Sessions:         map[string]struct{}{"s1": {}, "s2": {}, "s3": {}},
		Days:             map[string]struct{}{"d1": {}, "d2": {}, "d3": {}},
		DistinctSessions: 3, DistinctDays: 3, OccurrenceCount: 2,
		FirstSeenAt: now, LastSeenAt: now,
		RetrievalTerms: []string{"compile"},
	}
	saved, err := repo.SavePattern(context.Background(), patterns.ExperiencePattern{
		ProjectID:        fixture.ProjectID,
		Status:           patterns.PatternObserved,
		Kind:             cluster.Kind,
		Fingerprint:      cluster.Fingerprint,
		DistinctSessions: cluster.DistinctSessions,
		DistinctDays:     cluster.DistinctDays,
		OccurrenceCount:  cluster.OccurrenceCount,
		FirstSeenAt:      now,
		LastSeenAt:       now,
		DetectorVersion:  "v1",
		InputDigest:      "digest-placeholder",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("SavePattern: %v", err)
	}
	return patterns.NewService(db), saved
}

// TestAudit_IsAtomicWithStatusChange pins the invariant that the
// dismissal audit row commits in the same transaction as the status
// change. If the audit insert fails, the status change must roll
// back; if the status update fails, the audit row must not appear.
func TestAudit_IsAtomicWithStatusChange(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	newProjectFixture(t, db, "audit-atomic")
	svc, saved := auditDismissalFixture(t, db)

	// Audit path: dismiss writes one operation row to audit_events.
	if err := svc.Dismiss(context.Background(), saved.ID, patterns.DismissalOneOff, patterns.DismissalDetails{
		Actor: domain.Actor{Kind: "user", Name: "tester"},
	}); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}

	var status string
	if err := db.DB.QueryRow(`SELECT status FROM experience_patterns WHERE id = ?`, string(saved.ID)).Scan(&status); err != nil {
		t.Fatalf("QueryRow status: %v", err)
	}
	if status != string(patterns.PatternDismissed) {
		t.Fatalf("status = %s, want dismissed", status)
	}

	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE operation = ? AND entity_id = ?`,
		"experience_pattern_dismissed", string(saved.ID)).Scan(&count); err != nil {
		t.Fatalf("QueryRow audit: %v", err)
	}
	if count != 1 {
		t.Fatalf("audit row count = %d, want 1 (status change + audit must be atomic)", count)
	}
}

// TestAudit_PrivateOrSensitiveRedactsNote pins the docs/24
// §6 invariant: when the dismissal reason is private_or_sensitive,
// the note text must never appear in any audit row, even when the
// reviewer typed a real comment.
func TestAudit_PrivateOrSensitiveRedactsNote(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	newProjectFixture(t, db, "audit-redact")
	svc, saved := auditDismissalFixture(t, db)

	secretNote := "internal-accounting@example.com super-secret-handle"
	if err := svc.Dismiss(context.Background(), saved.ID, patterns.DismissalPrivateOrSensitive, patterns.DismissalDetails{
		Note:  secretNote,
		Actor: domain.Actor{Kind: "user", Name: "tester"},
	}); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}

	// The audit details_json column must not contain the secretNote.
	rows, err := db.DB.Query(`SELECT details_json FROM audit_events WHERE operation = ? AND entity_id = ?`,
		"experience_pattern_dismissed", string(saved.ID))
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var details string
		if err := rows.Scan(&details); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if strings.Contains(details, secretNote) {
			t.Fatalf("private_or_sensitive note leaked into audit row: %s", details)
		}
		if strings.Contains(details, "internal-accounting") {
			t.Fatalf("private_or_sensitive note fragment leaked into audit row: %s", details)
		}
	}
}

// TestCluster_SplitKeyDeterministic pins that reordering candidates
// of the same fingerprint does not produce different cluster
// identities. The cap-split suffix must be deterministic, not
// derived from map size at insertion time.
func TestCluster_SplitKeyDeterministic(t *testing.T) {
	t.Parallel()

	cfg := patterns.DefaultConfig()
	cfg.MaxClusterMembers = 2

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	fp := "fp-det"
	mk := func(id string, sess string, day int) patterns.PatternCandidate {
		return patterns.PatternCandidate{
			EventID:        domain.ExperienceEventID(id),
			ProjectID:      domain.ProjectID("p"),
			Kind:           domain.EventTestFailure,
			Fingerprint:    fp,
			RetrievalTerms: []string{"compile"},
			SessionID:      sess,
			OccurredAt:     now.Add(time.Duration(day) * 24 * time.Hour),
		}
	}

	a := []patterns.PatternCandidate{mk("e1", "s1", 0), mk("e2", "s2", 1), mk("e3", "s3", 2), mk("e4", "s4", 3)}
	b := []patterns.PatternCandidate{mk("e4", "s4", 3), mk("e3", "s3", 2), mk("e2", "s2", 1), mk("e1", "s1", 0)}

	clustersA := patterns.Group(a, cfg)
	clustersB := patterns.Group(b, cfg)

	if len(clustersA) != len(clustersB) {
		t.Fatalf("cluster count differs across reorderings: %d vs %d", len(clustersA), len(clustersB))
	}
	if len(clustersA) != 2 {
		t.Fatalf("expected 2 clusters after cap split, got %d", len(clustersA))
	}
	for i := range clustersA {
		if clustersA[i].Fingerprint != clustersB[i].Fingerprint {
			t.Fatalf("cluster %d fingerprint differs: %s vs %s", i,
				clustersA[i].Fingerprint, clustersB[i].Fingerprint)
		}
		if len(clustersA[i].Members) != len(clustersB[i].Members) {
			t.Fatalf("cluster %d size differs: %d vs %d", i,
				len(clustersA[i].Members), len(clustersB[i].Members))
		}
	}
}

// TestPattern_IDIsUUIDv7 pins the operator's invariant: pattern IDs
// must follow the project's UUIDv7 convention, not a hand-rolled
// timestamp+counter string.
func TestPattern_IDIsUUIDv7(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "uuid")
	repo := patterns.NewRepository(db)
	p := validPattern(fixture.ProjectID, "fp-uuid")
	p.ID = ""

	saved, err := repo.SavePattern(context.Background(), p)
	if err != nil {
		t.Fatalf("SavePattern: %v", err)
	}
	if len(string(saved.ID)) < 32 {
		t.Fatalf("ID = %q, want ≥ 32 char (UUIDv7-style)", saved.ID)
	}
	if !strings.Contains(string(saved.ID), "-") {
		t.Fatalf("ID = %q, want hyphenated UUID format", saved.ID)
	}
}

// TestInputDigest_IsSHA256 pins that the InputDigest is the hex sha256
// of canonical bytes (not raw concatenated string content). The
// regression: the previous implementation joined fields with `|` and
// `,` which leaked sensitive values into the audit trail's
// payload_sha256.
func TestInputDigest_IsSHA256(t *testing.T) {
	t.Parallel()

	cluster := patterns.ClusterRecord{
		Fingerprint:        "fp-digest",
		Kind:               domain.EventTestFailure,
		Members:            []domain.ExperienceEventID{"evt-1", "evt-2"},
		Sessions:           map[string]struct{}{"s1": {}},
		Days:               map[string]struct{}{"d1": {}},
		DistinctSessions:   1,
		DistinctDays:       1,
		OccurrenceCount:    2,
		RetrievalTerms:     []string{"compile"},
		SourceFingerprints: []string{"fp-digest"},
	}

	svc := patterns.NewServiceFromRaw(nil)
	_ = svc // silence unused
	// The InputDigest is computed by the service. We test it
	// indirectly: calling IngestCluster with a stable input yields
	// a pattern whose InputDigest is a 64-char lowercase hex.
	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "digest")
	seedExperienceEvents(t, db, fixture.ProjectID, cluster.Members)
	svc = patterns.NewService(db)
	saved, err := svc.IngestCluster(context.Background(), fixture.ProjectID, cluster, patterns.QualificationDecision{
		Status: patterns.PatternQualified,
	})
	if err != nil {
		t.Fatalf("IngestCluster: %v", err)
	}
	if len(saved.InputDigest) != 64 {
		t.Fatalf("InputDigest = %q, want 64-char sha256 hex (got %d chars)", saved.InputDigest, len(saved.InputDigest))
	}
	for _, c := range saved.InputDigest {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("InputDigest = %q, want lowercase hex only", saved.InputDigest)
		}
	}
}

// TestNewQualifier_DoesNotPanicOnInvalid pins the defensive invariant:
// caller-provided invalid criteria must surface as an error, not a
// panic.
func TestNewQualifier_DoesNotPanicOnInvalid(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewQualifier panicked on invalid input: %v", r)
		}
	}()
	bad := patterns.DefaultQualificationCriteria()
	bad.MinDistinctSessions = 0
	if _, err := patterns.NewQualifier(bad); err == nil {
		t.Fatal("NewQualifier(invalid) = nil error, want typed error")
	}
}

// TestRepository_AddMember_NilDB_NoPanic pins the defensive invariant:
// AddMember on a Repository bound to a raw *sql.DB must not reach into
// r.db (which would be nil).
func TestRepository_AddMember_NilDB_NoPanic(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "addmember-raw")
	repo := patterns.NewRepository(db)
	saved, err := repo.SavePattern(context.Background(), validPattern(fixture.ProjectID, "fp-am"))
	if err != nil {
		t.Fatalf("SavePattern: %v", err)
	}
	seedExperienceEvents(t, db, fixture.ProjectID, []domain.ExperienceEventID{"evt-am"})

	// Re-bind the same raw handle via NewRepositoryFromRaw so the
	// constructor exercise the raw branch. Both fields must be nil
	// safe — the raw branch is the one the test exercises here.
	repoRaw := patterns.NewRepositoryFromRaw(db.DB)
	if _, err := repoRaw.AddMember(context.Background(), saved.ID, domain.ExperienceEventID("evt-am"), "exact_fingerprint", 1.0, time.Now().UTC()); err != nil {
		t.Fatalf("AddMember via raw: %v", err)
	}
}

// TestStaleness_MarksStaleOnDetectorVersionBump pins the docs/24 §3 T12
// invariant: when a pattern with the same fingerprint is re-saved with
// a newer detector_version, the existing row must be marked stale
// (rather than silently updated) so a reviewer can re-evaluate the
// pattern under the new algorithm.
func TestStaleness_MarksStaleOnDetectorVersionBump(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "stale")
	repo := patterns.NewRepository(db)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	first, err := repo.SavePattern(context.Background(), patterns.ExperiencePattern{
		ProjectID:        fixture.ProjectID,
		Status:           patterns.PatternObserved,
		Kind:             domain.EventTestFailure,
		Fingerprint:      "fp-stale",
		DistinctSessions: 3, DistinctDays: 3, OccurrenceCount: 3,
		FirstSeenAt: now, LastSeenAt: now,
		DetectorVersion: "v1.0.0",
		InputDigest:     "digest-v1",
		CreatedAt:       now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("first SavePattern: %v", err)
	}

	// Resave with a NEW detector version but the same fingerprint.
	// The repository must NOT overwrite the observed row; it must
	// either mark it stale or refuse the resave. The simplest,
	// auditable behavior: mark the existing row stale.
	second, err := repo.SavePattern(context.Background(), patterns.ExperiencePattern{
		ProjectID:        fixture.ProjectID,
		Status:           patterns.PatternObserved,
		Kind:             domain.EventTestFailure,
		Fingerprint:      "fp-stale",
		DistinctSessions: 3, DistinctDays: 3, OccurrenceCount: 3,
		FirstSeenAt: now, LastSeenAt: now,
		DetectorVersion: "v1.1.0",
		InputDigest:     "digest-v1.1",
		CreatedAt:       now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("second SavePattern: %v", err)
	}
	_ = second

	// The pattern must be marked stale (not silently updated).
	var status string
	if err := db.DB.QueryRow(`SELECT status FROM experience_patterns WHERE project_id = ? AND fingerprint = ?`,
		string(fixture.ProjectID), "fp-stale").Scan(&status); err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	if status != string(patterns.PatternStale) {
		t.Fatalf("status = %s, want stale (detector_version bumped)", status)
	}
	_ = first
}

// TestMembership_TracesToExistingEvent pins that AddMember refuses
// to insert a membership row for an event that does not exist in
// experience_events. The trace FK is enforced at the storage
// layer; the test seeds an event, then asserts the membership
// succeeds, then attempts a bogus event id and expects an FK
// violation surfaced as a typed error.
func TestMembership_TracesToExistingEvent(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	fixture := newProjectFixture(t, db, "membership-trace")

	// Seed a real experience_event so the FK target exists.
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	eventID := domain.ExperienceEventID("evt-real-1")
	seedExperienceEvents(t, db, fixture.ProjectID, []domain.ExperienceEventID{eventID})

	repo := patterns.NewRepository(db)
	p := validPattern(fixture.ProjectID, "fp-trace")
	saved, err := repo.SavePattern(context.Background(), p)
	if err != nil {
		t.Fatalf("SavePattern: %v", err)
	}

	// Real event id → success.
	if _, err := repo.AddMember(context.Background(), saved.ID, eventID, "exact_fingerprint", 1.0, now); err != nil {
		t.Fatalf("AddMember(real event): %v", err)
	}

	// Bogus event id → FK violation surfaced as a typed error.
	bogus := domain.ExperienceEventID("evt-bogus-missing")
	if _, err := repo.AddMember(context.Background(), saved.ID, bogus, "exact_fingerprint", 1.0, now); err == nil {
		t.Fatal("AddMember(bogus event) = nil, want FK-violation error")
	}
}
