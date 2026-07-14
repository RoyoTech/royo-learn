package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/capture"
	"agent-royo-learn/internal/curate"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/evidence"
	"agent-royo-learn/internal/publish"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/storage/storagetest"

	"github.com/google/uuid"
)

// TestP1_E2E_ProcedurePreservedOnRepublish is the live end-to-end demo that
// proves P1 is fixed: two learnings in the same area are published
// sequentially, and BOTH preserve their "### Procedimiento" section in the
// resulting SKILL.md.
//
// This test uses a real SQLite DB, real capture/curate/publish services, and
// real file I/O — no mocks. It mirrors the actual user flow:
//
//	capture → curate (approve) → preview → publish (learning A)
//	capture → curate (approve) → preview → publish (learning B, same skill)
//
// The second publish triggers buildPublishContents, which (before the fix)
// parsed the existing body with the defective parseSkillSections and lost A's
// procedure. After the fix, sections are rebuilt from the DB, preserving both.
func TestP1_E2E_ProcedurePreservedOnRepublish(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	db := storagetest.OpenTemp(t)
	now := time.Now().UTC()

	// Use "padreseducadores.org" as the project key to also verify that the
	// dot-sanitization cleanup works: the skill directory should be
	// skills/padreseducadores-org-<area>/, NOT skills/padreseducadores.org-<area>/.
	project := &domain.Project{
		ID:            domain.ProjectID(uuid.Must(uuid.NewV7()).String()),
		ProjectKey:    "padreseducadores.org",
		DisplayName:   "Padres Educadores",
		CanonicalPath: projectRoot,
		Fingerprint:   "demo-p1",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := storage.WithTx(ctx, db, func(tx *sql.Tx) error {
		return storage.SaveProject(ctx, tx, project)
	}); err != nil {
		t.Fatalf("save project: %v", err)
	}

	actor := domain.Actor{Kind: "agent", Name: "e2e-demo"}
	evidenceSvc, err := evidence.NewService(projectRoot, nil)
	if err != nil {
		t.Fatalf("evidence service: %v", err)
	}
	captureService := capture.NewServiceWithEvidence(db, filepath.Join(projectRoot, ".royo-learn", "records"), evidenceSvc)

	// Shared retrieval terms — both learnings must resolve to the SAME skill.
	// The order is intentionally different to also verify P2 (deterministic area).
	sharedTerms := []string{"dashboard_data_cursos", "cadena continua Unidad Test"}

	// --- Learning A: the Unidad→Test rule with a 3-step procedure ---
	learningA, err := captureService.Capture(ctx, project.ID, &capture.CaptureInput{
		Title:          "Dashboard cadena continua Unidad→Test",
		Context:        "Dashboard de profesor en padreseducadores.org",
		Observation:    "La cadena Unidad→Test no debe tener huecos ni solapamientos.",
		Lesson:         "La cadena Unidad→Test no debe tener huecos.",
		Type:           domain.TypeProcedure,
		Scope:          domain.ScopeProject,
		Destination:    domain.DestSkill,
		Confidence:     domain.ConfidenceHigh,
		EvidenceLevel:  domain.EvidenceModerate,
		Recommended:    []string{"Extraer unidades y tests.", "Ordenar por fecha.", "Verificar huecos."},
		Limits:         "Solo para dashboards de profesor.",
		RetrievalTerms: sharedTerms,
		Actor:          actor,
	})
	if err != nil {
		t.Fatalf("capture A: %v", err)
	}
	t.Logf("Captured learning A: %s (status: %s)", learningA.LearningID, learningA.Status)

	// Evidence for learning A (required for curation approval), attached through
	// the public capture path rather than by writing SQL.
	if _, err := captureService.AddEvidence(ctx, project.ID, &capture.AddEvidenceInput{
		LearningID: learningA.LearningID,
		Items: []evidence.Item{{
			Kind:    domain.KindTest,
			Summary: "Integration test verifies the Unidad→Test chain has no gaps",
			Source:  "test://dashboard-cadena-continua",
			Content: "--- PASS: chain has no gaps",
		}},
		Actor: actor,
	}); err != nil {
		t.Fatalf("add evidence A: %v", err)
	}

	// --- Learning B: a related rule about test duration, same area ---
	learningB, err := captureService.Capture(ctx, project.ID, &capture.CaptureInput{
		Title:          "Verificar duración de tests en la cadena",
		Context:        "Dashboard de profesor en padreseducadores.org",
		Observation:    "Cada test en la cadena debe durar exactamente 7 días.",
		Lesson:         "Cada test en la cadena debe durar 7 días exactos.",
		Type:           domain.TypeProcedure,
		Scope:          domain.ScopeProject,
		Destination:    domain.DestSkill,
		Confidence:     domain.ConfidenceHigh,
		EvidenceLevel:  domain.EvidenceModerate,
		Recommended:    []string{"Confirmar que cada test dura 7 días.", "Verificar que la cadena cubre hasta el último test."},
		Limits:         "Solo para tests del dashboard de profesor.",
		RetrievalTerms: []string{"cadena continua Unidad Test", "dashboard_data_cursos"}, // same terms, different order
		Actor:          actor,
	})
	if err != nil {
		t.Fatalf("capture B: %v", err)
	}
	t.Logf("Captured learning B: %s (status: %s)", learningB.LearningID, learningB.Status)

	// Evidence for learning B (required for curation approval), attached through
	// the public capture path rather than by writing SQL.
	if _, err := captureService.AddEvidence(ctx, project.ID, &capture.AddEvidenceInput{
		LearningID: learningB.LearningID,
		Items: []evidence.Item{{
			Kind:    domain.KindTest,
			Summary: "Integration test verifies each test lasts 7 days",
			Source:  "test://dashboard-test-duracion",
			Content: "--- PASS: each test lasts 7 days",
		}},
		Actor: actor,
	}); err != nil {
		t.Fatalf("add evidence B: %v", err)
	}

	// --- Curate (approve) both learnings ---
	curateService := curate.NewService(db, filepath.Join(projectRoot, ".royo-learn", "records"))

	if _, err := curateService.Curate(ctx, project.ID, &curate.CurateInput{
		LearningID: learningA.LearningID,
		Decision:   domain.CurationApproveNewSkill,
		Rationale:  "Regla validada del dashboard",
		Actor:      actor,
	}); err != nil {
		t.Fatalf("curate A: %v", err)
	}
	t.Logf("Curated learning A: approved (new skill)")

	if _, err := curateService.Curate(ctx, project.ID, &curate.CurateInput{
		LearningID: learningB.LearningID,
		Decision:   domain.CurationApproveSkillUpdate,
		Rationale:  "Regla complementaria sobre duración de tests",
		Actor:      actor,
	}); err != nil {
		t.Fatalf("curate B: %v", err)
	}
	t.Logf("Curated learning B: approved (skill update)")

	// --- Update curation destinations to the area-based skill name ---
	// The curation service sets Path to <learning-id>/SKILL.md by default.
	// The multi-target publish path (which groups learnings by area into a
	// single skill file) is only activated when the destination path matches
	// the auto-derived name: SkillName(projectKey, SkillArea(learning)).
	// In a real deployment, the curator or a pre-publish step would set this.
	// Here we update the DB directly to simulate the correct flow.
	autoName := publish.SkillName("padreseducadores.org", publish.SkillArea(&domain.Learning{
		RetrievalTerms: sharedTerms,
	}))
	// The destination Path should be "<autoName>/SKILL.md" so that
	// filepath.Dir(path) == autoName, which activates the multi-target path.
	// The Publish function then sets dest.Path = autoName (just the directory).
	areaPath := autoName + "/SKILL.md"
	for _, lid := range []domain.LearningID{learningA.LearningID, learningB.LearningID} {
		readTx, txErr := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if txErr != nil {
			t.Fatalf("begin tx: %v", txErr)
		}
		curations, err := storage.ListCurationsByLearning(ctx, readTx, lid)
		readTx.Rollback()
		if err != nil || len(curations) == 0 {
			t.Fatalf("list curations for %s: %v", lid, err)
		}
		c := curations[0]
		c.Destination.Path = areaPath
		destJSON, _ := json.Marshal(c.Destination)
		_, err = db.DB.ExecContext(ctx,
			`UPDATE curations SET destination_json = ? WHERE id = ?`,
			string(destJSON), string(c.ID))
		if err != nil {
			t.Fatalf("update curation destination for %s: %v", lid, err)
		}
	}
	t.Logf("Updated curation destinations to area-based path: %s", areaPath)

	// --- Publish learning A ---
	publishService := publish.NewService(
		db,
		projectRoot,
		filepath.Join(projectRoot, ".royo-learn", "backups"),
		filepath.Join(projectRoot, ".royo-learn"),
	)

	previewA, err := publishService.Preview(ctx, project.ID, &publish.PreviewInput{
		LearningID: learningA.LearningID,
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("preview A: %v", err)
	}

	pubA, err := publishService.Publish(ctx, project.ID, &publish.PublishInput{
		LearningID:  learningA.LearningID,
		PreviewHash: previewA.Preview.PreviewHash,
		Force:       true,
		Actor:       actor,
	})
	if err != nil {
		t.Fatalf("publish A: %v", err)
	}
	if pubA.Publication.Status != domain.PubStatusCompleted {
		t.Fatalf("publish A status = %q, want %q", pubA.Publication.Status, domain.PubStatusCompleted)
	}
	t.Logf("Published learning A: status=%s", pubA.Publication.Status)

	// --- Publish learning B (triggers the re-publish path) ---
	previewB, err := publishService.Preview(ctx, project.ID, &publish.PreviewInput{
		LearningID: learningB.LearningID,
		Actor:      actor,
	})
	if err != nil {
		t.Fatalf("preview B: %v", err)
	}

	pubB, err := publishService.Publish(ctx, project.ID, &publish.PublishInput{
		LearningID:  learningB.LearningID,
		PreviewHash: previewB.Preview.PreviewHash,
		Force:       true,
		Actor:       actor,
	})
	if err != nil {
		t.Fatalf("publish B: %v", err)
	}
	if pubB.Publication.Status != domain.PubStatusCompleted {
		t.Fatalf("publish B status = %q, want %q", pubB.Publication.Status, domain.PubStatusCompleted)
	}
	t.Logf("Published learning B: status=%s", pubB.Publication.Status)

	// --- Read the resulting SKILL.md ---
	// The skill area is derived from the sorted-first retrieval term.
	// Terms: "dashboard_data_cursos", "cadena continua Unidad Test"
	// Sorted: "cadena continua Unidad Test" < "dashboard_data_cursos"
	// First: "cadena continua Unidad Test" → sanitized → "cadena-continua-unidad-test"
	// Project key sanitized: "padreseducadores.org" → "padreseducadores-org"
	// Skill name: "padreseducadores-org-cadena-continua-unidad-test"
	// Skill path: skills/padreseducadores-org-cadena-continua-unidad-test/SKILL.md
	skillPath := filepath.Join(projectRoot, "skills", "padreseducadores-org-cadena-continua-unidad-test", "SKILL.md")

	// List the skill directory to see what's there.
	skillDir := filepath.Join(projectRoot, "skills", "padreseducadores-org-cadena-continua-unidad-test")
	dirEntries, _ := os.ReadDir(skillDir)
	for _, de := range dirEntries {
		t.Logf("  skill dir entry: %s (dir=%v)", de.Name(), de.IsDir())
	}

	content, err := os.ReadFile(skillPath)
	if err != nil {
		// Try alternative read method.
		f, openErr := os.Open(skillPath)
		if openErr != nil {
			t.Fatalf("read SKILL.md at %s: ReadFile=%v, Open=%v", skillPath, err, openErr)
		}
		defer f.Close()
		stat, _ := f.Stat()
		t.Logf("  SKILL.md size: %d bytes", stat.Size())
		buf := make([]byte, stat.Size())
		_, readErr := f.Read(buf)
		if readErr != nil {
			t.Fatalf("read SKILL.md via Open: %v", readErr)
		}
		content = buf
	}

	skillContent := string(content)
	t.Logf("=== SKILL.md content ===\n%s\n=== end SKILL.md ===", skillContent)

	// --- Verify BOTH procedures are intact ---
	procASteps := []string{"Extraer unidades y tests.", "Ordenar por fecha.", "Verificar huecos."}
	procBSteps := []string{"Confirmar que cada test dura 7 días.", "Verificar que la cadena cubre hasta el último test."}

	for _, step := range procASteps {
		if !strings.Contains(skillContent, step) {
			t.Errorf("P1 E2E: learning A procedure step %q missing from SKILL.md", step)
		}
	}
	for _, step := range procBSteps {
		if !strings.Contains(skillContent, step) {
			t.Errorf("P1 E2E: learning B procedure step %q missing from SKILL.md", step)
		}
	}

	// Verify the "### Procedimiento" section header appears (at least twice — once per learning).
	procCount := strings.Count(skillContent, "### Procedimiento")
	if procCount < 2 {
		t.Errorf("P1 E2E: expected at least 2 '### Procedimiento' sections, got %d", procCount)
	}

	// Verify learning A's rule is intact (not corrupted by the re-publish).
	if !strings.Contains(skillContent, "La cadena Unidad→Test no debe tener huecos.") {
		t.Errorf("P1 E2E: learning A rule text missing from SKILL.md")
	}

	// Verify learning B's rule is intact.
	if !strings.Contains(skillContent, "Cada test en la cadena debe durar 7 días exactos.") {
		t.Errorf("P1 E2E: learning B rule text missing from SKILL.md")
	}

	// Verify the skill directory name does NOT contain dots (cleanup check).
	if _, err := os.Stat(skillDir); err != nil {
		t.Errorf("P1 E2E: skill directory should be sanitized (dots→dashes): %s", skillDir)
	}
	// And the old-style directory (with dots) should NOT exist.
	oldDir := filepath.Join(projectRoot, "skills", "padreseducadores.org-cadena-continua-unidad-test")
	if _, err := os.Stat(oldDir); err == nil {
		t.Errorf("P1 E2E: old-style directory with dots should NOT exist: %s", oldDir)
	}

	t.Logf("P1 E2E: SUCCESS — both learnings' procedures preserved in SKILL.md")
}
