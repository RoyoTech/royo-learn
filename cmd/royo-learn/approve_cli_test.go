package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestCLI_ApprovalGate drives the whole Recorrido C contract through the public
// CLI: a shared-scope publication requires approval, is blocked without one, and
// proceeds once a matching approval_id is supplied. Nothing here writes SQL.
func TestCLI_ApprovalGate(t *testing.T) {
	root := initProject(t)

	captured := captureJSON(t,
		"--project-root", root,
		"--title", "Shared knowledge needs human approval",
		"--context", "Recorrido C closes the governance hole",
		"--observation", "Publishing to shared scope must require explicit approval",
		"--lesson", "Sensitive destinations are gated on a preview-bound approval",
		"--destination", "shared",
		"--evidence-level", "moderate",
		"--json",
	)
	learningID, _ := captured["learning_id"].(string)
	if learningID == "" {
		t.Fatal("capture returned no learning_id")
	}

	// Attach evidence so the learning can be approved (public path).
	runOK(t, "evidence", "add", learningID,
		"--project-root", root,
		"--kind", "test",
		"--summary", "The CLI approval-gate test reproduces the flow",
		"--content", "--- PASS: TestCLI_ApprovalGate",
		"--json")

	// Curate to approved for the shared destination.
	runOK(t, "curate",
		"--project-root", root,
		"--learning-id", learningID,
		"--action", "approve_shared_knowledge",
		"--rationale", "Evidence is attached and the shared lesson is reusable",
		"--json")

	// Preview: shared scope must require approval.
	preview := runJSON(t, "preview",
		"--project-root", root,
		"--learning-id", learningID,
		"--json")
	if requires, _ := preview["requires_approval"].(bool); !requires {
		t.Fatalf("shared-scope preview must require approval: %v", preview)
	}
	previewHash, _ := preview["preview_hash"].(string)
	if previewHash == "" {
		t.Fatal("preview returned no preview_hash")
	}

	// Publish without approval → blocked.
	var pubOut, pubErr bytes.Buffer
	if code := run([]string{"publish",
		"--project-root", root,
		"--learning-id", learningID,
		"--preview-hash", previewHash,
		"--json",
	}, &pubOut, &pubErr); code == exitSuccess {
		t.Fatalf("publish without approval succeeded; the gate is not enforced\nstdout: %s", pubOut.String())
	} else if !strings.Contains(pubErr.String(), "approval") {
		t.Fatalf("blocked publish error should mention approval: %s", pubErr.String())
	}

	// Approve the exact preview.
	appr := runJSON(t, "approve", learningID,
		"--project-root", root,
		"--preview-hash", previewHash,
		"--approved-by", "release-owner",
		"--reason", "Reviewed the shared knowledge diff",
		"--approval-evidence", "https://example.test/approvals/42",
		"--json")
	approvalID, _ := appr["approval_id"].(string)
	if approvalID == "" {
		t.Fatalf("approve returned no approval_id: %v", appr)
	}

	// Publish with the matching approval_id → accepted.
	published := runJSON(t, "publish",
		"--project-root", root,
		"--learning-id", learningID,
		"--preview-hash", previewHash,
		"--approval-id", approvalID,
		"--json")
	if status, _ := published["status"].(string); status != "completed" {
		t.Fatalf("publication status = %q, want completed (%v)", status, published)
	}

	// A different, unrelated approval_id must not authorize this preview.
	var reuseOut, reuseErr bytes.Buffer
	if code := run([]string{"publish",
		"--project-root", root,
		"--learning-id", learningID,
		"--preview-hash", previewHash,
		"--approval-id", "not-the-real-approval",
		"--json",
	}, &reuseOut, &reuseErr); code == exitSuccess {
		t.Fatal("publish accepted a mismatched approval_id")
	}
}

// runOK runs a CLI command and fails the test on a non-success exit code.
func runOK(t *testing.T, args ...string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("%v exit = %d\nstderr: %s", args, code, stderr.String())
	}
}

// runJSON runs a CLI command, requires success, and decodes its --json payload.
func runJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("%v exit = %d\nstderr: %s", args, code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("%v stdout is not JSON: %v\n%s", args, err, stdout.String())
	}
	return out
}
