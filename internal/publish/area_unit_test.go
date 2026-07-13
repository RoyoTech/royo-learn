package publish

import (
	"strings"
	"testing"

	"agent-royo-learn/internal/domain"
)

// Unit tests for ResolveSkillArea (publish) and domain.ValidateExplicitArea.
// These isolate the area resolution logic from the e2e harness.

func TestResolveSkillArea_ExplicitWins(t *testing.T) {
	learning := &domain.Learning{RetrievalTerms: []string{"dashboard", "datos"}}
	got, err := ResolveSkillArea(learning, "area-explicita")
	if err != nil {
		t.Fatalf("ResolveSkillArea: unexpected error: %v", err)
	}
	if got != "area-explicita" {
		t.Errorf("ResolveSkillArea = %q, want %q (explicit must win)", got, "area-explicita")
	}
	// The explicit area must override even when derivation would differ.
	if got == SkillArea(learning) {
		t.Errorf("ResolveSkillArea returned the derived area; explicit must win")
	}
}

func TestResolveSkillArea_FallbackWhenEmpty(t *testing.T) {
	learning := &domain.Learning{RetrievalTerms: []string{"dashboard", "datos"}}
	got, err := ResolveSkillArea(learning, "")
	if err != nil {
		t.Fatalf("ResolveSkillArea: unexpected error: %v", err)
	}
	want := SkillArea(learning)
	if got != want {
		t.Errorf("ResolveSkillArea(empty) = %q, want derived %q", got, want)
	}
}

func TestResolveSkillArea_InvalidErrors(t *testing.T) {
	learning := &domain.Learning{RetrievalTerms: []string{"dashboard"}}
	// "!!!@#$" sanitizes to empty (no alphanumerics, dashes, or underscores) → error.
	if _, err := ResolveSkillArea(learning, "!!!@#$"); err == nil {
		t.Errorf("ResolveSkillArea(%q) expected error, got nil", "!!!@#$")
	}
	// Empty input falls back (no error).
	if _, err := ResolveSkillArea(learning, ""); err != nil {
		t.Errorf("ResolveSkillArea(empty) unexpected error: %v", err)
	}
}

func TestValidateExplicitArea_LengthLimit(t *testing.T) {
	long := strings.Repeat("a", domain.MaxSkillAreaLength+1)
	if _, err := domain.ValidateExplicitArea(long); err == nil {
		t.Errorf("ValidateExplicitArea(len=%d) expected error, got nil", len(long))
	}
	atLimit := strings.Repeat("a", domain.MaxSkillAreaLength)
	if got, err := domain.ValidateExplicitArea(atLimit); err != nil {
		t.Errorf("ValidateExplicitArea(len=%d) unexpected error: %v", len(atLimit), err)
	} else if got != atLimit {
		t.Errorf("ValidateExplicitArea(len=%d) = %q, want %q", len(atLimit), got, atLimit)
	}
}

func TestValidateExplicitArea_EmptyOK(t *testing.T) {
	got, err := domain.ValidateExplicitArea("")
	if err != nil {
		t.Fatalf("ValidateExplicitArea(empty) unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("ValidateExplicitArea(empty) = %q, want %q", got, "")
	}
}

func TestValidateExplicitArea_SanitizesAndLowers(t *testing.T) {
	got, err := domain.ValidateExplicitArea("Dashboard Datos")
	if err != nil {
		t.Fatalf("ValidateExplicitArea: unexpected error: %v", err)
	}
	if got != "dashboard-datos" {
		t.Errorf("ValidateExplicitArea(%q) = %q, want %q", "Dashboard Datos", got, "dashboard-datos")
	}
}

func TestValidateExplicitArea_SanitizesToEmptyErrors(t *testing.T) {
	if _, err := domain.ValidateExplicitArea("!!!@#$"); err == nil {
		t.Errorf("ValidateExplicitArea(%q) expected error (sanitizes to empty), got nil", "!!!@#$")
	}
}

func TestSanitizeSkillArea_DelegatesConsistently(t *testing.T) {
	// The publish-local sanitizeAreaName must produce the same output as the
	// canonical domain helper (it delegates, so they must agree).
	cases := []struct{ in, want string }{
		{"Dashboard Datos", "Dashboard-Datos"},
		{"foo_bar-baz", "foo_bar-baz"},
		{"a!!b", "ab"},
		{"", ""},
	}
	for _, c := range cases {
		if got := sanitizeAreaName(c.in); got != c.want {
			t.Errorf("sanitizeAreaName(%q) = %q, want %q", c.in, got, c.want)
		}
		if got := domain.SanitizeSkillArea(c.in); got != c.want {
			t.Errorf("domain.SanitizeSkillArea(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
