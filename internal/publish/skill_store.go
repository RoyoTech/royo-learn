package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agent-royo-learn/internal/domain"

	"gopkg.in/yaml.v3"
)

// --- Constants ---

const (
	// SkillsDir is the default directory where generated skills are written.
	SkillsDir = "skills"
	// SkillIndexSuffix is the suffix appended to the project key for the index skill.
	SkillIndexSuffix = "-conocimiento"
	// AgentsRefLine is the stable line inserted once into AGENTS.md/CLAUDE.md.
	AgentsRefLine = "<!-- royo-learn:managed start -->\n<!-- royo-learn:managed end -->\n"
	// agentsLineTemplate is the stable reference line injected into AGENTS.md.
	agentsLineTemplate = "> 📚 **Conocimiento del proyecto**: cargá la skill `%s-conocimiento` para ver el catálogo de reglas y procedimientos capturados."
	// managedRefBlockTemplate is the block injected via managed blocks.
	managedRefBlock = "<!-- royo-learn:managed start -->\n" +
		"> 📚 **Conocimiento del proyecto**: cargá la skill `%s-conocimiento` para ver el catálogo de reglas y procedimientos capturados.\n" +
		"<!-- royo-learn:managed end -->"
)

// --- Skill generation ---

// SkillSection represents a single learning's section within a skill file.
type SkillSection struct {
	LearningID   domain.LearningID
	Title        string
	Rule         string   // reusable_lesson
	Procedure    []string // recommended_procedure
	CanonExample string   // derived from context + observation
	Limits       string
}

// SkillFrontmatter holds the YAML frontmatter for a skill.
type SkillFrontmatter struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Source      string              `yaml:"source"`
	Project     string              `yaml:"project"`
	LearningIDs []domain.LearningID `yaml:"learning_ids"`
	UpdatedAt   string              `yaml:"updated_at"`
}

// SkillArea derives the skill area name from a learning's retrieval terms.
// Uses the first retrieval term as the primary area indicator, sanitized for filenames.
func SkillArea(learning *domain.Learning) string {
	if learning == nil || len(learning.RetrievalTerms) == 0 {
		return "general"
	}

	// Use the first retrieval term as the primary area.
	best := learning.RetrievalTerms[0]

	// Sanitize: keep alphanumeric, dash, underscore; replace spaces with dash.
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		case r == ' ':
			return '-'
		default:
			return -1
		}
	}, best)

	if sanitized == "" {
		return "general"
	}

	return strings.ToLower(sanitized)
}

// SkillName builds a skill name from project key and area.
func SkillName(projectKey, area string) string {
	return fmt.Sprintf("%s-%s", projectKey, area)
}

// IndexSkillName builds the index skill name from the project key.
func IndexSkillName(projectKey string) string {
	return fmt.Sprintf("%s%s", projectKey, SkillIndexSuffix)
}

// SkillPath returns the relative path for a skill file from project root.
func SkillPath(skillName string) string {
	return filepath.Join(SkillsDir, skillName, "SKILL.md")
}

// ExtractSection extracts the section for a specific learning from a skill file.
// Returns nil if the learning is not found.
func ExtractSection(content string, learningID domain.LearningID) *SkillSection {
	idStr := string(learningID)
	sections := parseSkillSections(content)
	for _, sec := range sections {
		if string(sec.LearningID) == idStr {
			return &sec
		}
	}
	return nil
}

// ListSkillLearningIDs returns all learning IDs referenced in a skill file.
func ListSkillLearningIDs(content string) []domain.LearningID {
	sections := parseSkillSections(content)
	ids := make([]domain.LearningID, 0, len(sections))
	for _, sec := range sections {
		ids = append(ids, sec.LearningID)
	}
	return ids
}

// --- Content generation ---

// GenerateSkillContent builds the full content of a skill file from frontmatter
// and a list of learning sections.
func GenerateSkillContent(fm SkillFrontmatter, sections []SkillSection) string {
	var b strings.Builder

	// YAML frontmatter — serialized with yaml.v3 for correctness on special chars.
	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		// Should never happen with this struct; fall back to a minimal safe block.
		yamlBytes = []byte(fmt.Sprintf("name: %s\ndescription: %q\nsource: royo-learn\nproject: %s\nlearning_ids: []\nupdated_at: %s\n",
			fm.Name, fm.Description, fm.Project, fm.UpdatedAt))
	}
	b.WriteString("---\n")
	b.Write(yamlBytes)
	b.WriteString("---\n\n")

	// Body: each section
	for i, sec := range sections {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		writeSkillSection(&b, sec)
	}

	return b.String()
}

// writeSkillSection writes one learning's section into the skill body.
func writeSkillSection(b *strings.Builder, sec SkillSection) {
	b.WriteString(fmt.Sprintf("## %s\n", sec.Title))
	b.WriteString(fmt.Sprintf("<!-- royo-learn:learning-id %s -->\n\n", sec.LearningID))

	// Rule
	b.WriteString("### Regla\n\n")
	b.WriteString(sec.Rule)
	b.WriteString("\n\n")

	// Procedure
	if len(sec.Procedure) > 0 {
		b.WriteString("### Procedimiento\n\n")
		for _, step := range sec.Procedure {
			b.WriteString(fmt.Sprintf("- %s\n", step))
		}
		b.WriteString("\n")
	}

	// Canonical example
	if sec.CanonExample != "" {
		b.WriteString("### Ejemplo canónico\n\n")
		b.WriteString(sec.CanonExample)
		b.WriteString("\n\n")
	}

	// Limits
	if sec.Limits != "" {
		b.WriteString("### Límites\n\n")
		b.WriteString(sec.Limits)
		b.WriteString("\n\n")
	}
}

// buildSkillSection creates a SkillSection from a domain.Learning.
func buildSkillSection(learning *domain.Learning) SkillSection {
	return SkillSection{
		LearningID:   learning.ID,
		Title:        learning.Title,
		Rule:         learning.ReusableLesson,
		Procedure:    learning.RecommendedProcedure,
		CanonExample: fmt.Sprintf("Contexto: %s\n\nObservación: %s", learning.Context, learning.Observation),
		Limits:       learning.Limits,
	}
}

// MergeLearningIntoSections adds or updates a learning section in a slice of sections.
// Returns the merged sections (new slice).
func MergeLearningIntoSections(existing []SkillSection, learning *domain.Learning) []SkillSection {
	newSec := buildSkillSection(learning)
	newID := string(learning.ID)

	found := false
	for i, sec := range existing {
		if string(sec.LearningID) == newID {
			existing[i] = newSec
			found = true
			break
		}
	}
	if !found {
		existing = append(existing, newSec)
	}

	return existing
}

// BuildDescription builds a skill description from learning triggers.
func BuildDescription(projectKey, area string, learnings []*domain.Learning) string {
	var triggers []string
	seen := make(map[string]bool)
	for _, l := range learnings {
		for _, t := range l.RetrievalTerms {
			if !seen[t] {
				seen[t] = true
				triggers = append(triggers, t)
			}
		}
	}

	sort.Strings(triggers)
	triggerList := strings.Join(triggers, ", ")

	return fmt.Sprintf("Trigger: %s. Reglas y procedimientos capturados de %s (área %s).",
		triggerList, projectKey, area)
}

// --- Index skill generation ---

// IndexEntry represents one entry in the index skill catalog.
type IndexEntry struct {
	SkillName   string
	Description string
}

// GenerateIndexContent builds the index skill (skill madre) content.
func GenerateIndexContent(projectKey string, entries []IndexEntry) string {
	var b strings.Builder

	fm := SkillFrontmatter{
		Name:        IndexSkillName(projectKey),
		Description: fmt.Sprintf("Catálogo de conocimiento capturado para %s. Cargá esta skill para descubrir qué áreas de conocimiento existen.", projectKey),
		Source:      "royo-learn",
		Project:     projectKey,
		LearningIDs: nil,
		UpdatedAt:   time.Now().UTC().Format("2006-01-02"),
	}
	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		yamlBytes = []byte(fmt.Sprintf("name: %s\ndescription: %q\n", fm.Name, fm.Description))
	}
	b.WriteString("---\n")
	b.Write(yamlBytes)
	b.WriteString("---\n\n")
	b.WriteString(fmt.Sprintf("# %s — Catálogo de conocimiento\n\n", projectKey))
	b.WriteString("Skills temáticas generadas automáticamente por royo-learn. Cada una contiene reglas, procedimientos, ejemplos y anti-patrones capturados del desarrollo.\n\n")

	if len(entries) == 0 {
		b.WriteString("_(sin skills hijas todavía — publicá tu primer learning con destino `skill`)_\n")
		return b.String()
	}

	b.WriteString("| Skill | Cuándo usarla |\n")
	b.WriteString("|-------|---------------|\n")
	for _, e := range entries {
		b.WriteString(fmt.Sprintf("| `%s` | %s |\n", e.SkillName, e.Description))
	}

	return b.String()
}

// --- AGENTS.md hook ---

// HasAgentsRef checks if AGENTS.md already contains the royo-learn index reference.
func HasAgentsRef(content string, projectKey string) bool {
	refLine := fmt.Sprintf(agentsLineTemplate, projectKey)
	return strings.Contains(content, refLine)
}

// BuildAgentsRefLine returns the reference line for AGENTS.md insertion.
func BuildAgentsRefLine(projectKey string) string {
	return fmt.Sprintf(agentsLineTemplate, projectKey)
}

// BuildAgentsRefManagedBlock returns the managed block for AGENTS.md.
func BuildAgentsRefManagedBlock(projectKey string) string {
	return fmt.Sprintf(managedRefBlock, projectKey)
}

// InsertAgentsRef inserts the royo-learn reference into AGENTS.md content.
// If a managed block already exists, it inserts the ref line inside it.
// If no managed block exists, it creates one with the ref line.
func InsertAgentsRef(content string, projectKey string) (string, bool) {
	// If the reference is already there, don't modify.
	if HasAgentsRef(content, projectKey) {
		return content, false
	}

	blockContent := BuildAgentsRefManagedBlock(projectKey)

	// Try inserting via managed block system.
	return InsertManagedBlock(content, blockContent), true
}

// --- Target resolution for skills ---

// SkillPublishTargets holds all targets for a skill publication.
type SkillPublishTargets struct {
	// ChildSkill is the main skill file target.
	ChildSkill TargetResolution
	// IndexSkill is the index catalog target.
	IndexSkill TargetResolution
	// AgentsRef is the AGENTS.md one-time hook target (nil if already hooked or not applicable).
	AgentsRef *TargetResolution
}

// ResolveSkillPublishTargets resolves all targets for a skill publication.
// This extends the single-target resolveSkillTarget to handle the full set:
// child skill, index skill, and optional AGENTS.md hook.
func ResolveSkillPublishTargets(projectRoot string, dest *domain.Destination, projectKey string, needAgentsHook bool) (*SkillPublishTargets, error) {
	// Child skill target.
	childPath := SkillPath(dest.Path) // dest.Path holds the skill name like "padreseducadores-dashboard-datos"
	childFull := filepath.Join(projectRoot, childPath)
	_, childErr := os.Stat(childFull)
	childExists := childErr == nil
	childOp := domain.OpCreate
	if childExists {
		childOp = domain.OpReplace
	}

	child := TargetResolution{
		Root:      SkillsDir,
		Path:      filepath.Join(dest.Path, "SKILL.md"),
		Operation: childOp,
		Exists:    childExists,
		IsManaged: false,
	}

	// Index skill target.
	indexName := IndexSkillName(projectKey)
	indexPath := SkillPath(indexName)
	indexFull := filepath.Join(projectRoot, indexPath)
	_, indexErr := os.Stat(indexFull)
	indexExists := indexErr == nil
	indexOp := domain.OpCreate
	if indexExists {
		indexOp = domain.OpReplace
	}

	index := TargetResolution{
		Root:      SkillsDir,
		Path:      filepath.Join(indexName, "SKILL.md"),
		Operation: indexOp,
		Exists:    indexExists,
		IsManaged: false,
	}

	result := &SkillPublishTargets{
		ChildSkill: child,
		IndexSkill: index,
	}

	// AGENTS.md hook (one-time).
	if needAgentsHook {
		agentsPath := "AGENTS.md"
		agentsFull := filepath.Join(projectRoot, agentsPath)
		_, agentsErr := os.Stat(agentsFull)
		agentsExists := agentsErr == nil
		agentsOp := domain.OpCreate
		if agentsExists {
			agentsOp = domain.OpReplaceManagedBlock
		}

		result.AgentsRef = &TargetResolution{
			Root:      ".",
			Path:      agentsPath,
			Operation: agentsOp,
			Exists:    agentsExists,
			IsManaged: true,
		}
	}

	return result, nil
}

// Flatten converts SkillPublishTargets to a slice of TargetResolution for processing.
func (s *SkillPublishTargets) Flatten() []TargetResolution {
	targets := []TargetResolution{s.ChildSkill, s.IndexSkill}
	if s.AgentsRef != nil {
		targets = append(targets, *s.AgentsRef)
	}
	return targets
}

// --- Parsing helpers ---

// parseSkillSections parses learning sections from a skill file content.
// Sections start with a "## Title" header and contain a "<!-- royo-learn:learning-id -->"
// marker, followed by "### Regla", "### Procedimiento", etc.
func parseSkillSections(content string) []SkillSection {
	var sections []SkillSection
	lines := strings.Split(content, "\n")

	var current *SkillSection
	var currentField *string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect section header (## Title).
		if strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ") {
			// Save previous section.
			if current != nil && current.Title != "" {
				sections = append(sections, *current)
			}
			current = &SkillSection{
				Title: strings.TrimPrefix(trimmed, "## "),
			}
			currentField = nil
			continue
		}

		// Detect learning ID comment.
		if strings.HasPrefix(trimmed, "<!-- royo-learn:learning-id ") && strings.HasSuffix(trimmed, " -->") {
			if current != nil {
				idStr := strings.TrimPrefix(trimmed, "<!-- royo-learn:learning-id ")
				idStr = strings.TrimSuffix(idStr, " -->")
				current.LearningID = domain.LearningID(strings.TrimSpace(idStr))
			}
			continue
		}

		if current == nil {
			continue
		}

		// Detect sub-section headers.
		switch {
		case strings.HasPrefix(trimmed, "### Regla"):
			currentField = &current.Rule
			current.Rule = ""
		case strings.HasPrefix(trimmed, "### Procedimiento"):
			currentField = nil
		case strings.HasPrefix(trimmed, "### Ejemplo canónico"):
			currentField = &current.CanonExample
			current.CanonExample = ""
		case strings.HasPrefix(trimmed, "### Límites"):
			currentField = &current.Limits
			current.Limits = ""
		default:
			if currentField != nil {
				if *currentField != "" {
					*currentField += "\n"
				}
				*currentField += line
			} else if trimmed != "" && !strings.HasPrefix(trimmed, "<!--") && !strings.HasPrefix(trimmed, "---") {
				// Accumulate into rule before first sub-header.
				if current.Rule != "" {
					current.Rule += "\n"
				}
				current.Rule += line
			}
		}
	}

	if current != nil && current.Title != "" {
		sections = append(sections, *current)
	}

	return sections
}

// --- Discovery helpers ---

// DiscoverChildSkills scans the skills directory for child skills of a project.
func DiscoverChildSkills(projectRoot, projectKey string) ([]IndexEntry, error) {
	skillsDir := filepath.Join(projectRoot, SkillsDir)
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("DiscoverChildSkills: read dir: %w", err)
	}

	prefix := projectKey + "-"
	var result []IndexEntry

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip the index skill itself.
		if name == IndexSkillName(projectKey) {
			continue
		}
		// Only match skills belonging to this project.
		if !strings.HasPrefix(name, prefix) {
			continue
		}

		skillFile := filepath.Join(skillsDir, name, "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}

		description := extractDescription(string(data))
		trigger := extractTrigger(name, description)
		result = append(result, IndexEntry{
			SkillName:   name,
			Description: trigger,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].SkillName < result[j].SkillName
	})

	return result, nil
}

// ParseFrontmatter extracts and unmarshals the YAML frontmatter block from skill content.
func ParseFrontmatter(content string) (SkillFrontmatter, error) {
	var fm SkillFrontmatter
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return fm, fmt.Errorf("ParseFrontmatter: no opening --- found")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return fm, fmt.Errorf("ParseFrontmatter: no closing --- found")
	}
	block := strings.Join(lines[1:end], "\n")
	err := yaml.Unmarshal([]byte(block), &fm)
	return fm, err
}

// extractDescription extracts the description from YAML frontmatter.
func extractDescription(content string) string {
	fm, err := ParseFrontmatter(content)
	if err != nil {
		return ""
	}
	return fm.Description
}

// extractTrigger derives a short "when to use" summary from the skill name and description.
func extractTrigger(name, description string) string {
	// Use a portion of the description as the trigger.
	if len(description) > 120 {
		// Try to find the trigger list within the description.
		if idx := strings.Index(description, "Trigger:"); idx >= 0 {
			trigger := description[idx:]
			if len(trigger) > 120 {
				trigger = trigger[:120] + "…"
			}
			return trigger
		}
		return description[:120] + "…"
	}
	return description
}
