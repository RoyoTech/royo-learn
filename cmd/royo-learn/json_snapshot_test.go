package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Versioned JSON snapshots (plan §Tramo 5).
//
// The JSON payloads of learning, evidence, preview, approval, publication,
// occurrence, error and status are a PUBLIC contract: agents and Skills parse
// them. A renamed key, a dropped field or a changed type breaks every consumer
// silently, and no other test in this repo would notice.
//
// Each payload is captured through the public CLI — the surface a consumer
// actually sees — and compared against a golden file. Volatile values (ids,
// timestamps, hashes, temp paths, host details) are normalized to typed
// placeholders so the snapshot pins the SHAPE and the stable values (statuses,
// booleans, enums) without breaking on every run. Regenerate deliberately with:
//
//	go test ./cmd/royo-learn -run TestContract_JSONSnapshots -update-snapshots
//
// Regenerating is a contract change. If a diff appears that you did not intend,
// the code broke the contract — not the golden file.

var updateSnapshots = flag.Bool("update-snapshots", false, "rewrite the JSON contract snapshots")

func TestContract_JSONSnapshots(t *testing.T) {
	root := initProject(t)

	learning := captureJSON(t,
		"--project-root", root,
		"--title", "JSON payloads are a public contract",
		"--context", "Tramo 5 pins the shape agents parse",
		"--observation", "A renamed key breaks every consumer silently",
		"--lesson", "Every public payload carries a versioned snapshot",
		"--destination", "shared",
		"--evidence-level", "moderate",
		"--json",
	)
	learningID, _ := learning["learning_id"].(string)
	if learningID == "" {
		t.Fatal("capture returned no learning_id")
	}

	evidence := runJSON(t, "evidence", "add", learningID,
		"--project-root", root,
		"--kind", "test",
		"--summary", "The snapshot test captures every public payload",
		"--content", "--- PASS: TestContract_JSONSnapshots",
		"--json")

	runOK(t, "curate",
		"--project-root", root,
		"--learning-id", learningID,
		"--action", "approve_shared_knowledge",
		"--rationale", "The lesson is reusable and carries evidence",
		"--json")

	preview := runJSON(t, "preview",
		"--project-root", root,
		"--learning-id", learningID,
		"--json")
	previewHash, _ := preview["preview_hash"].(string)
	if previewHash == "" {
		t.Fatal("preview returned no preview_hash")
	}

	// The error envelope, captured from a real refusal rather than constructed
	// by hand: publishing a shared learning with no approval must be blocked.
	var errOut, errErr bytes.Buffer
	if code := run([]string{"publish",
		"--project-root", root,
		"--learning-id", learningID,
		"--preview-hash", previewHash,
		"--json",
	}, &errOut, &errErr); code == exitSuccess {
		t.Fatal("publish without approval succeeded; cannot capture the error envelope")
	}
	errEnvelope := decodeJSON(t, "error", errErr.Bytes())

	approval := runJSON(t, "approve", learningID,
		"--project-root", root,
		"--preview-hash", previewHash,
		"--approved-by", "release-owner",
		"--reason", "Reviewed the diff",
		"--approval-evidence", "https://example.test/approvals/1",
		"--json")

	publication := runJSON(t, "publish",
		"--project-root", root,
		"--learning-id", learningID,
		"--preview-hash", previewHash,
		"--approval-id", stringField(approval, "approval_id"),
		"--apply",
		"--json")

	occurrence := runJSON(t, "occurrence",
		"--project-root", root,
		"--learning-id", learningID,
		"--summary", "the same gap recurred",
		"--json")

	status := runJSON(t, "status", learningID, "--project-root", root, "--json")

	// `get` returns the full learning projection, which is a richer contract
	// than capture's acknowledgement — both are public, so both are pinned.
	got := runJSON(t, "get", learningID, "--project-root", root, "--json")

	for _, tc := range []struct {
		name    string
		payload any
	}{
		{"learning-capture", learning},
		{"learning-get", got},
		{"evidence", evidence},
		{"preview", preview},
		{"approval", approval},
		{"publication", publication},
		{"occurrence", occurrence},
		{"status", status},
		{"error", errEnvelope},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertSnapshot(t, tc.name, tc.payload, root)
		})
	}
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func decodeJSON(t *testing.T, what string, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("%s payload is not a JSON object: %v\n%s", what, err, string(data))
	}
	return m
}

// assertSnapshot normalizes a payload and compares it to its golden file.
func assertSnapshot(t *testing.T, name string, payload any, root string) {
	t.Helper()

	normalized := normalizeSnapshot(payload, root)
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	data = append(data, '\n')

	golden := filepath.Join("testdata", "json", name+".json")
	if *updateSnapshots {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(golden, data, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("missing snapshot %s: %v\nregenerate with: go test ./cmd/royo-learn -run TestContract_JSONSnapshots -update-snapshots", golden, err)
	}
	if string(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))) != string(data) {
		t.Fatalf("the %s JSON contract changed.\n--- want (%s)\n%s\n--- got\n%s\n"+
			"If this change is intentional it is a CONTRACT change: regenerate with "+
			"-update-snapshots and say so in the commit.",
			name, golden, want, data)
	}
}

// These match ANYWHERE in a string, not just as a whole value: payloads such as
// the preview diff embed ids and hashes inside larger text, and a snapshot that
// only normalized whole values would differ on every run.
var (
	uuidRe      = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	sha256Re    = regexp.MustCompile(`(?i)\b[0-9a-f]{64}\b`)
	timestampRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?`)
	shortHexRe  = regexp.MustCompile(`(?i)^[0-9a-f]{12,63}$`)
)

// normalizeSnapshot replaces values that legitimately differ between runs with
// typed placeholders, so the golden pins the contract instead of the run. Keys,
// nesting, types and stable values (statuses, enums, booleans) survive intact —
// those are the parts a consumer depends on.
func normalizeSnapshot(v any, root string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out[k] = normalizeSnapshot(t[k], root)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = normalizeSnapshot(t[i], root)
		}
		return out
	case string:
		return normalizeString(t, root)
	case float64:
		// Counts and durations vary; the contract is that the key is a number.
		return "<number>"
	default:
		return v
	}
}

func normalizeString(s, root string) string {
	if s == "" {
		return ""
	}
	if root != "" {
		s = strings.ReplaceAll(s, root, "<project-root>")
		s = strings.ReplaceAll(s, strings.ReplaceAll(root, `\`, `/`), "<project-root>")
	}
	s = uuidRe.ReplaceAllString(s, "<uuid>")
	s = sha256Re.ReplaceAllString(s, "<sha256>")
	s = timestampRe.ReplaceAllString(s, "<timestamp>")

	// Path separators are OS-native inside payloads, and this snapshot has to
	// mean the same thing on Linux, macOS and Windows CI.
	s = strings.ReplaceAll(s, `\`, "/")

	if shortHexRe.MatchString(s) {
		return "<hex>"
	}
	// A path under the project root is already portable once the root is
	// replaced; only a path pointing outside it (a home config, a temp file)
	// is still machine-specific.
	if filepath.IsAbs(s) {
		return "<abs-path>"
	}
	return s
}

// TestContract_JSONSnapshotsCoverEveryPublicPayload keeps the snapshot set
// honest: the plan names eight payloads, and a snapshot file that quietly
// disappears would otherwise take its contract coverage with it.
func TestContract_JSONSnapshotsCoverEveryPublicPayload(t *testing.T) {
	required := []string{
		"learning-capture", "learning-get", "evidence", "preview", "approval",
		"publication", "occurrence", "status", "error",
	}
	for _, name := range required {
		path := filepath.Join("testdata", "json", name+".json")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s", fmt.Sprintf("missing JSON contract snapshot for %q (%s): %v", name, path, err))
		}
	}
}
