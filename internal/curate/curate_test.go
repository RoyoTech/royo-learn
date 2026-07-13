package curate

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/storage/storagetest"
	"agent-royo-learn/internal/testutil"

	"github.com/google/uuid"
)

// setupCurateDB creates a temporary DB with migrations and a test project.
func setupCurateDB(t *testing.T) (*storage.DB, *domain.Project) {
	t.Helper()

	db := storagetest.OpenTemp(t)

	proj := &domain.Project{
		ID:            domain.ProjectID(uuid.Must(uuid.NewV7()).String()),
		ProjectKey:    "curate-test",
		DisplayName:   "Curate Test",
		CanonicalPath: testutil.TempDir(t),
		GitRemote:     "",
		Fingerprint:   "fp-cur-proj",
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

	return db, proj
}

// saveTestLearning inserts a learning with the given fields and returns it.
func saveTestLearning(t *testing.T, db *storage.DB, proj *domain.Project, status domain.LearningStatus, evidenceLevel domain.EvidenceLevel) *domain.Learning {
	t.Helper()

	now := time.Now().UTC()
	ctx := context.Background()
	learning := &domain.Learning{
		ID:                  domain.LearningID(uuid.Must(uuid.NewV7()).String()),
		ProjectID:           proj.ID,
		Status:              status,
		Type:                domain.TypeProcedure,
		Title:               "Test Learning for Curation",
		Context:             "Testing curation flow",
		Observation:         "The curation service works correctly",
		ReusableLesson:      "Always test curation transitions",
		ScopeGuess:          domain.ScopeProject,
		Confidence:          domain.ConfidenceHigh,
		EvidenceLevel:       evidenceLevel,
		ProposedDestination: domain.DestProject,
		NormalizedHash:      "hash-curate-test-" + string(status),
		Fingerprint:         "fp-curate-test-" + string(status),
		Revision:            1,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

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

// saveTestEvidence inserts a test evidence record for a learning.
func saveTestEvidence(t *testing.T, db *storage.DB, learningID domain.LearningID) {
	t.Helper()

	ctx := context.Background()
	evt := &domain.Evidence{
		ID:         domain.EvidenceID(uuid.Must(uuid.NewV7()).String()),
		LearningID: learningID,
		Kind:       domain.KindTest,
		URI:        "test://evidence",
		Summary:    "Test evidence for curation",
		SHA256:     "abc123",
		Redacted:   false,
	}

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := storage.SaveEvidence(ctx, tx, evt); err != nil {
		tx.Rollback()
		t.Fatalf("SaveEvidence: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestCurateApproveCaptured(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	learning := saveTestLearning(t, db, proj, domain.StatusCaptured, domain.EvidenceModerate)
	saveTestEvidence(t, db, learning.ID)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: learning.ID,
		Decision:   domain.CurationApproveProjectKnowledge,
		Rationale:  "Evidence is sufficient and learning is valid",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	result, err := svc.Curate(context.Background(), proj.ID, input)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if result.CurationID == "" {
		t.Fatal("Curate returned empty CurationID")
	}
	if result.NewStatus != domain.StatusApproved {
		t.Errorf("NewStatus = %q, want %q", result.NewStatus, domain.StatusApproved)
	}

	// Verify learning was updated in DB.
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx read: %v", err)
	}
	defer tx.Rollback()

	updated, err := storage.GetLearning(ctx, tx, learning.ID)
	if err != nil {
		t.Fatalf("GetLearning: %v", err)
	}
	if updated.Status != domain.StatusApproved {
		t.Errorf("Learning status in DB = %q, want %q", updated.Status, domain.StatusApproved)
	}
	if updated.Revision != 2 {
		t.Errorf("Learning revision = %d, want 2", updated.Revision)
	}
	if updated.ApprovedDestination == nil {
		t.Fatal("ApprovedDestination is nil after approve-project-knowledge decision")
	}
	if updated.ApprovedDestination.Type != domain.DestProject {
		t.Errorf("ApprovedDestination.Type = %q, want %q", updated.ApprovedDestination.Type, domain.DestProject)
	}

	// Verify curation record was created.
	curations, err := storage.ListCurationsByLearning(ctx, tx, learning.ID)
	if err != nil {
		t.Fatalf("ListCurationsByLearning: %v", err)
	}
	if len(curations) == 0 {
		t.Fatal("No curation record found")
	}
	if curations[0].Decision != domain.CurationApproveProjectKnowledge {
		t.Errorf("Curation decision = %q, want %q", curations[0].Decision, domain.CurationApproveProjectKnowledge)
	}
}

func TestCurateRejectCaptured(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	learning := saveTestLearning(t, db, proj, domain.StatusCaptured, domain.EvidenceWeak)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: learning.ID,
		Decision:   domain.CurationReject,
		Rationale:  "Not enough evidence, learning is speculative",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	result, err := svc.Curate(context.Background(), proj.ID, input)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if result.NewStatus != domain.StatusRejected {
		t.Errorf("NewStatus = %q, want %q", result.NewStatus, domain.StatusRejected)
	}

	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx read: %v", err)
	}
	defer tx.Rollback()

	updated, err := storage.GetLearning(ctx, tx, learning.ID)
	if err != nil {
		t.Fatalf("GetLearning: %v", err)
	}
	if updated.Status != domain.StatusRejected {
		t.Errorf("Learning status in DB = %q, want %q", updated.Status, domain.StatusRejected)
	}
}

func TestCurateNeedsEvidence(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	learning := saveTestLearning(t, db, proj, domain.StatusCaptured, domain.EvidenceWeak)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: learning.ID,
		Decision:   domain.CurationNeedsEvidence,
		Rationale:  "More evidence required before approval",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	result, err := svc.Curate(context.Background(), proj.ID, input)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if result.NewStatus != domain.StatusNeedsEvidence {
		t.Errorf("NewStatus = %q, want %q", result.NewStatus, domain.StatusNeedsEvidence)
	}
}

func TestCurateApproveWithoutEvidenceFails(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	// Learning with insufficient evidence and NO evidence records.
	learning := saveTestLearning(t, db, proj, domain.StatusCaptured, domain.EvidenceInsufficient)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: learning.ID,
		Decision:   domain.CurationApproveProjectKnowledge,
		Rationale:  "Trying to approve without evidence",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	_, err := svc.Curate(context.Background(), proj.ID, input)
	if err == nil {
		t.Fatal("Expected error for approving learning with insufficient evidence")
	}

	// Verify learning was NOT updated.
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx read: %v", err)
	}
	defer tx.Rollback()

	unchanged, err := storage.GetLearning(ctx, tx, learning.ID)
	if err != nil {
		t.Fatalf("GetLearning: %v", err)
	}
	if unchanged.Status != domain.StatusCaptured {
		t.Errorf("Learning status should remain %q, got %q", domain.StatusCaptured, unchanged.Status)
	}
}

func TestCurateApproveWithWeakEvidenceFails(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	learning := saveTestLearning(t, db, proj, domain.StatusCaptured, domain.EvidenceWeak)
	saveTestEvidence(t, db, learning.ID) // has evidence but level is weak

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: learning.ID,
		Decision:   domain.CurationApproveProjectKnowledge,
		Rationale:  "Trying to approve with weak evidence level",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	_, err := svc.Curate(context.Background(), proj.ID, input)
	if err == nil {
		t.Fatal("Expected error for approving learning with weak evidence level")
	}
}

func TestCurateInvalidTransition(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	// rejected is a terminal state.
	learning := saveTestLearning(t, db, proj, domain.StatusRejected, domain.EvidenceStrong)
	saveTestEvidence(t, db, learning.ID)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: learning.ID,
		Decision:   domain.CurationApproveProjectKnowledge,
		Rationale:  "Cannot approve a rejected learning",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	_, err := svc.Curate(context.Background(), proj.ID, input)
	if err == nil {
		t.Fatal("Expected error for invalid transition from rejected to approved")
	}
}

func TestCurateLearningNotFound(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: "nonexistent-id",
		Decision:   domain.CurationApproveProjectKnowledge,
		Rationale:  "This should fail",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	_, err := svc.Curate(context.Background(), proj.ID, input)
	if err == nil {
		t.Fatal("Expected error for nonexistent learning")
	}
}

func TestCurateNilInput(t *testing.T) {
	t.Parallel()

	db, _ := setupCurateDB(t)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	_, err := svc.Curate(context.Background(), "proj-1", nil)
	if err == nil {
		t.Fatal("Expected error for nil input")
	}
}

func TestCurateEmptyLearningID(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: "",
		Decision:   domain.CurationApproveProjectKnowledge,
		Rationale:  "Empty ID should fail",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	_, err := svc.Curate(context.Background(), proj.ID, input)
	if err == nil {
		t.Fatal("Expected error for empty learning ID")
	}
}

func TestCurateEmptyDecision(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	learning := saveTestLearning(t, db, proj, domain.StatusCaptured, domain.EvidenceModerate)
	saveTestEvidence(t, db, learning.ID)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: learning.ID,
		Decision:   "",
		Rationale:  "Empty decision should fail",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	_, err := svc.Curate(context.Background(), proj.ID, input)
	if err == nil {
		t.Fatal("Expected error for empty decision")
	}
}

func TestCurateRecordUpdated(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	learning := saveTestLearning(t, db, proj, domain.StatusCaptured, domain.EvidenceModerate)
	saveTestEvidence(t, db, learning.ID)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: learning.ID,
		Decision:   domain.CurationApproveProjectKnowledge,
		Rationale:  "Approval for record test",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	_, err := svc.Curate(context.Background(), proj.ID, input)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	// Check that a record file was written.
	recordPath := filepath.Join(recordsDir, string(learning.ID)+".md")
	info, err := os.Stat(recordPath)
	if err != nil {
		t.Fatalf("Record file not found at %s: %v", recordPath, err)
	}
	if info.Size() == 0 {
		t.Fatal("Record file is empty")
	}
}

func TestCurateApproveNeedsEvidenceStatus(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	learning := saveTestLearning(t, db, proj, domain.StatusNeedsEvidence, domain.EvidenceStrong)
	saveTestEvidence(t, db, learning.ID)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: learning.ID,
		Decision:   domain.CurationApproveProjectKnowledge,
		Rationale:  "Now has sufficient evidence",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	result, err := svc.Curate(context.Background(), proj.ID, input)
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}

	if result.NewStatus != domain.StatusApproved {
		t.Errorf("NewStatus = %q, want %q", result.NewStatus, domain.StatusApproved)
	}
}

// saveTestSkillLearning inserts a learning destined for a skill, with
// retrieval terms, so DeriveSkillArea has input to work with. Returns it.
func saveTestSkillLearning(t *testing.T, db *storage.DB, proj *domain.Project, retrievalTerms []string) *domain.Learning {
	t.Helper()

	now := time.Now().UTC()
	ctx := context.Background()
	learning := &domain.Learning{
		ID:                  domain.LearningID(uuid.Must(uuid.NewV7()).String()),
		ProjectID:           proj.ID,
		Status:              domain.StatusCaptured,
		Type:                domain.TypeProcedure,
		Title:               "Test Skill Learning for Curation",
		Context:             "Testing skill area derivation at curate time",
		Observation:         "The curated destination should carry a concrete area",
		ReusableLesson:      "Derive the area deterministically from retrieval terms",
		ScopeGuess:          domain.ScopeProject,
		Confidence:          domain.ConfidenceHigh,
		EvidenceLevel:       domain.EvidenceModerate,
		ProposedDestination: domain.DestSkill,
		RetrievalTerms:      retrievalTerms,
		NormalizedHash:      "hash-skill-curate-test",
		Fingerprint:         "fp-skill-curate-test",
		Revision:            1,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

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

func TestCurateApproveNewSkillWithoutAreaDerives(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	terms := []string{"go", "testing"}
	learning := saveTestSkillLearning(t, db, proj, terms)
	saveTestEvidence(t, db, learning.ID)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: learning.ID,
		Decision:   domain.CurationApproveNewSkill,
		Rationale:  "Skill decision without explicit area should derive one",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
		// Area left empty: derivation must kick in.
	}

	if _, err := svc.Curate(context.Background(), proj.ID, input); err != nil {
		t.Fatalf("Curate: %v", err)
	}

	// Read back the persisted learning and verify the approved destination
	// carries the derived area (not empty).
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx read: %v", err)
	}
	defer tx.Rollback()

	updated, err := storage.GetLearning(ctx, tx, learning.ID)
	if err != nil {
		t.Fatalf("GetLearning: %v", err)
	}
	if updated.ApprovedDestination == nil {
		t.Fatal("ApprovedDestination is nil after skill approval")
	}
	if updated.ApprovedDestination.Type != domain.DestSkill {
		t.Errorf("ApprovedDestination.Type = %q, want %q", updated.ApprovedDestination.Type, domain.DestSkill)
	}
	wantArea := domain.DeriveSkillArea(&domain.Learning{RetrievalTerms: terms})
	if wantArea == "" {
		t.Fatalf("test setup: DeriveSkillArea returned empty for terms %v", terms)
	}
	if got := updated.ApprovedDestination.Area; got != wantArea {
		t.Errorf("ApprovedDestination.Area = %q, want derived %q", got, wantArea)
	}
	if got := updated.ApprovedDestination.Area; got == "" {
		t.Error("ApprovedDestination.Area should not be empty for skill decision without explicit area")
	}

	// The curation record must carry the same derived area.
	curations, err := storage.ListCurationsByLearning(ctx, tx, learning.ID)
	if err != nil {
		t.Fatalf("ListCurationsByLearning: %v", err)
	}
	if len(curations) == 0 {
		t.Fatal("No curation record found")
	}
	if curations[0].Destination.Area != wantArea {
		t.Errorf("Curation.Destination.Area = %q, want %q", curations[0].Destination.Area, wantArea)
	}
}

func TestCurateApproveSkillUpdateWithoutAreaDerives(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	terms := []string{"refactor", "hexagonal"}
	learning := saveTestSkillLearning(t, db, proj, terms)
	saveTestEvidence(t, db, learning.ID)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: learning.ID,
		Decision:   domain.CurationApproveSkillUpdate,
		Rationale:  "Skill update without explicit area should derive one",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
	}

	if _, err := svc.Curate(context.Background(), proj.ID, input); err != nil {
		t.Fatalf("Curate: %v", err)
	}

	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx read: %v", err)
	}
	defer tx.Rollback()

	updated, err := storage.GetLearning(ctx, tx, learning.ID)
	if err != nil {
		t.Fatalf("GetLearning: %v", err)
	}
	if updated.ApprovedDestination == nil {
		t.Fatal("ApprovedDestination is nil after skill-update approval")
	}
	wantArea := domain.DeriveSkillArea(&domain.Learning{RetrievalTerms: terms})
	if got := updated.ApprovedDestination.Area; got != wantArea {
		t.Errorf("ApprovedDestination.Area = %q, want derived %q", got, wantArea)
	}
}

func TestCurateApproveNewSkillWithExplicitAreaPreserved(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	// Terms that would derive to something OTHER than the explicit area.
	terms := []string{"zzz", "aaa"}
	learning := saveTestSkillLearning(t, db, proj, terms)
	saveTestEvidence(t, db, learning.ID)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	const explicitArea = "custom-area"
	input := &CurateInput{
		LearningID: learning.ID,
		Decision:   domain.CurationApproveNewSkill,
		Rationale:  "Explicit area must win over derivation",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
		Area:       explicitArea,
	}

	if _, err := svc.Curate(context.Background(), proj.ID, input); err != nil {
		t.Fatalf("Curate: %v", err)
	}

	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx read: %v", err)
	}
	defer tx.Rollback()

	updated, err := storage.GetLearning(ctx, tx, learning.ID)
	if err != nil {
		t.Fatalf("GetLearning: %v", err)
	}
	if updated.ApprovedDestination == nil {
		t.Fatal("ApprovedDestination is nil after skill approval")
	}
	// Explicit area must be preserved (not overridden by derivation).
	if got := updated.ApprovedDestination.Area; got != explicitArea {
		t.Errorf("ApprovedDestination.Area = %q, want explicit %q", got, explicitArea)
	}
	// Sanity: the derived area would differ, proving explicit won.
	derived := domain.DeriveSkillArea(&domain.Learning{RetrievalTerms: terms})
	if derived == explicitArea {
		t.Fatalf("test setup invalid: derived area %q == explicit %q", derived, explicitArea)
	}
}

func TestCurateApproveProjectKnowledgeAreaStaysEmpty(t *testing.T) {
	t.Parallel()

	db, proj := setupCurateDB(t)
	// Non-skill decision: area derivation must NOT apply, even with terms.
	learning := saveTestLearning(t, db, proj, domain.StatusCaptured, domain.EvidenceModerate)
	learning.RetrievalTerms = []string{"go", "testing"}
	// Persist the terms update so the loaded learning reflects them.
	ctx0 := context.Background()
	tx0, err := db.DB.BeginTx(ctx0, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := storage.UpdateLearning(ctx0, tx0, learning); err != nil {
		tx0.Rollback()
		t.Fatalf("UpdateLearning: %v", err)
	}
	if err := tx0.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	saveTestEvidence(t, db, learning.ID)

	recordsDir := testutil.TempDir(t)
	svc := NewService(db, recordsDir)

	input := &CurateInput{
		LearningID: learning.ID,
		Decision:   domain.CurationApproveProjectKnowledge,
		Rationale:  "Non-skill decision must not derive an area",
		Actor:      domain.Actor{Kind: "agent", Name: "test-curator", Model: "test-model", SessionID: "sess-001"},
		// Area left empty: derivation must NOT kick in for non-skill decisions.
	}

	if _, err := svc.Curate(context.Background(), proj.ID, input); err != nil {
		t.Fatalf("Curate: %v", err)
	}

	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx read: %v", err)
	}
	defer tx.Rollback()

	updated, err := storage.GetLearning(ctx, tx, learning.ID)
	if err != nil {
		t.Fatalf("GetLearning: %v", err)
	}
	if updated.ApprovedDestination == nil {
		t.Fatal("ApprovedDestination is nil after project-knowledge approval")
	}
	if updated.ApprovedDestination.Type != domain.DestProject {
		t.Errorf("ApprovedDestination.Type = %q, want %q", updated.ApprovedDestination.Type, domain.DestProject)
	}
	if got := updated.ApprovedDestination.Area; got != "" {
		t.Errorf("ApprovedDestination.Area = %q, want empty (non-skill decision must not derive area)", got)
	}
}

func TestDeriveDestination(t *testing.T) {
	learningID := domain.LearningID("018f-safe-learning-id")
	tests := []struct {
		name     string
		decision domain.CurationDecision
		proposed domain.DestinationType
		want     domain.Destination
	}{
		{
			name:     "reject has no publication target",
			decision: domain.CurationReject,
			proposed: domain.DestSkill,
			want:     domain.Destination{Type: domain.DestNone},
		},
		{
			name:     "needs evidence has no publication target",
			decision: domain.CurationNeedsEvidence,
			proposed: domain.DestProject,
			want:     domain.Destination{Type: domain.DestNone},
		},
		{
			name:     "merge has no publication target",
			decision: domain.CurationMerge,
			proposed: domain.DestProject,
			want:     domain.Destination{Type: domain.DestNone},
		},
		{
			name:     "project knowledge",
			decision: domain.CurationApproveProjectKnowledge,
			proposed: domain.DestProject,
			want: domain.Destination{
				Type: domain.DestProject, Root: ".royo-learn",
				Path: filepath.Join("knowledge", string(learningID)+".md"),
			},
		},
		{
			name:     "shared knowledge",
			decision: domain.CurationApproveSharedKnowledge,
			proposed: domain.DestShared,
			want: domain.Destination{
				Type: domain.DestShared, Root: "shared",
				Path: filepath.Join("knowledge", string(learningID)+".md"), Required: true,
			},
		},
		{
			name:     "new skill",
			decision: domain.CurationApproveNewSkill,
			proposed: domain.DestSkill,
			want: domain.Destination{
				Type: domain.DestSkill, Root: "skills",
				Path: filepath.Join(string(learningID), "SKILL.md"), Required: true,
			},
		},
		{
			name:     "skill update",
			decision: domain.CurationApproveSkillUpdate,
			proposed: domain.DestSkill,
			want: domain.Destination{
				Type: domain.DestSkill, Root: "skills",
				Path: filepath.Join(string(learningID), "SKILL.md"), Required: true,
			},
		},
		{
			name:     "agents rule",
			decision: domain.CurationApproveAgentsRule,
			proposed: domain.DestAgentsRule,
			want: domain.Destination{
				Type: domain.DestAgentsRule, Root: ".", Path: "AGENTS.md", Required: true,
			},
		},
		{
			name:     "regression test",
			decision: domain.CurationApproveTest,
			proposed: domain.DestProject,
			want: domain.Destination{
				Type: domain.DestProject, Root: ".",
				Path: filepath.Join("tests", string(learningID)+"_test.go"), Required: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			learning := &domain.Learning{ID: learningID, ProposedDestination: tt.proposed}
			got, err := deriveDestination(tt.decision, learning, "")
			if err != nil {
				t.Fatalf("deriveDestination: %v", err)
			}
			if *got != tt.want {
				t.Errorf("destination = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestDeriveDestinationRejectsDecisionMismatch(t *testing.T) {
	learning := &domain.Learning{
		ID:                  "018f-safe-learning-id",
		ProposedDestination: domain.DestProject,
	}

	_, err := deriveDestination(domain.CurationApproveNewSkill, learning, "")
	if err == nil {
		t.Fatal("expected destination mismatch error")
	}
}

func TestDeriveDestinationRejectsUnsafeLearningID(t *testing.T) {
	for _, learningID := range []domain.LearningID{"../escape", `..\escape`} {
		t.Run(string(learningID), func(t *testing.T) {
			learning := &domain.Learning{
				ID:                  learningID,
				ProposedDestination: domain.DestSkill,
			}

			_, err := deriveDestination(domain.CurationApproveNewSkill, learning, "")
			if err == nil {
				t.Fatal("expected unsafe learning ID error")
			}
		})
	}
}
