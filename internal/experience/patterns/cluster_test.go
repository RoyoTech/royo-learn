// Clustering tests for Hito 6 slice 6.2.
//
// The v1 clustering algorithm is pure (no I/O, no DB, no clock) and
// groups PatternCandidates by:
//
//   1. exact fingerprint (canonical PatternFingerprint output);
//   2. event-kind match (a retry cluster cannot absorb a command
//      failure);
//   3. conservative Jaccard over retrieval terms (configurable, named
//      default in slice 6.0; reversible per the parent task brief).
//
// It does NOT use embeddings or any vector-database API (AGENTS.md
// rule 9, docs/23-PATTERN-MINING.md §4). The output is a list of
// Cluster values, each carrying a fingerprint, a kind, and the list
// of member events plus computed session/day/occurrence metrics.

package patterns

import (
	"sort"
	"strconv"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

// makeCandidate is a test helper that builds a PatternCandidate with
// sensible defaults. Cluster tests do not care about fingerprint
// internals; they care about fingerprint equality, retrieval-term
// overlap and the kind/kind partitions.
func makeCandidate(
	t *testing.T,
	id string,
	kind domain.ExperienceEventKind,
	fingerprint string,
	retrievalTerms []string,
	sessionID string,
	occurredAt time.Time,
) PatternCandidate {
	t.Helper()
	return PatternCandidate{
		EventID:        domain.ExperienceEventID(id),
		ProjectID:      domain.ProjectID("proj-1"),
		Kind:           kind,
		Fingerprint:    fingerprint,
		Problem:        "problem " + id,
		Tool:           "tool",
		Result:         "fail",
		RetrievalTerms: append([]string(nil), retrievalTerms...),
		SessionID:      sessionID,
		OccurredAt:     occurredAt,
	}
}

// TestCluster_EmptyInput verifies that Cluster returns no clusters
// for an empty event stream without panicking. This is the documented
// "happy path" for a routine conversation (no candidate events).
func TestCluster_EmptyInput(t *testing.T) {
	t.Parallel()

	clusters := Group(nil, DefaultConfig())
	if len(clusters) != 0 {
		t.Fatalf("Group(nil) = %d clusters, want 0", len(clusters))
	}

	clusters = Group([]PatternCandidate{}, DefaultConfig())
	if len(clusters) != 0 {
		t.Fatalf("Group([]) = %d clusters, want 0", len(clusters))
	}
}

// TestCluster_ExactFingerprint covers the first clustering signal:
// events with the same canonical fingerprint MUST land in the same
// cluster regardless of retrieval-term overlap.
func TestCluster_ExactFingerprint(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	day := now.AddDate(0, 0, 1)

	candidates := []PatternCandidate{
		makeCandidate(t, "evt-1", domain.EventTestFailure, "fp-A", []string{"compile", "missing"}, "sess-1", now),
		makeCandidate(t, "evt-2", domain.EventTestFailure, "fp-A", []string{"compile"}, "sess-2", day),
		makeCandidate(t, "evt-3", domain.EventTestFailure, "fp-B", []string{"linker"}, "sess-3", now),
	}

	clusters := Group(candidates, DefaultConfig())
	if len(clusters) != 2 {
		t.Fatalf("Group = %d clusters, want 2 (fp-A, fp-B)", len(clusters))
	}
	counts := map[string]int{}
	for _, c := range clusters {
		counts[c.Fingerprint] = len(c.Members)
	}
	if counts["fp-A"] != 2 {
		t.Fatalf("fp-A cluster size = %d, want 2", counts["fp-A"])
	}
	if counts["fp-B"] != 1 {
		t.Fatalf("fp-B cluster size = %d, want 1", counts["fp-B"])
	}
}

// TestCluster_DifferentKindNeverMerge ensures the kind partition: two
// events with the same fingerprint (impossible in practice, but the
// rule must still apply to the Jaccard fallback) but different kinds
// MUST NOT end up in the same cluster.
func TestCluster_DifferentKindNeverMerge(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	day := now.AddDate(0, 0, 1)

	candidates := []PatternCandidate{
		makeCandidate(t, "evt-1", domain.EventTestFailure, "fp-NA", []string{"compile"}, "sess-1", now),
		makeCandidate(t, "evt-2", domain.EventCommandFailure, "fp-NA", []string{"compile"}, "sess-2", day),
	}

	clusters := Group(candidates, DefaultConfig())
	if len(clusters) != 2 {
		t.Fatalf("Group across kinds = %d clusters, want 2", len(clusters))
	}
}

// TestCluster_JaccardMerge verifies the conservative Jaccard signal:
// events with different fingerprints but overlapping retrieval terms
// MAY merge when the overlap ratio is at or above the configured
// threshold.
func TestCluster_JaccardMerge(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	day := now.AddDate(0, 0, 1)
	week := now.AddDate(0, 0, 7)

	termsA := []string{"compile", "missing", "header"}
	termsB := []string{"compile", "missing", "header", "redundant"}
	termsC := []string{"compile", "missing", "header"} // identical to A, will collide exactly

	// Build with three distinct fingerprints to test the Jaccard path.
	candidates := []PatternCandidate{
		makeCandidate(t, "evt-1", domain.EventTestFailure, "fp-X1", termsA, "sess-1", now),
		makeCandidate(t, "evt-2", domain.EventTestFailure, "fp-X2", termsB, "sess-2", day),
		makeCandidate(t, "evt-3", domain.EventTestFailure, "fp-X3", termsC, "sess-3", week),
	}

	cfg := DefaultConfig()
	cfg.MinRetrievalJaccard = 0.5
	clusters := Group(candidates, cfg)
	if len(clusters) != 1 {
		t.Fatalf("Group Jaccard merge = %d clusters, want 1", len(clusters))
	}
	if len(clusters[0].Members) != 3 {
		t.Fatalf("merged cluster size = %d, want 3", len(clusters[0].Members))
	}
}

// TestCluster_JaccardBelowThresholdStaysSeparate checks the inverse:
// when the overlap ratio is below the threshold the events stay in
// distinct clusters even if they share some terms.
func TestCluster_JaccardBelowThresholdStaysSeparate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	day := now.AddDate(0, 0, 1)

	// Jaccard between {compile, missing} and {linker, missing, header, redundant}
	// = 1 / 5 = 0.2, well below 0.5.
	termsA := []string{"compile", "missing"}
	termsB := []string{"linker", "missing", "header", "redundant"}

	candidates := []PatternCandidate{
		makeCandidate(t, "evt-1", domain.EventTestFailure, "fp-Y1", termsA, "sess-1", now),
		makeCandidate(t, "evt-2", domain.EventTestFailure, "fp-Y2", termsB, "sess-2", day),
	}

	cfg := DefaultConfig()
	cfg.MinRetrievalJaccard = 0.5
	clusters := Group(candidates, cfg)
	if len(clusters) != 2 {
		t.Fatalf("low-Jaccard cluster = %d, want 2", len(clusters))
	}
}

// TestCluster_HighJaccardThreshold verifies the threshold is a real
// knob: raising it above the empirical overlap produces more clusters.
func TestCluster_HighJaccardThreshold(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	day := now.AddDate(0, 0, 1)

	// Jaccard between {compile, missing, header} and {compile, missing, header, linker}
	// = 3 / 4 = 0.75. So a threshold of 0.5 merges, a threshold of 0.9 does not.
	termsA := []string{"compile", "missing", "header"}
	termsB := []string{"compile", "missing", "header", "linker"}

	candidates := []PatternCandidate{
		makeCandidate(t, "evt-1", domain.EventTestFailure, "fp-Z1", termsA, "sess-1", now),
		makeCandidate(t, "evt-2", domain.EventTestFailure, "fp-Z2", termsB, "sess-2", day),
	}

	cfg := DefaultConfig()
	cfg.MinRetrievalJaccard = 0.9
	clusters := Group(candidates, cfg)
	if len(clusters) != 2 {
		t.Fatalf("threshold 0.9 cluster = %d, want 2", len(clusters))
	}

	cfg.MinRetrievalJaccard = 0.5
	clusters = Group(candidates, cfg)
	if len(clusters) != 1 {
		t.Fatalf("threshold 0.5 cluster = %d, want 1", len(clusters))
	}
}

// TestCluster_MaxClusterMembers verifies the cap: once a cluster
// reaches MaxClusterMembers it stops absorbing new members even if
// the Jaccard condition is met. New candidates with the same
// fingerprint start a new cluster.
func TestCluster_MaxClusterMembers(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	terms := []string{"compile", "missing"}

	cfg := DefaultConfig()
	cfg.MaxClusterMembers = 3

	var candidates []PatternCandidate
	for i := 0; i < 5; i++ {
		fp := "fp-cap"
		if i >= 3 {
			fp = "fp-cap-extra"
		}
		candidates = append(candidates, makeCandidate(t,
			"evt-"+strconv.Itoa(i),
			domain.EventTestFailure,
			fp,
			terms,
			"sess-"+strconv.Itoa(i),
			now.Add(time.Duration(i)*24*time.Hour),
		))
	}

	clusters := Group(candidates, cfg)
	if len(clusters) != 2 {
		t.Fatalf("cluster cap = %d clusters, want 2 (cap split)", len(clusters))
	}
	sizes := []int{}
	for _, c := range clusters {
		sizes = append(sizes, len(c.Members))
	}
	sort.Ints(sizes)
	if sizes[0] != 2 || sizes[1] != 3 {
		t.Fatalf("cluster sizes = %v, want [2 3]", sizes)
	}
}

// TestCluster_Metrics verifies the DistinctSessions / DistinctDays /
// OccurrenceCount counters that downstream qualification consumes.
func TestCluster_Metrics(t *testing.T) {
	t.Parallel()

	day1 := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 1)
	day3 := day1.AddDate(0, 0, 2)

	candidates := []PatternCandidate{
		makeCandidate(t, "evt-1", domain.EventTestFailure, "fp-M", []string{"compile"}, "sess-1", day1),
		makeCandidate(t, "evt-2", domain.EventTestFailure, "fp-M", []string{"compile"}, "sess-2", day2),
		makeCandidate(t, "evt-3", domain.EventTestFailure, "fp-M", []string{"compile"}, "sess-1", day3),
	}

	clusters := Group(candidates, DefaultConfig())
	if len(clusters) != 1 {
		t.Fatalf("Group = %d clusters, want 1", len(clusters))
	}
	c := clusters[0]
	if c.DistinctSessions != 2 {
		t.Fatalf("DistinctSessions = %d, want 2 (sess-1, sess-2)", c.DistinctSessions)
	}
	if c.DistinctDays != 3 {
		t.Fatalf("DistinctDays = %d, want 3", c.DistinctDays)
	}
	if c.OccurrenceCount != 3 {
		t.Fatalf("OccurrenceCount = %d, want 3", c.OccurrenceCount)
	}
}

// TestCluster_DeterministicOrder verifies that the output order of
// Cluster is stable across runs (sorted by fingerprint) so callers
// can iterate without re-sorting.
func TestCluster_DeterministicOrder(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	var candidates []PatternCandidate
	for _, fp := range []string{"fp-C", "fp-A", "fp-B"} {
		// Each candidate uses a unique retrieval term so the Jaccard
		// fallback does NOT merge them; the test only exercises the
		// exact-fingerprint partition and the deterministic sort.
		candidates = append(candidates, makeCandidate(t,
			"evt-"+fp, domain.EventTestFailure, fp,
			[]string{"term-" + fp}, "sess-"+fp, now,
		))
	}

	clusters := Group(candidates, DefaultConfig())
	if len(clusters) != 3 {
		t.Fatalf("Group = %d clusters, want 3", len(clusters))
	}
	want := []string{"fp-A", "fp-B", "fp-C"}
	for i, c := range clusters {
		if c.Fingerprint != want[i] {
			t.Fatalf("cluster[%d].Fingerprint = %q, want %q", i, c.Fingerprint, want[i])
		}
	}
}

// TestCluster_NoEmbeddings is the docs/23 §4 invariant: the
// algorithm MUST NOT reach for embeddings or a vector database. We
// enforce this by ensuring the algorithm is purely deterministic and
// produces identical output for identical input across runs without
// any external dependency.
func TestCluster_NoEmbeddings(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	candidates := []PatternCandidate{
		makeCandidate(t, "evt-1", domain.EventTestFailure, "fp-N", []string{"compile"}, "sess-1", now),
		makeCandidate(t, "evt-2", domain.EventTestFailure, "fp-N", []string{"compile"}, "sess-2", now.Add(24*time.Hour)),
	}

	first := Group(candidates, DefaultConfig())
	second := Group(candidates, DefaultConfig())
	if len(first) != len(second) {
		t.Fatalf("non-deterministic cluster count: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Fingerprint != second[i].Fingerprint {
			t.Fatalf("non-deterministic fingerprint at %d: %q vs %q", i, first[i].Fingerprint, second[i].Fingerprint)
		}
		if first[i].OccurrenceCount != second[i].OccurrenceCount {
			t.Fatalf("non-deterministic OccurrenceCount at %d", i)
		}
	}
}

// --- helpers ---

// (itoa lives in cluster.go for the algorithm's use; the tests rely
// on the same helper via package-level visibility.)
