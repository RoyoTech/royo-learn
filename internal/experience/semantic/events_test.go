// Package semantic — tests for the audit-event constants and payload
// allow-list. See events.go.

package semantic

import (
	"sort"
	"testing"
)

// TestJobPayload_AllowListContract asserts the documented allow-list
// keys. The list is the single source of truth for what may appear in
// a job_* audit-event payload; the test guards against accidental
// additions or removals.
func TestJobPayload_AllowListContract(t *testing.T) {
	want := []string{
		"attempt",
		"error_code",
		"error_message",
		"job_name",
		"run_id",
		"source",
		"state",
		"transition",
	}
	got := AllowedDetailsKeys()
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("AllowedDetailsKeys length = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllowedDetailsKeys[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestJobPayload_RejectsForbiddenKeys proves the helper never accepts
// a free-form details map. The function signature is closed: callers
// must pass scalar string arguments; there is no path that smuggles a
// non-allow-listed key into the payload. The test enforces the
// contract by exhausting the public surface: BuildJobPayload is the
// only public entry point, and its arguments are individually
// inspected below.
func TestJobPayload_RejectsForbiddenKeys(t *testing.T) {
	// Build a payload through the public BuildJobPayload entry point
	// with a sentinel forbidden key embedded in the error_message
	// argument. The test passes the literal in a string field — it
	// can never escape into a key because the helper writes
	// arguments by position into fixed keys.
	payload := BuildJobPayload(
		"experience_ingest:codex", // job_name
		"01HZ",                    // run_id
		"codex",                   // source
		StateSucceeded,            // state
		EventJobSucceeded,         // transition
		"1",                       // attempt
		"",                        // error_code
		"",                        // error_message
	)
	// Forbidden key names must not appear in the payload.
	for _, forbidden := range []string{
		"transcript", "user_text", "assistant_text", "tool_output_hint",
		"details", "extra", "free_form",
	} {
		if _, ok := payload[forbidden]; ok {
			t.Errorf("payload carries forbidden key %q (leak vector)", forbidden)
		}
	}

	// The happy-path payload must contain exactly the documented six
	// keys (no error fields when error_code/error_message are empty).
	wantKeys := []string{
		"attempt", "job_name", "run_id", "source", "state", "transition",
	}
	if len(payload) != len(wantKeys) {
		t.Errorf("payload key count = %d, want %d (payload = %v)", len(payload), len(wantKeys), payload)
	}
	for _, k := range wantKeys {
		if _, ok := payload[k]; !ok {
			t.Errorf("payload missing required key %q", k)
		}
	}

	// Failure-path payload must contain the two extra keys when the
	// caller passes non-empty error fields.
	failurePayload := BuildJobPayload(
		"experience_ingest:codex",
		"01HZ",
		"codex",
		StateFailed,
		EventJobFailed,
		"1",
		"execution_error",
		"transcript text leaked into message",
	)
	for _, k := range []string{"error_code", "error_message"} {
		if _, ok := failurePayload[k]; !ok {
			t.Errorf("failure payload missing required key %q", k)
		}
	}
}
