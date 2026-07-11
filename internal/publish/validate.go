package publish

import (
	"fmt"
	"strings"
)

// SkillValidationResult records the result of validating a skill file.
type SkillValidationResult struct {
	Valid          bool
	HasFrontmatter bool
	Issues         []string
}

// ValidateSkill checks that a skill file is syntactically valid:
// - Contains YAML front matter between --- delimiters
// - Has a 'name' field in the front matter
// - Has a 'description' field in the front matter
func ValidateSkill(content []byte) SkillValidationResult {
	result := SkillValidationResult{}
	text := string(content)

	// Check for front matter delimiters.
	if !strings.HasPrefix(strings.TrimSpace(text), "---") {
		result.Issues = append(result.Issues, "missing YAML front matter (must start with ---)")
		return result
	}

	result.HasFrontmatter = true

	// Find the closing --- delimiter.
	rest := text[3:] // skip opening ---
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		result.Issues = append(result.Issues, "unclosed YAML front matter (missing closing ---)")
		return result
	}

	frontmatter := rest[:endIdx]

	// Check required fields.
	if !strings.Contains(frontmatter, "name:") {
		result.Issues = append(result.Issues, "missing required field 'name' in front matter")
	}
	if !strings.Contains(frontmatter, "description:") {
		result.Issues = append(result.Issues, "missing required field 'description' in front matter")
	}

	result.Valid = len(result.Issues) == 0
	return result
}

// BuildSkillContent constructs a skill file from learning data.
func BuildSkillContent(title, description, lesson, procedure string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: %s\n", strings.ToLower(strings.ReplaceAll(title, " ", "-"))))
	b.WriteString(fmt.Sprintf("description: %s\n", description))
	b.WriteString("license: MIT\n")
	b.WriteString("---\n\n")
	b.WriteString(fmt.Sprintf("# %s\n\n", title))
	b.WriteString(lesson)
	if procedure != "" {
		b.WriteString(fmt.Sprintf("\n\n## Procedure\n\n%s\n", procedure))
	}
	return b.String()
}
