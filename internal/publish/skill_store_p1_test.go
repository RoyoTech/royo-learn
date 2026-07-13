package publish

import (
	"strings"
	"testing"

	"agent-royo-learn/internal/domain"
)

// P1 — [BLOCKER] Procedure lost on round-trip.
//
// parseSkillSections sets currentField = nil on "### Procedimiento", so the
// "- step" bullets fall to the default branch and (with currentField == nil)
// accumulate into current.Rule. SkillSection.Procedure stays empty.
//
// In buildPublishContents, when a child skill ALREADY EXISTS, the existing
// content is parsed with parseSkillSections and then MergeLearningIntoSections
// is called. So previously-published learnings lose their "### Procedimiento".

// TestP1_ProcedurePreservedOnParse asserts the CORRECT desired behavior:
// after GenerateSkillContent → parseSkillSections, the Procedure field must
// contain the same steps that were written. This MUST FAIL currently because
// the parser drops Procedure into Rule.
func TestP1_ProcedurePreservedOnParse(t *testing.T) {
	section := SkillSection{
		LearningID:   "019f588c-0861-7350-bf36-87d7b74d91d0",
		Title:        "Dashboard cadena continua",
		Rule:         "La cadena Unidad→Test no debe tener huecos.",
		Procedure:    []string{"Extraer unidades y tests.", "Ordenar por fecha.", "Verificar huecos."},
		CanonExample: "Contexto: dashboard_data_cursos.\n\nObservación: hay huecos.",
		Limits:       "Solo para dashboards de profesor.",
	}

	fm := SkillFrontmatter{
		Name:        "padreseducadores-dashboard_data_cursos",
		Description: "Trigger: dashboard_data_cursos. Reglas.",
		Source:      "royo-learn",
		Project:     "padreseducadores.org",
		LearningIDs: []domain.LearningID{section.LearningID},
		UpdatedAt:   "2026-07-12",
	}

	content := GenerateSkillContent(fm, []SkillSection{section})

	// Sanity: the generated content DOES contain the procedure section.
	if !strings.Contains(content, "### Procedimiento") {
		t.Fatalf("generated content must contain ### Procedimiento — generator is fine:\n%s", content)
	}
	for _, step := range section.Procedure {
		if !strings.Contains(content, step) {
			t.Fatalf("generated content must contain step %q:\n%s", step, content)
		}
	}

	// Now parse it back — this is where the bug manifests.
	parsed := parseSkillSections(content)
	if len(parsed) != 1 {
		t.Fatalf("expected 1 section, got %d", len(parsed))
	}

	// DESIRED: Procedure must round-trip with the 3 steps.
	if len(parsed[0].Procedure) != 3 {
		t.Errorf("Procedure lost on round-trip: got %d steps, want 3 (steps ended up in Rule: %q)",
			len(parsed[0].Procedure), parsed[0].Rule)
	}

	// DESIRED: each step must be present in the Procedure slice.
	for i, want := range section.Procedure {
		if i >= len(parsed[0].Procedure) {
			break
		}
		if parsed[0].Procedure[i] != want {
			t.Errorf("Procedure[%d] = %q, want %q", i, parsed[0].Procedure[i], want)
		}
	}

	// DESIRED: Rule must NOT contain the procedure steps (they belong in Procedure).
	for _, step := range section.Procedure {
		if strings.Contains(parsed[0].Rule, step) {
			t.Errorf("Rule should not contain procedure step %q — it leaked into Rule: %q", step, parsed[0].Rule)
		}
	}
}

// TestP1_ProcedureSurvivesRePublish simulates buildPublishContents when a
// SECOND learning is published to the same area as an EXISTING skill.
//
// Flow (mirrors publish_op.go:388-416):
//  1. Learning A (with procedure) is published → GenerateSkillContent.
//  2. Learning B is published to the same area → parseSkillSections(existing),
//     MergeLearningIntoSections(B), GenerateSkillContent again.
//  3. Parse the final content and assert A's procedure is intact.
//
// This MUST FAIL currently because parseSkillSections in step 2 drops A's
// procedure into Rule, and the regenerated content has no "### Procedimiento"
// for A.
func TestP1_ProcedureSurvivesRePublish(t *testing.T) {
	learningA := &domain.Learning{
		ID:                   "019f588c-0861-7350-bf36-87d7b74d91d0",
		Title:                "Dashboard A",
		ReusableLesson:       "Regla A.",
		RecommendedProcedure: []string{"Step A1", "Step A2", "Step A3"},
		Context:              "ctx A",
		Observation:          "obs A",
		Limits:               "limits A",
		RetrievalTerms:       []string{"dashboard_data_cursos"},
	}

	learningB := &domain.Learning{
		ID:                   "019f588c-9999-7350-bf36-87d7b74d91d0",
		Title:                "Dashboard B",
		ReusableLesson:       "Regla B.",
		RecommendedProcedure: []string{"Step B1", "Step B2"},
		Context:              "ctx B",
		Observation:          "obs B",
		Limits:               "limits B",
		RetrievalTerms:       []string{"dashboard_data_cursos"},
	}

	// Step 1: publish learning A → generate initial skill content.
	sectionA := buildSkillSection(learningA)
	fm := SkillFrontmatter{
		Name:        "padreseducadores-dashboard_data_cursos",
		Description: "Trigger: dashboard_data_cursos.",
		Source:      "royo-learn",
		Project:     "padreseducadores.org",
		LearningIDs: []domain.LearningID{learningA.ID},
		UpdatedAt:   "2026-07-12",
	}
	initialContent := GenerateSkillContent(fm, []SkillSection{sectionA})

	// Step 2: simulate re-publish of learning B into the SAME skill file.
	// This mirrors buildPublishContents (publish_op.go:391-416):
	//   read existing → parseSkillSections → MergeLearningIntoSections → GenerateSkillContent.
	var sections []SkillSection
	sections = parseSkillSections(initialContent) // BUG: A's procedure lost here.
	sections = MergeLearningIntoSections(sections, learningB)

	ids := make([]domain.LearningID, 0, len(sections))
	for _, sec := range sections {
		ids = append(ids, sec.LearningID)
	}
	fm2 := SkillFrontmatter{
		Name:        "padreseducadores-dashboard_data_cursos",
		Description: "Trigger: dashboard_data_cursos.",
		Source:      "royo-learn",
		Project:     "padreseducadores.org",
		LearningIDs: ids,
		UpdatedAt:   "2026-07-12",
	}
	rePublished := GenerateSkillContent(fm2, sections)

	// Step 3: parse the re-published content and verify A's procedure survived.
	final := parseSkillSections(rePublished)

	var sectionAFinal *SkillSection
	for i := range final {
		if final[i].LearningID == learningA.ID {
			sectionAFinal = &final[i]
			break
		}
	}
	if sectionAFinal == nil {
		t.Fatalf("learning A section not found in re-published content")
	}

	// DESIRED: A's procedure must still have 3 steps after the round-trip.
	if len(sectionAFinal.Procedure) != 3 {
		t.Errorf("P1: learning A procedure lost after re-publish of B: got %d steps, want 3\n"+
			"sectionAFinal.Rule = %q\nsectionAFinal.Procedure = %v",
			len(sectionAFinal.Procedure), sectionAFinal.Rule, sectionAFinal.Procedure)
	}

	// DESIRED: A's procedure steps must be the original ones.
	for i, want := range learningA.RecommendedProcedure {
		if i >= len(sectionAFinal.Procedure) {
			break
		}
		if sectionAFinal.Procedure[i] != want {
			t.Errorf("P1: Procedure[%d] = %q, want %q", i, sectionAFinal.Procedure[i], want)
		}
	}
}
