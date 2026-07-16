package main

import (
	"bytes"
	"testing"
)

// TestCLI_RollbackRevokesPublishedState is the inverse of what Recorrido D
// proved. D proved that a FAILED publication never leaves a false `published`.
// Nobody ever proved that a SUCCESSFUL rollback revokes it.
//
// Both halves of D18 are asserted here, through the public CLI only:
//
//  1. After a successful publish, SQLite says `published` and the derived
//     Markdown record must say the same: `doctor` is clean WITHOUT anyone
//     running `rebuild-index` first. A derived artifact that goes stale the
//     instant the truth changes is not a derived artifact, it is a lie.
//  2. After a successful rollback, the learning is no longer `published` — the
//     published file is gone, so the state that claims otherwise is false. It
//     returns to `approved` (D18), and `doctor` is clean again.
//
// Nothing here opens the database or writes SQL: a test that can only pass by
// reaching around the public interface proves nothing about the product.
func TestCLI_RollbackRevokesPublishedState(t *testing.T) {
	root := initProject(t)

	captured := captureJSON(t,
		"--project-root", root,
		"--title", "A rolled back publication must not stay published",
		"--context", "Tramo 4 §4.7 exposed a state that outlives the file it describes",
		"--observation", "RollbackPublication updated the publication but never the learning",
		"--lesson", "A successful rollback revokes the published state of the learning",
		"--destination", "shared",
		"--evidence-level", "moderate",
		"--json",
	)
	learningID, _ := captured["learning_id"].(string)
	if learningID == "" {
		t.Fatal("capture returned no learning_id")
	}

	runOK(t, "evidence", "add", learningID,
		"--project-root", root,
		"--kind", "test",
		"--summary", "This test reproduces the false published state",
		"--content", "--- FAIL: cli-sensitive/final-doctor [doctor] exited 1",
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

	appr := runJSON(t, "approve", learningID,
		"--project-root", root,
		"--preview-hash", previewHash,
		"--approved-by", "release-owner",
		"--reason", "Reviewed the diff",
		"--approval-evidence", "https://example.test/approvals/18",
		"--json")
	approvalID, _ := appr["approval_id"].(string)
	if approvalID == "" {
		t.Fatalf("approve returned no approval_id: %v", appr)
	}

	published := runJSON(t, "publish",
		"--project-root", root,
		"--learning-id", learningID,
		"--preview-hash", previewHash,
		"--approval-id", approvalID,
		"--apply",
		"--json")
	if status, _ := published["status"].(string); status != "completed" {
		t.Fatalf("publication status = %q, want completed (%v)", status, published)
	}
	publicationID, _ := published["publication_id"].(string)
	if publicationID == "" {
		t.Fatalf("publish returned no publication_id: %v", published)
	}

	// D18(b): publish mutated the truth, so the record must already agree with
	// it. No rebuild-index is allowed to run before this check — the healthy
	// path must be healthy on its own.
	assertDoctorClean(t, root, "after publish")

	if got := learningStatus(t, root, learningID); got != "published" {
		t.Fatalf("learning status after publish = %q, want published", got)
	}

	runOK(t, "rollback",
		"--project-root", root,
		"--journal-id", publicationID,
		"--json")

	// D18(a): the file is restored, so `published` is no longer true.
	if got := learningStatus(t, root, learningID); got == "published" {
		t.Fatal("learning is still published after a successful rollback: " +
			"the published file is gone but the state still claims it exists")
	} else if got != "approved" {
		t.Fatalf("learning status after rollback = %q, want approved (D18)", got)
	}

	assertDoctorClean(t, root, "after rollback")
}

// learningStatus reads a learning's status through the public `get` command.
func learningStatus(t *testing.T, root, learningID string) string {
	t.Helper()
	m := runJSON(t, "get", learningID, "--project-root", root, "--json")
	status, _ := m["status"].(string)
	return status
}

// assertDoctorClean requires `doctor` to exit 0. It reports the full JSON on
// failure: the exit code alone hides which check failed and why.
func assertDoctorClean(t *testing.T, root, when string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"doctor", "--project-root", root, "--json"}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("doctor %s exit = %d, want %d\nstdout: %s\nstderr: %s",
			when, code, exitSuccess, stdout.String(), stderr.String())
	}
}
