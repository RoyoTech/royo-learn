// Service tests for Hito 9 (slice 9.1/9.2).
//
// The service is the brain of the retrieval pipeline:
//
//   1. Sanitize the user query.
//   2. Call the repository with terms + a 4x overfetch limit.
//   3. Score each Candidate using the v1 components.
//   4. Sort deterministically by (score DESC, fingerprint ASC, id ASC).
//   5. Apply Limit + Offset and emit Hits.
//
// These tests pin the contract for ranking, score components and
// pagination. They use storagetest.OpenTemp so the database state
// is realistic (FTS5 trigger-driven sync).

package retrieval_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/retrieval"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/storage/storagetest"
)

// fixedNow returns a stable timestamp so recency decay tests are
// deterministic.
func fixedNow() time.Time {
	return time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
}

// newService constructs a Service with a controllable clock. We
// inject `now` so recency decay is reproducible.
func newService(t *testing.T, db *storage.DB, weights retrieval.Weights, now time.Time) *retrieval.Service {
	t.Helper()
	svc := retrieval.NewService(retrieval.NewRepository(db), weights)
	svc.SetNow(func() time.Time { return now })
	return svc
}

// TestService_DeterministicRanking proves the same query returns
// hits in the same order across runs. This is the foundational
// contract of Hito 9.
func TestService_DeterministicRanking(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "deterministic")
	seedLearnings(t, db, projectID)

	svc := newService(t, db, retrieval.DefaultWeights(), fixedNow())

	q := retrieval.Query{Text: "lexical retrieval", ProjectID: projectID, Limit: 10}
	r1, err := svc.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("first Search: %v", err)
	}
	r2, err := svc.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("second Search: %v", err)
	}
	if len(r1.Hits) != len(r2.Hits) {
		t.Fatalf("hit count differs across runs: %d vs %d", len(r1.Hits), len(r2.Hits))
	}
	for i := range r1.Hits {
		if r1.Hits[i].Learning.ID != r2.Hits[i].Learning.ID {
			t.Fatalf("position %d differs: %s vs %s", i, r1.Hits[i].Learning.ID, r2.Hits[i].Learning.ID)
		}
		if r1.Hits[i].Score != r2.Hits[i].Score {
			t.Fatalf("position %d score differs: %f vs %f", i, r1.Hits[i].Score, r2.Hits[i].Score)
		}
	}
}

// TestService_TiebreakerByFingerprint proves that two hits with the
// same score are ordered by fingerprint (then by id).
func TestService_TiebreakerByFingerprint(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "tie")

	// Two learnings with identical titles, evidence levels and
	// retrieval terms. Only the fingerprint + id differ.
	now := fixedNow()
	mk := func(id, fp, title string, terms []string) {
		l := &domain.Learning{
			ID:                  domain.LearningID(id),
			ProjectID:           projectID,
			Status:              domain.StatusApproved,
			Type:                domain.TypeProcedure,
			Title:               title,
			Context:             "ctx",
			Observation:         "obs",
			ReusableLesson:      "lesson",
			Confidence:          domain.ConfidenceHigh,
			EvidenceLevel:       domain.EvidenceModerate,
			ProposedDestination: domain.DestProject,
			RetrievalTerms:      terms,
			Fingerprint:         fp,
			NormalizedHash:      "nh-" + id,
			Actor:               domain.Actor{Kind: "system", Name: "test"},
			Revision:            1,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		saveLearning(t, db, projectID, l)
	}
	// Same FTS5 content (so bm25 is identical), different
	// fingerprint. Fingerprint "z" should come AFTER "a" in the
	// ascending tiebreaker.
	mk("L-z", "fingerprint-z", "shared title about plumbing", []string{"plumbing"})
	mk("L-a", "fingerprint-a", "shared title about plumbing", []string{"plumbing"})

	svc := newService(t, db, retrieval.DefaultWeights(), fixedNow())
	res, err := svc.Search(context.Background(), retrieval.Query{
		Text:      "plumbing",
		ProjectID: projectID,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Hits) < 2 {
		t.Fatalf("got %d hits, want >= 2 to test tiebreaker", len(res.Hits))
	}
	// The first hit should be the one with fingerprint "a"
	// (smaller string).
	if res.Hits[0].Learning.Fingerprint != "fingerprint-a" {
		t.Fatalf("tiebreaker top hit = %s, want fingerprint-a", res.Hits[0].Learning.Fingerprint)
	}
	if res.Hits[1].Learning.Fingerprint != "fingerprint-z" {
		t.Fatalf("tiebreaker second hit = %s, want fingerprint-z", res.Hits[1].Learning.Fingerprint)
	}
}

// TestService_ScoreComponentsPopulated verifies that every Hit
// exposes a non-zero breakdown. The exact numbers depend on the
// data; we only assert structure.
func TestService_ScoreComponentsPopulated(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "components")
	seedLearnings(t, db, projectID)

	svc := newService(t, db, retrieval.DefaultWeights(), fixedNow())
	res, err := svc.Search(context.Background(), retrieval.Query{
		Text:      "unicode tokenization",
		ProjectID: projectID,
		Limit:     5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("no hits; cannot verify components")
	}
	for _, h := range res.Hits {
		if h.Score == 0 {
			t.Errorf("Hit %s has zero score", h.Learning.ID)
		}
		// BM25 must be > 0 for a match.
		if h.Components.BM25 <= 0 {
			t.Errorf("Hit %s BM25 = %f, want > 0", h.Learning.ID, h.Components.BM25)
		}
	}
}

// TestService_LimitIsApplied verifies that the result honors
// Query.Limit.
func TestService_LimitIsApplied(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "limit-svc")
	seedLearnings(t, db, projectID)
	// Add extras so there are many candidates to cap.
	now := fixedNow()
	for i := 0; i < 5; i++ {
		l := &domain.Learning{
			ID:                  domain.LearningID("L-extra-svc-" + string(rune('a'+i))),
			ProjectID:           projectID,
			Status:              domain.StatusApproved,
			Type:                domain.TypeProcedure,
			Title:               "extra lexical retrieval note " + string(rune('a'+i)),
			Context:             "ctx",
			Observation:         "obs",
			ReusableLesson:      "lesson",
			Confidence:          domain.ConfidenceHigh,
			EvidenceLevel:       domain.EvidenceModerate,
			ProposedDestination: domain.DestProject,
			RetrievalTerms:      []string{"lexical", "retrieval"},
			Fingerprint:         "fp-extra-svc-" + string(rune('a'+i)),
			NormalizedHash:      "nh-extra-svc-" + string(rune('a'+i)),
			Actor:               domain.Actor{Kind: "system", Name: "test"},
			Revision:            1,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		saveLearning(t, db, projectID, l)
	}

	svc := newService(t, db, retrieval.DefaultWeights(), fixedNow())
	res, err := svc.Search(context.Background(), retrieval.Query{
		Text:      "lexical",
		ProjectID: projectID,
		Limit:     2,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Hits) > 2 {
		t.Fatalf("Search(limit=2) returned %d hits, want <= 2", len(res.Hits))
	}
}

// TestService_OffsetSkipsTopHits verifies Offset=N skips the first
// N candidates.
func TestService_OffsetSkipsTopHits(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "offset")
	seedLearnings(t, db, projectID)

	// Add extra learnings that all share the "retrieval" token so we
	// can exercise offset.
	now := fixedNow()
	for i := 0; i < 5; i++ {
		l := &domain.Learning{
			ID:                  domain.LearningID("L-offset-extra-" + string(rune('a'+i))),
			ProjectID:           projectID,
			Status:              domain.StatusApproved,
			Type:                domain.TypeProcedure,
			Title:               "offset test " + string(rune('a'+i)) + " retrieval",
			Context:             "ctx",
			Observation:         "obs",
			ReusableLesson:      "lesson",
			Confidence:          domain.ConfidenceHigh,
			EvidenceLevel:       domain.EvidenceModerate,
			ProposedDestination: domain.DestProject,
			RetrievalTerms:      []string{"retrieval", "offset"},
			Fingerprint:         "fp-offset-" + string(rune('a'+i)),
			NormalizedHash:      "nh-offset-" + string(rune('a'+i)),
			Actor:               domain.Actor{Kind: "system", Name: "test"},
			Revision:            1,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		saveLearning(t, db, projectID, l)
	}

	svc := newService(t, db, retrieval.DefaultWeights(), fixedNow())

	all, err := svc.Search(context.Background(), retrieval.Query{
		Text:      "retrieval",
		ProjectID: projectID,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search(no-offset): %v", err)
	}
	if len(all.Hits) < 3 {
		t.Skipf("need >= 3 hits to test offset; got %d", len(all.Hits))
	}

	offset, err := svc.Search(context.Background(), retrieval.Query{
		Text:      "retrieval",
		ProjectID: projectID,
		Limit:     10,
		Offset:    2,
	})
	if err != nil {
		t.Fatalf("Search(offset=2): %v", err)
	}
	if len(offset.Hits) != len(all.Hits)-2 {
		t.Fatalf("offset result len = %d, want %d", len(offset.Hits), len(all.Hits)-2)
	}
	if offset.Hits[0].Learning.ID != all.Hits[2].Learning.ID {
		t.Fatalf("offset[0] = %s, want %s", offset.Hits[0].Learning.ID, all.Hits[2].Learning.ID)
	}
}

// TestService_NoMatchesReturnsEmptyNotError verifies a no-match
// query is exit-0-friendly.
func TestService_NoMatchesReturnsEmptyNotError(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "no-match")
	seedLearnings(t, db, projectID)

	svc := newService(t, db, retrieval.DefaultWeights(), fixedNow())
	res, err := svc.Search(context.Background(), retrieval.Query{
		Text:      "nonexistenttermxyz9x7q",
		ProjectID: projectID,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search error = %v, want nil", err)
	}
	if len(res.Hits) != 0 {
		t.Fatalf("Search = %d hits, want 0", len(res.Hits))
	}
	if res.Total != 0 {
		t.Fatalf("res.Total = %d, want 0", res.Total)
	}
}

// TestService_SanitizationFailureEmptyNotError verifies that a
// query that sanitizes to nothing (control chars only) yields an
// empty result, not an error.
func TestService_SanitizationFailureEmptyNotError(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "sanit-fail")
	seedLearnings(t, db, projectID)

	svc := newService(t, db, retrieval.DefaultWeights(), fixedNow())
	res, err := svc.Search(context.Background(), retrieval.Query{
		Text:      "\x00\x01\x02",
		ProjectID: projectID,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search error = %v, want nil", err)
	}
	if len(res.Hits) != 0 {
		t.Fatalf("Search = %d hits, want 0", len(res.Hits))
	}
}

// TestService_TooManyTermsWrapsAsDomainError verifies the
// ErrTooManyTerms is wrapped as a domain ErrInvalidArgument so the
// CLI/MCP can map it to exit code 2.
func TestService_TooManyTermsWrapsAsDomainError(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "too-many")
	seedLearnings(t, db, projectID)

	svc := newService(t, db, retrieval.DefaultWeights(), fixedNow())
	_, err := svc.Search(context.Background(), retrieval.Query{
		Text:      strings.Repeat("a ", 20),
		ProjectID: projectID,
		Limit:     10,
	})
	if err == nil {
		t.Fatal("Search error = nil, want typed error")
	}
	if derr, ok := domain.AsDomainError(err); !ok || derr.Code != domain.ErrInvalidArgument {
		t.Fatalf("error = %v, want ErrInvalidArgument domain error", err)
	}
}

// TestService_RecencyDecayFresh verifies that a 1-day-old learning
// gets recency = 1.0.
func TestService_RecencyDecayFresh(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "recency-fresh")

	now := fixedNow()
	fresh := now.Add(-24 * time.Hour) // 1 day old
	l := &domain.Learning{
		ID:                  domain.LearningID("L-fresh"),
		ProjectID:           projectID,
		Status:              domain.StatusApproved,
		Type:                domain.TypeProcedure,
		Title:               "Fresh learning about golang",
		Context:             "ctx",
		Observation:         "obs",
		ReusableLesson:      "lesson",
		Confidence:          domain.ConfidenceHigh,
		EvidenceLevel:       domain.EvidenceModerate,
		ProposedDestination: domain.DestProject,
		RetrievalTerms:      []string{"golang"},
		Fingerprint:         "fp-fresh",
		NormalizedHash:      "nh-fresh",
		Actor:               domain.Actor{Kind: "system", Name: "test"},
		Revision:            1,
		CreatedAt:           fresh,
		UpdatedAt:           fresh,
	}
	saveLearning(t, db, projectID, l)

	svc := newService(t, db, retrieval.DefaultWeights(), now)
	res, err := svc.Search(context.Background(), retrieval.Query{
		Text:      "golang",
		ProjectID: projectID,
		Limit:     5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("no hits; cannot verify recency")
	}
	if got := res.Hits[0].Components.Recency; got != 1.0 {
		t.Fatalf("recency for 1-day-old = %f, want 1.0", got)
	}
}

// TestService_RecencyDecay verifies that a ~200-day-old learning
// gets recency around 0.45 (linear decay 365 days).
func TestService_RecencyDecay(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "recency-200d")

	now := fixedNow()
	old := now.Add(-200 * 24 * time.Hour)
	l := &domain.Learning{
		ID:                  domain.LearningID("L-old"),
		ProjectID:           projectID,
		Status:              domain.StatusApproved,
		Type:                domain.TypeProcedure,
		Title:               "Old learning about legacy",
		Context:             "ctx",
		Observation:         "obs",
		ReusableLesson:      "lesson",
		Confidence:          domain.ConfidenceHigh,
		EvidenceLevel:       domain.EvidenceModerate,
		ProposedDestination: domain.DestProject,
		RetrievalTerms:      []string{"legacy"},
		Fingerprint:         "fp-old",
		NormalizedHash:      "nh-old",
		Actor:               domain.Actor{Kind: "system", Name: "test"},
		Revision:            1,
		CreatedAt:           old,
		UpdatedAt:           old,
	}
	saveLearning(t, db, projectID, l)

	svc := newService(t, db, retrieval.DefaultWeights(), now)
	res, err := svc.Search(context.Background(), retrieval.Query{
		Text:      "legacy",
		ProjectID: projectID,
		Limit:     5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("no hits")
	}
	got := res.Hits[0].Components.Recency
	// 1.0 - 200/365 ≈ 0.452. Tolerate ±0.02 for date math.
	if got < 0.43 || got > 0.47 {
		t.Fatalf("recency for 200-day-old = %f, want ~0.45", got)
	}
}

// TestService_TitleExactBoostsRank verifies a learning whose title
// matches the query verbatim ranks above one that merely shares a
// token.
func TestService_TitleExactBoostsRank(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "title-exact")

	now := fixedNow()
	mk := func(id, title string) {
		l := &domain.Learning{
			ID:                  domain.LearningID(id),
			ProjectID:           projectID,
			Status:              domain.StatusApproved,
			Type:                domain.TypeProcedure,
			Title:               title,
			Context:             "ctx",
			Observation:         "obs",
			ReusableLesson:      "lesson",
			Confidence:          domain.ConfidenceHigh,
			EvidenceLevel:       domain.EvidenceModerate,
			ProposedDestination: domain.DestProject,
			RetrievalTerms:      []string{"lexical"},
			Fingerprint:         "fp-" + id,
			NormalizedHash:      "nh-" + id,
			Actor:               domain.Actor{Kind: "system", Name: "test"},
			Revision:            1,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		saveLearning(t, db, projectID, l)
	}
	mk("L-exact", "lexical retrieval")
	mk("L-loose", "lexical retrieval alternatives and notes")

	svc := newService(t, db, retrieval.DefaultWeights(), now)
	res, err := svc.Search(context.Background(), retrieval.Query{
		Text:      "lexical retrieval",
		ProjectID: projectID,
		Limit:     5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Hits) < 2 {
		t.Fatalf("need >= 2 hits, got %d", len(res.Hits))
	}
	if res.Hits[0].Learning.ID != domain.LearningID("L-exact") {
		t.Fatalf("top hit = %s, want L-exact", res.Hits[0].Learning.ID)
	}
	if res.Hits[0].Components.TitleExact != 1.0 {
		t.Fatalf("top hit title_exact = %f, want 1.0", res.Hits[0].Components.TitleExact)
	}
}

// TestService_RetrievalTermsOverlapBoostsRank verifies a learning
// whose retrieval_terms intersect the query ranks above one that
// only shares FTS5 tokens.
func TestService_RetrievalTermsOverlapBoostsRank(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "rt-overlap")

	now := fixedNow()
	mk := func(id, title string, terms []string) {
		l := &domain.Learning{
			ID:                  domain.LearningID(id),
			ProjectID:           projectID,
			Status:              domain.StatusApproved,
			Type:                domain.TypeProcedure,
			Title:               title,
			Context:             "ctx",
			Observation:         "obs",
			ReusableLesson:      "lesson",
			Confidence:          domain.ConfidenceHigh,
			EvidenceLevel:       domain.EvidenceModerate,
			ProposedDestination: domain.DestProject,
			RetrievalTerms:      terms,
			Fingerprint:         "fp-" + id,
			NormalizedHash:      "nh-" + id,
			Actor:               domain.Actor{Kind: "system", Name: "test"},
			Revision:            1,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		saveLearning(t, db, projectID, l)
	}
	// Query contains "kubernetes"; L-overlap has it in retrieval_terms
	// AND the title; L-no has it ONLY in the title.
	mk("L-overlap", "kubernetes deployment guide", []string{"kubernetes", "deployment", "guide"})
	mk("L-no", "kubernetes rollout checklist", []string{"rollout", "checklist"})

	svc := newService(t, db, retrieval.DefaultWeights(), now)
	res, err := svc.Search(context.Background(), retrieval.Query{
		Text:      "kubernetes",
		ProjectID: projectID,
		Limit:     5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Hits) < 2 {
		t.Fatalf("need >= 2 hits, got %d", len(res.Hits))
	}
	if res.Hits[0].Learning.ID != domain.LearningID("L-overlap") {
		t.Fatalf("top hit = %s, want L-overlap", res.Hits[0].Learning.ID)
	}
	if res.Hits[0].Components.RetrievalTerms != 1.0 {
		t.Fatalf("retrieval_terms = %f, want 1.0", res.Hits[0].Components.RetrievalTerms)
	}
}

// TestService_EvidenceLevelScore verifies the per-level mapping:
// strong=1.0, moderate=0.7, weak=0.4, insufficient=0.1.
func TestService_EvidenceLevelScore(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "evidence-level")
	now := fixedNow()

	cases := []struct {
		name  string
		level domain.EvidenceLevel
		want  float64
	}{
		{"strong", domain.EvidenceStrong, 1.0},
		{"moderate", domain.EvidenceModerate, 0.7},
		{"weak", domain.EvidenceWeak, 0.4},
		{"insufficient", domain.EvidenceInsufficient, 0.1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Subtests share a single SQLite handle; do not parallelize
			// them or SQLITE_BUSY races the writes.
			id := domain.LearningID("L-ev-" + tc.name)
			l := &domain.Learning{
				ID:                  id,
				ProjectID:           projectID,
				Status:              domain.StatusApproved,
				Type:                domain.TypeProcedure,
				Title:               "evidence test " + tc.name,
				Context:             "ctx",
				Observation:         "obs",
				ReusableLesson:      "lesson",
				Confidence:          domain.ConfidenceHigh,
				EvidenceLevel:       tc.level,
				ProposedDestination: domain.DestProject,
				RetrievalTerms:      []string{"evidencetest"},
				Fingerprint:         "fp-" + tc.name,
				NormalizedHash:      "nh-" + tc.name,
				Actor:               domain.Actor{Kind: "system", Name: "test"},
				Revision:            1,
				CreatedAt:           now,
				UpdatedAt:           now,
			}
			saveLearning(t, db, projectID, l)

			svc := newService(t, db, retrieval.DefaultWeights(), now)
			res, err := svc.Search(context.Background(), retrieval.Query{
				Text:      "evidencetest",
				ProjectID: projectID,
				Limit:     5,
			})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(res.Hits) == 0 {
				t.Fatal("no hits")
			}
			for _, h := range res.Hits {
				if h.Learning.ID == id {
					if h.Components.EvidenceLevel != tc.want {
						t.Fatalf("evidence_level[%s] = %f, want %f", tc.name, h.Components.EvidenceLevel, tc.want)
					}
					return
				}
			}
			t.Fatalf("learning %s not found in hits", id)
		})
	}
}
