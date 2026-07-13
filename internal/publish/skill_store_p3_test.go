package publish

import (
	"strings"
	"testing"

	"agent-royo-learn/internal/domain"
)

// P3 — [MEDIUM] "### Anti-patrón" never populated (dead code).
//
// buildSkillSection sets AntiPattern: "" with a TODO; writeSkillSection only
// emits the section if non-empty. So it is dead code.
//
// This test documents the dead-code state: generates skill content from a
// SkillSection and asserts the output does NOT contain the literal
// "### Anti-patrón" (desired behavior after fix = section removed, OR section
// populated from limits/curation).
//
// This test PASSES vacuously: it confirms the section is never emitted today,
// which is the dead-code state to be removed/fixed.
func TestP3_AntiPatternDeadCodeNotEmitted(t *testing.T) {
	// buildSkillSection always sets AntiPattern: "" (skill_store.go:203).
	learning := &domain.Learning{
		ID:              "019f588c-0861-7350-bf36-87d7b74d91d0",
		Title:           "Test",
		ReusableLesson:  "Rule.",
		Limits:          "Some limits.",
		Context:         "ctx",
		Observation:     "obs",
	}
	sec := buildSkillSection(learning)

	if sec.AntiPattern != "" {
		t.Errorf("P3: buildSkillSection should set AntiPattern to \"\" (dead code), got %q", sec.AntiPattern)
	}

	fm := SkillFrontmatter{
		Name:        "test-area",
		Description: "desc",
		Source:      "royo-learn",
		Project:     "proj",
		LearningIDs: []domain.LearningID{learning.ID},
		UpdatedAt:   "2026-07-12",
	}
	content := GenerateSkillContent(fm, []SkillSection{sec})

	// Desired (after fix): section is removed OR populated. Today it is absent.
	if strings.Contains(content, "### Anti-patrón") {
		t.Errorf("P3: output should NOT contain ### Anti-patrón (dead code) — got:\n%s", content)
	}

	// Honest report: this test passes vacuously — it confirms the section is
	// never emitted, which documents the dead-code state.
	t.Log("P3 test passes vacuously; confirms section is never emitted — dead code to be removed.")
}
