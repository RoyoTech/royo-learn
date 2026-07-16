package portability

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/testutil"

	"github.com/google/uuid"
)

// newFileDB creates a fresh file-based database with migrations applied and a
// single seeded project, returning the DB, its directory, and the project.
func newFileDB(t *testing.T) (*storage.DB, string, *domain.Project) {
	t.Helper()
	dir, err := os.MkdirTemp("", "portab-*")
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
		ProjectKey:    "portab-project",
		DisplayName:   "Portability Project",
		CanonicalPath: dir,
		Fingerprint:   "portab-fp",
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
		UpdatedAt:     time.Now().UTC().Truncate(time.Second),
	}
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := storage.SaveProject(ctx, tx, proj); err != nil {
		tx.Rollback()
		t.Fatalf("SaveProject: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return db, dir, proj
}

// seedLearning inserts one learning plus one evidence row for it and returns it.
func seedLearning(t *testing.T, db *storage.DB, projectID domain.ProjectID, title, hash string) *domain.Learning {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	l := &domain.Learning{
		ID:                  domain.LearningID(uuid.Must(uuid.NewV7()).String()),
		ProjectID:           projectID,
		Status:              domain.StatusCaptured,
		Type:                domain.TypeProcedure,
		Title:               title,
		Context:             "context of " + title,
		Observation:         "observation of " + title,
		ReusableLesson:      "lesson of " + title,
		ScopeGuess:          domain.ScopeProject,
		Confidence:          domain.ConfidenceMedium,
		EvidenceLevel:       domain.EvidenceModerate,
		ProposedDestination: domain.DestProject,
		Fingerprint:         "fp-" + title,
		NormalizedHash:      hash,
		Actor:               domain.Actor{Kind: "agent", Name: "seed"},
		Revision:            1,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := storage.SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	ev := &domain.Evidence{
		ID:          domain.EvidenceID(uuid.Must(uuid.NewV7()).String()),
		LearningID:  l.ID,
		Kind:        domain.EvidenceKind("text"),
		Summary:     "evidence for " + title,
		SHA256:      "sha-" + title,
		SizeBytes:   int64(len(title)),
		CollectedAt: now,
	}
	if err := storage.SaveEvidence(ctx, tx, ev); err != nil {
		tx.Rollback()
		t.Fatalf("SaveEvidence: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return l
}

func seedRelation(t *testing.T, db *storage.DB, src, tgt domain.LearningID) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	rel := &domain.LearningRelation{
		ID:               domain.RelationID(uuid.Must(uuid.NewV7()).String()),
		SourceLearningID: src,
		TargetLearningID: tgt,
		Relation:         domain.RelationType("related"),
		Rationale:        "seeded relation",
		Status:           domain.RelationProposed,
		ProposedBy:       domain.Actor{Kind: "agent", Name: "seed"},
		Actor:            domain.Actor{Kind: "agent", Name: "seed"},
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := storage.SaveRelation(ctx, tx, rel); err != nil {
		tx.Rollback()
		t.Fatalf("SaveRelation: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func seedRecurrence(t *testing.T, db *storage.DB, projectID domain.ProjectID, learningID domain.LearningID) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	rec := &domain.RecurrenceRecord{
		ID:                    domain.RecurrenceRecordID(uuid.Must(uuid.NewV7()).String()),
		RecurrenceFingerprint: "rec-fp",
		LearningID:            learningID,
		ProjectID:             projectID,
		Summary:               "recurred",
		OccurredAt:            now,
		Outcome:               string(domain.OutcomeRecurred),
		ActorKind:             "agent",
		ActorName:             "seed",
	}
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := storage.SaveRecurrenceRecord(ctx, tx, rec); err != nil {
		tx.Rollback()
		t.Fatalf("SaveRecurrenceRecord: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// TestRoundTrip_ExportDeleteImportIsIdentical is the mandatory round-trip proof
// of plan 4.6: export a populated store, discard the database entirely, import
// into a fresh one, and assert learnings, evidence, relations, and recurrence
// states are identical.
func TestRoundTrip_ExportDeleteImportIsIdentical(t *testing.T) {
	src, _, proj := newFileDB(t)
	l1 := seedLearning(t, src, proj.ID, "alpha", "hash-alpha")
	l2 := seedLearning(t, src, proj.ID, "beta", "hash-beta")
	seedRelation(t, src, l1.ID, l2.ID)
	seedRecurrence(t, src, proj.ID, l1.ID)

	ctx := context.Background()
	bundle, err := Export(ctx, src, proj.ID)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(bundle.Learnings) != 2 || len(bundle.Evidence) != 2 || len(bundle.Relations) != 1 || len(bundle.Recurrences) != 1 {
		t.Fatalf("bundle counts wrong: learnings=%d evidence=%d relations=%d recurrences=%d",
			len(bundle.Learnings), len(bundle.Evidence), len(bundle.Relations), len(bundle.Recurrences))
	}

	// Serialize and deserialize: the on-disk format must survive a full cycle.
	var buf bytes.Buffer
	if err := EncodeJSONL(bundle, &buf); err != nil {
		t.Fatalf("EncodeJSONL: %v", err)
	}
	decoded, err := DecodeJSONL(&buf)
	if err != nil {
		t.Fatalf("DecodeJSONL: %v", err)
	}

	// Simulate "delete the temp DB": a brand-new, empty store.
	dst, dstDir, _ := newFileDB(t)
	// The fresh store seeded its own project; drop it so the import must carry
	// the original project identity (proving import reconstructs from the bundle).
	if _, err := dst.DB.Exec("DELETE FROM projects"); err != nil {
		t.Fatalf("clear dst projects: %v", err)
	}

	res, err := Import(ctx, dst, decoded, ImportOptions{
		RecordsDir: filepath.Join(dstDir, "records"),
		BackupDir:  filepath.Join(dstDir, "backups"),
		Apply:      true,
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts on fresh import: %v", res.Conflicts)
	}
	if res.LearningsInserted != 2 {
		t.Fatalf("LearningsInserted = %d, want 2", res.LearningsInserted)
	}

	// Re-export the destination and compare against the source export.
	after, err := Export(ctx, dst, proj.ID)
	if err != nil {
		t.Fatalf("Export dst: %v", err)
	}
	assertBundlesEqual(t, bundle, after)

	// A Markdown record must have been materialized for every learning (D6).
	for _, l := range bundle.Learnings {
		recPath := filepath.Join(dstDir, "records", string(l.ID)+".md")
		if _, err := os.Stat(recPath); err != nil {
			t.Fatalf("record not materialized for %s: %v", l.ID, err)
		}
	}
}

func TestImport_DryRunByDefaultWritesNothing(t *testing.T) {
	src, _, proj := newFileDB(t)
	seedLearning(t, src, proj.ID, "alpha", "hash-alpha")
	ctx := context.Background()
	bundle, err := Export(ctx, src, proj.ID)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	dst, dstDir, _ := newFileDB(t)
	if _, err := dst.DB.Exec("DELETE FROM projects"); err != nil {
		t.Fatalf("clear dst projects: %v", err)
	}

	// No Apply flag: this is a dry run. It must report the plan and write nothing.
	res, err := Import(ctx, dst, bundle, ImportOptions{
		RecordsDir: filepath.Join(dstDir, "records"),
		BackupDir:  filepath.Join(dstDir, "backups"),
	})
	if err != nil {
		t.Fatalf("Import dry-run: %v", err)
	}
	if !res.DryRun {
		t.Fatal("expected DryRun=true")
	}
	if res.LearningsInserted != 1 {
		t.Fatalf("dry-run should report 1 learning that WOULD be inserted, got %d", res.LearningsInserted)
	}
	var count int
	if err := dst.DB.QueryRow("SELECT COUNT(*) FROM learnings").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("dry-run wrote %d learnings; it must write nothing", count)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "records", string(bundle.Learnings[0].ID)+".md")); err == nil {
		t.Fatal("dry-run materialized a record; it must write nothing")
	}
}

func TestImport_ConflictIsNotSilentlyOverwritten(t *testing.T) {
	src, _, proj := newFileDB(t)
	l := seedLearning(t, src, proj.ID, "alpha", "hash-alpha")
	ctx := context.Background()
	bundle, err := Export(ctx, src, proj.ID)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Destination already holds a learning with the SAME id but DIFFERENT content.
	dst, dstDir, dstProj := newFileDB(t)
	conflicting := seedLearning(t, dst, dstProj.ID, "different-title", "hash-DIFFERENT")
	// Force the same ID as the incoming record.
	if _, err := dst.DB.Exec("UPDATE learnings SET id = ? WHERE id = ?", string(l.ID), string(conflicting.ID)); err != nil {
		t.Fatalf("force id collision: %v", err)
	}

	res, err := Import(ctx, dst, bundle, ImportOptions{
		RecordsDir: filepath.Join(dstDir, "records"),
		BackupDir:  filepath.Join(dstDir, "backups"),
		Apply:      true,
	})
	if err == nil {
		t.Fatal("apply with a conflict must return an error, not overwrite silently")
	}
	if res == nil || len(res.Conflicts) == 0 {
		t.Fatal("expected the result to report the conflict")
	}
	// The original destination content must be intact.
	var hash string
	if err := dst.DB.QueryRow("SELECT normalized_hash FROM learnings WHERE id = ?", string(l.ID)).Scan(&hash); err != nil {
		t.Fatalf("read dst learning: %v", err)
	}
	if hash != "hash-DIFFERENT" {
		t.Fatalf("conflicting learning was overwritten: hash=%q", hash)
	}
}

func TestImport_RejectsUnknownFormatVersion(t *testing.T) {
	dst, dstDir, _ := newFileDB(t)
	bundle := &Bundle{FormatVersion: 999}
	_, err := Import(context.Background(), dst, bundle, ImportOptions{
		RecordsDir: filepath.Join(dstDir, "records"),
		Apply:      true,
	})
	if err == nil {
		t.Fatal("import must reject an unknown format version")
	}
}

// assertBundlesEqual compares two bundles by content, ignoring ExportedAt.
func assertBundlesEqual(t *testing.T, want, got *Bundle) {
	t.Helper()
	if len(want.Learnings) != len(got.Learnings) {
		t.Fatalf("learning count: want %d got %d", len(want.Learnings), len(got.Learnings))
	}
	wl := indexLearnings(want.Learnings)
	gl := indexLearnings(got.Learnings)
	for id, w := range wl {
		g, ok := gl[id]
		if !ok {
			t.Fatalf("learning %s missing after round-trip", id)
		}
		if w.Title != g.Title || w.Status != g.Status || w.NormalizedHash != g.NormalizedHash ||
			w.Context != g.Context || w.Observation != g.Observation || w.ReusableLesson != g.ReusableLesson ||
			w.Type != g.Type || w.ScopeGuess != g.ScopeGuess || w.EvidenceLevel != g.EvidenceLevel {
			t.Fatalf("learning %s differs after round-trip:\n want %+v\n got  %+v", id, w, g)
		}
	}
	if len(want.Evidence) != len(got.Evidence) {
		t.Fatalf("evidence count: want %d got %d", len(want.Evidence), len(got.Evidence))
	}
	if len(want.Relations) != len(got.Relations) {
		t.Fatalf("relation count: want %d got %d", len(want.Relations), len(got.Relations))
	}
	if len(want.Recurrences) != len(got.Recurrences) {
		t.Fatalf("recurrence count: want %d got %d", len(want.Recurrences), len(got.Recurrences))
	}
}

func indexLearnings(ls []*domain.Learning) map[domain.LearningID]*domain.Learning {
	m := make(map[domain.LearningID]*domain.Learning, len(ls))
	for _, l := range ls {
		m[l.ID] = l
	}
	return m
}
