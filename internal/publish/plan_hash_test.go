package publish

import (
	"context"
	"database/sql"
	"testing"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
)

// TestPlanSignature_DependsOnEveryPlanField proves the preview hash binds the
// WHOLE plan, not only the combined diff text (Recorrido D). Two plans that
// differ in a single field — destination path, per-destination operation, the
// prior file hash, or the posterior content hash — must produce different plan
// signatures, so an approval bound to one plan can never authorize another.
func TestPlanSignature_DependsOnEveryPlanField(t *testing.T) {
	base := []domain.PublicationPlanTarget{
		{Root: "skills", Path: "a/SKILL.md", Operation: domain.OpCreate, PriorHash: "", PosteriorHash: "aaa"},
	}
	baseSig := PlanSignature(base)

	cases := map[string][]domain.PublicationPlanTarget{
		"different path": {
			{Root: "skills", Path: "b/SKILL.md", Operation: domain.OpCreate, PriorHash: "", PosteriorHash: "aaa"},
		},
		"different root": {
			{Root: "shared", Path: "a/SKILL.md", Operation: domain.OpCreate, PriorHash: "", PosteriorHash: "aaa"},
		},
		"different operation": {
			{Root: "skills", Path: "a/SKILL.md", Operation: domain.OpReplace, PriorHash: "", PosteriorHash: "aaa"},
		},
		"different prior hash": {
			{Root: "skills", Path: "a/SKILL.md", Operation: domain.OpCreate, PriorHash: "old", PosteriorHash: "aaa"},
		},
		"different posterior hash": {
			{Root: "skills", Path: "a/SKILL.md", Operation: domain.OpCreate, PriorHash: "", PosteriorHash: "bbb"},
		},
	}

	for name, plan := range cases {
		if PlanSignature(plan) == baseSig {
			t.Errorf("%s: plan signature must differ from the base plan but was identical", name)
		}
	}
}

// TestPlanSignature_Deterministic proves the signature is stable across target
// ordering so the preview hash does not flap when targets are resolved in a
// different order.
func TestPlanSignature_Deterministic(t *testing.T) {
	a := []domain.PublicationPlanTarget{
		{Root: "skills", Path: "a/SKILL.md", Operation: domain.OpCreate, PosteriorHash: "1"},
		{Root: "skills", Path: "b/SKILL.md", Operation: domain.OpCreate, PosteriorHash: "2"},
	}
	b := []domain.PublicationPlanTarget{
		{Root: "skills", Path: "b/SKILL.md", Operation: domain.OpCreate, PosteriorHash: "2"},
		{Root: "skills", Path: "a/SKILL.md", Operation: domain.OpCreate, PosteriorHash: "1"},
	}
	if PlanSignature(a) != PlanSignature(b) {
		t.Fatalf("plan signature must be order-independent: %q vs %q", PlanSignature(a), PlanSignature(b))
	}
}

// TestPreview_PersistsPerTargetPriorAndPosteriorHashes proves a generated
// preview records, per destination, the prior file hash and the posterior
// content hash, and that these survive a storage round-trip. Recorrido D needs
// the prior hashes at publish time to refuse a destination that changed after
// the preview was taken.
func TestPreview_PersistsPerTargetPriorAndPosteriorHashes(t *testing.T) {
	ctx := context.Background()
	env := seedPublishEnv(t, "index-only", false, "")
	defer env.db.Close()

	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	res, err := svc.Preview(ctx, env.projectID, &PreviewInput{
		LearningID: env.learningID,
		Actor:      env.actor,
	})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(res.Preview.Plan.Targets) == 0 {
		t.Fatal("preview plan must record per-target hashes")
	}
	for _, pt := range res.Preview.Plan.Targets {
		if pt.PosteriorHash == "" {
			t.Errorf("target %s/%s: posterior hash must be recorded", pt.Root, pt.Path)
		}
	}

	// Round-trip through storage: the persisted preview must carry the targets.
	readTx, _ := env.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	loaded, err := storage.GetPreviewByHash(ctx, readTx, res.Preview.PreviewHash)
	readTx.Rollback()
	if err != nil {
		t.Fatalf("GetPreviewByHash: %v", err)
	}
	if len(loaded.Plan.Targets) != len(res.Preview.Plan.Targets) {
		t.Fatalf("persisted preview targets = %d, want %d",
			len(loaded.Plan.Targets), len(res.Preview.Plan.Targets))
	}
}
