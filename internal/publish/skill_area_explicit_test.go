package publish

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/curate"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/record"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/storage/storagetest"

	"github.com/google/uuid"
)

// P2 — Explicit skill area.
//
// SkillArea derives the area from sorted retrieval_terms, so two learnings of
// the same domain with DIFFERENT vocabulary land in different skills, and the
// auto-derived area name can be ugly. The fix lets the curator set an EXPLICIT
// area in curate_learning; if provided it wins; if omitted, the deterministic
// derivation is the fallback.
//
// These tests use a real SQLite DB, real curate/publish services, and
// real file I/O — no mocks. They mirror internal/integration/p1_procedure_e2e_test.go.

// p2Harness holds the shared setup for P2 e2e tests.
type p2Harness struct {
	ctx         context.Context
	project     *domain.Project
	db          *storage.DB
	projectRoot string
	curateSvc   *curate.Service
	publishSvc  *Service
	actor       domain.Actor
	now         time.Time
}

func newP2Harness(t *testing.T) *p2Harness {
	t.Helper()
	ctx := context.Background()
	projectRoot := t.TempDir()
	db := storagetest.OpenTemp(t)
	now := time.Now().UTC()

	project := &domain.Project{
		ID:            domain.ProjectID(uuid.Must(uuid.NewV7()).String()),
		ProjectKey:    "padreseducadores.org",
		DisplayName:   "Padres Educadores",
		CanonicalPath: projectRoot,
		Fingerprint:   "demo-p2",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := storage.WithTx(ctx, db, func(tx *sql.Tx) error {
		return storage.SaveProject(ctx, tx, project)
	}); err != nil {
		t.Fatalf("save project: %v", err)
	}

	actor := domain.Actor{Kind: "agent", Name: "p2-demo"}
	return &p2Harness{
		ctx:         ctx,
		project:     project,
		db:          db,
		projectRoot: projectRoot,
		curateSvc:   curate.NewService(db, filepath.Join(projectRoot, ".royo-learn", "records")),
		publishSvc: NewService(
			db,
			projectRoot,
			filepath.Join(projectRoot, ".royo-learn", "backups"),
			filepath.Join(projectRoot, ".royo-learn"),
		),
		actor: actor,
		now:   now,
	}
}

type p2CaptureInput struct {
	Title          string
	Context        string
	Observation    string
	Lesson         string
	Type           domain.LearningType
	Destination    domain.DestinationType
	Recommended    []string
	Limits         string
	RetrievalTerms []string
	EvidenceLevel  domain.EvidenceLevel
	Confidence     domain.Confidence
	Scope          domain.Scope
}

// captureAndEvidence captures a learning and attaches one evidence record
// (required for curation approval). Returns the captured learning ID.
func (h *p2Harness) captureAndEvidence(t *testing.T, input p2CaptureInput) domain.LearningID {
	t.Helper()
	if input.EvidenceLevel == "" {
		input.EvidenceLevel = domain.EvidenceModerate
	}
	if input.Confidence == "" {
		input.Confidence = domain.ConfidenceHigh
	}
	if input.Scope == "" {
		input.Scope = domain.ScopeProject
	}
	if input.Destination == "" {
		input.Destination = domain.DestSkill
	}
	learning := &domain.Learning{
		ID:                   domain.LearningID(uuid.Must(uuid.NewV7()).String()),
		ProjectID:            h.project.ID,
		Status:               domain.StatusCaptured,
		Type:                 input.Type,
		Title:                input.Title,
		Context:              input.Context,
		Observation:          input.Observation,
		ReusableLesson:       input.Lesson,
		RecommendedProcedure: input.Recommended,
		Limits:               input.Limits,
		ScopeGuess:           input.Scope,
		Confidence:           input.Confidence,
		EvidenceLevel:        input.EvidenceLevel,
		ProposedDestination:  input.Destination,
		RetrievalTerms:       input.RetrievalTerms,
		Actor:                h.actor,
		Revision:             1,
		CreatedAt:            h.now,
		UpdatedAt:            h.now,
	}
	hash, err := domain.ComputeHash(learning)
	if err != nil {
		t.Fatalf("hash %q: %v", input.Title, err)
	}
	learning.NormalizedHash = hash
	learning.Fingerprint = hash
	ev := &domain.Evidence{
		ID:          domain.EvidenceID(uuid.Must(uuid.NewV7()).String()),
		LearningID:  learning.ID,
		Kind:        domain.KindTest,
		URI:         "test://" + string(learning.ID),
		Summary:     "evidence for " + input.Title,
		SHA256:      "p2-ev-" + string(learning.ID),
		CollectedAt: h.now,
	}
	if err := storage.WithTx(h.ctx, h.db, func(tx *sql.Tx) error {
		if err := storage.SaveLearning(h.ctx, tx, learning); err != nil {
			return err
		}
		return storage.SaveEvidence(h.ctx, tx, ev)
	}); err != nil {
		t.Fatalf("save learning and evidence for %s: %v", learning.ID, err)
	}
	if err := record.WriteRecord(filepath.Join(h.projectRoot, ".royo-learn", "records"), learning); err != nil {
		t.Fatalf("materialize learning %s: %v", learning.ID, err)
	}
	return learning.ID
}

// curateWithArea curates a learning with an optional explicit area.
func (h *p2Harness) curateWithArea(t *testing.T, lid domain.LearningID, decision domain.CurationDecision, area string) {
	t.Helper()
	if _, err := h.curateSvc.Curate(h.ctx, h.project.ID, &curate.CurateInput{
		LearningID: lid,
		Decision:   decision,
		Rationale:  "P2 curate with explicit area",
		Actor:      h.actor,
		Area:       area,
	}); err != nil {
		t.Fatalf("curate %s: %v", lid, err)
	}
}

// previewAndPublish runs preview then publish for a learning, returning the
// publication status.
func (h *p2Harness) previewAndPublish(t *testing.T, lid domain.LearningID) domain.PublicationStatus {
	t.Helper()
	prev, err := h.publishSvc.Preview(h.ctx, h.project.ID, &PreviewInput{
		LearningID: lid,
		Actor:      h.actor,
	})
	if err != nil {
		t.Fatalf("preview %s: %v", lid, err)
	}
	pub, err := h.publishSvc.Publish(h.ctx, h.project.ID, &PublishInput{
		Apply:       true,
		LearningID:  lid,
		PreviewHash: prev.Preview.PreviewHash,
		Force:       true,
		Actor:       h.actor,
	})
	if err != nil {
		t.Fatalf("publish %s: %v", lid, err)
	}
	if pub.Publication.Status != domain.PubStatusCompleted {
		t.Fatalf("publish %s status = %q, want %q", lid, pub.Publication.Status, domain.PubStatusCompleted)
	}
	return pub.Publication.Status
}

// expectedSkillPath returns the absolute path to a skill file given the area.
func (h *p2Harness) expectedSkillPath(area string) string {
	name := SkillName(h.project.ProjectKey, area)
	return filepath.Join(h.projectRoot, SkillsDir, name, "SKILL.md")
}

// TestP2_ExplicitAreaGroupsDifferentTerms is the core P2 acceptance: two
// learnings with DIFFERENT retrieval_terms, both curated with the SAME
// explicit area, land in the SAME skill file even though their terms differ.
// Must not regress P1 (both procedures preserved on republish).
func TestP2_ExplicitAreaGroupsDifferentTerms(t *testing.T) {
	h := newP2Harness(t)
	const area = "dashboard-datos"

	// Learning A: terms ["dashboard","datos"], 3-step procedure.
	lidA := h.captureAndEvidence(t, p2CaptureInput{
		Title:          "Dashboard de datos del profesor",
		Context:        "Dashboard de profesor en padreseducadores.org",
		Observation:    "El dashboard agrega datos de cursos y alumnos.",
		Lesson:         "El dashboard debe agregar datos correctamente.",
		Type:           domain.TypeProcedure,
		Destination:    domain.DestSkill,
		Recommended:    []string{"Cargar datos.", "Filtrar por curso.", "Verificar totales."},
		Limits:         "Solo para dashboards de profesor.",
		RetrievalTerms: []string{"dashboard", "datos"},
	})

	// Learning B: DIFFERENT terms ["graficos","reportes"], 2-step procedure.
	lidB := h.captureAndEvidence(t, p2CaptureInput{
		Title:          "Reportes gráficos del dashboard",
		Context:        "Dashboard de profesor en padreseducadores.org",
		Observation:    "Los reportes gráficos deben usar los mismos datos.",
		Lesson:         "Los reportes gráficos comparten la fuente de datos.",
		Type:           domain.TypeProcedure,
		Destination:    domain.DestSkill,
		Recommended:    []string{"Generar gráfico.", "Exportar reporte."},
		Limits:         "Solo para reportes del dashboard.",
		RetrievalTerms: []string{"graficos", "reportes"},
	})

	// Curate both with the SAME explicit area. A creates the skill; B updates it.
	h.curateWithArea(t, lidA, domain.CurationApproveNewSkill, area)
	h.curateWithArea(t, lidB, domain.CurationApproveSkillUpdate, area)

	// Publish sequentially.
	h.previewAndPublish(t, lidA)
	h.previewAndPublish(t, lidB)

	// BOTH must land in skills/padreseducadores-org-dashboard-datos/SKILL.md.
	skillPath := h.expectedSkillPath(area)
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read SKILL.md at %s: %v", skillPath, err)
	}
	skillContent := string(content)
	t.Logf("=== P2 SKILL.md (%s) ===\n%s\n=== end ===", area, skillContent)

	// Both procedures must be present.
	for _, step := range []string{"Cargar datos.", "Filtrar por curso.", "Verificar totales."} {
		if !strings.Contains(skillContent, step) {
			t.Errorf("P2: learning A procedure step %q missing from SKILL.md", step)
		}
	}
	for _, step := range []string{"Generar gráfico.", "Exportar reporte."} {
		if !strings.Contains(skillContent, step) {
			t.Errorf("P2: learning B procedure step %q missing from SKILL.md", step)
		}
	}

	// "### Procedimiento" must appear at least twice (once per learning).
	procCount := strings.Count(skillContent, "### Procedimiento")
	if procCount < 2 {
		t.Errorf("P2: expected at least 2 '### Procedimiento' sections, got %d", procCount)
	}

	// The frontmatter name must be the explicit area skill name.
	if fm, err := ParseFrontmatter(skillContent); err == nil {
		wantName := SkillName(h.project.ProjectKey, area)
		if fm.Name != wantName {
			t.Errorf("P2: frontmatter name = %q, want %q", fm.Name, wantName)
		}
	} else {
		t.Errorf("P2: parse frontmatter: %v", err)
	}

	// Sanity: the two learnings, if derived automatically, would produce
	// DIFFERENT areas — proving the explicit area is what groups them.
	areaA := SkillArea(&domain.Learning{RetrievalTerms: []string{"dashboard", "datos"}})
	areaB := SkillArea(&domain.Learning{RetrievalTerms: []string{"graficos", "reportes"}})
	if areaA == areaB {
		t.Logf("note: derived areas coincide (%q); test still valid but less discriminating", areaA)
	}
	if areaA == area || areaB == area {
		t.Errorf("P2: explicit area %q should differ from derived areas (%q, %q) to prove grouping", area, areaA, areaB)
	}
}

// TestP2_ExplicitAreaOverridesDerivation proves that an explicit area wins
// over the automatic derivation, even when the derived area would be
// something else entirely.
func TestP2_ExplicitAreaOverridesDerivation(t *testing.T) {
	h := newP2Harness(t)
	const area = "dashboard-datos"

	// Terms that derive to "anti-patrn" (sorted first), NOT "dashboard-datos".
	terms := []string{"anti-patrn", "zzz"}
	derived := SkillArea(&domain.Learning{RetrievalTerms: terms})
	if derived == area {
		t.Fatalf("test setup invalid: derived area %q == explicit %q", derived, area)
	}
	t.Logf("derived area = %q (explicit will override to %q)", derived, area)

	lidA := h.captureAndEvidence(t, p2CaptureInput{
		Title:          "Evitar anti-patrón 3b",
		Context:        "Refactor en padreseducadores.org",
		Observation:    "El anti-patrón 3b aparece al acoplar lógica de presentación.",
		Lesson:         "No acoples lógica de presentación en el dominio.",
		Type:           domain.TypeProcedure,
		Destination:    domain.DestSkill,
		Recommended:    []string{"Separar capas.", "Inyectar dependencias."},
		Limits:         "Solo para arquitectura hexagonal.",
		RetrievalTerms: terms,
	})

	h.curateWithArea(t, lidA, domain.CurationApproveNewSkill, area)
	h.previewAndPublish(t, lidA)

	// The published skill must be the explicit area, NOT the derived one.
	skillPath := h.expectedSkillPath(area)
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("P2: expected skill at %s (explicit area), got: %v", skillPath, err)
	}
	// The derived-area skill must NOT exist.
	derivedPath := h.expectedSkillPath(derived)
	if _, err := os.Stat(derivedPath); err == nil {
		t.Errorf("P2: derived-area skill should NOT exist at %s (explicit area overrode derivation)", derivedPath)
	}
}

// TestP2_NoAreaFallbackStillDerives guards the fallback: when no explicit area
// is provided, the deterministic derivation from retrieval_terms drives the
// skill name. This mirrors the P1 flow (a pre-publish step sets the
// area-based destination path), proving the fallback path is unchanged.
func TestP2_NoAreaFallbackStillDerives(t *testing.T) {
	h := newP2Harness(t)

	terms := []string{"dashboard", "datos"}
	derived := SkillArea(&domain.Learning{RetrievalTerms: terms})
	wantName := SkillName(h.project.ProjectKey, derived)

	lidA := h.captureAndEvidence(t, p2CaptureInput{
		Title:          "Dashboard datos fallback",
		Context:        "Dashboard de profesor en padreseducadores.org",
		Observation:    "Sin área explícita, se deriva de los términos.",
		Lesson:         "La derivación automática sigue funcionando.",
		Type:           domain.TypeProcedure,
		Destination:    domain.DestSkill,
		Recommended:    []string{"Paso uno.", "Paso dos."},
		Limits:         "Solo fallback.",
		RetrievalTerms: terms,
	})

	// Curate with NO explicit area (fallback).
	h.curateWithArea(t, lidA, domain.CurationApproveNewSkill, "")

	// In the no-area fallback, multi-target activates only when the
	// destination path matches the derived name (the pre-P2 flow). Simulate
	// the curator/pre-publish step by setting the path to the derived area,
	// exactly as p1_procedure_e2e_test.go does. This proves the fallback
	// derivation flows through publish unchanged.
	setCurationPathToArea(t, h, lidA, wantName)

	h.previewAndPublish(t, lidA)

	skillPath := filepath.Join(h.projectRoot, SkillsDir, wantName, "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("P2 fallback: expected skill at %s, got: %v", skillPath, err)
	}
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("P2 fallback: read: %v", err)
	}
	if fm, perr := ParseFrontmatter(string(content)); perr == nil {
		if fm.Name != wantName {
			t.Errorf("P2 fallback: frontmatter name = %q, want %q", fm.Name, wantName)
		}
	} else {
		t.Errorf("P2 fallback: parse frontmatter: %v", perr)
	}
}

// TestP2_PreviewMatchesPublishPath proves preview == publish: the path
// Preview reports is the SAME path Publish then writes. This also covers the
// preview path-doubling fix (previously preview set autoName + "/SKILL.md",
// which ResolveSkillPublishTargets doubled to autoName/SKILL.md/SKILL.md).
func TestP2_PreviewMatchesPublishPath(t *testing.T) {
	h := newP2Harness(t)
	const area = "dashboard-datos"
	wantPath := filepath.Join(SkillName(h.project.ProjectKey, area), "SKILL.md")

	lidA := h.captureAndEvidence(t, p2CaptureInput{
		Title:          "Preview==publish path check",
		Context:        "Dashboard de profesor en padreseducadores.org",
		Observation:    "Preview y publish deben coincidir en la ruta destino.",
		Lesson:         "La ruta de preview debe ser la misma que la de publish.",
		Type:           domain.TypeProcedure,
		Destination:    domain.DestSkill,
		Recommended:    []string{"Verificar preview.", "Verificar publish."},
		Limits:         "Solo preview==publish.",
		RetrievalTerms: []string{"dashboard", "datos"},
	})

	h.curateWithArea(t, lidA, domain.CurationApproveNewSkill, area)

	// Preview.
	prev, err := h.publishSvc.Preview(h.ctx, h.project.ID, &PreviewInput{
		LearningID: lidA,
		Actor:      h.actor,
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	// The first target (child skill) path must equal the expected relative path.
	if len(prev.Targets) == 0 {
		t.Fatalf("P2: preview returned no targets")
	}
	gotTargetPath := prev.Targets[0].Path
	if gotTargetPath != wantPath {
		t.Errorf("P2: targets[0].Path = %q, want %q", gotTargetPath, wantPath)
	}
	// The plan's TargetPath must also match.
	if prev.Preview.Plan.TargetPath != wantPath {
		t.Errorf("P2: plan.TargetPath = %q, want %q", prev.Preview.Plan.TargetPath, wantPath)
	}
	// No path doubling (the bug used to produce autoName/SKILL.md/SKILL.md).
	if strings.Contains(gotTargetPath, "SKILL.md/SKILL.md") {
		t.Errorf("P2: path-doubling bug present in preview: %q", gotTargetPath)
	}

	// Publish using the SAME preview (do not re-preview — that would conflict)
	// and confirm the file lands at the exact path preview reported.
	pub, err := h.publishSvc.Publish(h.ctx, h.project.ID, &PublishInput{
		Apply:       true,
		LearningID:  lidA,
		PreviewHash: prev.Preview.PreviewHash,
		Force:       true,
		Actor:       h.actor,
	})
	if err != nil {
		t.Fatalf("P2: publish: %v", err)
	}
	if pub.Publication.Status != domain.PubStatusCompleted {
		t.Fatalf("P2: publish status = %q, want %q", pub.Publication.Status, domain.PubStatusCompleted)
	}

	fullPath := filepath.Join(h.projectRoot, SkillsDir, wantPath)
	if _, err := os.Stat(fullPath); err != nil {
		t.Fatalf("P2: published file not at preview path %s: %v", fullPath, err)
	}
}

// setCurationPathToArea updates the latest curation's destination path to
// "<area>/SKILL.md" so that filepath.Dir(path) == area, activating the
// multi-target publish path. Mirrors the manual DB update in
// p1_procedure_e2e_test.go. Used only for the no-area fallback test.
func setCurationPathToArea(t *testing.T, h *p2Harness, lid domain.LearningID, areaName string) {
	t.Helper()
	readTx, txErr := h.db.DB.BeginTx(h.ctx, &sql.TxOptions{ReadOnly: true})
	if txErr != nil {
		t.Fatalf("begin tx: %v", txErr)
	}
	curations, err := storage.ListCurationsByLearning(h.ctx, readTx, lid)
	readTx.Rollback()
	if err != nil || len(curations) == 0 {
		t.Fatalf("list curations for %s: %v", lid, err)
	}
	c := curations[0]
	c.Destination.Path = areaName + "/SKILL.md"
	destJSON := marshalAnyForTest(c.Destination)
	if _, err := h.db.DB.ExecContext(h.ctx,
		`UPDATE curations SET destination_json = ? WHERE id = ?`,
		destJSON, string(c.ID)); err != nil {
		t.Fatalf("update curation destination for %s: %v", lid, err)
	}
}

// marshalAnyForTest is a thin wrapper to encode the destination for the
// manual DB update in the fallback test (keeps encoding consistent with the
// storage layer without importing an unexported helper).
func marshalAnyForTest(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
