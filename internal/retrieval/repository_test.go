// Repository tests for Hito 9 (slice 9.1).
//
// The repository is a thin wrapper over the FTS5 virtual table
// (learnings_fts, migration 001). It must:
//
//   - Return Candidates with the bm25() raw score attached.
//   - Filter by project_id so two projects' learnings are isolated.
//   - Return [] without error when no terms match.
//   - Return [] when terms is empty.
//   - Re-read the updated row after an UPDATE on the underlying
//     learnings table (the FTS5 trigger DELETE+INSERTs on UPDATE,
//     so the new row should be visible after the update).
//
// The tests use storagetest.OpenTemp so the migration pipeline runs
// exactly as in production.

package retrieval_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/retrieval"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/storage/storagetest"
)

// newProjectFixture seeds a project row and returns its ID. Mirrors
// the helper in internal/experience/patterns/storage_test.go but
// lives here so the retrieval tests are self-contained.
func newProjectFixture(t *testing.T, db *storage.DB, name string) domain.ProjectID {
	t.Helper()
	canonical := filepath.Join(t.TempDir(), name)
	project := &domain.Project{
		ID:            domain.ProjectID("proj-" + strings.ReplaceAll(name, " ", "-")),
		ProjectKey:    "key-" + name,
		DisplayName:   name,
		CanonicalPath: canonical,
		Fingerprint:   "fp-" + name,
		CreatedAt:     time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if err := storage.SaveProject(ctx, tx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return project.ID
}

// saveLearning inserts a learning row directly so we don't need the
// full capture.Service for repository tests.
func saveLearning(t *testing.T, db *storage.DB, projectID domain.ProjectID, l *domain.Learning) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("saveLearning BeginTx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if err := storage.SaveLearning(ctx, tx, l); err != nil {
		t.Fatalf("saveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("saveLearning commit: %v", err)
	}
}

// seedLearnings inserts a handful of learnings with distinct titles,
// evidence levels and retrieval_terms so the repository tests have
// realistic data to search.
func seedLearnings(t *testing.T, db *storage.DB, projectID domain.ProjectID) (strong, moderate, weak string) {
	t.Helper()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	type seed struct {
		title          string
		retrievalTerms []string
		evidenceLevel  domain.EvidenceLevel
	}
	seeds := []seed{
		{
			title:          "Strong evidence about lexical retrieval",
			retrievalTerms: []string{"lexical", "retrieval", "fts"},
			evidenceLevel:  domain.EvidenceStrong,
		},
		{
			title:          "Moderate evidence about unicode tokenization",
			retrievalTerms: []string{"unicode", "tokenization", "i18n"},
			evidenceLevel:  domain.EvidenceModerate,
		},
		{
			title:          "Weak evidence about an unrelated topic",
			retrievalTerms: []string{"deployment", "kubernetes"},
			evidenceLevel:  domain.EvidenceWeak,
		},
	}
	ids := make([]domain.LearningID, 0, len(seeds))
	for idx, s := range seeds {
		retrievalJSON, _ := json.Marshal(s.retrievalTerms)
		l := &domain.Learning{
			ID:                  domain.LearningID("L-" + string(projectID) + "-" + string(rune('a'+idx))),
			ProjectID:           projectID,
			Status:              domain.StatusApproved,
			Type:                domain.TypeProcedure,
			Title:               s.title,
			Context:             "context",
			Observation:         "observation",
			ReusableLesson:      "lesson",
			Confidence:          domain.ConfidenceHigh,
			EvidenceLevel:       s.evidenceLevel,
			ProposedDestination: domain.DestProject,
			RetrievalTerms:      s.retrievalTerms,
			Fingerprint:         "fp-" + string(projectID) + "-" + string(rune('a'+idx)),
			NormalizedHash:      "nh-" + string(projectID) + "-" + string(rune('a'+idx)),
			Actor:               domain.Actor{Kind: "system", Name: "test"},
			Revision:            1,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		// marshalStringSlice does the same as storage internals; we
		// go through SaveLearning for consistency.
		l.RetrievalTerms = retrievalTermsToBacking(s.retrievalTerms)
		_ = retrievalJSON // keep the marshalling intent obvious
		saveLearning(t, db, projectID, l)
		ids = append(ids, l.ID)
	}
	return string(ids[0]), string(ids[1]), string(ids[2])
}

// retrievalTermsToBacking is a tiny helper that mirrors the JSON
// shape storage uses to persist RetrievalTerms. SaveLearning
// marshals via marshalStringSlice, so any []string works as long
// as it is not nil; we keep this thin to avoid duplicating
// storage internals.
func retrievalTermsToBacking(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// TestRepository_Search_EmptyTermsReturnsEmpty verifies the
// "no terms" early-exit path.
func TestRepository_Search_EmptyTermsReturnsEmpty(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "empty-terms")
	seedLearnings(t, db, projectID)

	repo := retrieval.NewRepository(db)
	got, err := repo.Search(context.Background(), projectID, nil, 10)
	if err != nil {
		t.Fatalf("Search(nil) error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Search(nil) = %d candidates, want 0", len(got))
	}
}

// TestRepository_Search_NoMatchesReturnsEmpty verifies the
// "FTS5 matched nothing" path is not an error.
func TestRepository_Search_NoMatchesReturnsEmpty(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "no-match")
	seedLearnings(t, db, projectID)

	repo := retrieval.NewRepository(db)
	got, err := repo.Search(context.Background(), projectID, []string{"nonexistenttermxyz"}, 10)
	if err != nil {
		t.Fatalf("Search(no-match) error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Search(no-match) = %d candidates, want 0", len(got))
	}
}

// TestRepository_Search_SingleMatch verifies the happy path: one
// term in the query matches one learning in the FTS5 index.
func TestRepository_Search_SingleMatch(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "single")
	seedLearnings(t, db, projectID)

	repo := retrieval.NewRepository(db)
	got, err := repo.Search(context.Background(), projectID, []string{"unicode"}, 10)
	if err != nil {
		t.Fatalf("Search(unicode) error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Search(unicode) = %d candidates, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Title, "unicode") {
		t.Fatalf("matched title = %q, want unicode-related", got[0].Title)
	}
}

// TestRepository_Search_RespectsProjectFilter verifies the
// project_id filter: a learning that matches the term but lives
// in another project is NOT returned.
func TestRepository_Search_RespectsProjectFilter(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectA := newProjectFixture(t, db, "projA")
	projectB := newProjectFixture(t, db, "projB")
	seedLearnings(t, db, projectA)
	seedLearnings(t, db, projectB)

	repo := retrieval.NewRepository(db)
	got, err := repo.Search(context.Background(), projectA, []string{"unicode"}, 10)
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	for _, c := range got {
		if c.ProjectID != projectA {
			t.Fatalf("got candidate %s in project %s, want %s", c.ID, c.ProjectID, projectA)
		}
	}
}

// TestRepository_Search_BM25Populated verifies the bm25() raw score
// is non-zero for matching rows. A zero score would mean the JOIN
// is missing or the FTS5 helper is silently returning 0.
func TestRepository_Search_BM25Populated(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "bm25")
	seedLearnings(t, db, projectID)

	repo := retrieval.NewRepository(db)
	got, err := repo.Search(context.Background(), projectID, []string{"unicode"}, 10)
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(got) == 0 {
		t.Fatal("Search returned no candidates; cannot verify BM25")
	}
	if got[0].BM25Score == 0 {
		t.Fatalf("BM25Score = 0, want non-zero for a real match")
	}
}

// TestRepository_Search_TriggerRebuildAfterUpdate verifies the
// FTS5 trigger (learnings_au DELETE+INSERT) keeps the index in
// sync: an UPDATE that changes the title is visible to the next
// Search without an explicit rebuild.
func TestRepository_Search_TriggerRebuildAfterUpdate(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "trigger")
	_, moderate, _ := seedLearnings(t, db, projectID)

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	l := &domain.Learning{
		ID:                  domain.LearningID(moderate),
		ProjectID:           projectID,
		Status:              domain.StatusApproved,
		Type:                domain.TypeProcedure,
		Title:               "Renamed title about kubernetes pipelines",
		Context:             "context",
		Observation:         "observation",
		ReusableLesson:      "lesson",
		Confidence:          domain.ConfidenceHigh,
		EvidenceLevel:       domain.EvidenceModerate,
		ProposedDestination: domain.DestProject,
		RetrievalTerms:      []string{"deployment", "kubernetes", "pipelines"},
		Fingerprint:         "fp-Rena-" + moderate,
		NormalizedHash:      "nh-Rena-" + moderate,
		Actor:               domain.Actor{Kind: "system", Name: "test"},
		Revision:            2,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := storage.UpdateLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("UpdateLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	repo := retrieval.NewRepository(db)
	got, err := repo.Search(context.Background(), projectID, []string{"pipelines"}, 10)
	if err != nil {
		t.Fatalf("Search(pipelines) error = %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("Search(pipelines) = 0 candidates; trigger did not rebuild the FTS5 row")
	}
	if !strings.Contains(got[0].Title, "kubernetes") {
		t.Fatalf("matched title = %q, want one that contains kubernetes", got[0].Title)
	}
}

// TestRepository_Search_LimitIsApplied verifies the LIMIT clause
// caps the number of returned candidates.
func TestRepository_Search_LimitIsApplied(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	projectID := newProjectFixture(t, db, "limit")
	seedLearnings(t, db, projectID)

	// Add 5 more learnings that all share the same token so the
	// search yields many candidates.
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		l := &domain.Learning{
			ID:                  domain.LearningID("L-extra-" + string(rune('a'+i))),
			ProjectID:           projectID,
			Status:              domain.StatusApproved,
			Type:                domain.TypeProcedure,
			Title:               "extra lexical retrieval note " + string(rune('a'+i)),
			Context:             "context",
			Observation:         "observation",
			ReusableLesson:      "lesson",
			Confidence:          domain.ConfidenceHigh,
			EvidenceLevel:       domain.EvidenceModerate,
			ProposedDestination: domain.DestProject,
			RetrievalTerms:      []string{"lexical", "retrieval"},
			Fingerprint:         "fp-extra-" + string(rune('a'+i)),
			NormalizedHash:      "nh-extra-" + string(rune('a'+i)),
			Actor:               domain.Actor{Kind: "system", Name: "test"},
			Revision:            1,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		saveLearning(t, db, projectID, l)
	}

	repo := retrieval.NewRepository(db)
	got, err := repo.Search(context.Background(), projectID, []string{"lexical"}, 3)
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(got) > 3 {
		t.Fatalf("Search(limit=3) returned %d candidates, want <= 3", len(got))
	}
}

// keep sql import used by future expansion without leaving a stale import.
var _ = sql.LevelDefault
