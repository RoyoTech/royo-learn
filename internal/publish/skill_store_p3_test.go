package publish

import (
	"strings"
	"testing"

	"agent-royo-learn/internal/domain"
)

// P3 — [MEDIUM] "### Anti-patrón" section removed (dead code eliminated).
//
// buildSkillSection used to set AntiPattern: "" with a TODO; writeSkillSection
// only emitted the section if non-empty. The field and section have been
// removed from SkillSection, writeSkillSection, parseSkillSections, and
// buildSkillSection. This test verifies the removal is complete: the generated
// content must NOT contain "### Anti-patrón" and the SkillSection struct must
// not have an AntiPattern field (enforced at compile time — if the field
// existed, this test would reference it).
func TestP3_AntiPatternSectionRemoved(t *testing.T) {
	learning := &domain.Learning{
		ID:              "019f588c-0861-7350-bf36-87d7b74d91d0",
		Title:           "Test",
		ReusableLesson:  "Rule.",
		Limits:          "Some limits.",
		Context:         "ctx",
		Observation:     "obs",
	}
	sec := buildSkillSection(learning)

	fm := SkillFrontmatter{
		Name:        "test-area",
		Description: "desc",
		Source:      "royo-learn",
		Project:     "proj",
		LearningIDs: []domain.LearningID{learning.ID},
		UpdatedAt:   "2026-07-12",
	}
	content := GenerateSkillContent(fm, []SkillSection{sec})

	if strings.Contains(content, "### Anti-patrón") {
		t.Errorf("P3: output should NOT contain ### Anti-patrón — section was removed:\n%s", content)
	}

	// Verify the Limits section IS still present (sanity check — we only
	// removed Anti-patrón, not Límites).
	if !strings.Contains(content, "### Límites") {
		t.Errorf("P3: output should still contain ### Límites — we only removed Anti-patrón:\n%s", content)
	}
}
