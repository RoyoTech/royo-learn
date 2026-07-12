package curate

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/testutil"
)

// saveLearningForRelation saves a minimal learning for relation testing.
func saveLearningForRelation(t *testing.T, db *storage.DB, proj *domain.Project, title string) *domain.Learning {
	t.Helper()

	now := time.Now().UTC()
	learning := &domain.Learning{
		ID:                  domain.LearningID("rel-test-" + title),
		ProjectID:           proj.ID,
		Status:              domain.StatusCaptured,
		Type:                domain.TypeProcedure,
		Title:               title,
		Context:             "Relation testing",
		Observation:         "Testing relation creation",
		ReusableLesson:      "Test relations thoroughly",
		ScopeGuess:          domain.ScopeProject,
		Confidence:          domain.ConfidenceMedium,
		EvidenceLevel:       domain.EvidenceModerate,
		ProposedDestination: domain.DestProject,
		NormalizedHash:      "hash-rel-" + title,
		Fingerprint:         "fp-rel-" + title,
		Revision:            1,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := storage.SaveLearning(ctx, tx, learning); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	return learning
}

func TestRelateCreatesRelation(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	src := saveLearningForRelation(t, db, proj, "Source Learning")
	tgt := saveLearningForRelation(t, db, proj, "Target Learning")

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &RelateInput{
		SourceLearningID: src.ID,
		TargetLearningID: tgt.ID,
		RelationType:     domain.RelationSupersedes,
		Rationale:        "Source supersedes target because it's more current",
		Actor:            domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	result, err := svc.Relate(context.Background(), input)
	if err != nil {
		t.Fatalf("Relate: %v", err)
	}

	if result.RelationID == "" {
		t.Fatal("Relate returned empty RelationID")
	}

	// Verify relation exists in DB.
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx read: %v", err)
	}
	defer tx.Rollback()

	relations, err := storage.ListRelationsBySource(ctx, tx, src.ID)
	if err != nil {
		t.Fatalf("ListRelationsBySource: %v", err)
	}
	if len(relations) == 0 {
		t.Fatal("No relation found in DB")
	}
	if relations[0].Relation != domain.RelationSupersedes {
		t.Errorf("Relation type = %q, want %q", relations[0].Relation, domain.RelationSupersedes)
	}
	if relations[0].SourceLearningID != src.ID {
		t.Errorf("Source = %q, want %q", relations[0].SourceLearningID, src.ID)
	}
	if relations[0].TargetLearningID != tgt.ID {
		t.Errorf("Target = %q, want %q", relations[0].TargetLearningID, tgt.ID)
	}
}

func TestRelateSelfRelationFails(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	src := saveLearningForRelation(t, db, proj, "Self-Relation Learning")

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &RelateInput{
		SourceLearningID: src.ID,
		TargetLearningID: src.ID,
		RelationType:     domain.RelationRelated,
		Rationale:        "Self-relation should fail",
		Actor:            domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	_, err := svc.Relate(context.Background(), input)
	if err == nil {
		t.Fatal("Expected error for self-relation")
	}
}

func TestRelateDuplicateFails(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	src := saveLearningForRelation(t, db, proj, "Dup Source")
	tgt := saveLearningForRelation(t, db, proj, "Dup Target")

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &RelateInput{
		SourceLearningID: src.ID,
		TargetLearningID: tgt.ID,
		RelationType:     domain.RelationRelated,
		Rationale:        "First relation",
		Actor:            domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	// First relation should succeed.
	_, err := svc.Relate(context.Background(), input)
	if err != nil {
		t.Fatalf("First Relate: %v", err)
	}

	// Second identical relation should fail.
	_, err = svc.Relate(context.Background(), input)
	if err == nil {
		t.Fatal("Expected error for duplicate relation")
	}
}

func TestRelateNonexistentSource(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	tgt := saveLearningForRelation(t, db, proj, "Valid Target")

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &RelateInput{
		SourceLearningID: "nonexistent-source",
		TargetLearningID: tgt.ID,
		RelationType:     domain.RelationRelated,
		Rationale:        "Source doesn't exist",
		Actor:            domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	_, err := svc.Relate(context.Background(), input)
	if err == nil {
		t.Fatal("Expected error for nonexistent source")
	}
}

func TestRelateNonexistentTarget(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	src := saveLearningForRelation(t, db, proj, "Valid Source")

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &RelateInput{
		SourceLearningID: src.ID,
		TargetLearningID: "nonexistent-target",
		RelationType:     domain.RelationRelated,
		Rationale:        "Target doesn't exist",
		Actor:            domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	_, err := svc.Relate(context.Background(), input)
	if err == nil {
		t.Fatal("Expected error for nonexistent target")
	}
}

func TestRelateNilInput(t *testing.T) {
	t.Parallel()

	db, _ := setupCurateDB(t)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	_, err := svc.Relate(context.Background(), nil)
	if err == nil {
		t.Fatal("Expected error for nil input")
	}
}

func TestRelateEmptySourceID(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	tgt := saveLearningForRelation(t, db, proj, "Target For Empty Source")

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &RelateInput{
		SourceLearningID: "",
		TargetLearningID: tgt.ID,
		RelationType:     domain.RelationRelated,
		Rationale:        "Empty source ID should fail",
		Actor:            domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	_, err := svc.Relate(context.Background(), input)
	if err == nil {
		t.Fatal("Expected error for empty source ID")
	}
}

func TestRelateEmptyTargetID(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	src := saveLearningForRelation(t, db, proj, "Source For Empty Target")

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &RelateInput{
		SourceLearningID: src.ID,
		TargetLearningID: "",
		RelationType:     domain.RelationRelated,
		Rationale:        "Empty target ID should fail",
		Actor:            domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	_, err := svc.Relate(context.Background(), input)
	if err == nil {
		t.Fatal("Expected error for empty target ID")
	}
}
