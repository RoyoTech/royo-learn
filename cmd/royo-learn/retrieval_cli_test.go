package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The tests in this file drive Block 1 of the Recorrido E expansion (D17):
// the public CLI commands `get`, `search` and `occurrence`. Every assertion is a
// real business effect observed through the public CLI. Nothing here reaches
// around the interface with direct SQL.

// captureForRetrieval captures one learning through the CLI and returns its ID.
func captureForRetrieval(t *testing.T, root, title string) string {
	t.Helper()
	out := captureJSON(t,
		"--project-root", root,
		"--title", title,
		"--context", "Retrieval commands must reach captured learnings",
		"--observation", "get, search and occurrence are public CLI entry points",
		"--lesson", "A capability nobody can invoke is not finished (plan 1.4)",
		"--destination", "project",
		"--evidence-level", "moderate",
		"--json",
	)
	id, _ := out["learning_id"].(string)
	if id == "" {
		t.Fatal("capture returned no learning_id")
	}
	return id
}

// TestCLI_GetReturnsLearning proves `royo-learn get <id>` retrieves the exact
// learning by ID, not merely that it prints valid JSON.
func TestCLI_GetReturnsLearning(t *testing.T) {
	root := initProject(t)
	id := captureForRetrieval(t, root, "Get must return the exact learning")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"get", id, "--project-root", root, "--json"}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("get exit = %d, want %d\nstderr: %s", code, exitSuccess, stderr.String())
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("get stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if got["id"] != id {
		t.Fatalf("get returned id %v, want %q", got["id"], id)
	}
	if got["title"] != "Get must return the exact learning" {
		t.Fatalf("get returned title %v, want the captured title", got["title"])
	}
	if got["status"] != "captured" {
		t.Fatalf("get returned status %v, want captured", got["status"])
	}
}

// TestCLI_GetUnknownIDFails proves a missing learning is an error, never a
// silent empty success.
func TestCLI_GetUnknownIDFails(t *testing.T) {
	root := initProject(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{"get", "does-not-exist", "--project-root", root, "--json"}, &stdout, &stderr)
	if code == exitSuccess {
		t.Fatalf("get of an unknown id succeeded; absence of data must not be success.\nstdout: %s", stdout.String())
	}
}

// TestCLI_SearchFindsCapturedLearning proves `royo-learn search <query>` returns
// the matching learning and labels its source, not merely that JSON is JSON.
func TestCLI_SearchFindsCapturedLearning(t *testing.T) {
	root := initProject(t)
	id := captureForRetrieval(t, root, "Zylographic marmoset indexing")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"search", "zylographic", "--project-root", root, "--json"}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("search exit = %d, want %d\nstderr: %s", code, exitSuccess, stderr.String())
	}
	var results []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("search stdout is not a JSON array: %v\n%s", err, stdout.String())
	}
	found := false
	for _, r := range results {
		if r["id"] == id {
			found = true
			if r["source"] != "royo_learn" {
				t.Errorf("result source = %v, want royo_learn", r["source"])
			}
		}
	}
	if !found {
		t.Fatalf("search did not return the captured learning %q\n%s", id, stdout.String())
	}
}

// TestCLI_SearchWithoutMatchReturnsEmptyNotError proves an empty result set is a
// valid, distinct outcome — and that the earlier find test was not vacuous.
func TestCLI_SearchWithoutMatchReturnsEmptyNotError(t *testing.T) {
	root := initProject(t)
	captureForRetrieval(t, root, "Ordinary title")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"search", "nonexistentterm9x7q", "--project-root", root, "--json"}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("search exit = %d, want %d\nstderr: %s", code, exitSuccess, stderr.String())
	}
	var results []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("search stdout is not a JSON array: %v\n%s", err, stdout.String())
	}
	if len(results) != 0 {
		t.Fatalf("search for a nonexistent term returned %d results, want 0", len(results))
	}
}

// TestCLI_OccurrenceRecordsAndIsIdempotent proves `royo-learn occurrence` records
// a recurrence with a real, countable effect and applies D5 idempotency: the same
// idempotency key on a retry does not create a second record.
func TestCLI_OccurrenceRecordsAndIsIdempotent(t *testing.T) {
	root := initProject(t)
	id := captureForRetrieval(t, root, "Occurrence must be countable")

	report := func(key string) map[string]any {
		var stdout, stderr bytes.Buffer
		args := []string{
			"occurrence",
			"--learning-id", id,
			"--summary", "the same failure was observed again",
			"--outcome", "resolved",
			"--retrieved", "true",
			"--skill-activated", "true",
			"--project-root", root,
			"--json",
		}
		if key != "" {
			args = append(args, "--idempotency-key", key)
		}
		if code := run(args, &stdout, &stderr); code != exitSuccess {
			t.Fatalf("occurrence exit = %d, want %d\nstderr: %s", code, exitSuccess, stderr.String())
		}
		var out map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("occurrence stdout is not JSON: %v\n%s", err, stdout.String())
		}
		return out
	}

	count := func() int {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"recurrences", "--learning-id", id, "--project-root", root, "--json"}, &stdout, &stderr); code != exitSuccess {
			t.Fatalf("recurrences exit = %d: %s", code, stderr.String())
		}
		var recs []map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &recs); err != nil {
			t.Fatalf("recurrences stdout is not a JSON array: %v\n%s", err, stdout.String())
		}
		return len(recs)
	}

	// First report with a key: a new recurrence is created.
	first := report("retry-key-1")
	if first["new"] != true {
		t.Fatalf("first occurrence new = %v, want true", first["new"])
	}
	recID, _ := first["recurrence_id"].(string)
	if recID == "" {
		t.Fatal("occurrence returned no recurrence_id")
	}
	if first["outcome"] != "resolved" {
		t.Errorf("occurrence outcome = %v, want resolved", first["outcome"])
	}
	if got := count(); got != 1 {
		t.Fatalf("after first report, recurrence count = %d, want 1", got)
	}

	// Same key: D5 technical retry. No second record, same recurrence_id.
	retry := report("retry-key-1")
	if retry["new"] != false {
		t.Fatalf("retry with same idempotency key new = %v, want false", retry["new"])
	}
	if retry["recurrence_id"] != recID {
		t.Errorf("retry recurrence_id = %v, want the original %q", retry["recurrence_id"], recID)
	}
	if got := count(); got != 1 {
		t.Fatalf("after idempotent retry, recurrence count = %d, want 1 (D5 duplicated a recurrence)", got)
	}

	// A distinct report (no key) is a real second occurrence.
	second := report("")
	if second["new"] != true {
		t.Fatalf("second distinct occurrence new = %v, want true", second["new"])
	}
	if got := count(); got != 2 {
		t.Fatalf("after a distinct report, recurrence count = %d, want 2", got)
	}
}

// TestCLI_OccurrenceUnknownLearningFails proves reporting against a missing
// learning is an error, not a silent no-op.
func TestCLI_OccurrenceUnknownLearningFails(t *testing.T) {
	root := initProject(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{"occurrence", "--learning-id", "missing", "--summary", "x", "--project-root", root, "--json"}, &stdout, &stderr)
	if code == exitSuccess {
		t.Fatalf("occurrence against a missing learning succeeded; stdout: %s", stdout.String())
	}
	if !strings.Contains(strings.ToLower(stderr.String()), "not found") && !strings.Contains(strings.ToLower(stderr.String()), "learning") {
		t.Fatalf("error does not identify the missing learning: %s", stderr.String())
	}
}
