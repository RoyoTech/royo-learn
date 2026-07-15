package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Block 2 of the Recorrido E expansion (D17): the MCP tools
// learning_report_occurrence (write), learning_status (read) and
// learning_rollback (destructive, admin only). Every assertion is a real state
// change observed through the public MCP session.

// captureProjectLearning captures a low-impact project-scope learning with
// evidence through the public tools and returns its id.
func captureProjectLearning(t *testing.T, ts *testServer, ctx context.Context, seed string) string {
	t.Helper()
	capture := mustCallJSON(t, ts, ctx, "learning_capture", map[string]any{
		"title":                "Project learning " + seed,
		"type":                 "procedure",
		"context":              "A low-impact local learning (" + seed + ").",
		"observation":          "It is routed to project scope and needs no human approval (" + seed + ").",
		"reusable_lesson":      "Local project knowledge publishes without a gate (" + seed + ").",
		"scope_guess":          "project",
		"confidence":           "high",
		"evidence_level":       "moderate",
		"proposed_destination": "project",
		"actor":                map[string]any{"kind": "agent", "name": "recorrido-e"},
		"evidence": []map[string]any{
			{
				"kind":    "test",
				"summary": "Evidence that unblocks approval",
				"source":  "test://recorrido-e",
				"content": "--- PASS: TestMCP_Block2",
			},
		},
	})
	id, _ := capture["learning_id"].(string)
	if id == "" {
		t.Fatalf("capture returned no learning_id: %v", capture)
	}
	return id
}

func TestMCP_ReportOccurrence_RecordsAndIsIdempotent(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "agent")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	id := captureProjectLearning(t, ts, ctx, "occ")

	countRecurrences := func() int {
		res := mustCallJSONArray(t, ts, ctx, "learning_list_recurrences", map[string]any{"learning_id": id})
		return len(res)
	}

	first := mustCallJSON(t, ts, ctx, "learning_report_occurrence", map[string]any{
		"learning_id":     id,
		"summary":         "the same failure recurred",
		"outcome":         "resolved",
		"retrieved":       true,
		"skill_activated": true,
		"idempotency_key": "occ-key-1",
		"actor":           map[string]any{"kind": "agent", "name": "recorrido-e"},
	})
	if first["new"] != true {
		t.Fatalf("first report new = %v, want true", first["new"])
	}
	recID, _ := first["recurrence_id"].(string)
	if recID == "" {
		t.Fatal("report returned no recurrence_id")
	}
	if got := countRecurrences(); got != 1 {
		t.Fatalf("after first report, count = %d, want 1", got)
	}

	// Same idempotency key: D5 technical retry, no second record.
	retry := mustCallJSON(t, ts, ctx, "learning_report_occurrence", map[string]any{
		"learning_id":     id,
		"idempotency_key": "occ-key-1",
		"actor":           map[string]any{"kind": "agent", "name": "recorrido-e"},
	})
	if retry["new"] != false {
		t.Fatalf("retry new = %v, want false", retry["new"])
	}
	if got := countRecurrences(); got != 1 {
		t.Fatalf("after idempotent retry, count = %d, want 1 (D5 violated)", got)
	}
}

func TestMCP_Status_ReflectsLifecycle(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "read")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// A read profile cannot capture; set up state on an agent server, then read
	// status from a read server pointed at the same DB is not trivial here, so
	// capture on this agent-capable server instead.
	agent := newTestServer(t, "agent")
	id := captureProjectLearning(t, agent, ctx, "status")

	status := mustCallJSON(t, agent, ctx, "learning_status", map[string]any{"learning_id": id})
	if status["status"] != "captured" {
		t.Fatalf("status = %v, want captured", status["status"])
	}
	if status["learning_id"] != id {
		t.Fatalf("status learning_id = %v, want %q", status["learning_id"], id)
	}

	// learning_status must also be reachable from a read profile.
	if !toolServed(t, ts, ctx, "learning_status") {
		t.Fatal("learning_status must be served in the read profile")
	}
}

func TestMCP_Rollback_RestoresAndBlocksDoubleRollback(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, "admin")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	id := captureProjectLearning(t, ts, ctx, "rb")

	mustCallJSON(t, ts, ctx, "learning_curate", map[string]any{
		"learning_id": id,
		"decision":    "approve_project_knowledge",
		"rationale":   "The evidence proves this local learning is reusable and safe to publish.",
		"actor":       map[string]any{"kind": "human", "name": "curator"},
	})

	preview := mustCallJSON(t, ts, ctx, "learning_publication_preview", map[string]any{
		"learning_id": id,
		"actor":       map[string]any{"kind": "human", "name": "publisher"},
	})
	if preview["requires_approval"] == true {
		t.Fatal("a project-scope learning must NOT require approval; the low-impact path is over-blocked")
	}
	previewHash, _ := preview["preview_hash"].(string)

	published := mustCallJSON(t, ts, ctx, "learning_publish", map[string]any{
		"learning_id":  id,
		"preview_hash": previewHash,
		"apply":        true,
		"actor":        map[string]any{"kind": "human", "name": "publisher"},
	})
	pubID, _ := published["publication_id"].(string)
	if pubID == "" {
		t.Fatalf("publish returned no publication_id: %v", published)
	}
	if published["status"] != "completed" {
		t.Fatalf("publication status = %v, want completed", published["status"])
	}

	// The learning itself is now published — learning_status must report it.
	statusAfter := mustCallJSON(t, ts, ctx, "learning_status", map[string]any{"learning_id": id})
	if statusAfter["status"] != "published" {
		t.Fatalf("learning_status after publish = %v, want published", statusAfter["status"])
	}

	// Find the file that was written so we can prove rollback removes it.
	writtenFiles := filesUnder(t, filepath.Join(ts.root, ".royo-learn", "knowledge"))

	rolled := mustCallJSON(t, ts, ctx, "learning_rollback", map[string]any{
		"publication_id": pubID,
		"actor":          map[string]any{"kind": "human", "name": "publisher"},
	})
	if rolled["status"] != "rolled_back" {
		t.Fatalf("rollback status = %v, want rolled_back", rolled["status"])
	}

	// The published file must be gone (it was a new file).
	for _, f := range writtenFiles {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("rollback did not remove the published file %s", f)
		}
	}

	// Rolling back an already-rolled-back publication must fail.
	again, err := ts.callTool(ctx, "learning_rollback", map[string]any{
		"publication_id": pubID,
		"actor":          map[string]any{"kind": "human", "name": "publisher"},
	})
	if err != nil {
		t.Fatalf("second rollback: transport error: %v", err)
	}
	if !again.IsError {
		t.Fatal("rolling back an already-rolled-back publication must fail")
	}
}

func TestMCP_Rollback_NotServedInReadOrAgent(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, profile := range []string{"read", "agent"} {
		ts := newTestServer(t, profile)
		if toolServed(t, ts, ctx, "learning_rollback") {
			t.Errorf("destructive tool learning_rollback must not be served in profile %q", profile)
		}
	}
}

// mustCallJSONArray calls a tool and decodes its response as a JSON array.
func mustCallJSONArray(t *testing.T, ts *testServer, ctx context.Context, name string, args map[string]any) []map[string]any {
	t.Helper()
	result, err := ts.callTool(ctx, name, args)
	if err != nil {
		t.Fatalf("%s: transport error: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s: tool error: %s", name, toolText(result))
	}
	var out []map[string]any
	if err := json.Unmarshal([]byte(toolText(result)), &out); err != nil {
		t.Fatalf("%s: response not JSON array: %v (%s)", name, err, toolText(result))
	}
	return out
}

// toolServed reports whether a tool name appears in tools/list for the session.
func toolServed(t *testing.T, ts *testServer, ctx context.Context, name string) bool {
	t.Helper()
	listed, err := ts.session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

// filesUnder returns the regular files under dir (recursively), if it exists.
func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info != nil && !info.IsDir() {
			out = append(out, path)
		}
		return nil
	})
	return out
}
