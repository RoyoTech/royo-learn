package integration_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-royo-learn/internal/capture"
	"agent-royo-learn/internal/curate"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/publish"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/storage/storagetest"

	"github.com/google/uuid"
)

func TestCaptureCuratePublishFlow(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	db := storagetest.OpenTemp(t)
	now := time.Now().UTC()
	project := &domain.Project{
		ID:            domain.ProjectID(uuid.Must(uuid.NewV7()).String()),
		ProjectKey:    "capture-curate-publish",
		DisplayName:   "Capture Curate Publish",
		CanonicalPath: projectRoot,
		Fingerprint:   "capture-curate-publish",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := storage.WithTx(ctx, db, func(tx *sql.Tx) error {
		return storage.SaveProject(ctx, tx, project)
	}); err != nil {
		t.Fatalf("save project: %v", err)
	}

	actor := domain.Actor{Kind: "agent", Name: "integration-test"}
	captureService := capture.NewService(db, filepath.Join(projectRoot, ".royo-learn", "records"))
	captured, err := captureService.Capture(ctx, project.ID, &capture.CaptureInput{
		Title:         "Capture Curate Publish",
		Context:       "Integration testing",
		Observation:   "The complete learning flow needs a persisted destination",
		Lesson:        "Curations must carry their publication destination",
		Type:          domain.TypeProcedure,
		Scope:         domain.ScopeProject,
		Destination:   domain.DestSkill,
		Confidence:    domain.ConfidenceHigh,
		EvidenceLevel: domain.EvidenceModerate,
		Recommended:   []string{"Capture", "Curate", "Publish"},
		Actor:         actor,
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	evidence := &domain.Evidence{
		ID:          domain.EvidenceID(uuid.Must(uuid.NewV7()).String()),
		LearningID:  captured.LearningID,
		Kind:        domain.KindTest,
		URI:         "test://capture-curate-publish",
		Summary:     "The integration test reproduces the complete flow",
		SHA256:      "capture-curate-publish",
		CollectedAt: now,
	}
	if err := storage.WithTx(ctx, db, func(tx *sql.Tx) error {
		return storage.SaveEvidence(ctx, tx, evidence)
	}); err != nil {
		t.Fatalf("save evidence: %v", err)
	}

	curateService := curate.NewService(db, filepath.Join(projectRoot, ".royo-learn", "records"))
	curated, err := curateService.Curate(ctx, project.ID, &curate.CurateInput{
		LearningID: captured.LearningID,
		Decision:   domain.CurationApproveNewSkill,
		Rationale:  "Evidence proves the learning is reusable",
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("curate: %v", err)
	}

	readTx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("begin curation read: %v", err)
	}
	persistedCuration, err := storage.GetCuration(ctx, readTx, curated.CurationID)
	_ = readTx.Rollback()
	if err != nil {
		t.Fatalf("get curation: %v", err)
	}
	if persistedCuration.Destination == nil {
		t.Fatal("curation destination is nil")
	}
	destination := persistedCuration.Destination
	if destination.Type != domain.DestSkill || destination.Root == "" || destination.Path == "" || !destination.Required {
		t.Fatalf("incomplete destination: %+v", destination)
	}

	publishService := publish.NewService(
		db,
		projectRoot,
		filepath.Join(projectRoot, ".royo-learn", "backups"),
		filepath.Join(projectRoot, ".royo-learn"),
	)
	preview, err := publishService.Preview(ctx, project.ID, &publish.PreviewInput{
		LearningID: captured.LearningID,
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.Preview.RequiresApproval {
		t.Fatal("new project skill unexpectedly requires human approval")
	}

	published, err := publishService.Publish(ctx, project.ID, &publish.PublishInput{
		LearningID:  captured.LearningID,
		PreviewHash: preview.Preview.PreviewHash,
		Force:       true,
		Actor:       actor,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if published.Publication.Status != domain.PubStatusCompleted {
		t.Fatalf("publication status = %q, want %q", published.Publication.Status, domain.PubStatusCompleted)
	}

	targetPath := filepath.Join(projectRoot, destination.Root, destination.Path)
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("published target %q: %v", targetPath, err)
	}
}
