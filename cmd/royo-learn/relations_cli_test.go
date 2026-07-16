package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestCLI_RelationProposeThenConfirm proves the plan 4.5 lifecycle through the
// public CLI: `curate --action relate` proposes a relation, and
// `curate --action confirm-relation` confirms it. Nothing reaches around the
// interface with direct SQL.
func TestCLI_RelationProposeThenConfirm(t *testing.T) {
	root := initProject(t)

	src := captureForRetrieval(t, root, "Relation source learning")
	tgt := captureForRetrieval(t, root, "Relation target learning")

	// Propose.
	var out, errBuf bytes.Buffer
	code := run([]string{
		"curate",
		"--project-root", root,
		"--learning-id", src,
		"--action", "relate",
		"--target-id", tgt,
		"--relation", "duplicate_of",
		"--rationale", "these look like the same lesson",
		"--json",
	}, &out, &errBuf)
	if code != exitSuccess {
		t.Fatalf("relate exit = %d, want %d\nstderr: %s", code, exitSuccess, errBuf.String())
	}
	var proposed map[string]any
	if err := json.Unmarshal(out.Bytes(), &proposed); err != nil {
		t.Fatalf("relate stdout is not JSON: %v\n%s", err, out.String())
	}
	if proposed["status"] != "proposed" {
		t.Fatalf("relate status = %v, want proposed", proposed["status"])
	}
	relationID, _ := proposed["relation_id"].(string)
	if relationID == "" {
		t.Fatal("relate returned no relation_id")
	}

	// Confirm.
	out.Reset()
	errBuf.Reset()
	code = run([]string{
		"curate",
		"--project-root", root,
		"--action", "confirm-relation",
		"--relation-id", relationID,
		"--json",
	}, &out, &errBuf)
	if code != exitSuccess {
		t.Fatalf("confirm-relation exit = %d, want %d\nstderr: %s", code, exitSuccess, errBuf.String())
	}
	var confirmed map[string]any
	if err := json.Unmarshal(out.Bytes(), &confirmed); err != nil {
		t.Fatalf("confirm-relation stdout is not JSON: %v\n%s", err, out.String())
	}
	if confirmed["status"] != "confirmed" {
		t.Fatalf("confirm-relation status = %v, want confirmed", confirmed["status"])
	}
}

// TestCLI_ConfirmUnknownRelationFails proves confirming a missing relation is a
// visible CLI error, never a silent success.
func TestCLI_ConfirmUnknownRelationFails(t *testing.T) {
	root := initProject(t)
	var out, errBuf bytes.Buffer
	code := run([]string{
		"curate",
		"--project-root", root,
		"--action", "confirm-relation",
		"--relation-id", "does-not-exist",
		"--json",
	}, &out, &errBuf)
	if code == exitSuccess {
		t.Fatalf("confirm-relation of an unknown relation should fail, got exit 0\nstdout: %s", out.String())
	}
}
