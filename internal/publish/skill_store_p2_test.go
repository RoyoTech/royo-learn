package publish

import (
	"testing"

	"agent-royo-learn/internal/domain"
)

// P2 — [BLOCKER] Area from RetrievalTerms[0] is order-dependent.
//
// SkillArea returns learning.RetrievalTerms[0] sanitized. Two learnings of the
// same domain with the same terms in DIFFERENT order resolve to different
// areas if the first term differs. This breaks the "one skill per area"
// invariant: the same domain knowledge would be split across multiple skill
// files depending on term ordering.

// TestP2_SkillAreaOrderIndependent asserts the CORRECT desired behavior:
// two learnings with the SAME RetrievalTerms in different order must resolve
// to the SAME area. This MUST FAIL currently because SkillArea uses
// RetrievalTerms[0] only.
func TestP2_SkillAreaOrderIndependent(t *testing.T) {
	termsA := []string{"dashboard", "datos"}
	termsB := []string{"datos", "dashboard"}

	l1 := &domain.Learning{
		ID:             "019f588c-0861-7350-bf36-87d7b74d91d0",
		RetrievalTerms: termsA,
	}
	l2 := &domain.Learning{
		ID:             "019f588c-9999-7350-bf36-87d7b74d91d0",
		RetrievalTerms: termsB,
	}

	area1 := SkillArea(l1)
	area2 := SkillArea(l2)

	if area1 != area2 {
		t.Errorf("P2: SkillArea is order-dependent: l1=%q l2=%q (same terms, different order should resolve to same area)",
			area1, area2)
	}
}

// TestP2_SameAreaResolvesSameSkillPath asserts that two learnings with the
// same terms in different order publish to the SAME skill file path. This
// MUST FAIL currently.
func TestP2_SameAreaResolvesSameSkillPath(t *testing.T) {
	termsA := []string{"dashboard", "datos"}
	termsB := []string{"datos", "dashboard"}

	l1 := &domain.Learning{
		ID:             "019f588c-0861-7350-bf36-87d7b74d91d0",
		RetrievalTerms: termsA,
	}
	l2 := &domain.Learning{
		ID:             "019f588c-9999-7350-bf36-87d7b74d91d0",
		RetrievalTerms: termsB,
	}

	projectKey := "padreseducadores.org"
	path1 := SkillPath(SkillName(projectKey, SkillArea(l1)))
	path2 := SkillPath(SkillName(projectKey, SkillArea(l2)))

	if path1 != path2 {
		t.Errorf("P2: same-area learnings resolve to different skill paths: %q vs %q — "+
			"term order should not change the destination skill file", path1, path2)
	}
}
