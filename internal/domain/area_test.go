package domain

import "testing"

// Tests for DeriveSkillArea, the deterministic skill-area derivation moved
// from internal/publish/skill_store.go::SkillArea so curate and publish share
// one implementation. The publish tests (TestSkillArea_*) remain the
// source-of-truth for derivation behavior; these mirror the same cases
// directly against the domain function.

func TestDeriveSkillArea_Nil(t *testing.T) {
	t.Parallel()
	if got := DeriveSkillArea(nil); got != "general" {
		t.Errorf("DeriveSkillArea(nil) = %q, want %q", got, "general")
	}
}

func TestDeriveSkillArea_NoTerms(t *testing.T) {
	t.Parallel()
	learning := &Learning{}
	if got := DeriveSkillArea(learning); got != "general" {
		t.Errorf("DeriveSkillArea(no terms) = %q, want %q", got, "general")
	}
}

func TestDeriveSkillArea_EmptyTerms(t *testing.T) {
	t.Parallel()
	learning := &Learning{RetrievalTerms: []string{}}
	if got := DeriveSkillArea(learning); got != "general" {
		t.Errorf("DeriveSkillArea(empty terms) = %q, want %q", got, "general")
	}
}

func TestDeriveSkillArea_SortedFirstWins(t *testing.T) {
	t.Parallel()
	learning := &Learning{RetrievalTerms: []string{"go", "testing"}}
	// Case-insensitive sort: "go" < "testing" → "go".
	if got := DeriveSkillArea(learning); got != "go" {
		t.Errorf("DeriveSkillArea([go, testing]) = %q, want %q", got, "go")
	}
}

func TestDeriveSkillArea_CaseInsensitiveSort(t *testing.T) {
	t.Parallel()
	learning := &Learning{RetrievalTerms: []string{"Testing", "go"}}
	// Lowercased comparison: "go" < "testing" → "go", regardless of original case.
	if got := DeriveSkillArea(learning); got != "go" {
		t.Errorf("DeriveSkillArea([Testing, go]) = %q, want %q", got, "go")
	}
}

func TestDeriveSkillArea_SpaceToDashLowercased(t *testing.T) {
	t.Parallel()
	learning := &Learning{RetrievalTerms: []string{"Go Testing"}}
	// Single term: space → dash, lowercased → "go-testing".
	if got := DeriveSkillArea(learning); got != "go-testing" {
		t.Errorf("DeriveSkillArea([Go Testing]) = %q, want %q", got, "go-testing")
	}
}

func TestDeriveSkillArea_SpecialCharsKeptLetters(t *testing.T) {
	t.Parallel()
	// "!!!special" sorts before "go" ('!' < 'g'), and sanitizes to "special"
	// (punctuation dropped, letters kept) — NOT empty, so the result is
	// "special", not "general". This matches the existing SkillArea behavior.
	learning := &Learning{RetrievalTerms: []string{"!!!special", "go"}}
	if got := DeriveSkillArea(learning); got != "special" {
		t.Errorf("DeriveSkillArea([!!!special, go]) = %q, want %q", got, "special")
	}
}

func TestDeriveSkillArea_SanitizesToEmptyFallback(t *testing.T) {
	t.Parallel()
	// "!!!@#$" sorts before "go" but sanitizes to empty (no alphanumerics,
	// dashes, or underscores) → falls back to "general".
	learning := &Learning{RetrievalTerms: []string{"!!!@#$", "go"}}
	if got := DeriveSkillArea(learning); got != "general" {
		t.Errorf("DeriveSkillArea([!!!@#$, go]) = %q, want %q", got, "general")
	}
}

func TestDeriveSkillArea_DoesNotMutateInput(t *testing.T) {
	t.Parallel()
	terms := []string{"testing", "go"}
	learning := &Learning{RetrievalTerms: terms}
	_ = DeriveSkillArea(learning)
	// The caller's slice must remain in its original order (copy + sort on a
	// local slice, not in-place).
	if len(learning.RetrievalTerms) != 2 ||
		learning.RetrievalTerms[0] != "testing" ||
		learning.RetrievalTerms[1] != "go" {
		t.Errorf("DeriveSkillArea mutated input terms: got %v, want [testing go]", learning.RetrievalTerms)
	}
}

func TestDeriveSkillArea_MultiwordSort(t *testing.T) {
	t.Parallel()
	// Mirrors the existing publish TestSkillArea_FromRetrievalTerms case:
	// "cadena continua Unidad Test" sorts first (case-insensitive) and
	// sanitizes to "cadena-continua-unidad-test".
	learning := &Learning{
		RetrievalTerms: []string{"dashboard_data_cursos", "distribución fechas", "cadena continua Unidad Test"},
	}
	want := "cadena-continua-unidad-test"
	if got := DeriveSkillArea(learning); got != want {
		t.Errorf("DeriveSkillArea(multiword) = %q, want %q", got, want)
	}
}
