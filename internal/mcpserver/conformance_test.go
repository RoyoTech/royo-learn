package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPConformance_Initialize verifies the MCP initialize handshake.
func TestMCPConformance_Initialize(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "standard")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// The session is already initialized via newTestServer.
	// Verify we can call a tool (proves the server is alive after init).
	result, err := ts.callTool(ctx, "doctor", map[string]any{})
	if err != nil {
		t.Fatalf("initialize+doctor: %v", err)
	}
	if result.IsError {
		txt := result.Content[0].(*mcp.TextContent)
		t.Fatalf("doctor returned error after init: %s", txt.Text)
	}
}

// TestMCPConformance_ListToolsAllProfiles verifies tool listing across all profiles.
func TestMCPConformance_ListToolsAllProfiles(t *testing.T) {
	t.Parallel()

	type profileTest struct {
		name          string
		profile       string
		minToolCount  int
		requiredTools []string
		forbidden     []string
	}

	tests := []profileTest{
		{
			name:         "standard",
			profile:      "standard",
			minToolCount: 8,
			requiredTools: []string{
				"capture_learning",
				"search_learnings",
				"curate_learning",
				"preview_publication",
				"publish_learning",
				"list_learnings",
				"get_learning",
				"doctor",
				"list_recurrences",
				"compute_metrics",
			},
		},
		{
			name:         "full",
			profile:      "full",
			minToolCount: 9,
			requiredTools: []string{
				"capture_learning",
				"search_learnings",
				"curate_learning",
				"preview_publication",
				"publish_learning",
				"list_learnings",
				"get_learning",
				"doctor",
				"list_recurrences",
				"compute_metrics",
			},
		},
		{
			// D2 narrows this profile deliberately. In v0.1.9 "minimal" served
			// capture_learning — a WRITE — and withheld get and list, which are
			// reads. docs/04-CLI-SPEC.md defines read as "search and get", so the
			// read profile now serves reads only. The deprecated name keeps
			// working and maps onto read; its tool set is what changes.
			name:         "minimal maps to read and is read-only",
			profile:      "minimal",
			minToolCount: 4,
			requiredTools: []string{
				"learning_search",
				"learning_get",
				"learning_list",
				"learning_doctor",
				// v0.1.9 aliases remain callable.
				"search_learnings",
				"get_learning",
				"list_learnings",
				"doctor",
			},
			forbidden: []string{
				// Writes have no place in a read profile.
				"learning_capture",
				"capture_learning",
				"learning_curate",
				"curate_learning",
				"learning_publication_preview",
				"preview_publication",
				"learning_publish",
				"publish_learning",
			},
		},
		{
			name:         "canonical read profile",
			profile:      "read",
			minToolCount: 4,
			requiredTools: []string{
				"learning_search",
				"learning_get",
				"learning_list",
				"learning_doctor",
			},
			forbidden: []string{"learning_capture", "learning_curate", "learning_publish"},
		},
		{
			name:         "canonical agent profile",
			profile:      "agent",
			minToolCount: 9,
			requiredTools: []string{
				"learning_capture",
				"learning_search",
				"learning_get",
				"learning_list",
				"learning_curate",
				"learning_publication_preview",
				// D2: learning_publish moves into agent now that Recorrido C's
				// approval gate protects sensitive publications, together with
				// the human-approval tool that authorizes them.
				"learning_approve",
				"learning_publish",
				"learning_doctor",
				"learning_list_recurrences",
				"learning_compute_metrics",
			},
		},
		{
			name:         "canonical admin profile",
			profile:      "admin",
			minToolCount: 10,
			requiredTools: []string{
				"learning_capture",
				"learning_search",
				"learning_get",
				"learning_list",
				"learning_curate",
				"learning_publication_preview",
				"learning_approve",
				"learning_publish",
				"learning_doctor",
				"learning_list_recurrences",
				"learning_compute_metrics",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t, tt.profile)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result, err := ts.session.ListTools(ctx, nil)
			if err != nil {
				t.Fatalf("ListTools(%s): %v", tt.profile, err)
			}

			toolNames := make(map[string]bool)
			for _, tool := range result.Tools {
				toolNames[tool.Name] = true
				// Every tool must have a description.
				if tool.Description == "" {
					t.Errorf("tool %q has empty description", tool.Name)
				}
				// Every tool must have a non-empty InputSchema.
				if tool.InputSchema == nil {
					t.Errorf("tool %q has nil InputSchema", tool.Name)
				}
			}

			if len(result.Tools) < tt.minToolCount {
				t.Errorf("%s profile: tool count = %d, want >= %d", tt.profile, len(result.Tools), tt.minToolCount)
			}

			for _, req := range tt.requiredTools {
				if !toolNames[req] {
					t.Errorf("%s profile missing required tool %q", tt.profile, req)
				}
			}

			for _, forbid := range tt.forbidden {
				if toolNames[forbid] {
					t.Errorf("%s profile should NOT include tool %q", tt.profile, forbid)
				}
			}
		})
	}
}

// TestMCPConformance_CallAllTools verifies every standard-profile tool can be called
// with valid input and returns structured JSON.
func TestMCPConformance_CallAllTools(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "standard")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Helper to check a tool call returns non-error content.
	assertToolOK := func(name string, args map[string]any) map[string]any {
		t.Helper()
		result, err := ts.callTool(ctx, name, args)
		if err != nil {
			t.Fatalf("%s transport error: %v", name, err)
		}
		if result.IsError {
			if len(result.Content) > 0 {
				txt := result.Content[0].(*mcp.TextContent)
				t.Fatalf("%s tool error: %s", name, txt.Text)
			}
			t.Fatalf("%s tool error (no content)", name)
		}
		if len(result.Content) == 0 {
			t.Fatalf("%s returned empty content", name)
		}
		txt := result.Content[0].(*mcp.TextContent)
		var data map[string]any
		if err := json.Unmarshal([]byte(txt.Text), &data); err != nil {
			t.Fatalf("%s output not valid JSON: %v\n%s", name, err, txt.Text)
		}
		return data
	}

	// 1. capture_learning
	capData := assertToolOK("capture_learning", map[string]any{
		"title":           "Conformance Test Capture",
		"type":            "procedure",
		"context":         "conformance test context",
		"observation":     "conformance test observation",
		"reusable_lesson": "conformance tests verify protocol correctness",
		"scope_guess":     "project",
		"confidence":      "high",
		"evidence_level":  "moderate",
		"actor": map[string]any{
			"kind":       "agent",
			"name":       "conformance-tester",
			"model":      "test-v1",
			"session_id": "conformance-session",
		},
	})
	learningID, ok := capData["learning_id"].(string)
	if !ok || learningID == "" {
		t.Fatal("capture_learning missing learning_id")
	}
	if capData["new"] != true {
		t.Error("first capture should produce new=true")
	}
	if capData["status"] != "captured" {
		t.Errorf("status = %v, want captured", capData["status"])
	}

	// 2. search_learnings (returns array, not map)
	searchResult, err := ts.callTool(ctx, "search_learnings", map[string]any{
		"query": "conformance",
	})
	if err != nil {
		t.Fatalf("search_learnings: %v", err)
	}
	if searchResult.IsError {
		txt := searchResult.Content[0].(*mcp.TextContent)
		t.Fatalf("search_learnings error: %s", txt.Text)
	}
	txt := searchResult.Content[0].(*mcp.TextContent)
	var searchData []map[string]any
	if err := json.Unmarshal([]byte(txt.Text), &searchData); err != nil {
		t.Fatalf("search_learnings output not valid JSON: %v\n%s", err, txt.Text)
	}

	// 3. list_learnings (returns array)
	listResult, err := ts.callTool(ctx, "list_learnings", map[string]any{"limit": float64(10)})
	if err != nil {
		t.Fatalf("list_learnings: %v", err)
	}
	if listResult.IsError {
		txt := listResult.Content[0].(*mcp.TextContent)
		t.Fatalf("list_learnings error: %s", txt.Text)
	}
	txt2 := listResult.Content[0].(*mcp.TextContent)
	var listData []map[string]any
	if err := json.Unmarshal([]byte(txt2.Text), &listData); err != nil {
		t.Fatalf("list_learnings output not valid JSON: %v\n%s", err, txt2.Text)
	}

	// 4. get_learning
	getData := assertToolOK("get_learning", map[string]any{
		"learning_id": learningID,
	})
	if getData["title"] != "Conformance Test Capture" {
		t.Errorf("title = %v", getData["title"])
	}

	// 5. doctor
	docData := assertToolOK("doctor", map[string]any{})
	if okVal, _ := docData["ok"].(bool); !okVal {
		t.Error("doctor ok=false")
	}
	if _, hasVersion := docData["version"]; !hasVersion {
		t.Error("doctor missing version")
	}

	// 6. list_recurrences (returns array)
	recResult, err := ts.callTool(ctx, "list_recurrences", map[string]any{
		"learning_id": learningID,
	})
	if err != nil {
		t.Fatalf("list_recurrences: %v", err)
	}
	if recResult.IsError {
		txt := recResult.Content[0].(*mcp.TextContent)
		t.Fatalf("list_recurrences error: %s", txt.Text)
	}
	txt3 := recResult.Content[0].(*mcp.TextContent)
	var recData []map[string]any
	if err := json.Unmarshal([]byte(txt3.Text), &recData); err != nil {
		t.Fatalf("list_recurrences output not valid JSON: %v\n%s", err, txt3.Text)
	}

	// 7. compute_metrics (requires the learning to have recurrences; may fail for single capture)
	result, err := ts.callTool(ctx, "compute_metrics", map[string]any{
		"learning_id": learningID,
	})
	if err != nil {
		t.Fatalf("compute_metrics: %v", err)
	}
	if result.IsError {
		txt := result.Content[0].(*mcp.TextContent)
		// compute_metrics can legitimately fail for learnings without recurrences.
		var errMap map[string]any
		json.Unmarshal([]byte(txt.Text), &errMap)
		t.Logf("compute_metrics returned error (expected for single capture): %v", errMap)
	} else {
		txt := result.Content[0].(*mcp.TextContent)
		var metricsData map[string]any
		if err := json.Unmarshal([]byte(txt.Text), &metricsData); err != nil {
			t.Errorf("compute_metrics output not valid JSON: %v", err)
		}
	}

	// 8. curate_learning (may fail if evidence thresholds not met)
	curateResult, err := ts.callTool(ctx, "curate_learning", map[string]any{
		"learning_id": learningID,
		"decision":    "approve_project_knowledge",
		"rationale":   "conformance test curation with adequate rationale explanation",
		"actor": map[string]any{
			"kind": "human",
			"name": "conformance-curator",
		},
	})
	if err != nil {
		t.Fatalf("curate_learning: %v", err)
	}
	if curateResult.IsError {
		txt := curateResult.Content[0].(*mcp.TextContent)
		var errMap map[string]any
		json.Unmarshal([]byte(txt.Text), &errMap)
		t.Logf("curate_learning returned error (may need evidence): %v", errMap)
	} else {
		txt := curateResult.Content[0].(*mcp.TextContent)
		var curateData map[string]any
		json.Unmarshal([]byte(txt.Text), &curateData)
		if curateData["new_status"] != "approved" {
			t.Errorf("curate new_status = %v", curateData["new_status"])
		}
	}

	// 9. preview_publication (may fail if learning not approved)
	previewResult, err := ts.callTool(ctx, "preview_publication", map[string]any{
		"learning_id": learningID,
		"actor": map[string]any{
			"kind": "human",
			"name": "conformance-publisher",
		},
	})
	if err != nil {
		t.Fatalf("preview_publication: %v", err)
	}
	if previewResult.IsError {
		txt := previewResult.Content[0].(*mcp.TextContent)
		t.Logf("preview_publication returned error (expected if not approved): %s", txt.Text)
	} else {
		txt := previewResult.Content[0].(*mcp.TextContent)
		var previewData map[string]any
		if err := json.Unmarshal([]byte(txt.Text), &previewData); err != nil {
			t.Errorf("preview_publication output not valid JSON: %v", err)
		}
		if _, hasHash := previewData["preview_hash"]; !hasHash {
			t.Error("preview_publication missing preview_hash")
		}
	}
}

// TestMCPConformance_Shutdown verifies graceful shutdown.
func TestMCPConformance_Shutdown(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "standard")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Call a tool to verify server is alive.
	result, err := ts.callTool(ctx, "doctor", map[string]any{})
	if err != nil {
		t.Fatalf("pre-shutdown doctor: %v", err)
	}
	if result.IsError {
		txt := result.Content[0].(*mcp.TextContent)
		t.Fatalf("pre-shutdown doctor error: %s", txt.Text)
	}

	// Close the session (graceful shutdown).
	if err := ts.session.Close(); err != nil {
		t.Logf("session close returned error (non-fatal): %v", err)
	}

	// After close, further calls should fail.
	_, err = ts.callTool(ctx, "doctor", map[string]any{})
	if err == nil {
		t.Error("expected error calling tool after shutdown")
	}
}

// TestMCPConformance_EmptyToolArgs verifies tools handle empty args gracefully.
func TestMCPConformance_EmptyToolArgs(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "standard")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// doctor tool accepts empty args.
	result, err := ts.callTool(ctx, "doctor", map[string]any{})
	if err != nil {
		t.Fatalf("doctor with empty args: %v", err)
	}
	if result.IsError {
		txt := result.Content[0].(*mcp.TextContent)
		t.Fatalf("doctor with empty args returned error: %s", txt.Text)
	}

	// capture_learning with empty args should return error (required fields).
	result, err = ts.callTool(ctx, "capture_learning", map[string]any{})
	if err != nil {
		t.Fatalf("capture_learning with empty args: %v", err)
	}
	if !result.IsError {
		t.Error("capture_learning with empty args should return IsError")
	}
}

// TestMCPConformance_SchemasHaveDescriptions verifies all tool schemas have descriptions.
func TestMCPConformance_SchemasHaveDescriptions(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "standard")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	for _, tool := range result.Tools {
		if tool.Name == "" {
			t.Error("tool has empty name")
		}
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has nil schema", tool.Name)
		}
	}
}

// TestMCPConformance_ServerInstructionsContainsProfile verifies the instructions
// describe the ACTIVE profile.
//
// This test previously asserted a literal list of 10 tool names, which enshrined
// the defect named by D14: the instructions promised all 10 tools in every
// profile while minimal registered 3 and standard 9. Asserting the literal list
// made the lie a requirement. The agreement between the instructions and the
// real registry is now asserted, per profile, by
// TestContract_InstructionsAgreeWithToolsList in contract_test.go.
func TestMCPConformance_ServerInstructionsContainsProfile(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "admin")

	instructions := ts.server.Instructions()
	if instructions == "" {
		t.Fatal("expected non-empty server instructions")
	}

	for _, phrase := range []string{"royo-learn", "Profile: admin"} {
		if !contains(instructions, phrase) {
			t.Errorf("instructions missing phrase %q", phrase)
		}
	}
}

// TestMCPConformance_StdioTransportContract verifies the server can be created
// for stdio (no actual stdio connection — just validates config).
func TestMCPConformance_StdioTransportContract(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "standard")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// The in-memory transport test already validates the server contract.
	// Verify the session is alive by calling a tool.
	result, err := ts.callTool(ctx, "doctor", map[string]any{})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if result.IsError {
		txt := result.Content[0].(*mcp.TextContent)
		t.Fatalf("doctor error: %s", txt.Text)
	}
}

// TestMCPConformance_ErrorResponseFormat verifies error responses have correct MCP format.
func TestMCPConformance_ErrorResponseFormat(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "standard")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get a non-existent learning — must return error with IsError=true and structured content.
	result, err := ts.callTool(ctx, "get_learning", map[string]any{
		"learning_id": "nonexistent-id-that-does-not-exist-00000",
	})
	if err != nil {
		t.Fatalf("get_learning transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("get_learning for non-existent ID should set IsError=true")
	}
	if len(result.Content) == 0 {
		t.Fatal("error response has no content")
	}

	txt := result.Content[0].(*mcp.TextContent)
	var errData map[string]any
	if err := json.Unmarshal([]byte(txt.Text), &errData); err != nil {
		t.Fatalf("error content not valid JSON: %v\n%s", err, txt.Text)
	}
	inner, nested := errData["error"].(map[string]any)
	if !nested {
		t.Fatalf("error response is not nested under error: %v", errData)
	}
	code, hasCode := inner["code"].(string)
	if !hasCode || code == "" {
		t.Errorf("error response missing 'code' field: %v", inner)
	}
	msg, hasMsg := inner["message"].(string)
	if !hasMsg || msg == "" {
		t.Errorf("error response missing 'message' field: %v", inner)
	}
}
