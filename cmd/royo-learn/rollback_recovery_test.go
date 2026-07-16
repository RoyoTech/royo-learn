package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/publish"
	"agent-royo-learn/internal/storage"
)

func TestRollbackListDiscoversInterruptedPublication(t *testing.T) {
	root := t.TempDir()
	learningID := domain.LearningID(setupApprovedLearning(t, root))
	db, err := storage.Open(filepath.Join(root, ".royo-learn", "royo-learn.db"))
	if err != nil {
		t.Fatal(err)
	}
	publication := &domain.Publication{
		ID: "interrupted-publication", LearningID: learningID, PreviewHash: "preview",
		Targets: []domain.TargetEntry{{Root: "skills", Path: "demo/SKILL.md", Operation: domain.OpCreate}},
		Rollback: []domain.RollbackEntry{{Path: "skills/demo/SKILL.md", RecoveryState: domain.RecoveryPending}},
		Status: domain.PubStatusInProgress, StartedAt: time.Now().UTC(),
	}
	if err := storage.WithTx(context.Background(), db, func(tx *sql.Tx) error {
		return storage.SavePublication(context.Background(), tx, publication)
	}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	journal, err := publish.NewJournal(root, filepath.Join(root, ".royo-learn"))
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Append(publish.JournalEntry{PublicationID: string(publication.ID), LearningID: string(learningID), RollbackStatus: "attempting"}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if exit := run([]string{"rollback", "--project-root", root, "--list", "--json"}, &stdout, &stderr); exit != exitSuccess {
		t.Fatalf("rollback --list exit=%d stderr=%s", exit, stderr.String())
	}
	var candidates []publish.RecoveryCandidate
	if err := json.Unmarshal(stdout.Bytes(), &candidates); err != nil {
		t.Fatalf("decode candidates: %v output=%s", err, stdout.String())
	}
	if len(candidates) != 1 || candidates[0].PublicationID != publication.ID || candidates[0].JournalStatus != "attempting" {
		t.Fatalf("candidates = %+v", candidates)
	}
}
