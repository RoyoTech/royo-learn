package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"agent-royo-learn/internal/domain"
)

func TestResolveTarget_Skill(t *testing.T) {
	tmpDir := t.TempDir()
	curation := &domain.Curation{
		Decision: domain.CurationApproveNewSkill,
		Destination: &domain.Destination{
			Type:     domain.DestSkill,
			Root:     "skills",
			Path:     "test-skill/SKILL.md",
			Required: true,
		},
	}

	// Create the skills directory.
	skillDir := filepath.Join(tmpDir, "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	targets, err := ResolveTarget(tmpDir, curation)
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}

	target := targets[0]
	if target.Root != filepath.Join(tmpDir, "skills") {
		t.Errorf("Root = %q, want containing skills", target.Root)
	}
	if target.Path != "test-skill/SKILL.md" {
		t.Errorf("Path = %q", target.Path)
	}
	if !target.IsManaged {
		t.Error("expected IsManaged for skill target")
	}
}

func TestResolveTarget_AgentsRule(t *testing.T) {
	tmpDir := t.TempDir()

	curation := &domain.Curation{
		Decision: domain.CurationApproveAgentsRule,
		Destination: &domain.Destination{
			Type:     domain.DestAgentsRule,
			Root:     ".",
			Path:     "AGENTS.md",
			Required: true,
		},
	}

	targets, err := ResolveTarget(tmpDir, curation)
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}

	if targets[0].Path != "AGENTS.md" {
		t.Errorf("Path = %q", targets[0].Path)
	}
	if !targets[0].IsManaged {
		t.Error("expected IsManaged for agents target")
	}
}

func TestResolveTarget_PathEscapesRoot(t *testing.T) {
	tmpDir := t.TempDir()

	curation := &domain.Curation{
		Decision: domain.CurationApproveProjectKnowledge,
		Destination: &domain.Destination{
			Type:     domain.DestSkill,
			Root:     "skills",
			Path:     "../outside/SKILL.md",
			Required: false,
		},
	}

	_, err := ResolveTarget(tmpDir, curation)
	if err == nil {
		t.Fatal("expected path escape error, got nil")
	}
}

func TestResolveTarget_NilDestination(t *testing.T) {
	curation := &domain.Curation{
		Decision:    domain.CurationApproveProjectKnowledge,
		Destination: nil,
	}

	_, err := ResolveTarget("/tmp", curation)
	if err == nil {
		t.Fatal("expected error for nil destination")
	}
}

func TestGenerateDiff_NewFile(t *testing.T) {
	diff := GenerateDiff(nil, []byte("hello\nworld\n"), "test.md", false)

	if !strings.Contains(diff, "new file") {
		t.Error("diff should indicate new file")
	}
	if !strings.Contains(diff, "+hello") {
		t.Error("diff should contain added lines prefixed with +")
	}
}

func TestGenerateDiff_ExistingFileChanged(t *testing.T) {
	current := []byte("line1\nline2\nline3\n")
	proposed := []byte("line1\nline2_changed\nline3\n")

	diff := GenerateDiff(current, proposed, "test.md", true)

	if !strings.Contains(diff, "+line2_changed") || !strings.Contains(diff, "-line2") {
		t.Errorf("diff should show changes:\n%s", diff)
	}
}

func TestGenerateDiff_ExistingFileNoChanges(t *testing.T) {
	current := []byte("line1\nline2")
	proposed := []byte("line1\nline2")

	diff := GenerateDiff(current, proposed, "test.md", true)

	// Diff should only show unchanged context lines (no +/- on content lines).
	added := 0
	removed := 0
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}
		if strings.HasPrefix(line, "+") {
			added++
		}
		if strings.HasPrefix(line, "-") {
			removed++
		}
	}

	if added > 0 || removed > 0 {
		t.Errorf("diff should show no content changes (got +%d/-%d):\n%s", added, removed, diff)
	}
}

func TestDiffSummary(t *testing.T) {
	diff := "--- a/test.md\n+++ b/test.md\n line1\n-line2\n+line2_new\n+line3"
	summary := DiffSummary(diff, "test.md")

	if !strings.Contains(summary, "+2/-1") {
		t.Errorf("DiffSummary = %q, want containing +2/-1", summary)
	}
}

func TestEvaluatePolicies_PreferenceType(t *testing.T) {
	learning := &domain.Learning{
		Type: domain.TypePreference,
	}
	curation := &domain.Curation{
		Decision: domain.CurationApproveSharedKnowledge,
		Destination: &domain.Destination{
			Type: domain.DestShared,
		},
	}

	policies := EvaluatePolicies(learning, curation)

	if !RequiresHumanApproval(policies) {
		t.Error("preference type + shared destination should require human approval")
	}
}

func TestEvaluatePolicies_ProcedureType_Shared(t *testing.T) {
	learning := &domain.Learning{
		Type: domain.TypeProcedure,
	}
	curation := &domain.Curation{
		Decision: domain.CurationApproveSharedKnowledge,
		Destination: &domain.Destination{
			Type: domain.DestShared,
		},
	}

	policies := EvaluatePolicies(learning, curation)

	if RequiresHumanApproval(policies) {
		t.Error("procedure type + approved shared knowledge should NOT require human approval")
	}
}

func TestEvaluatePolicies_AgentsRuleNotApproved(t *testing.T) {
	learning := &domain.Learning{
		Type: domain.TypeDiagnostic,
	}
	curation := &domain.Curation{
		Decision: domain.CurationApproveProjectKnowledge,
		Destination: &domain.Destination{
			Type: domain.DestAgentsRule,
		},
	}

	policies := EvaluatePolicies(learning, curation)

	if !RequiresHumanApproval(policies) {
		t.Error("AGENTS.md destination without approve_agents_rule decision should require approval")
	}
}

func TestEvaluatePolicies_AllPass(t *testing.T) {
	learning := &domain.Learning{
		Type: domain.TypeProcedure,
	}
	curation := &domain.Curation{
		Decision: domain.CurationApproveProjectKnowledge,
		Destination: &domain.Destination{
			Type: domain.DestProject,
		},
	}

	policies := EvaluatePolicies(learning, curation)

	if RequiresHumanApproval(policies) {
		t.Error("procedure type + project destination should pass all policies")
	}
}

func TestEvaluateRisk(t *testing.T) {
	learning := &domain.Learning{}

	tests := []struct {
		name string
		dest domain.DestinationType
		want domain.RiskLevel
	}{
		{"shared high risk", domain.DestShared, domain.RiskHigh},
		{"agents high risk", domain.DestAgentsRule, domain.RiskHigh},
		{"required skill medium risk", domain.DestSkill, domain.RiskMedium},
		{"project low risk", domain.DestProject, domain.RiskLow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			curation := &domain.Curation{
				Destination: &domain.Destination{
					Type:     tt.dest,
					Required: tt.dest == domain.DestSkill,
				},
			}
			got := evaluateRisk(learning, curation)
			if got != tt.want {
				t.Errorf("evaluateRisk(%q) = %q, want %q", tt.dest, got, tt.want)
			}
		})
	}
}

func TestParseManagedBlocks(t *testing.T) {
	content := `# Header
Some text

<!-- royo-learn:managed start -->
managed content line 1
managed content line 2
<!-- royo-learn:managed end -->

More text

<!-- royo-learn:managed start -->
second block
<!-- royo-learn:managed end -->`

	blocks := ParseManagedBlocks(content)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 managed blocks, got %d", len(blocks))
	}

	if !strings.Contains(blocks[0].Content, "managed content line 1") {
		t.Errorf("block 0 content = %q", blocks[0].Content)
	}
	if !strings.Contains(blocks[1].Content, "second block") {
		t.Errorf("block 1 content = %q", blocks[1].Content)
	}
}

func TestFindManagedBlock(t *testing.T) {
	content := `<!-- royo-learn:managed start -->
block-id:test-123
some content
<!-- royo-learn:managed end -->`

	block, err := FindManagedBlock(content, "block-id:test-123")
	if err != nil {
		t.Fatalf("FindManagedBlock: %v", err)
	}
	if block == nil {
		t.Fatal("expected block, got nil")
	}
	if !strings.Contains(block.Content, "some content") {
		t.Errorf("block content = %q", block.Content)
	}
}

func TestReplaceManagedBlock(t *testing.T) {
	content := `prefix
<!-- royo-learn:managed start -->
block-id:test
old content
<!-- royo-learn:managed end -->
suffix`

	newContent, err := ReplaceManagedBlock(content, "block-id:test", "new content")
	if err != nil {
		t.Fatalf("ReplaceManagedBlock: %v", err)
	}

	if !strings.Contains(newContent, "new content") {
		t.Errorf("expected 'new content' in result:\n%s", newContent)
	}
	if strings.Contains(newContent, "old content") {
		t.Errorf("old content should be replaced:\n%s", newContent)
	}
	if !strings.Contains(newContent, "prefix") || !strings.Contains(newContent, "suffix") {
		t.Error("prefix and suffix should be preserved")
	}
}

func TestInsertManagedBlock_NoExisting(t *testing.T) {
	content := "existing content\n"
	result := InsertManagedBlock(content, "new managed block")

	if !strings.Contains(result, ManagedBlockStart) || !strings.Contains(result, ManagedBlockEnd) {
		t.Error("should contain managed block delimiters")
	}
	if !strings.Contains(result, "new managed block") {
		t.Error("should contain the new managed block content")
	}
}

func TestInsertManagedBlock_WithExisting(t *testing.T) {
	content := "before\n<!-- royo-learn:managed start -->\nexisting\n<!-- royo-learn:managed end -->\nafter\n"
	result := InsertManagedBlock(content, "new block")

	if !strings.Contains(result, "existing") {
		t.Error("should preserve existing managed block")
	}
	if !strings.Contains(result, "new block") {
		t.Error("should contain new managed block")
	}
}

func TestHasManagedBlocks(t *testing.T) {
	if !HasManagedBlocks("text <!-- royo-learn:managed start --> x <!-- royo-learn:managed end -->") {
		t.Error("should detect managed blocks")
	}
	if HasManagedBlocks("no managed blocks here") {
		t.Error("should not detect managed blocks in plain text")
	}
}

func TestValidateSkill_Valid(t *testing.T) {
	content := `---
name: test-skill
description: A test skill
license: MIT
---

# Test Skill
Content here.`

	result := ValidateSkill([]byte(content))
	if !result.Valid {
		t.Errorf("expected valid skill, got issues: %v", result.Issues)
	}
}

func TestValidateSkill_MissingName(t *testing.T) {
	content := `---
description: A test skill
---

# Test Skill`

	result := ValidateSkill([]byte(content))
	if result.Valid {
		t.Error("expected invalid skill due to missing name")
	}
}

func TestValidateSkill_NoFrontmatter(t *testing.T) {
	content := `# Test Skill
No front matter here.`

	result := ValidateSkill([]byte(content))
	if result.Valid {
		t.Error("expected invalid skill due to missing front matter")
	}
	if result.HasFrontmatter {
		t.Error("should not report frontmatter present")
	}
}

func TestValidateSkill_UnclosedFrontmatter(t *testing.T) {
	content := `---
name: test-skill
description: Missing closing
`

	result := ValidateSkill([]byte(content))
	if result.Valid {
		t.Error("expected invalid skill due to unclosed frontmatter")
	}
}

func TestBuildSkillContent(t *testing.T) {
	result := BuildSkillContent("My Skill", "Test description", "The lesson", "Step 1\nStep 2")

	if !strings.Contains(result, "name: my-skill") {
		t.Error("should contain normalized name")
	}
	if !strings.Contains(result, "description: Test description") {
		t.Error("should contain description")
	}
	if !strings.Contains(result, "# My Skill") {
		t.Error("should contain title heading")
	}
	if !strings.Contains(result, "The lesson") {
		t.Error("should contain lesson content")
	}
	if !strings.Contains(result, "## Procedure") {
		t.Error("should contain procedure section")
	}
	if !strings.Contains(result, "Step 1") {
		t.Error("should contain procedure steps")
	}
}

func TestHashContent(t *testing.T) {
	h1 := HashContent([]byte("hello"))
	h2 := HashContent([]byte("hello"))
	h3 := HashContent([]byte("world"))

	if h1 != h2 {
		t.Errorf("identical content should produce identical hash: %q vs %q", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("different content should produce different hash")
	}
	if len(h1) != 64 {
		t.Errorf("SHA-256 hash should be 64 hex chars, got %d", len(h1))
	}
}

func TestAtomicWriteAndHash(t *testing.T) {
	tmpDir := t.TempDir()
	writer := NewWriter(tmpDir)

	content := []byte("hello world")
	path := "test.txt"

	if err := writer.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	hash, err := HashFile(filepath.Join(tmpDir, path))
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}

	expected := HashContent(content)
	if hash != expected {
		t.Errorf("hash mismatch: got %q, want %q", hash, expected)
	}

	// Verify content.
	readback, err := os.ReadFile(filepath.Join(tmpDir, path))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(readback) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", readback, content)
	}
}

func TestCheckDirty_NoGit(t *testing.T) {
	tmpDir := t.TempDir()
	targets := []TargetResolution{
		{Root: ".", Path: "test.md"},
	}

	result, err := CheckDirtyWorktree(tmpDir, targets)
	if err != nil {
		t.Fatalf("CheckDirtyWorktree: %v", err)
	}

	if result.IsDirty {
		t.Error("non-git directory should not be dirty")
	}
}

func TestParseGitStatus(t *testing.T) {
	output := " M README.md\n M src/main.go\n?? newfile.txt\n"

	files := parseGitStatus(output)

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d: %v", len(files), files)
	}
	if files[0] != "README.md" {
		t.Errorf("files[0] = %q", files[0])
	}
	if files[1] != "src/main.go" {
		t.Errorf("files[1] = %q", files[1])
	}
}

// --- Policy edge cases ------------------------------------------

func TestEvaluatePolicies_AgentsRuleApproved(t *testing.T) {
	learning := &domain.Learning{Type: domain.TypeDiagnostic}
	curation := &domain.Curation{
		Decision: domain.CurationApproveAgentsRule,
		Destination: &domain.Destination{
			Type: domain.DestAgentsRule,
		},
	}

	policies := EvaluatePolicies(learning, curation)
	if RequiresHumanApproval(policies) {
		t.Error("AGENTS.md with approve_agents_rule decision should NOT require human approval")
	}
}

func TestEvaluatePolicies_NilDestination(t *testing.T) {
	learning := &domain.Learning{Type: domain.TypePreference}
	curation := &domain.Curation{
		Decision:    domain.CurationApproveProjectKnowledge,
		Destination: nil,
	}

	policies := EvaluatePolicies(learning, curation)
	// All policies should pass when destination is nil (defensive guard).
	for _, p := range policies {
		if !p.Passed {
			t.Errorf("policy %q should pass with nil destination, got: %s", p.PolicyName, p.Reason)
		}
	}
}

func TestEvaluatePolicies_SharedWithoutApproval(t *testing.T) {
	learning := &domain.Learning{Type: domain.TypeProcedure}
	curation := &domain.Curation{
		Decision: domain.CurationApproveProjectKnowledge,
		Destination: &domain.Destination{
			Type: domain.DestShared,
		},
	}

	policies := EvaluatePolicies(learning, curation)
	if !RequiresHumanApproval(policies) {
		t.Error("shared destination without approve_shared_knowledge decision should require approval")
	}
}

func TestPolicyCount(t *testing.T) {
	learning := &domain.Learning{Type: domain.TypeProcedure}
	curation := &domain.Curation{
		Decision:    domain.CurationApproveProjectKnowledge,
		Destination: &domain.Destination{Type: domain.DestProject},
	}

	policies := EvaluatePolicies(learning, curation)
	if len(policies) != 3 {
		t.Fatalf("expected 3 policy evaluations, got %d", len(policies))
	}
}

// --- Dirty worktree detection -----------------------------------

func TestCheckDirty_CleanGitRepo(t *testing.T) {
	// This test verifies the git status parsing path.
	// When git returns empty output, it's a clean tree.
	result := parseGitStatus("")
	if len(result) != 0 {
		t.Errorf("empty git status should produce 0 files, got %d", len(result))
	}
}

func TestCheckDirty_ShortLineIgnored(t *testing.T) {
	// Lines shorter than 4 chars should be ignored.
	result := parseGitStatus("M\n??\n")
	if len(result) != 0 {
		t.Errorf("short lines should be ignored, got %d: %v", len(result), result)
	}
}

func TestCheckDirty_SpacesInPath(t *testing.T) {
	result := parseGitStatus(" M path/with spaces/file.md\n")
	if len(result) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result))
	}
	if result[0] != "path/with spaces/file.md" {
		t.Errorf("path = %q, want with spaces", result[0])
	}
}

// --- Managed block edge cases -----------------------------------

func TestParseManagedBlocks_Empty(t *testing.T) {
	blocks := ParseManagedBlocks("")
	if len(blocks) != 0 {
		t.Errorf("empty content should produce 0 blocks, got %d", len(blocks))
	}
}

func TestParseManagedBlocks_NoEndMarker(t *testing.T) {
	content := "<!-- royo-learn:managed start -->\nunclosed block\n"
	blocks := ParseManagedBlocks(content)
	if len(blocks) != 0 {
		t.Errorf("unclosed block should not be counted, got %d", len(blocks))
	}
}

func TestParseManagedBlocks_EndWithoutStart(t *testing.T) {
	content := "<!-- royo-learn:managed end -->\norphan end\n"
	blocks := ParseManagedBlocks(content)
	if len(blocks) != 0 {
		t.Errorf("end without start should not produce a block, got %d", len(blocks))
	}
}

func TestParseManagedBlocks_NestedStart(t *testing.T) {
	content := "<!-- royo-learn:managed start -->\n<!-- royo-learn:managed start -->\nnested\n<!-- royo-learn:managed end -->\n<!-- royo-learn:managed end -->"
	blocks := ParseManagedBlocks(content)
	// The implementation treats consecutive starts as restarting the block.
	if len(blocks) == 0 {
		t.Error("should find at least one block in nested markers")
	}
}

func TestFindManagedBlock_NotFound(t *testing.T) {
	content := "<!-- royo-learn:managed start -->\nsome content\n<!-- royo-learn:managed end -->"
	_, err := FindManagedBlock(content, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent block ID")
	}
}

func TestReplaceManagedBlock_NonExistent(t *testing.T) {
	content := "<!-- royo-learn:managed start -->\nblock-a\n<!-- royo-learn:managed end -->"
	_, err := ReplaceManagedBlock(content, "block-b", "new")
	if err == nil {
		t.Error("expected error replacing non-existent block")
	}
}

func TestInsertManagedBlock_EmptyContent(t *testing.T) {
	result := InsertManagedBlock("", "only block")
	if !strings.Contains(result, "only block") {
		t.Error("should insert block into empty content")
	}
	if !strings.Contains(result, ManagedBlockStart) {
		t.Error("should add managed block start marker")
	}
}

// --- Rollback restore tests -------------------------------------

func TestBackupAndRestore(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	// Create a file to back up.
	srcDir := filepath.Join(tmpDir, "src")
	os.MkdirAll(srcDir, 0o755)
	srcPath := filepath.Join(srcDir, "test.txt")
	originalContent := []byte("original content v1")
	if err := os.WriteFile(srcPath, originalContent, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	mgr := NewBackupManager(tmpDir, backupDir)
	entry, err := mgr.BackupFile(filepath.Join("src", "test.txt"))
	if err != nil {
		t.Fatalf("BackupFile: %v", err)
	}

	// Modify the original.
	if err := os.WriteFile(srcPath, []byte("modified content v2"), 0o644); err != nil {
		t.Fatalf("modify source: %v", err)
	}

	// Restore from backup.
	if err := mgr.RestoreFile(*entry); err != nil {
		t.Fatalf("RestoreFile: %v", err)
	}

	// Verify content restored.
	restored, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read restored: %v", err)
	}
	if string(restored) != string(originalContent) {
		t.Errorf("restored content = %q, want %q", string(restored), string(originalContent))
	}
}

func TestBackupNonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	mgr := NewBackupManager(tmpDir, backupDir)
	entry, err := mgr.BackupFile("nonexistent.txt")
	if err != nil {
		t.Fatalf("BackupFile non-existent should succeed (noop): %v", err)
	}
	if entry.Checksum != "" {
		t.Errorf("non-existent backup should have empty checksum, got %q", entry.Checksum)
	}
}

func TestRestoreAll_MixedResults(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	// Create one real file, one non-existent.
	srcDir := filepath.Join(tmpDir, "src")
	os.MkdirAll(srcDir, 0o755)
	srcPath := filepath.Join(srcDir, "real.txt")
	os.WriteFile(srcPath, []byte("real"), 0o644)

	mgr := NewBackupManager(tmpDir, backupDir)
	entry1, _ := mgr.BackupFile(filepath.Join("src", "real.txt"))
	entry2, _ := mgr.BackupFile("nonexistent.txt")

	// Delete the real file.
	os.Remove(srcPath)

	results := mgr.RestoreAll([]BackupEntry{*entry1, *entry2})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Success {
		t.Error("restore of real file should succeed")
	}
}

// --- Journal tests -----------------------------------------------

func TestJournalAppend(t *testing.T) {
	tmpDir := t.TempDir()
	journalDir := filepath.Join(tmpDir, "journal")

	j, err := NewJournal(journalDir)
	if err != nil {
		t.Fatalf("NewJournal: %v", err)
	}

	// Append two entries.
	for i := 0; i < 2; i++ {
		entry := JournalEntry{
			PublicationID: fmt.Sprintf("pub-%d", i),
			LearningID:    fmt.Sprintf("learn-%d", i),
		}
		if err := j.Append(entry); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	// Read back the journal file and verify line count.
	data, err := os.ReadFile(filepath.Join(journalDir, "publish-journal.jsonl"))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 journal lines, got %d\n%s", len(lines), string(data))
	}

	// Verify each line is valid JSON.
	for _, line := range lines {
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("journal line not valid JSON: %v\n%s", err, line)
		}
	}
}

func TestJournalEntryFields(t *testing.T) {
	tmpDir := t.TempDir()
	journalDir := filepath.Join(tmpDir, "journal")

	j, _ := NewJournal(journalDir)

	entry := JournalEntry{
		PublicationID:  "pub-123",
		LearningID:     "learn-456",
		Targets:        []domain.TargetEntry{{Path: "test.md"}},
		BackupPaths:    []string{"/tmp/backup.bak"},
		Diff:           "+test",
		Verification:   []domain.ValidationResult{{Check: "ok", Pass: true}},
		RollbackStatus: "rolled_back",
	}
	if err := j.Append(entry); err != nil {
		t.Fatalf("Append: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(journalDir, "publish-journal.jsonl"))
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse journal: %v", err)
	}
	if parsed["publication_id"] != "pub-123" {
		t.Errorf("publication_id = %v", parsed["publication_id"])
	}
	if parsed["rollback_status"] != "rolled_back" {
		t.Errorf("rollback_status = %v", parsed["rollback_status"])
	}
}

// --- Write failure modes ----------------------------------------

func TestWriteFile_ReadOnlyDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping permission test on Windows (os.Chmod on dirs behaves differently)")
	}
	tmpDir := t.TempDir()
	roDir := filepath.Join(tmpDir, "readonly")
	os.MkdirAll(roDir, 0o555) // read+execute only

	writer := NewWriter(roDir)
	err := writer.WriteFile("test.txt", []byte("hello"), 0o644)
	if err == nil {
		t.Error("expected error writing to read-only directory")
	}
}

func TestContentChanged_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	changed, err := ContentChanged(filepath.Join(tmpDir, "nonexistent.txt"), "abc123")
	if err != nil {
		t.Fatalf("ContentChanged non-existent: %v", err)
	}
	if !changed {
		t.Error("non-existent file with non-empty hash should report changed")
	}
}

func TestContentChanged_NonExistentEmptyHash(t *testing.T) {
	tmpDir := t.TempDir()
	changed, err := ContentChanged(filepath.Join(tmpDir, "nonexistent.txt"), "")
	if err != nil {
		t.Fatalf("ContentChanged non-existent empty hash: %v", err)
	}
	if changed {
		t.Error("non-existent file with empty hash should NOT report changed")
	}
}

func TestContentChanged_IdenticalFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "same.txt")
	content := []byte("unchanged")
	os.WriteFile(path, content, 0o644)

	expectedHash := HashContent(content)
	changed, err := ContentChanged(path, expectedHash)
	if err != nil {
		t.Fatalf("ContentChanged: %v", err)
	}
	if changed {
		t.Error("identical file should not report changed")
	}
}

func TestContentChanged_DifferentFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "diff.txt")
	content := []byte("actual content")
	os.WriteFile(path, content, 0o644)

	changed, err := ContentChanged(path, HashContent([]byte("different")))
	if err != nil {
		t.Fatalf("ContentChanged: %v", err)
	}
	if !changed {
		t.Error("different content should report changed")
	}
}

// --- Risk evaluation edge cases ---------------------------------

func TestEvaluateRisk_NilDestination(t *testing.T) {
	learning := &domain.Learning{}
	curation := &domain.Curation{Destination: nil}

	risk := evaluateRisk(learning, curation)
	if risk != domain.RiskLow {
		t.Errorf("nil destination should be low risk, got %q", risk)
	}
}

func TestEvaluateRisk_DestNone(t *testing.T) {
	learning := &domain.Learning{}
	curation := &domain.Curation{
		Destination: &domain.Destination{Type: domain.DestNone},
	}

	risk := evaluateRisk(learning, curation)
	if risk != domain.RiskLow {
		t.Errorf("dest none should be low risk, got %q", risk)
	}
}

// --- ValidateSkill edge cases -----------------------------------

func TestValidateSkill_Empty(t *testing.T) {
	result := ValidateSkill([]byte(""))
	if result.Valid {
		t.Error("empty content should be invalid")
	}
	if result.HasFrontmatter {
		t.Error("empty content should not have frontmatter")
	}
}

func TestValidateSkill_MissingDescription(t *testing.T) {
	content := "---\nname: test\n---\n\n# Test"
	result := ValidateSkill([]byte(content))
	if result.Valid {
		t.Error("missing description should be invalid")
	}
}

func TestBuildSkillContent_EmptyProcedure(t *testing.T) {
	result := BuildSkillContent("Title", "Desc", "Lesson", "")
	if strings.Contains(result, "## Procedure") {
		t.Error("empty procedure should not include Procedure section")
	}
}

// --- Diff edge cases ---------------------------------------------

func TestGenerateDiff_ShorterFile(t *testing.T) {
	current := []byte("line1\nline2\nline3\n")
	proposed := []byte("line1\n")

	diff := GenerateDiff(current, proposed, "test.md", true)
	if !strings.Contains(diff, "-line2") || !strings.Contains(diff, "-line3") {
		t.Errorf("diff should show removals:\n%s", diff)
	}
}

// --- verifyTargets -----------------------------------------------

func TestVerifyTargets_AllExist(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "skills", "test"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "skills", "test", "SKILL.md"), []byte("content"), 0o644)

	targets := []domain.TargetEntry{
		{Root: "skills", Path: "test/SKILL.md"},
	}
	results := verifyTargets(tmpDir, targets, "content")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Pass {
		t.Errorf("existing file should pass verification: %+v", results[0])
	}
}

func TestVerifyTargets_FileMissing(t *testing.T) {
	tmpDir := t.TempDir()
	targets := []domain.TargetEntry{
		{Root: ".", Path: "nonexistent.md"},
	}
	results := verifyTargets(tmpDir, targets, "content")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Pass {
		t.Error("missing file should fail verification")
	}
}

// --- rollbackAll -------------------------------------------------

func TestRollbackAll_Success(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	// Create file to back up.
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0o755)
	srcPath := filepath.Join(tmpDir, "src", "file.txt")
	os.WriteFile(srcPath, []byte("original"), 0o644)

	mgr := NewBackupManager(tmpDir, backupDir)
	entry, _ := mgr.BackupFile(filepath.Join("src", "file.txt"))

	// Delete original.
	os.Remove(srcPath)

	err := rollbackAll(mgr, []BackupEntry{*entry})
	if err != nil {
		t.Fatalf("rollbackAll: %v", err)
	}

	// Verify file is restored.
	if _, err := os.Stat(srcPath); err != nil {
		t.Fatalf("rollback should restore file: %v", err)
	}
}

func TestRollbackAll_NonexistentBackup(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	mgr := NewBackupManager(tmpDir, backupDir)

	// BackupEntry with empty BackupPath (file didn't exist before publish).
	entry := BackupEntry{
		OriginalPath: "nonexistent.txt",
		BackupPath:   "",
		Checksum:     "",
	}
	err := rollbackAll(mgr, []BackupEntry{entry})
	if err != nil {
		t.Fatalf("rollbackAll with empty backup: %v", err)
	}
}

// --- RollbackFromBackup ------------------------------------------

func TestRollbackFromBackup(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	os.MkdirAll(filepath.Join(tmpDir, "src"), 0o755)
	srcPath := filepath.Join(tmpDir, "src", "file.txt")
	os.WriteFile(srcPath, []byte("v1"), 0o644)

	mgr := NewBackupManager(tmpDir, backupDir)
	entry, _ := mgr.BackupFile(filepath.Join("src", "file.txt"))

	// Modify.
	os.WriteFile(srcPath, []byte("v2"), 0o644)

	rollbackEntries := []domain.RollbackEntry{
		{Path: filepath.Join("src", "file.txt"), Backup: entry.BackupPath},
	}
	results := RollbackFromBackup(mgr, rollbackEntries)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Errorf("rollback should succeed: %+v", results[0])
	}

	content, _ := os.ReadFile(srcPath)
	if string(content) != "v1" {
		t.Errorf("rollback content = %q, want v1", string(content))
	}
}

// --- resolveProjectTarget / resolveSharedTarget -----------------

func TestResolveTarget_Project(t *testing.T) {
	tmpDir := t.TempDir()
	curation := &domain.Curation{
		Decision: domain.CurationApproveProjectKnowledge,
		Destination: &domain.Destination{
			Type: domain.DestProject,
			Root: ".",
			Path: "config.md",
		},
	}

	targets, err := ResolveTarget(tmpDir, curation)
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Operation != domain.OpReplace {
		t.Errorf("project target should use OpReplace, got %q", targets[0].Operation)
	}
	if targets[0].IsManaged {
		t.Error("project target should not be managed")
	}
}

func TestResolveTarget_Shared(t *testing.T) {
	tmpDir := t.TempDir()
	curation := &domain.Curation{
		Decision: domain.CurationApproveSharedKnowledge,
		Destination: &domain.Destination{
			Type: domain.DestShared,
			Root: "shared",
			Path: "rules.md",
		},
	}

	targets, err := ResolveTarget(tmpDir, curation)
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if !targets[0].IsManaged {
		t.Error("shared target should be managed")
	}
}

func TestResolveTarget_UnknownType(t *testing.T) {
	tmpDir := t.TempDir()
	curation := &domain.Curation{
		Decision: domain.CurationApproveProjectKnowledge,
		Destination: &domain.Destination{
			Type: "unknown_type",
			Root: ".",
			Path: "test.md",
		},
	}

	_, err := ResolveTarget(tmpDir, curation)
	if err == nil {
		t.Fatal("expected error for unknown destination type")
	}
}

// --- CheckDirtyWorktree with git repo ----------------------------

func TestCheckDirty_InGitRepo_Clean(t *testing.T) {
	// Check if git is available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()

	// Initialize a real git repository.
	init := exec.Command("git", "init")
	init.Dir = tmpDir
	if out, err := init.CombinedOutput(); err != nil {
		t.Skipf("git init failed: %v\n%s", err, out)
	}

	// Commit something so the repo has a HEAD.
	readme := filepath.Join(tmpDir, "README.md")
	os.WriteFile(readme, []byte("# test"), 0o644)
	exec.Command("git", "-C", tmpDir, "add", "README.md").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.name", "Test").Run()
	exec.Command("git", "-C", tmpDir, "commit", "-m", "init").Run()

	// No targets means no dirty files.
	targets := []TargetResolution{}
	result, err := CheckDirtyWorktree(tmpDir, targets)
	if err != nil {
		t.Fatalf("CheckDirtyWorktree: %v", err)
	}
	if result.IsDirty {
		t.Error("clean repo with empty targets should not be dirty")
	}
}

// --- utcNowPublish -----------------------------------------------

func TestUTCNowPublish(t *testing.T) {
	t1 := utcNowPublish()
	t2 := utcNowPublish()
	if t2.Before(t1) {
		t.Error("time should not go backwards")
	}
	// Should be truncated to millisecond.
	if t1.Nanosecond()%1e6 != 0 {
		t.Errorf("should be truncated to millisecond, got %v", t1)
	}
}

// --- Target resolution edge cases ---------------------------------

func TestResolveTarget_NilCuration(t *testing.T) {
	_, err := ResolveTarget("/tmp", nil)
	if err == nil {
		t.Fatal("expected error for nil curation")
	}
}

func TestResolveTarget_SkillWithExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "skills", "existing-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("existing"), 0o644)

	curation := &domain.Curation{
		Decision: domain.CurationApproveSkillUpdate,
		Destination: &domain.Destination{
			Type:     domain.DestSkill,
			Root:     "skills",
			Path:     "existing-skill/SKILL.md",
			Required: true,
		},
	}

	targets, err := ResolveTarget(tmpDir, curation)
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if !targets[0].Exists {
		t.Error("existing file should be detected")
	}
	if targets[0].Operation != domain.OpReplaceManagedBlock {
		t.Errorf("existing skill should use OpReplaceManagedBlock, got %q", targets[0].Operation)
	}
}
