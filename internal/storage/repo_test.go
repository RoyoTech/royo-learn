package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupTestDB creates a temporary SQLite database, runs migrations, and
// inserts a test project. It returns the open DB and the project.
func setupTestDB(t *testing.T) (*DB, *domain.Project) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	proj := &domain.Project{
		ID:            domain.ProjectID(uuid.Must(uuid.NewV7()).String()),
		ProjectKey:    "test-project",
		DisplayName:   "Test Project",
		CanonicalPath: "/tmp/test-project",
		GitRemote:     "https://github.com/test/project.git",
		Fingerprint:   "abc123",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := SaveProject(ctx, tx, proj); err != nil {
		tx.Rollback()
		t.Fatalf("SaveProject: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	return db, proj
}

// newTestActor returns a standard Actor for test use.
func newTestActor() domain.Actor {
	return domain.Actor{
		Kind:      "agent",
		Name:      "test-agent",
		Model:     "test-model",
		SessionID: uuid.Must(uuid.NewV7()).String(),
	}
}

// now returns the current UTC time with millisecond truncation
// to avoid precision mismatches between Go and SQLite.
func utcNow() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

// newUUID returns a new UUID v7 string.
func newUUID() string {
	return uuid.Must(uuid.NewV7()).String()
}

// ---------------------------------------------------------------------------
// Project Repository Tests
// ---------------------------------------------------------------------------

func TestSaveProject(t *testing.T) {
	t.Parallel()

	db, _ := setupTestDB(t)
	ctx := context.Background()
	id := domain.ProjectID(newUUID())

	proj := &domain.Project{
		ID:            id,
		ProjectKey:    "save-test",
		DisplayName:   "Save Test",
		CanonicalPath: "/tmp/save-test",
		GitRemote:     "https://github.com/save/test.git",
		Fingerprint:   "fp-001",
		CreatedAt:     utcNow(),
		UpdatedAt:     utcNow(),
	}

	// Insert via transaction.
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := SaveProject(ctx, tx, proj); err != nil {
		tx.Rollback()
		t.Fatalf("SaveProject: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Verify retrieval.
	tx2, _ := db.DB.BeginTx(ctx, nil)
	defer tx2.Rollback()
	got, err := GetProject(ctx, tx2, id)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.ProjectKey != "save-test" {
		t.Errorf("ProjectKey = %q, want %q", got.ProjectKey, "save-test")
	}
}

func TestSaveProjectDuplicateKey(t *testing.T) {
	t.Parallel()

	db, _ := setupTestDB(t)
	ctx := context.Background()

	proj := &domain.Project{
		ID:            domain.ProjectID(newUUID()),
		ProjectKey:    "test-project", // same as setup project
		DisplayName:   "Duplicate",
		CanonicalPath: "/tmp/dup",
		Fingerprint:   "fp-dup",
		CreatedAt:     utcNow(),
		UpdatedAt:     utcNow(),
	}

	tx, _ := db.DB.BeginTx(ctx, nil)
	defer tx.Rollback()
	err := SaveProject(ctx, tx, proj)
	if err == nil {
		t.Fatal("expected duplicate key error, got nil")
	}
	var conflict *domain.ConflictError
	if !isDomainErrorType(err, &conflict) {
		t.Errorf("expected ConflictError, got %T: %v", err, err)
	}
}

func TestGetProjectByKey(t *testing.T) {
	t.Parallel()

	db, _ := setupTestDB(t)
	ctx := context.Background()

	tx, _ := db.DB.BeginTx(ctx, nil)
	defer tx.Rollback()
	got, err := GetProjectByKey(ctx, tx, "test-project")
	if err != nil {
		t.Fatalf("GetProjectByKey: %v", err)
	}
	if got.ProjectKey != "test-project" {
		t.Errorf("ProjectKey = %q, want %q", got.ProjectKey, "test-project")
	}
}

func TestGetProjectByKeyNotFound(t *testing.T) {
	t.Parallel()

	db, _ := setupTestDB(t)
	ctx := context.Background()

	tx, _ := db.DB.BeginTx(ctx, nil)
	defer tx.Rollback()
	_, err := GetProjectByKey(ctx, tx, "nonexistent")
	if err == nil {
		t.Fatal("expected not found error, got nil")
	}
	var notFound *domain.NotFoundError
	if !isDomainErrorType(err, &notFound) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

// ---------------------------------------------------------------------------
// Learning Repository Tests
// ---------------------------------------------------------------------------

func newTestLearning(projID domain.ProjectID) *domain.Learning {
	now := utcNow()
	return &domain.Learning{
		ID:                   domain.LearningID(newUUID()),
		ProjectID:            projID,
		Status:               domain.StatusCaptured,
		Type:                 domain.TypeProcedure,
		Title:                "Test Learning",
		Context:              "Testing context",
		Observation:          "Something was observed",
		ReusableLesson:       "Always test your code",
		RecommendedProcedure: []string{"step1", "step2"},
		Limits:               "Only applies to Go projects",
		ScopeGuess:           domain.ScopeProject,
		Confidence:           domain.ConfidenceHigh,
		EvidenceLevel:        domain.EvidenceModerate,
		ProposedDestination:  domain.DestProject,
		RetrievalTerms:       []string{"testing", "quality"},
		Fingerprint:          "fp-learn-001",
		NormalizedHash:       "hash001",
		Actor:                newTestActor(),
		Revision:             1,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

func TestSaveAndGetLearning(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()
	l := newTestLearning(proj.ID)

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Read back.
	tx2, _ := db.DB.BeginTx(ctx, nil)
	defer tx2.Rollback()
	got, err := GetLearning(ctx, tx2, l.ID)
	if err != nil {
		t.Fatalf("GetLearning: %v", err)
	}
	if got.Title != "Test Learning" {
		t.Errorf("Title = %q, want %q", got.Title, "Test Learning")
	}
	if got.Status != domain.StatusCaptured {
		t.Errorf("Status = %q, want %q", got.Status, domain.StatusCaptured)
	}
}

func TestSaveLearningDuplicate(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()
	l1 := newTestLearning(proj.ID)
	l2 := newTestLearning(proj.ID)
	l2.ID = l1.ID // same ID

	tx, _ := db.DB.BeginTx(ctx, nil)
	defer tx.Rollback()
	if err := SaveLearning(ctx, tx, l1); err != nil {
		t.Fatalf("SaveLearning (first): %v", err)
	}
	err := SaveLearning(ctx, tx, l2)
	if err == nil {
		t.Fatal("expected duplicate error, got nil")
	}
}

func TestUpdateLearning(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()
	l := newTestLearning(proj.ID)

	// Save.
	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Update.
	l.Title = "Updated Learning"
	l.Status = domain.StatusApproved
	l.UpdatedAt = utcNow()

	tx2, _ := db.DB.BeginTx(ctx, nil)
	if err := UpdateLearning(ctx, tx2, l); err != nil {
		tx2.Rollback()
		t.Fatalf("UpdateLearning: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Verify.
	tx3, _ := db.DB.BeginTx(ctx, nil)
	defer tx3.Rollback()
	got, err := GetLearning(ctx, tx3, l.ID)
	if err != nil {
		t.Fatalf("GetLearning: %v", err)
	}
	if got.Title != "Updated Learning" {
		t.Errorf("Title = %q after update", got.Title)
	}
	if got.Status != domain.StatusApproved {
		t.Errorf("Status = %q after update", got.Status)
	}
}

func TestListLearningsByProject(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()

	// Save 3 learnings.
	tx, _ := db.DB.BeginTx(ctx, nil)
	defer tx.Rollback()
	for i := 0; i < 3; i++ {
		l := newTestLearning(proj.ID)
		if err := SaveLearning(ctx, tx, l); err != nil {
			t.Fatalf("SaveLearning %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// List.
	tx2, _ := db.DB.BeginTx(ctx, nil)
	defer tx2.Rollback()
	list, err := ListLearnings(ctx, tx2, proj.ID, domain.LearningFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListLearnings: %v", err)
	}
	if len(list) < 3 {
		t.Errorf("ListLearnings returned %d, want at least 3", len(list))
	}

	// List with filter by status.
	list2, err := ListLearnings(ctx, tx2, proj.ID, domain.LearningFilter{
		Status: []domain.LearningStatus{domain.StatusCaptured},
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("ListLearnings with status filter: %v", err)
	}
	if len(list2) < 3 {
		t.Errorf("status-filtered ListLearnings returned %d, want at least 3", len(list2))
	}
}

func TestFindLearningByHash(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()
	l := newTestLearning(proj.ID)

	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	tx2, _ := db.DB.BeginTx(ctx, nil)
	defer tx2.Rollback()
	found, err := FindByHash(ctx, tx2, proj.ID, "hash001")
	if err != nil {
		t.Fatalf("FindByHash: %v", err)
	}
	if found == nil {
		t.Fatal("FindByHash returned nil for existing hash")
	}
	if found.Title != "Test Learning" {
		t.Errorf("Title = %q", found.Title)
	}
}

func TestFindLearningByHashNotFound(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()

	tx, _ := db.DB.BeginTx(ctx, nil)
	defer tx.Rollback()
	found, err := FindByHash(ctx, tx, proj.ID, "nonexistent-hash")
	if err != nil {
		t.Fatalf("FindByHash: %v", err)
	}
	if found != nil {
		t.Fatal("FindByHash returned result for nonexistent hash")
	}
}

func TestSaveAndGetRevisions(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()
	l := newTestLearning(proj.ID)

	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Save a revision.
	err := SaveRevision(ctx, db.DB, l.ID, 1, *l, "sha256-abc")
	if err != nil {
		t.Fatalf("SaveRevision: %v", err)
	}

	// Get revisions.
	revs, err := GetRevisions(ctx, db.DB, l.ID)
	if err != nil {
		t.Fatalf("GetRevisions: %v", err)
	}
	if len(revs) != 1 {
		t.Fatalf("GetRevisions returned %d, want 1", len(revs))
	}
	if revs[0].PayloadSHA256 != "sha256-abc" {
		t.Errorf("PayloadSHA256 = %q", revs[0].PayloadSHA256)
	}
}

// ---------------------------------------------------------------------------
// Evidence Repository Tests
// ---------------------------------------------------------------------------

func newTestEvidence(learningID domain.LearningID) *domain.Evidence {
	exitCode := 0
	return &domain.Evidence{
		ID:          domain.EvidenceID(newUUID()),
		LearningID:  learningID,
		Kind:        domain.KindTest,
		URI:         "file:///test/evidence.txt",
		Summary:     "Test evidence summary",
		SHA256:      "sha256-evid-001",
		Command:     []string{"go", "test", "./..."},
		ExitCode:    &exitCode,
		Redacted:    false,
		SizeBytes:   1024,
		CollectedAt: utcNow(),
	}
}

func TestSaveAndListEvidence(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()
	l := newTestLearning(proj.ID)

	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	ev := newTestEvidence(l.ID)
	tx2, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveEvidence(ctx, tx2, ev); err != nil {
		tx2.Rollback()
		t.Fatalf("SaveEvidence: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// List.
	tx3, _ := db.DB.BeginTx(ctx, nil)
	defer tx3.Rollback()
	list, err := ListEvidenceByLearning(ctx, tx3, l.ID)
	if err != nil {
		t.Fatalf("ListEvidenceByLearning: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListEvidenceByLearning returned %d, want 1", len(list))
	}
	if list[0].Kind != domain.KindTest {
		t.Errorf("Kind = %q", list[0].Kind)
	}
}

func TestGetEvidence(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()
	l := newTestLearning(proj.ID)

	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	ev := newTestEvidence(l.ID)
	tx2, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveEvidence(ctx, tx2, ev); err != nil {
		tx2.Rollback()
		t.Fatalf("SaveEvidence: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	tx3, _ := db.DB.BeginTx(ctx, nil)
	defer tx3.Rollback()
	got, err := GetEvidence(ctx, tx3, ev.ID)
	if err != nil {
		t.Fatalf("GetEvidence: %v", err)
	}
	if got.Summary != "Test evidence summary" {
		t.Errorf("Summary = %q", got.Summary)
	}
}

// ---------------------------------------------------------------------------
// Relation Repository Tests
// ---------------------------------------------------------------------------

func newTestRelation(sourceID, targetID domain.LearningID) *domain.LearningRelation {
	conf := 0.95
	return &domain.LearningRelation{
		ID:               domain.RelationID(newUUID()),
		SourceLearningID: sourceID,
		TargetLearningID: targetID,
		Relation:         domain.RelationRelated,
		Confidence:       &conf,
		Rationale:        "They are related by topic",
		Actor:            newTestActor(),
		CreatedAt:        utcNow(),
		UpdatedAt:        utcNow(),
	}
}

func TestSaveAndListRelations(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()
	l1 := newTestLearning(proj.ID)
	l2 := newTestLearning(proj.ID)

	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveLearning(ctx, tx, l1); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning l1: %v", err)
	}
	if err := SaveLearning(ctx, tx, l2); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning l2: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	rel := newTestRelation(l1.ID, l2.ID)
	tx2, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveRelation(ctx, tx2, rel); err != nil {
		tx2.Rollback()
		t.Fatalf("SaveRelation: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// List by source.
	tx3, _ := db.DB.BeginTx(ctx, nil)
	defer tx3.Rollback()
	list, err := ListRelationsBySource(ctx, tx3, l1.ID)
	if err != nil {
		t.Fatalf("ListRelationsBySource: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListRelationsBySource returned %d, want 1", len(list))
	}

	// List by target.
	list2, err := ListRelationsByTarget(ctx, tx3, l2.ID)
	if err != nil {
		t.Fatalf("ListRelationsByTarget: %v", err)
	}
	if len(list2) != 1 {
		t.Fatalf("ListRelationsByTarget returned %d, want 1", len(list2))
	}
}

func TestSaveRelationSelfReference(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()
	l := newTestLearning(proj.ID)

	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	rel := newTestRelation(l.ID, l.ID) // self-reference
	tx2, _ := db.DB.BeginTx(ctx, nil)
	defer tx2.Rollback()
	err := SaveRelation(ctx, tx2, rel)
	if err == nil {
		t.Fatal("expected self-reference error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Curation Repository Tests
// ---------------------------------------------------------------------------

func newTestCuration(learningID domain.LearningID) *domain.Curation {
	return &domain.Curation{
		ID:                domain.CurationID(newUUID()),
		LearningID:        learningID,
		Decision:          domain.CurationApproveProjectKnowledge,
		Rationale:         "Good evidence and clear lesson",
		Validation:        []domain.ValidationResult{{Check: "has-evidence", Pass: true, Note: ""}},
		AcceptanceChecks:  []domain.Check{{Name: "go-test", Command: "go test ./...", Expected: "pass"}},
		RollbackCondition: "If tests fail, revert",
		Actor:             newTestActor(),
		CreatedAt:         utcNow(),
	}
}

func TestSaveAndGetCuration(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()
	l := newTestLearning(proj.ID)

	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	c := newTestCuration(l.ID)
	tx2, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveCuration(ctx, tx2, c); err != nil {
		tx2.Rollback()
		t.Fatalf("SaveCuration: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	tx3, _ := db.DB.BeginTx(ctx, nil)
	defer tx3.Rollback()
	got, err := GetCuration(ctx, tx3, c.ID)
	if err != nil {
		t.Fatalf("GetCuration: %v", err)
	}
	if got.Decision != domain.CurationApproveProjectKnowledge {
		t.Errorf("Decision = %q", got.Decision)
	}
}

func TestListCurationsByLearning(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()
	l := newTestLearning(proj.ID)

	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	c := newTestCuration(l.ID)
	tx2, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveCuration(ctx, tx2, c); err != nil {
		tx2.Rollback()
		t.Fatalf("SaveCuration: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	tx3, _ := db.DB.BeginTx(ctx, nil)
	defer tx3.Rollback()
	list, err := ListCurationsByLearning(ctx, tx3, l.ID)
	if err != nil {
		t.Fatalf("ListCurationsByLearning: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListCurationsByLearning returned %d, want 1", len(list))
	}
}

// ---------------------------------------------------------------------------
// Occurrence Repository Tests
// ---------------------------------------------------------------------------

func newTestOccurrence(projID domain.ProjectID) *domain.Occurrence {
	learningID := domain.LearningID(newUUID())
	return &domain.Occurrence{
		ID:                   domain.OccurrenceID(newUUID()),
		LearningID:           &learningID,
		ProjectID:            projID,
		Fingerprint:          "fp-occ-001",
		Summary:              "Pattern detected during deployment",
		Evidence:             []domain.EvidenceRef{},
		LearningWasRetrieved: boolPtr(true),
		SkillWasActivated:    boolPtr(false),
		Outcome:              domain.OutcomeDetectedEarly,
		OccurredAt:           utcNow(),
		Actor:                newTestActor(),
	}
}

func boolPtr(b bool) *bool { return &b }

func TestSaveAndListOccurrences(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()

	// Create a learning first so FK is satisfied.
	l := newTestLearning(proj.ID)
	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveLearning(ctx, tx, l); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	occ := newTestOccurrence(proj.ID)
	occ.LearningID = &l.ID
	tx2, _ := db.DB.BeginTx(ctx, nil)
	if err := SaveOccurrence(ctx, tx2, occ); err != nil {
		tx2.Rollback()
		t.Fatalf("SaveOccurrence: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	tx3, _ := db.DB.BeginTx(ctx, nil)
	defer tx3.Rollback()
	list, err := ListOccurrences(ctx, tx3, proj.ID, 10)
	if err != nil {
		t.Fatalf("ListOccurrences: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListOccurrences returned %d, want 1", len(list))
	}
}

// ---------------------------------------------------------------------------
// Audit Repository Tests
// ---------------------------------------------------------------------------

func newTestAuditEvent() *domain.AuditEvent {
	return &domain.AuditEvent{
		ID:            domain.AuditEventID(newUUID()),
		OccurredAt:    utcNow(),
		Actor:         newTestActor(),
		Operation:     "create",
		EntityType:    "learning",
		EntityID:      "learning-123",
		PayloadSHA256: "sha256-audit",
		Result:        "success",
		Details:       map[string]any{"note": "test audit event"},
	}
}

func TestRecordAndListAuditEvents(t *testing.T) {
	t.Parallel()

	db, _ := setupTestDB(t)
	ctx := context.Background()

	evt := newTestAuditEvent()
	if err := RecordEvent(ctx, db.DB, evt); err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}

	events, err := ListEvents(ctx, db.DB, AuditEventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) < 1 {
		t.Fatal("ListEvents returned 0 events")
	}
	found := false
	for _, e := range events {
		if e.ID == evt.ID {
			found = true
			if e.Operation != "create" {
				t.Errorf("Operation = %q", e.Operation)
			}
		}
	}
	if !found {
		t.Error("inserted audit event not found in list")
	}
}

func TestAuditAppendOnly(t *testing.T) {
	t.Parallel()

	db, _ := setupTestDB(t)
	ctx := context.Background()

	evt1 := newTestAuditEvent()
	if err := RecordEvent(ctx, db.DB, evt1); err != nil {
		t.Fatalf("RecordEvent 1: %v", err)
	}

	evt2 := newTestAuditEvent()
	evt2.EntityID = evt1.EntityID // same entity, different event
	if err := RecordEvent(ctx, db.DB, evt2); err != nil {
		t.Fatalf("RecordEvent 2: %v", err)
	}

	// Verify both events exist — INSERT-only means both are preserved.
	events, err := ListEvents(ctx, db.DB, AuditEventFilter{Limit: 100})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}

	count := 0
	for _, e := range events {
		if e.EntityID == evt1.EntityID {
			count++
		}
	}
	if count < 2 {
		t.Errorf("expected 2 audit events for entity %q, got %d", evt1.EntityID, count)
	}
}

// ---------------------------------------------------------------------------
// FTS5 Search Tests
// ---------------------------------------------------------------------------

func TestFTSSearch(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()

	// Save learnings with searchable content.
	tx, _ := db.DB.BeginTx(ctx, nil)
	l1 := newTestLearning(proj.ID)
	l1.Title = "Database Connection Pooling"
	l1.Context = "PostgreSQL connection management"
	l1.Observation = "Pool exhaustion caused timeouts"
	l1.ReusableLesson = "Always configure max connections"
	l1.RetrievalTerms = []string{"database", "postgresql", "pooling"}

	l2 := newTestLearning(proj.ID)
	l2.Title = "Go Error Wrapping"
	l2.Context = "Error handling patterns in Go"
	l2.Observation = "Unwrapped errors lost context"
	l2.ReusableLesson = "Use fmt.Errorf with %w"
	l2.RetrievalTerms = []string{"go", "error", "wrapping"}

	if err := SaveLearning(ctx, tx, l1); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning l1: %v", err)
	}
	if err := SaveLearning(ctx, tx, l2); err != nil {
		tx.Rollback()
		t.Fatalf("SaveLearning l2: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Search for "database".
	results, err := Search(ctx, db, proj.ID, "database")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 1 {
		t.Fatal("Search returned 0 results for 'database'")
	}

	// Search for something that shouldn't match.
	results2, err := Search(ctx, db, proj.ID, "ruby")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results2) != 0 {
		t.Errorf("Search for 'ruby' returned %d results, want 0", len(results2))
	}

	// Search with special characters — must not error.
	results3, err := Search(ctx, db, proj.ID, "DROP TABLE learnings")
	if err != nil {
		t.Fatalf("Search with special chars: %v", err)
	}
	_ = results3

	// Search with empty query — should return all or gracefully handle.
	results4, err := Search(ctx, db, proj.ID, "")
	if err != nil {
		t.Fatalf("Search empty: %v", err)
	}
	_ = results4
}

// ---------------------------------------------------------------------------
// Transaction Tests
// ---------------------------------------------------------------------------

func TestWithTxCommit(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()

	l := newTestLearning(proj.ID)
	err := WithTx(ctx, db, func(tx *sql.Tx) error {
		return SaveLearning(ctx, tx, l)
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}

	// Verify learning was persisted.
	tx, _ := db.DB.BeginTx(ctx, nil)
	defer tx.Rollback()
	got, err := GetLearning(ctx, tx, l.ID)
	if err != nil {
		t.Fatalf("GetLearning after WithTx: %v", err)
	}
	if got == nil {
		t.Fatal("learning not persisted after WithTx commit")
	}
}

func TestWithTxRollback(t *testing.T) {
	t.Parallel()

	db, proj := setupTestDB(t)
	ctx := context.Background()

	l := newTestLearning(proj.ID)
	err := WithTx(ctx, db, func(tx *sql.Tx) error {
		if err := SaveLearning(ctx, tx, l); err != nil {
			return err
		}
		return sql.ErrTxDone // forces rollback
	})
	if err == nil {
		t.Fatal("expected error from WithTx rollback, got nil")
	}

	// Verify learning was NOT persisted.
	tx, _ := db.DB.BeginTx(ctx, nil)
	defer tx.Rollback()
	_, getErr := GetLearning(ctx, tx, l.ID)
	if getErr == nil {
		t.Fatal("learning persisted despite rollback")
	}
}

// ---------------------------------------------------------------------------
// Helper: type assertion for domain errors
// ---------------------------------------------------------------------------

func isDomainErrorType(err error, target interface{}) bool {
	switch target.(type) {
	case **domain.NotFoundError:
		_, ok := err.(*domain.NotFoundError)
		return ok
	case **domain.ConflictError:
		_, ok := err.(*domain.ConflictError)
		return ok
	case **domain.ValidationError:
		_, ok := err.(*domain.ValidationError)
		return ok
	case **domain.PermissionError:
		_, ok := err.(*domain.PermissionError)
		return ok
	default:
		return false
	}
}
