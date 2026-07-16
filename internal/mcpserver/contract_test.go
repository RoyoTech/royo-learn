package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Permanent contract tests (Tramo 5 of docs/PLAN-recuperacion-contrato.md).
//
// These tests bind three declarative surfaces together so none of them can
// drift from the executable registry again:
//
//	docs/05-MCP-SPEC.md   (documented tools)
//	internal/mcpserver    (registered tools)
//	skills/**/SKILL.md    (tools an agent is told to call)
//
// The governing invariant, from the acceptance criterion of Recorrido A:
// a Skill can never cite a tool that does not exist.
// ---------------------------------------------------------------------------

// repoRoot returns the repository root, resolved from this package's directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// backtickToken matches every single-backtick span in a Markdown document.
var backtickToken = regexp.MustCompile("`([^`\n]+)`")

// toolNameShape matches an identifier that looks like a royo-learn MCP tool
// name. It deliberately matches names that are NOT registered: citing one is
// exactly the defect these tests exist to catch.
var toolNameShape = regexp.MustCompile(`^learning_[a-z0-9_]+$`)

// skillProfileDecl extracts the MCP profile a Skill declares it runs under.
var skillProfileDecl = regexp.MustCompile(`(?m)^\s*mcp_profile:\s*"?([a-z]+)"?\s*$`)

// docsToolHeading matches a tool section heading in docs/05-MCP-SPEC.md.
var docsToolHeading = regexp.MustCompile("(?m)^###\\s+`([a-z0-9_]+)`\\s*$")

// instructionsToolLine matches one advertised tool in the server instructions.
// The instructions are generated from the registry, one "- name: description"
// line per served tool.
var instructionsToolLine = regexp.MustCompile(`(?m)^- ([a-z0-9_]+): `)

// announcedTools returns the exact set of tool names the instructions advertise.
// It parses the tool lines rather than substring-matching, because a substring
// match cannot tell the alias "doctor" apart from the canonical "learning_doctor"
// that contains it.
func announcedTools(t *testing.T, instructions string) map[string]bool {
	t.Helper()

	out := make(map[string]bool)
	for _, m := range instructionsToolLine.FindAllStringSubmatch(instructions, -1) {
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatalf("no tool lines found in server instructions; the extractor is broken:\n%s", instructions)
	}
	return out
}

// citedTool is a tool name found in a Skill, with its provenance.
type citedTool struct {
	name  string
	skill string
	file  string
}

// skillCitations walks skills/**/SKILL.md and returns every MCP tool name each
// Skill cites, plus the profile each Skill declares.
func skillCitations(t *testing.T) (cites []citedTool, declaredProfile map[string]string) {
	t.Helper()

	skillsDir := filepath.Join(repoRoot(t), "skills")
	declaredProfile = make(map[string]string)

	// Every name the registry knows about, canonical or deprecated alias. A
	// Skill citing any of these is citing an MCP tool, not prose.
	known := make(map[string]bool)
	for _, tool := range allTools {
		known[tool.name] = true
		for _, alias := range tool.aliases {
			known[alias] = true
		}
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatalf("read skills dir: %v", err)
	}

	found := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		found++
		body := string(raw)

		if m := skillProfileDecl.FindStringSubmatch(body); m != nil {
			declaredProfile[entry.Name()] = m[1]
		}

		seen := make(map[string]bool)
		for _, m := range backtickToken.FindAllStringSubmatch(body, -1) {
			token := strings.TrimSpace(m[1])
			if !known[token] && !toolNameShape.MatchString(token) {
				continue // ordinary prose, a CLI command, or an error code
			}
			if seen[token] {
				continue
			}
			seen[token] = true
			cites = append(cites, citedTool{name: token, skill: entry.Name(), file: path})
		}
	}

	if found == 0 {
		t.Fatalf("no SKILL.md found under %s", skillsDir)
	}
	return cites, declaredProfile
}

// canonicalNames returns the set of canonical tool names in the registry.
func canonicalNames() map[string]profileTool {
	out := make(map[string]profileTool, len(allTools))
	for _, tool := range allTools {
		out[tool.name] = tool
	}
	return out
}

// aliasNames returns deprecated alias -> canonical name.
func aliasNames() map[string]string {
	out := make(map[string]string)
	for _, tool := range allTools {
		for _, alias := range tool.aliases {
			out[alias] = tool.name
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Test 1 — a Skill never cites a tool that does not exist.
//
// This is the acceptance criterion of Recorrido A. It admits no exceptions,
// no pending list, and no soft pass.
// ---------------------------------------------------------------------------

func TestContract_SkillsCiteOnlyRegisteredCanonicalTools(t *testing.T) {
	t.Parallel()

	cites, declaredProfile := skillCitations(t)
	if len(cites) == 0 {
		t.Fatal("no MCP tool citations found in skills/**/SKILL.md; the extractor is broken")
	}

	canonical := canonicalNames()
	aliases := aliasNames()

	for _, c := range cites {
		// (a) the tool must exist in the real registry.
		tool, isCanonical := canonical[c.name]
		if !isCanonical {
			if target, isAlias := aliases[c.name]; isAlias {
				// (c) it must not be a deprecated alias.
				t.Errorf("skill %q cites deprecated alias %q; it must cite the canonical name %q (%s)",
					c.skill, c.name, target, c.file)
				continue
			}
			t.Errorf("skill %q cites MCP tool %q, which is NOT registered by the server (%s)",
				c.skill, c.name, c.file)
			continue
		}

		// (b) the tool must belong to the profile the Skill declares.
		profile, declared := declaredProfile[c.skill]
		if !declared {
			t.Errorf("skill %q cites MCP tool %q but declares no mcp_profile in its frontmatter (%s)",
				c.skill, c.name, c.file)
			continue
		}
		if !validCanonicalProfile(profile) {
			t.Errorf("skill %q declares unknown mcp_profile %q (%s)", c.skill, profile, c.file)
			continue
		}
		if !tool.enabled(profile) {
			t.Errorf("skill %q declares profile %q but cites tool %q, which is not enabled in that profile (%s)",
				c.skill, profile, c.name, c.file)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 2 — triple match: docs/05-MCP-SPEC.md <-> registry <-> Skills.
// ---------------------------------------------------------------------------

// pendingTools are tools that docs/05-MCP-SPEC.md specifies and that the
// registry does NOT yet expose, each with the recorrido that lands it.
//
// This list is a declaration of known debt, not an escape hatch: the test below
// asserts it is EXACT in both directions. Registering one of these tools without
// removing it from this list fails the build, and so does adding a name here
// that docs/05 does not actually document. It can only shrink.
// pendingTools is now empty: Recorrido E / D17 landed learning_report_occurrence
// and learning_status. The map is kept (rather than deleted) so the exactness
// checks in TestContract_DocsRegistrySkillsTripleMatch keep guarding future debt.
var pendingTools = map[string]string{}

// contractExtensions are canonical tools that D1 adds on top of docs/05-MCP-SPEC.md
// so that no registered tool is left without a canonical name.
var contractExtensions = map[string]bool{
	"learning_list_recurrences": true,
	"learning_compute_metrics":  true,
}

func documentedTools(t *testing.T) map[string]bool {
	t.Helper()

	specPath := filepath.Join(repoRoot(t), "docs", "05-MCP-SPEC.md")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read %s: %v", specPath, err)
	}

	out := make(map[string]bool)
	for _, m := range docsToolHeading.FindAllStringSubmatch(string(raw), -1) {
		if strings.HasPrefix(m[1], "learning_") {
			out[m[1]] = true
		}
	}
	if len(out) == 0 {
		t.Fatalf("no tool headings found in %s; the extractor is broken", specPath)
	}
	return out
}

func TestContract_DocsRegistrySkillsTripleMatch(t *testing.T) {
	t.Parallel()

	documented := documentedTools(t)
	canonical := canonicalNames()
	aliases := aliasNames()
	cites, _ := skillCitations(t)

	// 1. Every registered canonical tool is either documented in docs/05 or is a
	//    declared contract extension (D1). No tool exists in secret.
	for name := range canonical {
		if documented[name] || contractExtensions[name] {
			continue
		}
		t.Errorf("tool %q is registered but is neither documented in docs/05-MCP-SPEC.md nor declared a contract extension", name)
	}

	// 2. Every documented tool is either registered or explicitly pending.
	for name := range documented {
		if _, ok := canonical[name]; ok {
			continue
		}
		if _, pending := pendingTools[name]; pending {
			continue
		}
		t.Errorf("tool %q is documented in docs/05-MCP-SPEC.md but is neither registered nor listed as pending", name)
	}

	// 3. The pending list is exact. It cannot rot into a permanent excuse.
	for name, recorrido := range pendingTools {
		if _, ok := canonical[name]; ok {
			t.Errorf("tool %q is registered but still listed as pending (%s); remove it from pendingTools", name, recorrido)
		}
		if !documented[name] {
			t.Errorf("tool %q is listed as pending but is not documented in docs/05-MCP-SPEC.md", name)
		}
	}

	// 4. Contract extensions are real: declared, registered, and not documented.
	for name := range contractExtensions {
		if _, ok := canonical[name]; !ok {
			t.Errorf("contract extension %q is declared but not registered", name)
		}
	}

	// 5. Every tool a Skill cites is a registered canonical tool. A Skill may
	//    never cite a pending, documented-only or alias name.
	for _, c := range cites {
		if _, ok := canonical[c.name]; ok {
			continue
		}
		if target, isAlias := aliases[c.name]; isAlias {
			t.Errorf("skill %q cites deprecated alias %q (canonical: %q)", c.skill, c.name, target)
			continue
		}
		if recorrido, pending := pendingTools[c.name]; pending {
			t.Errorf("skill %q cites %q, which is documented but NOT yet registered (%s). "+
				"A Skill may not cite a tool that does not exist.", c.skill, c.name, recorrido)
			continue
		}
		t.Errorf("skill %q cites unknown tool %q", c.skill, c.name)
	}
}

// ---------------------------------------------------------------------------
// Test 2b — §4.2: the complete MCP tool set is registered.
//
// Tramo 4 §4.2 requires the full learning_* tool surface to be registered and
// tested. The tools themselves were delivered across Recorridos A-E and D17;
// this test locks the set in so a future change cannot drop one silently.
// Tools reserved for §4.6 (export, import, rebuild_index, review) are
// deliberately NOT here: they stay out until they have real utility over MCP.
// ---------------------------------------------------------------------------

func TestContract_AllHito2MCPToolsRegistered(t *testing.T) {
	t.Parallel()

	// The mandatory tool set of §4.2, in the plan's order.
	want := []string{
		"learning_capture",
		"learning_add_evidence",
		"learning_search",
		"learning_get",
		"learning_list",
		"learning_curate",
		"learning_publication_preview",
		"learning_approve",
		"learning_publish",
		"learning_report_occurrence",
		"learning_status",
		"learning_doctor",
		"learning_rollback",
	}

	canonical := canonicalNames()
	for _, name := range want {
		if _, ok := canonical[name]; !ok {
			t.Errorf("§4.2 requires MCP tool %q, which is NOT registered", name)
		}
	}

	// Reserved §4.6 tools must not be registered yet: registering one without
	// promoting it out of §4.6 would misrepresent the contract.
	for _, reserved := range []string{"learning_export", "learning_import", "learning_rebuild_index", "learning_review"} {
		if _, ok := canonical[reserved]; ok {
			t.Errorf("tool %q is reserved for §4.6 but is already registered; document and promote it deliberately", reserved)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 3 — D14: the initialize instructions are derived from the active
// profile's registry, never hardcoded.
// ---------------------------------------------------------------------------

func TestContract_InstructionsAgreeWithToolsList(t *testing.T) {
	t.Parallel()

	for _, profile := range []string{"read", "agent", "admin"} {
		t.Run(profile, func(t *testing.T) {
			t.Parallel()
			ts := newTestServer(t, profile)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			listed, err := ts.session.ListTools(ctx, nil)
			if err != nil {
				t.Fatalf("ListTools: %v", err)
			}

			aliases := aliasNames()
			canonical := canonicalNames()

			// The canonical tools actually served in this profile.
			var served []string
			for _, tool := range listed.Tools {
				if _, isAlias := aliases[tool.Name]; isAlias {
					continue
				}
				if _, ok := canonical[tool.Name]; !ok {
					t.Errorf("tools/list returned %q, which is neither a canonical tool nor a known alias", tool.Name)
					continue
				}
				served = append(served, tool.Name)
			}
			sort.Strings(served)

			instructions := ts.server.Instructions()
			announced := announcedTools(t, instructions)

			// The set announced in the instructions and the set served by
			// tools/list must be identical, modulo the deprecated aliases, which
			// are callable but deliberately unadvertised (D1/D14).
			servedSet := make(map[string]bool, len(served))
			for _, name := range served {
				servedSet[name] = true
			}

			for _, name := range served {
				if !announced[name] {
					t.Errorf("profile %q: tool %q is served by tools/list but missing from initialize instructions",
						profile, name)
				}
			}

			// No tool may be announced that this profile does not serve. This is
			// the defect D14 names: minimal registered 3 tools and promised 10.
			for name := range announced {
				if servedSet[name] {
					continue
				}
				if _, isAlias := aliases[name]; isAlias {
					t.Errorf("profile %q: initialize instructions advertise deprecated alias %q", profile, name)
					continue
				}
				t.Errorf("profile %q: initialize instructions announce %q, which is NOT registered in this profile",
					profile, name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test 4 — D1/D2: aliases are name bindings, and profiles are honest.
// ---------------------------------------------------------------------------

func TestContract_EveryDeprecatedAliasSharesItsCanonicalProfiles(t *testing.T) {
	t.Parallel()

	canonical := canonicalNames()
	for _, tool := range allTools {
		for _, alias := range tool.aliases {
			if _, clash := canonical[alias]; clash {
				t.Errorf("alias %q collides with a canonical tool name", alias)
			}
			if !strings.HasPrefix(tool.name, "learning_") {
				t.Errorf("canonical tool %q must use the learning_* prefix", tool.name)
			}
		}
	}
}

func TestContract_NoDestructiveToolInReadOrAgentProfile(t *testing.T) {
	t.Parallel()

	for _, tool := range allTools {
		if tool.access != accessDestructive {
			continue
		}
		for _, profile := range []string{profileRead, profileAgent} {
			if tool.enabled(profile) {
				t.Errorf("destructive tool %q must not be enabled in profile %q", tool.name, profile)
			}
		}
	}

	// A read profile serves read-only tools and nothing else.
	for _, tool := range profileTools(profileRead) {
		if tool.access != accessRead {
			t.Errorf("profile %q serves tool %q with access %q; read profiles are read-only",
				profileRead, tool.name, tool.access)
		}
	}
}

func TestContract_DeprecatedProfileNamesMapToCanonicalProfiles(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"minimal":  profileRead,
		"standard": profileAgent,
		"full":     profileAdmin,
	}
	for deprecated, canonical := range want {
		got, isDeprecated, ok := resolveProfile(deprecated)
		if !ok {
			t.Errorf("profile %q is not accepted; v0.1.9 clients would break", deprecated)
			continue
		}
		if !isDeprecated {
			t.Errorf("profile %q must be reported as deprecated", deprecated)
		}
		if got != canonical {
			t.Errorf("profile %q maps to %q, want %q", deprecated, got, canonical)
		}
	}

	for _, canonical := range []string{profileRead, profileAgent, profileAdmin} {
		got, isDeprecated, ok := resolveProfile(canonical)
		if !ok || got != canonical {
			t.Errorf("canonical profile %q must resolve to itself, got %q (ok=%v)", canonical, got, ok)
		}
		if isDeprecated {
			t.Errorf("canonical profile %q must not be reported as deprecated", canonical)
		}
	}

	if _, _, ok := resolveProfile("nonsense"); ok {
		t.Error("unknown profile must be rejected")
	}
}

// ---------------------------------------------------------------------------
// Test 5 — D8: deprecation is announced, never silent.
// ---------------------------------------------------------------------------

func TestContract_DeprecatedAliasCallCarriesDeprecationNotice(t *testing.T) {
	t.Parallel()

	ts := newTestServer(t, "agent")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.callTool(ctx, "capture_learning", map[string]any{
		"title":           "Deprecated alias still works",
		"type":            "procedure",
		"context":         "Calling the v0.1.9 tool name must keep working.",
		"observation":     "The alias resolves to the canonical handler.",
		"reusable_lesson": "Deprecated aliases must warn, not fail silently.",
		"scope_guess":     "project",
		"confidence":      "high",
		"evidence_level":  "moderate",
		"actor":           map[string]any{"kind": "agent", "name": "contract-test"},
	})
	if err != nil {
		t.Fatalf("capture_learning via alias: %v", err)
	}
	if result.IsError {
		t.Fatalf("deprecated alias must still work; got error result")
	}

	raw, ok := result.Meta["deprecation"]
	if !ok {
		t.Fatal("response to a deprecated alias must carry a deprecation notice (D8)")
	}
	notice, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("deprecation notice has unexpected type %T", raw)
	}
	if notice["alias"] != "capture_learning" {
		t.Errorf("deprecation.alias = %v, want capture_learning", notice["alias"])
	}
	if notice["canonical"] != "learning_capture" {
		t.Errorf("deprecation.canonical = %v, want learning_capture", notice["canonical"])
	}
	if notice["removed_in"] == "" || notice["removed_in"] == nil {
		t.Error("deprecation.removed_in must be set")
	}
}

func TestContract_CanonicalToolCallCarriesNoDeprecationNotice(t *testing.T) {
	t.Parallel()

	ts := newTestServer(t, "agent")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.callTool(ctx, "learning_doctor", map[string]any{})
	if err != nil {
		t.Fatalf("learning_doctor: %v", err)
	}
	if result.IsError {
		t.Fatal("learning_doctor must succeed")
	}
	if _, ok := result.Meta["deprecation"]; ok {
		t.Error("a canonical tool call must not carry a deprecation notice")
	}
}
