package publish

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-royo-learn/internal/domain"
)

// --- SkillArea tests ---

func TestSkillArea_FromRetrievalTerms(t *testing.T) {
	learning := &domain.Learning{
		RetrievalTerms: []string{"dashboard_data_cursos", "distribución fechas", "cadena continua Unidad Test"},
	}
	area := SkillArea(learning)
	// Should pick the longest/most specific term.
	if area != "dashboard_data_cursos" {
		t.Errorf("SkillArea = %q, want %q", area, "dashboard_data_cursos")
	}
}

func TestSkillArea_EmptyTerms(t *testing.T) {
	learning := &domain.Learning{}
	area := SkillArea(learning)
	if area != "general" {
		t.Errorf("SkillArea with empty terms = %q, want %q", area, "general")
	}
}

func TestSkillArea_NilLearning(t *testing.T) {
	area := SkillArea(nil)
	if area != "general" {
		t.Errorf("SkillArea(nil) = %q, want %q", area, "general")
	}
}

func TestSkillArea_SpecialChars(t *testing.T) {
	learning := &domain.Learning{
		RetrievalTerms: []string{"profe_*3b_ec_*"},
	}
	area := SkillArea(learning)
	if strings.Contains(area, "*") {
		t.Errorf("area should not contain special chars: %q", area)
	}
}

func TestSkillName(t *testing.T) {
	name := SkillName("padreseducadores.org", "dashboard-datos")
	if name != "padreseducadores.org-dashboard-datos" {
		t.Errorf("SkillName = %q", name)
	}
}

func TestIndexSkillName(t *testing.T) {
	name := IndexSkillName("padreseducadores.org")
	if name != "padreseducadores.org-conocimiento" {
		t.Errorf("IndexSkillName = %q, want padreseducadores.org-conocimiento", name)
	}
}

// --- GenerateSkillContent tests ---

func TestGenerateSkillContent_Basic(t *testing.T) {
	sections := []SkillSection{
		{
			LearningID:   "test-id-1",
			Title:        "Test Skill",
			Rule:         "Always do X before Y.",
			Procedure:    []string{"Step 1: Check X", "Step 2: Do Y"},
			CanonExample: "Example context here.",
			Limits:       "Only applies to A and B.",
		},
	}

	fm := SkillFrontmatter{
		Name:        "myproj-test-area",
		Description: "Trigger: test, area. Reglas capturadas.",
		Source:      "royo-learn",
		Project:     "myproj",
		LearningIDs: []domain.LearningID{"test-id-1"},
		UpdatedAt:   "2026-07-12",
	}

	content := GenerateSkillContent(fm, sections)

	// Frontmatter checks.
	if !strings.HasPrefix(content, "---\n") {
		t.Error("content should start with YAML frontmatter")
	}
	if !strings.Contains(content, "name: myproj-test-area") {
		t.Error("frontmatter should contain name")
	}
	if !strings.Contains(content, "source: royo-learn") {
		t.Error("frontmatter should contain source")
	}
	if !strings.Contains(content, "project: myproj") {
		t.Error("frontmatter should contain project")
	}
	if !strings.Contains(content, "test-id-1") {
		t.Error("frontmatter should contain learning_ids entry")
	}
	if !strings.Contains(content, "updated_at:") {
		t.Error("frontmatter should contain updated_at")
	}

	// Body checks.
	if !strings.Contains(content, "## Test Skill") {
		t.Error("body should contain section title")
	}
	if !strings.Contains(content, "<!-- royo-learn:learning-id test-id-1 -->") {
		t.Error("body should contain learning ID marker")
	}
	if !strings.Contains(content, "### Regla") {
		t.Error("body should contain Regla section")
	}
	if !strings.Contains(content, "Always do X before Y.") {
		t.Error("body should contain the rule")
	}
	if !strings.Contains(content, "### Procedimiento") {
		t.Error("body should contain Procedimiento section")
	}
	if !strings.Contains(content, "### Ejemplo canónico") {
		t.Error("body should contain Ejemplo canónico section")
	}
	if !strings.Contains(content, "### Límites") {
		t.Error("body should contain Límites section")
	}
}

func TestGenerateSkillContent_MultipleSections(t *testing.T) {
	sections := []SkillSection{
		{LearningID: "id-1", Title: "Rule A", Rule: "Rule A body."},
		{LearningID: "id-2", Title: "Rule B", Rule: "Rule B body."},
	}

	fm := SkillFrontmatter{
		Name:        "proj-area",
		Description: "desc",
		Source:      "royo-learn",
		Project:     "proj",
		LearningIDs: []domain.LearningID{"id-1", "id-2"},
		UpdatedAt:   "2026-07-12",
	}

	content := GenerateSkillContent(fm, sections)

	if !strings.Contains(content, "id-1") || !strings.Contains(content, "id-2") {
		t.Error("frontmatter should list both learning IDs")
	}
	if !strings.Contains(content, "## Rule A") {
		t.Error("should contain first section")
	}
	if !strings.Contains(content, "## Rule B") {
		t.Error("should contain second section")
	}
}

func TestGenerateSkillContent_NoProcedure(t *testing.T) {
	sections := []SkillSection{
		{LearningID: "id-1", Title: "Simple", Rule: "Just a rule."},
	}

	fm := SkillFrontmatter{
		Name:        "proj-area", Description: "desc",
		Source: "royo-learn", Project: "proj",
		LearningIDs: []domain.LearningID{"id-1"}, UpdatedAt: "2026-07-12",
	}

	content := GenerateSkillContent(fm, sections)
	if strings.Contains(content, "### Procedimiento") {
		t.Error("should not contain procedimiento when empty")
	}
}

// --- MergeLearningIntoSections tests ---

func TestMergeLearningIntoSections_NewSection(t *testing.T) {
	existing := []SkillSection{}
	learning := &domain.Learning{
		ID:              "new-id",
		Title:           "New Rule",
		ReusableLesson:  "Always test first.",
		RecommendedProcedure: []string{"Run tests", "Fix bugs"},
		Limits:          "Only in dev.",
		Context:         "Test context",
		Observation:     "Test observation",
	}

	merged := MergeLearningIntoSections(existing, learning)
	if len(merged) != 1 {
		t.Fatalf("expected 1 section, got %d", len(merged))
	}
	if merged[0].LearningID != "new-id" {
		t.Errorf("LearningID = %q", merged[0].LearningID)
	}
	if merged[0].Rule != "Always test first." {
		t.Errorf("Rule = %q", merged[0].Rule)
	}
}

func TestMergeLearningIntoSections_UpdateExisting(t *testing.T) {
	existing := []SkillSection{
		{LearningID: "id-1", Title: "Old Title", Rule: "Old rule."},
	}
	learning := &domain.Learning{
		ID:              "id-1",
		Title:           "Updated Title",
		ReusableLesson:  "Updated rule.",
		RecommendedProcedure: []string{"New step"},
		Context:         "New context",
		Observation:     "New observation",
	}

	merged := MergeLearningIntoSections(existing, learning)
	if len(merged) != 1 {
		t.Fatalf("expected 1 section after update, got %d", len(merged))
	}
	if merged[0].Title != "Updated Title" {
		t.Errorf("Title not updated: %q", merged[0].Title)
	}
	if merged[0].Rule != "Updated rule." {
		t.Errorf("Rule not updated: %q", merged[0].Rule)
	}
}

func TestMergeLearningIntoSections_IdempotentRePublish(t *testing.T) {
	// Re-publishing the same learning twice should NOT create a duplicate.
	sections := []SkillSection{
		{LearningID: "id-1", Title: "Rule A", Rule: "A body."},
	}
	learning := &domain.Learning{
		ID:              "id-1",
		Title:           "Rule A",
		ReusableLesson:  "A body.",
		RecommendedProcedure: []string{},
		Context:         "ctx",
		Observation:     "obs",
	}

	merged := MergeLearningIntoSections(sections, learning)
	if len(merged) != 1 {
		t.Fatalf("re-publish should not duplicate: got %d sections", len(merged))
	}
}

// --- GenerateIndexContent tests ---

func TestGenerateIndexContent_Empty(t *testing.T) {
	content := GenerateIndexContent("testproj", nil)
	if !strings.Contains(content, "name: testproj-conocimiento") {
		t.Error("index should have the correct name")
	}
	if !strings.Contains(content, "sin skills hijas todavía") {
		t.Error("empty index should show placeholder message")
	}
}

func TestGenerateIndexContent_WithEntries(t *testing.T) {
	entries := []IndexEntry{
		{SkillName: "testproj-dashboard", Description: "Trigger: dashboard_data"},
		{SkillName: "testproj-auth", Description: "Trigger: login, auth"},
	}

	content := GenerateIndexContent("testproj", entries)

	if !strings.Contains(content, "testproj-dashboard") {
		t.Error("index should list dashboard skill")
	}
	if !strings.Contains(content, "testproj-auth") {
		t.Error("index should list auth skill")
	}
	if !strings.Contains(content, "| `testproj-dashboard` |") {
		t.Error("index should use markdown table")
	}
}

// --- AGENTS.md hook tests ---

func TestHasAgentsRef_NotFound(t *testing.T) {
	content := "# My Project\n\nSome content.\n"
	if HasAgentsRef(content, "myproj") {
		t.Error("should not find ref in clean content")
	}
}

func TestHasAgentsRef_Found(t *testing.T) {
	ref := BuildAgentsRefLine("myproj")
	content := "# My Project\n\n" + ref + "\n\nMore content.\n"
	if !HasAgentsRef(content, "myproj") {
		t.Error("should find ref when present")
	}
}

func TestInsertAgentsRef_FirstTime(t *testing.T) {
	content := "# My Project\n\nHello.\n"
	newContent, changed := InsertAgentsRef(content, "myproj")
	if !changed {
		t.Error("first insert should report changed=true")
	}
	if !strings.Contains(newContent, "myproj-conocimiento") {
		t.Error("should contain index skill reference")
	}
	if !strings.Contains(newContent, "<!-- royo-learn:managed start -->") {
		t.Error("should use managed block")
	}
}

func TestInsertAgentsRef_Idempotent(t *testing.T) {
	content := "# My Project\n\nHello.\n"
	// First insert.
	c1, changed1 := InsertAgentsRef(content, "myproj")
	if !changed1 {
		t.Fatal("first insert should change")
	}
	// Second insert.
	c2, changed2 := InsertAgentsRef(c1, "myproj")
	if changed2 {
		t.Error("second insert should NOT change (already present)")
	}
	if c1 != c2 {
		t.Error("content should be identical after idempotent insert")
	}
}

// --- parseSkillSections tests ---

func TestParseSkillSections_Single(t *testing.T) {
	content := `---
name: proj-area
description: desc
---
## Test Rule
<!-- royo-learn:learning-id id-1 -->

### Regla

Always do X.

### Procedimiento

- Step 1
- Step 2

### Límites

Only in dev.
`

	sections := parseSkillSections(content)
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	if sections[0].LearningID != "id-1" {
		t.Errorf("LearningID = %q, want %q", sections[0].LearningID, "id-1")
	}
	if sections[0].Title != "Test Rule" {
		t.Errorf("Title = %q, want %q", sections[0].Title, "Test Rule")
	}
	if !strings.Contains(sections[0].Rule, "Always do X") {
		t.Errorf("Rule missing: %q", sections[0].Rule)
	}
	// Procedure text is accumulated into rule body since the parser
	// doesn't split into array — it captures raw text.
	if !strings.Contains(sections[0].Rule, "Step 1") {
		t.Errorf("Rule should contain procedure text: %q", sections[0].Rule)
	}
	if !strings.Contains(sections[0].Rule, "Step 2") {
		t.Errorf("Rule should contain procedure text: %q", sections[0].Rule)
	}
	if !strings.Contains(sections[0].Limits, "Only in dev") {
		t.Errorf("Limits missing: %q", sections[0].Limits)
	}
}

func TestParseSkillSections_Multiple(t *testing.T) {
	content := `---
name: proj-area
description: desc
---
## Rule A
<!-- royo-learn:learning-id id-a -->

### Regla

Rule A body.

## Rule B
<!-- royo-learn:learning-id id-b -->

### Regla

Rule B body.
`

	sections := parseSkillSections(content)
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	if sections[0].LearningID != "id-a" {
		t.Errorf("first section LearningID = %q", sections[0].LearningID)
	}
	if sections[1].LearningID != "id-b" {
		t.Errorf("second section LearningID = %q", sections[1].LearningID)
	}
}

func TestParseSkillSections_RePublishUpdatesSection(t *testing.T) {
	// Simulate re-publishing: parse existing sections, merge update, regenerate.
	content := `---
name: proj-area
description: desc
source: royo-learn
project: proj
learning_ids: [id-1]
updated_at: 2026-07-12
---
## Old Title
<!-- royo-learn:learning-id id-1 -->

### Regla

Old rule.

### Procedimiento

- Old step
`

	sections := parseSkillSections(content)
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}

	// Now "re-publish" with updated learning.
	learning := &domain.Learning{
		ID:              "id-1",
		Title:           "Updated Title",
		ReusableLesson:  "Updated rule.",
		RecommendedProcedure: []string{"New step", "Extra step"},
		Limits:          "New limits.",
		Context:         "ctx",
		Observation:     "obs",
	}

	merged := MergeLearningIntoSections(sections, learning)
	if len(merged) != 1 {
		t.Fatalf("should still have 1 section, got %d", len(merged))
	}
	if merged[0].Title != "Updated Title" {
		t.Errorf("title should be updated: %q", merged[0].Title)
	}
	if merged[0].Rule != "Updated rule." {
		t.Errorf("rule should be updated: %q", merged[0].Rule)
	}
	if len(merged[0].Procedure) != 2 {
		t.Errorf("procedure should have 2 steps, got %d", len(merged[0].Procedure))
	}
}

// --- ResolveSkillPublishTargets tests ---

func TestResolveSkillPublishTargets_NewSkills(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "skills"), 0o755)

	dest := &domain.Destination{
		Type: domain.DestSkill,
		Root: "skills",
		Path: "testproj-dashboard-datos",
	}

	result, err := ResolveSkillPublishTargets(tmpDir, dest, "testproj", true)
	if err != nil {
		t.Fatalf("ResolveSkillPublishTargets: %v", err)
	}

	// Should have 3 targets: child, index, agents.
	targets := result.Flatten()
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}

	// Child skill — use filepath.Join for platform-aware comparison.
	expectedChild := filepath.Join("testproj-dashboard-datos", "SKILL.md")
	if targets[0].Path != expectedChild {
		t.Errorf("child path = %q, want %q", targets[0].Path, expectedChild)
	}
	if targets[0].Exists {
		t.Error("child skill should not exist yet")
	}
	if targets[0].Operation != domain.OpCreate {
		t.Errorf("child op = %q, want create", targets[0].Operation)
	}

	// Index skill.
	expectedIndex := filepath.Join("testproj-conocimiento", "SKILL.md")
	if targets[1].Path != expectedIndex {
		t.Errorf("index path = %q, want %q", targets[1].Path, expectedIndex)
	}

	// AGENTS.md hook.
	if targets[2].Path != "AGENTS.md" {
		t.Errorf("agents path = %q", targets[2].Path)
	}
	if targets[2].Root != "." {
		t.Errorf("agents root = %q, want .", targets[2].Root)
	}
}

func TestResolveSkillPublishTargets_NoAgentsHook(t *testing.T) {
	tmpDir := t.TempDir()
	dest := &domain.Destination{
		Type: domain.DestSkill,
		Root: "skills",
		Path: "testproj-dashboard",
	}

	result, err := ResolveSkillPublishTargets(tmpDir, dest, "testproj", false)
	if err != nil {
		t.Fatalf("ResolveSkillPublishTargets: %v", err)
	}

	targets := result.Flatten()
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets (no agents hook), got %d", len(targets))
	}
	if result.AgentsRef != nil {
		t.Error("AgentsRef should be nil when needAgentsHook is false")
	}
}

func TestResolveSkillPublishTargets_ExistingSkill(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "skills", "testproj-existing")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("existing"), 0o644)

	dest := &domain.Destination{
		Type: domain.DestSkill,
		Root: "skills",
		Path: "testproj-existing",
	}

	result, err := ResolveSkillPublishTargets(tmpDir, dest, "testproj", false)
	if err != nil {
		t.Fatalf("ResolveSkillPublishTargets: %v", err)
	}

	if !result.ChildSkill.Exists {
		t.Error("existing skill should be detected")
	}
	if result.ChildSkill.Operation != domain.OpReplace {
		t.Errorf("existing skill op = %q, want replace", result.ChildSkill.Operation)
	}
}

// --- DiscoverChildSkills tests ---

func TestDiscoverChildSkills_NoSkills(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "skills"), 0o755)

	entries, err := DiscoverChildSkills(tmpDir, "testproj")
	if err != nil {
		t.Fatalf("DiscoverChildSkills: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestDiscoverChildSkills_WithSkills(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a child skill.
	skillDir := filepath.Join(tmpDir, "skills", "testproj-dashboard")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: testproj-dashboard
description: "Trigger: dashboard_data, fechas. Rules for dashboard."
---
`), 0o644)

	// Create a non-project skill (should be excluded).
	otherDir := filepath.Join(tmpDir, "skills", "otherproject-auth")
	os.MkdirAll(otherDir, 0o755)
	os.WriteFile(filepath.Join(otherDir, "SKILL.md"), []byte("---\nname: otherproject-auth\n---\n"), 0o644)

	// Create the index skill itself (should be excluded).
	indexDir := filepath.Join(tmpDir, "skills", "testproj-conocimiento")
	os.MkdirAll(indexDir, 0o755)
	os.WriteFile(filepath.Join(indexDir, "SKILL.md"), []byte("index"), 0o644)

	entries, err := DiscoverChildSkills(tmpDir, "testproj")
	if err != nil {
		t.Fatalf("DiscoverChildSkills: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (only testproj-dashboard), got %d", len(entries))
	}
	if entries[0].SkillName != "testproj-dashboard" {
		t.Errorf("SkillName = %q", entries[0].SkillName)
	}
}

// --- BuildDescription tests ---

func TestBuildDescription(t *testing.T) {
	learnings := []*domain.Learning{
		{RetrievalTerms: []string{"dashboard", "datos", "fechas"}},
		{RetrievalTerms: []string{"dashboard", "auth"}},
	}

	desc := BuildDescription("testproj", "dashboard", learnings)
	if !strings.Contains(desc, "Trigger:") {
		t.Error("description should start with Trigger:")
	}
	if !strings.Contains(desc, "auth") {
		t.Error("description should include trigger 'auth'")
	}
	if !strings.Contains(desc, "dashboard") {
		t.Error("description should include trigger 'dashboard'")
	}
}

// --- buildSkillSection tests ---

func TestBuildSkillSection(t *testing.T) {
	learning := &domain.Learning{
		ID:              "test-id",
		Title:           "Test Learning",
		ReusableLesson:  "Always validate input.",
		RecommendedProcedure: []string{"Step 1", "Step 2"},
		Limits:          "Only for web forms.",
		Context:         "Web development",
		Observation:     "Users submitted malformed data.",
	}

	sec := buildSkillSection(learning)
	if sec.LearningID != "test-id" {
		t.Errorf("LearningID = %q", sec.LearningID)
	}
	if sec.Rule != "Always validate input." {
		t.Errorf("Rule = %q", sec.Rule)
	}
	if len(sec.Procedure) != 2 {
		t.Errorf("expected 2 procedures, got %d", len(sec.Procedure))
	}
	if !strings.Contains(sec.CanonExample, "Web development") {
		t.Error("canonical example should include context")
	}
	if !strings.Contains(sec.CanonExample, "malformed data") {
		t.Error("canonical example should include observation")
	}
	if sec.Limits != "Only for web forms." {
		t.Errorf("Limits = %q", sec.Limits)
	}
}
