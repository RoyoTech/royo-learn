package curate

import (
	"context"
	"database/sql"
	"testing"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/testutil"
)

// TestRelation_ProposeThenConfirm proves the plan 4.5 lifecycle: the agent
// proposes a relation (never auto-confirmed) and curation confirms it. The
// persisted relation records its type, both learning IDs, who proposed, and who
// confirmed.
func TestRelation_ProposeThenConfirm(t *testing.T) {
	db, proj := setupCurateDB(t)
	src := saveLearningForRelation(t, db, proj, "Proposer Source")
	tgt := saveLearningForRelation(t, db, proj, "Proposer Target")
	svc := NewService(db, testutil.TempDir(t))
	ctx := context.Background()

	proposer := domain.Actor{Kind: "agent", Name: "session-bot", Model: "test-model"}
	res, err := svc.ProposeRelation(ctx, &RelateInput{
		SourceLearningID: src.ID,
		TargetLearningID: tgt.ID,
		RelationType:     domain.RelationDuplicateOf,
		Rationale:        "looks like the same lesson",
		Actor:            proposer,
	})
	if err != nil {
		t.Fatalf("ProposeRelation: %v", err)
	}

	// A proposal is NOT a confirmed equivalence: the system suggests, a human
	// confirms.
	got := getRelation(t, db, res.RelationID)
	if got.Status != domain.RelationProposed {
		t.Fatalf("after propose, status = %q, want %q", got.Status, domain.RelationProposed)
	}
	if got.ProposedBy.Name != proposer.Name || got.ProposedBy.Kind != proposer.Kind {
		t.Fatalf("ProposedBy = %+v, want %+v", got.ProposedBy, proposer)
	}
	if got.ConfirmedBy != nil {
		t.Fatalf("a proposed relation must have no confirmer, got %+v", got.ConfirmedBy)
	}
	if got.Relation != domain.RelationDuplicateOf || got.SourceLearningID != src.ID || got.TargetLearningID != tgt.ID {
		t.Fatalf("relation identity not persisted: %+v", got)
	}

	// Curation confirms.
	confirmer := domain.Actor{Kind: "human", Name: "curator"}
	if err := svc.ConfirmRelation(ctx, res.RelationID, confirmer); err != nil {
		t.Fatalf("ConfirmRelation: %v", err)
	}

	got = getRelation(t, db, res.RelationID)
	if got.Status != domain.RelationConfirmed {
		t.Fatalf("after confirm, status = %q, want %q", got.Status, domain.RelationConfirmed)
	}
	if got.ConfirmedBy == nil || got.ConfirmedBy.Name != confirmer.Name {
		t.Fatalf("ConfirmedBy = %+v, want %+v", got.ConfirmedBy, confirmer)
	}
	if got.ConfirmedAt == nil || got.ConfirmedAt.IsZero() {
		t.Fatal("ConfirmedAt must be set on confirmation")
	}
}

// TestConfirmRelation_UnknownFails proves confirming a missing relation is an
// explicit error, never a silent no-op that fakes a confirmation.
func TestConfirmRelation_UnknownFails(t *testing.T) {
	db, _ := setupCurateDB(t)
	svc := NewService(db, testutil.TempDir(t))
	if err := svc.ConfirmRelation(context.Background(), domain.RelationID("does-not-exist"), domain.Actor{Kind: "human", Name: "curator"}); err == nil {
		t.Fatal("expected an error confirming an unknown relation")
	}
}

func getRelation(t *testing.T, db *storage.DB, id domain.RelationID) *domain.LearningRelation {
	t.Helper()
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()
	rel, err := storage.GetRelation(ctx, tx, id)
	if err != nil {
		t.Fatalf("GetRelation: %v", err)
	}
	if rel == nil {
		t.Fatalf("relation %q not found", id)
	}
	return rel
}
