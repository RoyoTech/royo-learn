package coherence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-royo-learn/internal/record"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/testutil"

	"github.com/google/uuid"
)

func newCoherenceDB(t *testing.T) (*storage.DB, string, domain.ProjectID) {
	t.Helper()
	dir, err := os.MkdirTemp("", "coherence-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	path := filepath.Join(dir, "royo-learn.db")
	db, err := storage.Open(path)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("Open: %v", err)
	}
	if err := storage.Migrate(db); err != nil {
		db.Close()
		os.RemoveAll(dir)
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_ = testutil.RemoveAllWithRetry(dir)
	})
	proj := &domain.Project{
		ID:            domain.ProjectID(uuid.Must(uuid.NewV7()).String()),
		ProjectKey:    "coherence-project",
		DisplayName:   "Coherence Project",
		CanonicalPath: dir,
		Fingerprint:   "fp",
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
		UpdatedAt:     time.Now().UTC().Truncate(time.Second),
	}
	ctx := context.Background()
	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := storage.SaveProject(ctx, tx, proj); err != nil {
		tx.Rollback()
		t.Fatalf("SaveProject: %v", err)
	}
	tx.Commit()
	return db, dir, proj.ID
}

func saveLearning(t *testing.T, db *storage.DB, projectID domain.ProjectID, title string) *domain.Learning {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	l := &domain.Learning{
		ID:                  domain.LearningID(uuid.Must(uuid.NewV7()).String()),
		ProjectID:           projectID,
		Status:              domain.StatusCaptured,
		Type:                domain.TypeProcedure,
		Title:               title,
		Context:             "ctx " + title,
		Observation:         "obs " + title,
		ReusableLesson:      "lesson " + title,
		ScopeGuess:          domain.ScopeProject,
		Confidence:          domain.ConfidenceMedium,
		EvidenceLevel:       domain.EvidenceModerate,
		ProposedDestination: domain.DestProject,
		Fingerprint:         "fp-" + title,
		NormalizedHash:      "hash-" + title,
		Actor:               domain.Actor{Kind: "agent", Name: "seed"},
		Revision:            1,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	ctx := context.Background()
	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := storage.SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	tx.Commit()
	return l
}

// TestAudit_DetectsEveryDivergenceKind is the detection half of the §4.7
// contract: doctor's coherence audit must surface a missing record, a divergent
// record, and an orphan record.
func TestAudit_DetectsEveryDivergenceKind(t *testing.T) {
	db, dir, projectID := newCoherenceDB(t)
	recordsDir := filepath.Join(dir, "records")
	ctx := context.Background()

	l1 := saveLearning(t, db, projectID, "coherent")
	l2 := saveLearning(t, db, projectID, "divergent")
	l3 := saveLearning(t, db, projectID, "missing")

	// l1: a faithful record. l2: a record, then mutate SQLite so it diverges.
	if err := record.WriteRecord(recordsDir, l1); err != nil {
		t.Fatalf("WriteRecord l1: %v", err)
	}
	if err := record.WriteRecord(recordsDir, l2); err != nil {
		t.Fatalf("WriteRecord l2: %v", err)
	}
	// Change l2's title in SQLite WITHOUT rewriting its record -> divergence.
	if _, err := db.DB.Exec("UPDATE learnings SET title = 'changed' WHERE id = ?", string(l2.ID)); err != nil {
		t.Fatalf("mutate l2: %v", err)
	}
	// l3: no record at all -> missing.
	// Orphan: a record with no learning behind it.
	orphan := &domain.Learning{
		ID: domain.LearningID(uuid.Must(uuid.NewV7()).String()), Status: domain.StatusCaptured,
		Type: domain.TypeProcedure, Title: "orphan", Context: "c", Observation: "o", ReusableLesson: "l",
	}
	if err := record.WriteRecord(recordsDir, orphan); err != nil {
		t.Fatalf("WriteRecord orphan: %v", err)
	}
	_ = l3

	divergences, err := Audit(ctx, db, projectID, recordsDir)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	got := map[DivergenceKind]int{}
	for _, d := range divergences {
		got[d.Kind]++
	}
	if got[MissingRecord] != 1 {
		t.Errorf("missing records = %d, want 1", got[MissingRecord])
	}
	if got[DivergentRecord] != 1 {
		t.Errorf("divergent records = %d, want 1", got[DivergentRecord])
	}
	if got[OrphanRecord] != 1 {
		t.Errorf("orphan records = %d, want 1", got[OrphanRecord])
	}
}

// TestRepair_RestoresCoherence is the repair half: rebuild-index must make every
// divergence disappear, materializing from SQLite (the truth) and dropping
// orphans.
func TestRepair_RestoresCoherence(t *testing.T) {
	db, dir, projectID := newCoherenceDB(t)
	recordsDir := filepath.Join(dir, "records")
	ctx := context.Background()

	l1 := saveLearning(t, db, projectID, "one")
	saveLearning(t, db, projectID, "two")
	// Only l1 gets a (stale) record; the rest are missing; add an orphan too.
	if err := record.WriteRecord(recordsDir, l1); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}
	if _, err := db.DB.Exec("UPDATE learnings SET title = 'changed' WHERE id = ?", string(l1.ID)); err != nil {
		t.Fatalf("mutate: %v", err)
	}
	orphan := &domain.Learning{
		ID: domain.LearningID(uuid.Must(uuid.NewV7()).String()), Status: domain.StatusCaptured,
		Type: domain.TypeProcedure, Title: "orphan", Context: "c", Observation: "o", ReusableLesson: "l",
	}
	if err := record.WriteRecord(recordsDir, orphan); err != nil {
		t.Fatalf("WriteRecord orphan: %v", err)
	}

	before, _ := Audit(ctx, db, projectID, recordsDir)
	if len(before) == 0 {
		t.Fatal("test setup produced no divergences to repair")
	}

	res, err := Repair(ctx, db, projectID, recordsDir)
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}
	if res.RecordsWritten != 2 {
		t.Errorf("RecordsWritten = %d, want 2", res.RecordsWritten)
	}
	if res.OrphansRemoved != 1 {
		t.Errorf("OrphansRemoved = %d, want 1", res.OrphansRemoved)
	}

	after, err := Audit(ctx, db, projectID, recordsDir)
	if err != nil {
		t.Fatalf("Audit after repair: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("divergences after repair = %d, want 0: %+v", len(after), after)
	}
}

// TestOutbox_MaterializationWindowIsRecoverable is the cut test that determines
// whether an outbox is required (plan §4.7 hard stop). It reproduces the exact
// failure window D6 warns about: SQLite has committed the learning but the
// Markdown materialization was lost (here: the record file never landed / was
// deleted after the commit). It then proves the window is fully recoverable by
// doctor detection + rebuild-index repair, WITHOUT any outbox or queue.
func TestOutbox_MaterializationWindowIsRecoverable(t *testing.T) {
	db, dir, projectID := newCoherenceDB(t)
	recordsDir := filepath.Join(dir, "records")
	ctx := context.Background()

	// SQLite commits the learning; the record write is lost (crash after commit).
	l := saveLearning(t, db, projectID, "committed-but-not-materialized")

	// The window exists and is DETECTABLE.
	divergences, err := Audit(ctx, db, projectID, recordsDir)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if len(divergences) != 1 || divergences[0].Kind != MissingRecord || divergences[0].LearningID != string(l.ID) {
		t.Fatalf("expected exactly one missing-record divergence for %s, got %+v", l.ID, divergences)
	}

	// The window is RECOVERABLE without an outbox: rebuild-index re-materializes.
	if _, err := Repair(ctx, db, projectID, recordsDir); err != nil {
		t.Fatalf("Repair: %v", err)
	}
	after, err := Audit(ctx, db, projectID, recordsDir)
	if err != nil {
		t.Fatalf("Audit after repair: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("materialization window NOT recovered by rebuild-index: %+v", after)
	}
	if _, statErr := os.Stat(filepath.Join(recordsDir, string(l.ID)+".md")); statErr != nil {
		t.Fatalf("record not materialized after recovery: %v", statErr)
	}
}
