// Benchmark tests for Hito 9.
//
// Goal: measure ns/op, allocs/op and MB/s for the retrieval
// pipeline so we can validate the docs/12 §108 gate:
//
//	p95 < 250ms with 1000 learnings on a single Windows/Linux/macOS
//	workstation (CI runs the same check).
//
// The benchmarks use the in-memory storagetest helper for speed
// (no filesystem overhead). Run with:
//
//	go test -bench=BenchmarkService_Search -benchmem -count=3 ./internal/retrieval/
//
// A 1000-learning dataset is seeded once per benchmark run via
// b.ResetTimer.

package retrieval_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/retrieval"
	"agent-royo-learn/internal/storage"
)

// seedLargeCorpus inserts n synthetic learnings with deterministic
// content. Each learning has a few unique retrieval terms so the
// query "alpha beta gamma" matches a known slice of the corpus.
func seedLargeCorpus(b *testing.B, db *storage.DB, projectID domain.ProjectID, n int) {
	b.Helper()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	for i := 0; i < n; i++ {
		title := fmt.Sprintf("Learning %d about alpha beta gamma and delta epsilon", i)
		l := &domain.Learning{
			ID:                  domain.LearningID(fmt.Sprintf("L-bench-%d", i)),
			ProjectID:           projectID,
			Status:              domain.StatusApproved,
			Type:                domain.TypeProcedure,
			Title:               title,
			Context:             fmt.Sprintf("Context for %d", i),
			Observation:         fmt.Sprintf("Observation %d", i),
			ReusableLesson:      fmt.Sprintf("Lesson %d", i),
			Confidence:          domain.ConfidenceHigh,
			EvidenceLevel:       domain.EvidenceModerate,
			ProposedDestination: domain.DestProject,
			RetrievalTerms:      []string{"alpha", "beta", "gamma", fmt.Sprintf("term-%d", i%10)},
			Fingerprint:         fmt.Sprintf("fp-bench-%d", i),
			NormalizedHash:      fmt.Sprintf("nh-bench-%d", i),
			Actor:               domain.Actor{Kind: "system", Name: "bench"},
			Revision:            1,
			CreatedAt:           now.Add(-time.Duration(i) * time.Minute),
			UpdatedAt:           now.Add(-time.Duration(i) * time.Minute),
		}
		tx, err := db.DB.BeginTx(ctx, nil)
		if err != nil {
			b.Fatalf("seed BeginTx: %v", err)
		}
		if err := storage.SaveLearning(ctx, tx, l); err != nil {
			tx.Rollback()
			b.Fatalf("seed SaveLearning[%d]: %v", i, err)
		}
		if err := tx.Commit(); err != nil {
			b.Fatalf("seed commit[%d]: %v", i, err)
		}
	}
}

// BenchmarkService_Search measures the full pipeline: Sanitize →
// Repository.Search → scoreAll → sortHitsDeterministic → slice.
//
// Dataset: 1000 synthetic learnings (docs/12 §108).
// Query: "alpha beta gamma" (3 terms).
func BenchmarkService_Search(b *testing.B) {
	db, projectID := newBenchDB(b, "bench-search")
	seedLargeCorpus(b, db, projectID, 1000)

	svc := retrieval.NewService(retrieval.NewRepository(db), retrieval.DefaultWeights())
	svc.SetNow(func() time.Time { return time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC) })

	q := retrieval.Query{
		Text:      "alpha beta gamma",
		ProjectID: projectID,
		Limit:     50,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := svc.Search(context.Background(), q); err != nil {
			b.Fatalf("Search: %v", err)
		}
	}
}

// BenchmarkRepository_Search measures only the SQL layer (no
// scoring, no sort).
func BenchmarkRepository_Search(b *testing.B) {
	db, projectID := newBenchDB(b, "bench-repo")
	seedLargeCorpus(b, db, projectID, 1000)

	repo := retrieval.NewRepository(db)
	terms := []string{"alpha", "beta", "gamma"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := repo.Search(context.Background(), projectID, terms, 50); err != nil {
			b.Fatalf("repo.Search: %v", err)
		}
	}
}

// BenchmarkSanitize measures the Sanitize() helper in isolation.
func BenchmarkSanitize(b *testing.B) {
	query := "alpha beta gamma delta epsilon"
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := retrieval.Sanitize(query); err != nil {
			b.Fatalf("Sanitize: %v", err)
		}
	}
}

// newBenchDB opens an in-memory SQLite database with migrations
// applied, seeds one project, and registers cleanup. The
// storagetest helper requires *testing.T; this variant handles
// *testing.B for benchmarks.
func newBenchDB(b *testing.B, name string) (*storage.DB, domain.ProjectID) {
	b.Helper()
	dsn := fmt.Sprintf("file:bench-%s?mode=memory&cache=shared", name)
	db, err := storage.Open(dsn)
	if err != nil {
		b.Fatalf("storage.Open: %v", err)
	}
	if err := storage.Migrate(db); err != nil {
		db.Close()
		b.Fatalf("storage.Migrate: %v", err)
	}
	b.Cleanup(func() {
		if err := db.Close(); err != nil {
			b.Errorf("bench db close: %v", err)
		}
	})

	canonical := name + "-dir"
	project := &domain.Project{
		ID:            domain.ProjectID("proj-" + name),
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
		b.Fatalf("BeginTx: %v", err)
	}
	if err := storage.SaveProject(ctx, tx, project); err != nil {
		tx.Rollback()
		b.Fatalf("SaveProject: %v", err)
	}
	if err := tx.Commit(); err != nil {
		b.Fatalf("commit: %v", err)
	}
	return db, project.ID
}
