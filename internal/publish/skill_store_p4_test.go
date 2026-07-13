package publish

import (
	"strings"
	"testing"

	"agent-royo-learn/internal/domain"
	"gopkg.in/yaml.v3"
)

// P4 — [MEDIUM] Hand-rolled YAML breaks on special chars.
//
// GenerateSkillContent builds frontmatter with fmt.Sprintf + escapeYAML;
// extractDescription hand-parses. SkillFrontmatter has unused yaml:"..." tags.
// go.mod already requires gopkg.in/yaml.v3 v3.0.1.
//
// This test asserts the CORRECT desired behavior: the frontmatter block
// produced by GenerateSkillContent must be valid YAML that round-trips
// EXACTLY through yaml.Unmarshal.
//
// The test exercises a Description with special chars: colon, double quote,
// hash, newline, and accents.

// extractFrontmatterBlock returns the YAML block between the first two "---"
// lines of the generated content.
func extractFrontmatterBlock(content string) string {
	lines := strings.Split(content, "\n")
	var block []string
	inFM := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if !inFM {
				inFM = true
				continue
			}
			break
		}
		if inFM {
			block = append(block, line)
		}
	}
	return "---\n" + strings.Join(block, "\n") + "\n---\n"
}

// TestP4_YAMLFrontmatterRoundTrips asserts that yaml.Unmarshal of the
// generated frontmatter recovers the EXACT original Description.
func TestP4_YAMLFrontmatterRoundTrips(t *testing.T) {
	// Description with: colon, double quote, hash, newline (actual), accents.
	originalDesc := "Descripción: regla \"con #comentarios\"\ny acentos áéíóú"

	fm := SkillFrontmatter{
		Name:        "padreseducadores-dashboard_data_cursos",
		Description: originalDesc,
		Source:      "royo-learn",
		Project:     "padreseducadores.org",
		LearningIDs: []domain.LearningID{"019f588c-0861-7350-bf36-87d7b74d91d0"},
		UpdatedAt:   "2026-07-12",
	}

	content := GenerateSkillContent(fm, nil)
	fmBlock := extractFrontmatterBlock(content)

	var parsed SkillFrontmatter
	if err := yaml.Unmarshal([]byte(fmBlock), &parsed); err != nil {
		t.Fatalf("P4: generated frontmatter is invalid YAML: %v\nblock:\n%s", err, fmBlock)
	}

	if parsed.Description != originalDesc {
		t.Errorf("P4: description did not round-trip exactly:\n  got:  %q\n  want: %q",
			parsed.Description, originalDesc)
	}
}

// TestP4_ExtractDescriptionRoundTrips asserts that the hand-rolled
// extractDescription recovers the EXACT original description. This exercises
// the code path used by DiscoverChildSkills. This MUST FAIL because
// extractDescription does not unescape \" or \n.
func TestP4_ExtractDescriptionRoundTrips(t *testing.T) {
	originalDesc := "Descripción: regla \"con #comentarios\"\ny acentos áéíóú"

	fm := SkillFrontmatter{
		Name:        "padreseducadores-dashboard_data_cursos",
		Description: originalDesc,
		Source:      "royo-learn",
		Project:     "padreseducadores.org",
		LearningIDs: []domain.LearningID{"019f588c-0861-7350-bf36-87d7b74d91d0"},
		UpdatedAt:   "2026-07-12",
	}

	content := GenerateSkillContent(fm, nil)
	recovered := extractDescription(content)

	if recovered != originalDesc {
		t.Errorf("P4: extractDescription did not round-trip:\n  got:  %q\n  want: %q",
			recovered, originalDesc)
	}
}
