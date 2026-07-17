package publish

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

func TestPublishRejectsPreviewOwnedByAnotherLearning(t *testing.T) {
	env := seedPublishEnv(t, "cross-learning/SKILL.md", false, "")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	preview := generatePreview(t, svc, env)

	otherID := domain.LearningID(uuid.Must(uuid.NewV7()).String())
	other := loadMaterializedLearning(t, env)
	other.ID = otherID
	other.NormalizedHash = "other-" + string(otherID)
	other.Fingerprint = "other-" + string(otherID)
	other.CreatedAt = utcNowPublish()
	other.UpdatedAt = other.CreatedAt
	if err := storage.WithTx(context.Background(), env.db, func(tx *sql.Tx) error {
		return storage.SaveLearning(context.Background(), tx, other)
	}); err != nil {
		t.Fatalf("seed second learning: %v", err)
	}
	if _, err := env.db.DB.Exec(`UPDATE publication_previews SET learning_id = ? WHERE preview_hash = ?`, otherID, preview.PreviewHash); err != nil {
		t.Fatalf("retarget preview: %v", err)
	}

	_, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: preview.PreviewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	assertDomainCode(t, err, domain.ErrPreviewHashMismatch)
}

func TestPublishRejectsMissingAndExtraPersistedPlanTargets(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*domain.PublicationPlan)
	}{
		{name: "missing target", mutate: func(plan *domain.PublicationPlan) { plan.Targets = nil }},
		{name: "extra target", mutate: func(plan *domain.PublicationPlan) {
			plan.Targets = append(plan.Targets, domain.PublicationPlanTarget{
				Root: ".", Path: "unexpected.md", Operation: domain.OpCreate,
				PosteriorHash: HashContent([]byte("unexpected")),
			})
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env := seedPublishEnv(t, "plan-set/SKILL.md", false, "")
			defer env.db.Close()
			svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
			preview := generatePreview(t, svc, env)
			mutateStoredPlan(t, env.db, preview.PreviewHash, tt.mutate)

			_, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
				LearningID: env.learningID, PreviewHash: preview.PreviewHash,
				Apply: true, Force: true, Actor: env.actor,
			})
			assertDomainCode(t, err, domain.ErrPreviewHashMismatch)
		})
	}
}

func TestPublishRejectsPosteriorHashTampering(t *testing.T) {
	env := seedPublishEnv(t, "posterior/SKILL.md", false, "")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	preview := generatePreview(t, svc, env)
	mutateStoredPlan(t, env.db, preview.PreviewHash, func(plan *domain.PublicationPlan) {
		plan.Targets[0].PosteriorHash = HashContent([]byte("different bytes"))
	})

	_, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: preview.PreviewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	assertDomainCode(t, err, domain.ErrPreviewHashMismatch)
}

func TestPublishAppliesPreviewedBytesAfterLearningDrift(t *testing.T) {
	env := seedPublishEnv(t, "content-drift/SKILL.md", false, "")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	preview := generatePreview(t, svc, env)
	wantHash := preview.Plan.Targets[0].PosteriorHash

	learning := loadMaterializedLearning(t, env)
	learning.Title = "Changed after preview"
	learning.UpdatedAt = utcNowPublish()
	if err := storage.WithTx(context.Background(), env.db, func(tx *sql.Tx) error {
		return storage.UpdateLearning(context.Background(), tx, learning)
	}); err != nil {
		t.Fatalf("mutate learning: %v", err)
	}

	result, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: preview.PreviewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	target := filepath.Join(env.projectRoot, result.Targets[0].Root, result.Targets[0].Path)
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if gotHash := HashContent(got); gotHash != wantHash {
		t.Fatalf("published hash = %s, want previewed posterior %s", gotHash, wantHash)
	}
}

func TestPublishRejectsCurationDestinationDrift(t *testing.T) {
	env := seedPublishEnv(t, "curation-before/SKILL.md", false, "")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	preview := generatePreview(t, svc, env)

	curation := &domain.Curation{
		ID: domain.CurationID(uuid.Must(uuid.NewV7()).String()), LearningID: env.learningID,
		Decision:    domain.CurationApproveNewSkill,
		Destination: &domain.Destination{Type: domain.DestSkill, Root: "skills", Path: "curation-after/SKILL.md", Required: true},
		Actor:       env.actor, CreatedAt: time.Now().UTC().Add(time.Second),
	}
	if err := storage.WithTx(context.Background(), env.db, func(tx *sql.Tx) error {
		return storage.SaveCuration(context.Background(), tx, curation)
	}); err != nil {
		t.Fatalf("save drifted curation: %v", err)
	}

	_, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: preview.PreviewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	assertDomainCode(t, err, domain.ErrPreviewHashMismatch)
}

func TestPublishDerivesApprovalFromExistingSkillImpact(t *testing.T) {
	env := seedPublishEnv(t, "sensitive/SKILL.md", true, "existing\n")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	preview := generatePreview(t, svc, env)
	if _, err := env.db.DB.Exec(`UPDATE publication_previews SET requires_approval = 0 WHERE preview_hash = ?`, preview.PreviewHash); err != nil {
		t.Fatalf("tamper approval flag: %v", err)
	}
	mutateStoredPlan(t, env.db, preview.PreviewHash, func(plan *domain.PublicationPlan) {
		plan.RequiresApproval = false
	})

	_, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: preview.PreviewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	assertDomainCode(t, err, domain.ErrApprovalRequired)
}

func generatePreview(t *testing.T, _ *Service, env *publishTestEnv) *domain.PublicationPreview {
	t.Helper()
	tx, err := env.db.DB.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("begin preview read: %v", err)
	}
	defer tx.Rollback()
	preview, err := storage.GetPreviewByHash(context.Background(), tx, env.previewHash)
	if err != nil {
		t.Fatalf("GetPreviewByHash: %v", err)
	}
	return preview
}

func mutateStoredPlan(t *testing.T, db *storage.DB, hash string, mutate func(*domain.PublicationPlan)) {
	t.Helper()
	var raw string
	if err := db.DB.QueryRow(`SELECT plan_json FROM publication_previews WHERE preview_hash = ?`, hash).Scan(&raw); err != nil {
		t.Fatalf("read plan: %v", err)
	}
	var plan domain.PublicationPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	mutate(&plan)
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("encode plan: %v", err)
	}
	if _, err := db.DB.Exec(`UPDATE publication_previews SET plan_json = ? WHERE preview_hash = ?`, string(encoded), hash); err != nil {
		t.Fatalf("update plan: %v", err)
	}
}

func assertDomainCode(t *testing.T, err error, code domain.ErrorCode) {
	t.Helper()
	domainErr, ok := domain.AsDomainError(err)
	if !ok || domainErr.Code != code {
		t.Fatalf("error = %v, want domain code %s", err, code)
	}
}
