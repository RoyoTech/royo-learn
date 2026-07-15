package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file is the Recorrido E end-to-end suite. It replaced a permissive
// predecessor whose steps accepted failure as success ("failure is acceptable",
// "soft pass"), never exercised publish/approve/rollback/occurrence, and treated
// absence of data as a pass. That predecessor is exactly how v0.1.9 shipped green
// while broken.
//
// Every step here asserts a REAL business effect. There are no soft passes: a
// command that exits non-zero when it should succeed (or zero when it should
// fail) fails the scenario loudly, and a scenario aborts on the first failure
// rather than reporting a green step over broken state.
//
// The same scenarios run two ways: the `royo-learn e2e` command (this file) and
// the Go tests in e2e_test.go call the identical scenario functions.

// e2eStepResult holds the result of a single e2e step.
type e2eStepResult struct {
	Step   string `json:"step"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}

// e2eResult is the JSON output for the e2e command.
type e2eResult struct {
	Passed  int             `json:"passed"`
	Failed  int             `json:"failed"`
	Total   int             `json:"total"`
	Steps   []e2eStepResult `json:"steps"`
	Summary string          `json:"summary"`
}

func runE2E(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("e2e", flag.ContinueOnError)
	tempFlag := fs.Bool("temp", false, "run in a temporary directory (required)")
	if err := fs.Parse(args); err != nil {
		_, _ = fmt.Fprintf(stderr, "e2e: %v\n", err)
		return exitFailure
	}
	if !*tempFlag {
		_, _ = fmt.Fprintf(stderr, "e2e: --temp is required\n")
		return exitFailure
	}

	tempDir, err := os.MkdirTemp("", "royo-learn-e2e-")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "e2e: cannot create temp dir: %v\n", err)
		return exitFailure
	}
	defer os.RemoveAll(tempDir)

	var steps []e2eStepResult
	steps = append(steps, runCLISensitiveScenario(filepath.Join(tempDir, "cli-sensitive"))...)
	steps = append(steps, runCLILowImpactScenario(filepath.Join(tempDir, "cli-lowimpact"))...)

	// MCP scenario over a real stdio subprocess (the command path). The Go-test
	// path drives the identical scenario over an in-memory MCP session.
	steps = append(steps, runMCPScenarioStdio(filepath.Join(tempDir, "mcp-sensitive"), stderr)...)

	result := tallySteps(steps)
	data, _ := json.MarshalIndent(result, "", "  ")
	_, _ = fmt.Fprintf(stdout, "%s\n", string(data))

	if result.Failed > 0 {
		return exitFailure
	}
	return exitSuccess
}

func tallySteps(steps []e2eStepResult) *e2eResult {
	result := &e2eResult{Steps: steps, Total: len(steps)}
	for i := range steps {
		if steps[i].Passed {
			result.Passed++
		} else {
			result.Failed++
		}
	}
	if result.Failed == 0 {
		result.Summary = fmt.Sprintf("All %d e2e steps passed", result.Total)
	} else {
		result.Summary = fmt.Sprintf("%d/%d steps passed, %d failed", result.Passed, result.Total, result.Failed)
	}
	return result
}

// ---------------------------------------------------------------------------
// scenario runner
// ---------------------------------------------------------------------------

// scenario accumulates step results and shared state. A step that fails aborts
// the scenario: dependent steps are never run, and their absence is never
// reported as success.
type scenario struct {
	name  string
	root  string
	steps []e2eStepResult

	learningID    string
	previewHash   string
	approvalID    string
	publicationID string
	agentsBefore  []byte
}

// step runs fn, records the outcome, and returns whether it passed. Callers must
// stop the scenario when it returns false.
func (s *scenario) step(name string, fn func() error) bool {
	err := fn()
	res := e2eStepResult{Step: s.name + "/" + name, Passed: err == nil}
	if err != nil {
		res.Error = err.Error()
	} else {
		res.Detail = name + " asserted its effect"
	}
	s.steps = append(s.steps, res)
	return err == nil
}

// cliJSON runs a CLI command that must succeed and returns its decoded JSON.
func cliJSON(root string, args ...string) (map[string]any, error) {
	full := append(args, "--project-root", root, "--json")
	var out, errb bytes.Buffer
	if code := run(full, &out, &errb); code != exitSuccess {
		return nil, fmt.Errorf("%v exited %d: %s", args, code, strings.TrimSpace(errb.String()))
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		return nil, fmt.Errorf("%v output not a JSON object: %v (%s)", args, err, out.String())
	}
	return m, nil
}

// cliJSONArray runs a CLI command that must succeed and returns a JSON array.
func cliJSONArray(root string, args ...string) ([]map[string]any, error) {
	full := append(args, "--project-root", root, "--json")
	var out, errb bytes.Buffer
	if code := run(full, &out, &errb); code != exitSuccess {
		return nil, fmt.Errorf("%v exited %d: %s", args, code, strings.TrimSpace(errb.String()))
	}
	var a []map[string]any
	if err := json.Unmarshal(out.Bytes(), &a); err != nil {
		return nil, fmt.Errorf("%v output not a JSON array: %v (%s)", args, err, out.String())
	}
	return a, nil
}

// cliMustFail runs a CLI command that MUST exit non-zero, returning its stderr.
func cliMustFail(root string, args ...string) (string, error) {
	full := append(args, "--project-root", root, "--json")
	var out, errb bytes.Buffer
	code := run(full, &out, &errb)
	if code == exitSuccess {
		return "", fmt.Errorf("%v was expected to fail but exited 0: %s", args, out.String())
	}
	return errb.String(), nil
}

// ---------------------------------------------------------------------------
// CLI scenario 1 — sensitive publication (AGENTS.md, approval REQUIRED)
//
// This scenario proves the approval gate FIRES: a non-preference learning routed
// to AGENTS.md cannot be published without an explicit human approval bound to
// the exact preview hash, and a rollback restores the file byte for byte.
// ---------------------------------------------------------------------------

func runCLISensitiveScenario(root string) []e2eStepResult {
	s := &scenario{name: "cli-sensitive", root: root}
	agentsPath := filepath.Join(root, "AGENTS.md")
	const marker = "E2E-SENSITIVE-LESSON: AGENTS.md must always require human approval"

	// 1. Create a temporary Git repo with a committed AGENTS.md, so the publish
	//    exercises the real dirty-worktree path and rollback has a baseline.
	if !s.step("create-git-repo", func() error {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return err
		}
		s.agentsBefore = []byte("# AGENTS\n\nOriginal project rules.\n")
		if err := os.WriteFile(agentsPath, s.agentsBefore, 0o644); err != nil {
			return err
		}
		return gitInitCommit(root)
	}) {
		return s.steps
	}

	// 2. init
	if !s.step("init", func() error {
		var b bytes.Buffer
		if code := run([]string{"init", "--project-root", root}, &b, &b); code != exitSuccess {
			return fmt.Errorf("init exited %d: %s", code, b.String())
		}
		if _, err := os.Stat(filepath.Join(root, ".royo-learn", "config.yaml")); err != nil {
			return fmt.Errorf("config.yaml missing after init: %v", err)
		}
		return nil
	}) {
		return s.steps
	}

	// 3. doctor — the system must be healthy on a fresh project.
	if !s.step("doctor", func() error {
		m, err := cliJSON(root, "doctor")
		if err != nil {
			return err
		}
		if ok, _ := m["ok"].(bool); !ok {
			return fmt.Errorf("doctor reports ok=false: %v", m)
		}
		return nil
	}) {
		return s.steps
	}

	// 4. capture WITH evidence, proposing the sensitive AGENTS.md destination.
	if !s.step("capture-with-evidence", func() error {
		m, err := cliJSON(root,
			"capture",
			"--title", "Sensitive rule for AGENTS.md",
			"--context", "Recorrido E proves the approval gate fires",
			"--observation", "A non-preference learning routed to AGENTS.md",
			"--lesson", marker,
			"--type", "diagnostic",
			"--destination", "agents_rule",
			"--evidence-level", "moderate",
			"--evidence-summary", "The failing test reproduced the governance hole",
			"--evidence-content", "--- PASS: TestApprovalGate",
			"--evidence-kind", "test",
		)
		if err != nil {
			return err
		}
		id, _ := m["learning_id"].(string)
		if id == "" {
			return fmt.Errorf("capture returned no learning_id")
		}
		if got, _ := m["status"].(string); got != "captured" {
			return fmt.Errorf("status = %q, want captured", got)
		}
		if cnt, _ := m["evidence_count"].(float64); cnt < 1 {
			return fmt.Errorf("evidence_count = %v, want >= 1", m["evidence_count"])
		}
		s.learningID = id
		return nil
	}) {
		return s.steps
	}

	// 5. get — the learning is retrievable by ID with the captured content.
	if !s.step("get", func() error {
		m, err := cliJSON(root, "get", s.learningID)
		if err != nil {
			return err
		}
		if m["id"] != s.learningID {
			return fmt.Errorf("get returned id %v, want %q", m["id"], s.learningID)
		}
		if m["status"] != "captured" {
			return fmt.Errorf("get status = %v, want captured", m["status"])
		}
		if m["title"] != "Sensitive rule for AGENTS.md" {
			return fmt.Errorf("get title = %v, want the captured title", m["title"])
		}
		return nil
	}) {
		return s.steps
	}

	// 6. search — the learning is findable by full-text query.
	if !s.step("search", func() error {
		results, err := cliJSONArray(root, "search", "AGENTS")
		if err != nil {
			return err
		}
		for _, r := range results {
			if r["id"] == s.learningID {
				if r["source"] != "royo_learn" {
					return fmt.Errorf("result source = %v, want royo_learn", r["source"])
				}
				return nil
			}
		}
		return fmt.Errorf("search did not return the captured learning %q", s.learningID)
	}) {
		return s.steps
	}

	// 7. curate — approve for the AGENTS.md destination; status becomes approved.
	if !s.step("curate", func() error {
		m, err := cliJSON(root, "curate",
			"--learning-id", s.learningID,
			"--action", "approve_agents_rule",
			"--rationale", "The evidence proves this rule belongs in the shared governance surface",
		)
		if err != nil {
			return err
		}
		if got, _ := m["new_status"].(string); got != "approved" {
			return fmt.Errorf("new_status = %q, want approved", got)
		}
		return nil
	}) {
		return s.steps
	}

	// 8. preview — a sensitive destination must report requires_approval=true.
	if !s.step("preview", func() error {
		m, err := cliJSON(root, "preview", "--learning-id", s.learningID)
		if err != nil {
			return err
		}
		if req, _ := m["requires_approval"].(bool); !req {
			return fmt.Errorf("preview requires_approval = false; the AGENTS.md gate does not fire")
		}
		hash, _ := m["preview_hash"].(string)
		if hash == "" {
			return fmt.Errorf("preview returned no preview_hash")
		}
		s.previewHash = hash
		return nil
	}) {
		return s.steps
	}

	// 9. publish WITHOUT approval — MUST be refused with approval_required.
	if !s.step("publish-without-approval-refused", func() error {
		stderr, err := cliMustFail(root, "publish",
			"--learning-id", s.learningID,
			"--preview-hash", s.previewHash,
			"--apply",
		)
		if err != nil {
			return err
		}
		if !strings.Contains(stderr, "approval_required") {
			return fmt.Errorf("refusal did not cite approval_required: %s", stderr)
		}
		// The file must NOT have changed: the write never happened.
		cur, _ := os.ReadFile(agentsPath)
		if !bytes.Equal(cur, s.agentsBefore) {
			return fmt.Errorf("AGENTS.md changed despite a refused publish")
		}
		return nil
	}) {
		return s.steps
	}

	// 10. approve — bind an approval to the exact preview hash.
	if !s.step("approve", func() error {
		m, err := cliJSON(root, "approve", s.learningID,
			"--preview-hash", s.previewHash,
			"--approved-by", "e2e-human",
			"--reason", "Recorrido E authorizes this exact preview",
			"--approval-evidence", "session://recorrido-e",
		)
		if err != nil {
			return err
		}
		id, _ := m["approval_id"].(string)
		if id == "" {
			return fmt.Errorf("approve returned no approval_id")
		}
		s.approvalID = id
		return nil
	}) {
		return s.steps
	}

	// 11. publish WITH --apply + --preview-hash + --approval-id — success.
	if !s.step("publish-apply", func() error {
		m, err := cliJSON(root, "publish",
			"--learning-id", s.learningID,
			"--preview-hash", s.previewHash,
			"--approval-id", s.approvalID,
			"--apply",
		)
		if err != nil {
			return err
		}
		pubID, _ := m["publication_id"].(string)
		if pubID == "" {
			return fmt.Errorf("publish returned no publication_id")
		}
		if st, _ := m["status"].(string); st != "completed" {
			return fmt.Errorf("publication status = %q, want completed", st)
		}
		s.publicationID = pubID
		return nil
	}) {
		return s.steps
	}

	// 12. verify the file was actually written.
	if !s.step("verify-file-written", func() error {
		cur, err := os.ReadFile(agentsPath)
		if err != nil {
			return fmt.Errorf("AGENTS.md unreadable after publish: %v", err)
		}
		if bytes.Equal(cur, s.agentsBefore) {
			return fmt.Errorf("AGENTS.md was not modified by publish")
		}
		if !strings.Contains(string(cur), marker) {
			return fmt.Errorf("AGENTS.md does not contain the published lesson")
		}
		return nil
	}) {
		return s.steps
	}

	// 13. verify status == published (through the public get command).
	if !s.step("verify-status-published", func() error {
		m, err := cliJSON(root, "get", s.learningID)
		if err != nil {
			return err
		}
		if m["status"] != "published" {
			return fmt.Errorf("learning status = %v, want published", m["status"])
		}
		return nil
	}) {
		return s.steps
	}

	// 14. report occurrence — a real, countable recurrence.
	if !s.step("report-occurrence", func() error {
		m, err := cliJSON(root, "occurrence",
			"--learning-id", s.learningID,
			"--summary", "the same governance gap recurred",
			"--outcome", "prevented",
			"--retrieved", "true",
			"--skill-activated", "true",
			"--idempotency-key", "e2e-occ-1",
		)
		if err != nil {
			return err
		}
		if m["new"] != true {
			return fmt.Errorf("occurrence new = %v, want true", m["new"])
		}
		return nil
	}) {
		return s.steps
	}

	// 15. check metrics — the occurrence is reflected in the count.
	if !s.step("check-metrics", func() error {
		m, err := cliJSON(root, "metrics", "--learning-id", s.learningID)
		if err != nil {
			return err
		}
		cnt, _ := m["count"].(float64)
		if cnt < 1 {
			return fmt.Errorf("metrics count = %v, want >= 1", m["count"])
		}
		return nil
	}) {
		return s.steps
	}

	// 16. rollback — revert the publication.
	if !s.step("rollback", func() error {
		m, err := cliJSON(root, "rollback", "--journal-id", s.publicationID)
		if err != nil {
			return err
		}
		if m["status"] != "rolled_back" {
			return fmt.Errorf("rollback status = %v, want rolled_back", m["status"])
		}
		return nil
	}) {
		return s.steps
	}

	// 17. verify byte-for-byte restoration of AGENTS.md.
	if !s.step("verify-byte-for-byte-restoration", func() error {
		cur, err := os.ReadFile(agentsPath)
		if err != nil {
			return fmt.Errorf("AGENTS.md unreadable after rollback: %v", err)
		}
		if !bytes.Equal(cur, s.agentsBefore) {
			return fmt.Errorf("rollback did not restore AGENTS.md byte for byte:\nwant %q\n got %q",
				s.agentsBefore, cur)
		}
		return nil
	}) {
		return s.steps
	}

	// 18. the occurrence remains listed after rollback (recurrences are durable).
	if !s.step("verify-occurrence-listed", func() error {
		recs, err := cliJSONArray(root, "recurrences", "--learning-id", s.learningID)
		if err != nil {
			return err
		}
		if len(recs) < 1 {
			return fmt.Errorf("recurrences count = %d, want >= 1", len(recs))
		}
		return nil
	}) {
		return s.steps
	}

	// 19. final doctor — the system is still healthy after the full cycle.
	if !s.step("final-doctor", func() error {
		m, err := cliJSON(root, "doctor")
		if err != nil {
			return err
		}
		if ok, _ := m["ok"].(bool); !ok {
			return fmt.Errorf("final doctor reports ok=false: %v", m)
		}
		return nil
	}) {
		return s.steps
	}

	return s.steps
}

// ---------------------------------------------------------------------------
// CLI scenario 2 — low-impact publication (project scope, NO approval)
//
// A separate policy: this proves the approval gate does NOT over-block. A
// project-scope learning must publish with --apply and no approval at all.
// ---------------------------------------------------------------------------

func runCLILowImpactScenario(root string) []e2eStepResult {
	s := &scenario{name: "cli-lowimpact", root: root}

	if !s.step("init", func() error {
		var b bytes.Buffer
		if code := run([]string{"init", "--project-root", root}, &b, &b); code != exitSuccess {
			return fmt.Errorf("init exited %d: %s", code, b.String())
		}
		return nil
	}) {
		return s.steps
	}

	if !s.step("capture-with-evidence", func() error {
		m, err := cliJSON(root,
			"capture",
			"--title", "Low-impact local knowledge",
			"--context", "Recorrido E proves the gate does not over-block",
			"--observation", "A project-scope learning of low impact",
			"--lesson", "E2E-LOWIMPACT-LESSON: local knowledge publishes without a gate",
			"--type", "procedure",
			"--destination", "project",
			"--evidence-level", "moderate",
			"--evidence-summary", "Evidence that unblocks approval",
			"--evidence-content", "--- PASS: TestLowImpact",
			"--evidence-kind", "test",
		)
		if err != nil {
			return err
		}
		s.learningID, _ = m["learning_id"].(string)
		if s.learningID == "" {
			return fmt.Errorf("capture returned no learning_id")
		}
		return nil
	}) {
		return s.steps
	}

	if !s.step("curate-project", func() error {
		m, err := cliJSON(root, "curate",
			"--learning-id", s.learningID,
			"--action", "approve_project_knowledge",
			"--rationale", "The evidence proves this local learning is reusable and safe",
		)
		if err != nil {
			return err
		}
		if got, _ := m["new_status"].(string); got != "approved" {
			return fmt.Errorf("new_status = %q, want approved", got)
		}
		return nil
	}) {
		return s.steps
	}

	if !s.step("preview-not-over-blocked", func() error {
		m, err := cliJSON(root, "preview", "--learning-id", s.learningID)
		if err != nil {
			return err
		}
		if req, _ := m["requires_approval"].(bool); req {
			return fmt.Errorf("a project-scope learning requires_approval=true; the gate over-blocks")
		}
		s.previewHash, _ = m["preview_hash"].(string)
		if s.previewHash == "" {
			return fmt.Errorf("preview returned no preview_hash")
		}
		return nil
	}) {
		return s.steps
	}

	// Publish WITH --apply and NO approval-id — must succeed for low impact.
	if !s.step("publish-apply-without-approval", func() error {
		m, err := cliJSON(root, "publish",
			"--learning-id", s.learningID,
			"--preview-hash", s.previewHash,
			"--apply",
		)
		if err != nil {
			return err
		}
		if st, _ := m["status"].(string); st != "completed" {
			return fmt.Errorf("publication status = %q, want completed", st)
		}
		return nil
	}) {
		return s.steps
	}

	if !s.step("verify-status-published", func() error {
		m, err := cliJSON(root, "get", s.learningID)
		if err != nil {
			return err
		}
		if m["status"] != "published" {
			return fmt.Errorf("learning status = %v, want published", m["status"])
		}
		return nil
	}) {
		return s.steps
	}

	return s.steps
}

// gitInitCommit initialises a git repo at root and commits its current contents.
// Identity is passed inline so no global git config is required.
func gitInitCommit(root string) error {
	cmds := [][]string{
		{"init"},
		{"add", "-A"},
		{"-c", "user.email=e2e@royo.local", "-c", "user.name=e2e", "commit", "-m", "seed"},
	}
	for _, c := range cmds {
		cmd := exec.Command("git", c...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %v: %s", c, err, out)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// MCP scenario — sensitive publication over a real MCP session
// ---------------------------------------------------------------------------

// runMCPScenarioStdio sets up a project, spawns the royo-learn binary as an MCP
// server over stdio, connects a real MCP client, and runs the scenario. It is the
// command path; the Go tests run the identical mcpScenario over an in-memory
// session.
func runMCPScenarioStdio(root string, stderr io.Writer) []e2eStepResult {
	name := "mcp-sensitive"
	fail := func(step string, err error) []e2eStepResult {
		return []e2eStepResult{{Step: name + "/" + step, Passed: false, Error: err.Error()}}
	}

	if err := os.MkdirAll(root, 0o755); err != nil {
		return fail("setup", err)
	}
	var b bytes.Buffer
	if code := run([]string{"init", "--project-root", root}, &b, &b); code != exitSuccess {
		return fail("init", fmt.Errorf("init exited %d: %s", code, b.String()))
	}

	// Under `go test`, os.Executable() is the test binary, which cannot serve
	// MCP over stdio. The e2e Go test builds a real royo-learn binary and points
	// the scenario at it via ROYO_LEARN_E2E_BIN. The standalone `royo-learn e2e`
	// command leaves the variable unset and uses its own executable path.
	exe := os.Getenv("ROYO_LEARN_E2E_BIN")
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			return fail("locate-binary", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, "mcp-serve", "--tools", "admin", "--project-root", root)
	cmd.Stderr = stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "royo-learn-e2e", Version: "v1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return fail("connect", err)
	}
	// Shut the subprocess down before returning so no handle leaks.
	defer func() { _ = session.Close() }()

	return mcpScenario(ctx, session, root)
}

// mcpScenario runs the sensitive publication flow over a connected MCP session.
// It is transport-agnostic: the command path passes a stdio session, the tests
// pass an in-memory session.
func mcpScenario(ctx context.Context, session *mcp.ClientSession, root string) []e2eStepResult {
	s := &scenario{name: "mcp-sensitive", root: root}
	agentsPath := filepath.Join(root, "AGENTS.md")
	const marker = "E2E-MCP-LESSON: AGENTS.md always requires approval over MCP"
	actor := map[string]any{"kind": "agent", "name": "recorrido-e"}

	call := func(tool string, args map[string]any) (map[string]any, *mcp.CallToolResult, error) {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			return nil, nil, err
		}
		if res.IsError {
			return nil, res, nil
		}
		var m map[string]any
		if perr := json.Unmarshal([]byte(mcpText(res)), &m); perr != nil {
			return nil, res, fmt.Errorf("%s: response not a JSON object: %v (%s)", tool, perr, mcpText(res))
		}
		return m, res, nil
	}

	// tools/list — the new tools are present with correct annotations.
	if !s.step("tools-list", func() error {
		listed, err := session.ListTools(ctx, nil)
		if err != nil {
			return err
		}
		names := map[string]*mcp.Tool{}
		for _, t := range listed.Tools {
			names[t.Name] = t
		}
		for _, want := range []string{
			"learning_capture", "learning_get", "learning_search", "learning_curate",
			"learning_publication_preview", "learning_approve", "learning_publish",
			"learning_report_occurrence", "learning_status", "learning_rollback",
		} {
			if names[want] == nil {
				return fmt.Errorf("admin tools/list missing canonical tool %q", want)
			}
		}
		rb := names["learning_rollback"]
		if rb.Annotations == nil || rb.Annotations.DestructiveHint == nil || !*rb.Annotations.DestructiveHint {
			return fmt.Errorf("learning_rollback is not annotated destructive")
		}
		if st := names["learning_status"]; st.Annotations == nil || !st.Annotations.ReadOnlyHint {
			return fmt.Errorf("learning_status is not annotated read-only")
		}
		return nil
	}) {
		return s.steps
	}

	// capture (sensitive, with evidence)
	if !s.step("capture", func() error {
		m, res, err := call("learning_capture", map[string]any{
			"title":                "Sensitive MCP rule for AGENTS.md",
			"type":                 "diagnostic",
			"context":              "Recorrido E MCP scenario",
			"observation":          "A non-preference learning routed to AGENTS.md over MCP",
			"reusable_lesson":      marker,
			"scope_guess":          "project",
			"confidence":           "high",
			"evidence_level":       "moderate",
			"proposed_destination": "agents_rule",
			"actor":                actor,
			"evidence": []map[string]any{{
				"kind":    "test",
				"summary": "Evidence that unblocks approval",
				"source":  "test://recorrido-e",
				"content": "--- PASS: TestApprovalGate",
			}},
		})
		if err != nil {
			return err
		}
		if res != nil && res.IsError {
			return fmt.Errorf("capture errored: %s", mcpText(res))
		}
		s.learningID, _ = m["learning_id"].(string)
		if s.learningID == "" {
			return fmt.Errorf("capture returned no learning_id")
		}
		return nil
	}) {
		return s.steps
	}

	// get
	if !s.step("get", func() error {
		m, res, err := call("learning_get", map[string]any{"learning_id": s.learningID})
		if err != nil {
			return err
		}
		if res != nil && res.IsError {
			return fmt.Errorf("get errored: %s", mcpText(res))
		}
		if m["id"] != s.learningID || m["status"] != "captured" {
			return fmt.Errorf("get mismatch: id=%v status=%v", m["id"], m["status"])
		}
		return nil
	}) {
		return s.steps
	}

	// search
	if !s.step("search", func() error {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "learning_search", Arguments: map[string]any{"query": "AGENTS"}})
		if err != nil {
			return err
		}
		if res.IsError {
			return fmt.Errorf("search errored: %s", mcpText(res))
		}
		var arr []map[string]any
		if perr := json.Unmarshal([]byte(mcpText(res)), &arr); perr != nil {
			return fmt.Errorf("search response not an array: %v", perr)
		}
		for _, r := range arr {
			if r["id"] == s.learningID {
				return nil
			}
		}
		return fmt.Errorf("search did not return the captured learning")
	}) {
		return s.steps
	}

	// curate
	if !s.step("curate", func() error {
		m, res, err := call("learning_curate", map[string]any{
			"learning_id": s.learningID,
			"decision":    "approve_agents_rule",
			"rationale":   "The evidence proves this rule belongs in the governance surface",
			"actor":       map[string]any{"kind": "human", "name": "curator"},
		})
		if err != nil {
			return err
		}
		if res != nil && res.IsError {
			return fmt.Errorf("curate errored: %s", mcpText(res))
		}
		if m["new_status"] != "approved" {
			return fmt.Errorf("new_status = %v, want approved", m["new_status"])
		}
		return nil
	}) {
		return s.steps
	}

	// preview — requires_approval true
	if !s.step("preview", func() error {
		m, res, err := call("learning_publication_preview", map[string]any{
			"learning_id": s.learningID,
			"actor":       map[string]any{"kind": "human", "name": "publisher"},
		})
		if err != nil {
			return err
		}
		if res != nil && res.IsError {
			return fmt.Errorf("preview errored: %s", mcpText(res))
		}
		if req, _ := m["requires_approval"].(bool); !req {
			return fmt.Errorf("preview requires_approval=false; the MCP gate does not fire")
		}
		s.previewHash, _ = m["preview_hash"].(string)
		if s.previewHash == "" {
			return fmt.Errorf("preview returned no preview_hash")
		}
		return nil
	}) {
		return s.steps
	}

	// publish WITHOUT approval — must error with approval_required
	if !s.step("publish-without-approval-refused", func() error {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "learning_publish", Arguments: map[string]any{
			"learning_id":  s.learningID,
			"preview_hash": s.previewHash,
			"apply":        true,
			"actor":        actor,
		}})
		if err != nil {
			return err
		}
		if !res.IsError {
			return fmt.Errorf("publish without approval succeeded: %s", mcpText(res))
		}
		if !strings.Contains(mcpText(res), "approval") {
			return fmt.Errorf("refusal did not mention approval: %s", mcpText(res))
		}
		if _, statErr := os.Stat(agentsPath); statErr == nil {
			return fmt.Errorf("AGENTS.md was created despite a refused publish")
		}
		return nil
	}) {
		return s.steps
	}

	// approve
	if !s.step("approve", func() error {
		m, res, err := call("learning_approve", map[string]any{
			"learning_id":       s.learningID,
			"preview_hash":      s.previewHash,
			"approved_by":       "e2e-human",
			"reason":            "Recorrido E authorizes this exact preview over MCP",
			"approval_evidence": "session://recorrido-e",
			"actor":             map[string]any{"kind": "human", "name": "approver"},
		})
		if err != nil {
			return err
		}
		if res != nil && res.IsError {
			return fmt.Errorf("approve errored: %s", mcpText(res))
		}
		s.approvalID, _ = m["approval_id"].(string)
		if s.approvalID == "" {
			return fmt.Errorf("approve returned no approval_id")
		}
		return nil
	}) {
		return s.steps
	}

	// publish WITH approval — success, file written
	if !s.step("publish-apply", func() error {
		m, res, err := call("learning_publish", map[string]any{
			"learning_id":  s.learningID,
			"preview_hash": s.previewHash,
			"approval_id":  s.approvalID,
			"apply":        true,
			"actor":        actor,
		})
		if err != nil {
			return err
		}
		if res != nil && res.IsError {
			return fmt.Errorf("publish errored: %s", mcpText(res))
		}
		s.publicationID, _ = m["publication_id"].(string)
		if s.publicationID == "" {
			return fmt.Errorf("publish returned no publication_id")
		}
		cur, readErr := os.ReadFile(agentsPath)
		if readErr != nil {
			return fmt.Errorf("AGENTS.md not written: %v", readErr)
		}
		if !strings.Contains(string(cur), marker) {
			return fmt.Errorf("AGENTS.md does not contain the published lesson")
		}
		return nil
	}) {
		return s.steps
	}

	// report occurrence
	if !s.step("report-occurrence", func() error {
		m, res, err := call("learning_report_occurrence", map[string]any{
			"learning_id":     s.learningID,
			"summary":         "the same governance gap recurred over MCP",
			"outcome":         "prevented",
			"retrieved":       true,
			"skill_activated": true,
			"idempotency_key": "mcp-occ-1",
			"actor":           actor,
		})
		if err != nil {
			return err
		}
		if res != nil && res.IsError {
			return fmt.Errorf("report_occurrence errored: %s", mcpText(res))
		}
		if m["new"] != true {
			return fmt.Errorf("occurrence new = %v, want true", m["new"])
		}
		return nil
	}) {
		return s.steps
	}

	// status — published
	if !s.step("status", func() error {
		m, res, err := call("learning_status", map[string]any{"learning_id": s.learningID})
		if err != nil {
			return err
		}
		if res != nil && res.IsError {
			return fmt.Errorf("status errored: %s", mcpText(res))
		}
		if m["status"] != "published" {
			return fmt.Errorf("status = %v, want published", m["status"])
		}
		return nil
	}) {
		return s.steps
	}

	// rollback — reverts and removes the created file
	if !s.step("rollback", func() error {
		m, res, err := call("learning_rollback", map[string]any{
			"publication_id": s.publicationID,
			"actor":          actor,
		})
		if err != nil {
			return err
		}
		if res != nil && res.IsError {
			return fmt.Errorf("rollback errored: %s", mcpText(res))
		}
		if m["status"] != "rolled_back" {
			return fmt.Errorf("rollback status = %v, want rolled_back", m["status"])
		}
		if _, statErr := os.Stat(agentsPath); statErr == nil {
			return fmt.Errorf("rollback did not remove the created AGENTS.md")
		}
		return nil
	}) {
		return s.steps
	}

	return s.steps
}

// mcpText extracts the first text content of a tool result.
func mcpText(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}
