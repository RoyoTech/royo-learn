package experience

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestAcceptanceValidEnvelopeCreatesSessionAndTurn(t *testing.T) {
	db, project, root := newExperienceTestDB(t)
	result, err := NewService(db).IngestEnvelope(context.Background(), project.ID, validEnvelope(root, "accept-create"))
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Session == nil || result.Turn == nil || !result.Created {
		t.Fatalf("result = %#v", result)
	}
}

func TestAcceptanceExactRetryDoesNotDuplicate(t *testing.T) {
	db, project, root := newExperienceTestDB(t)
	svc := NewService(db)
	envelope := validEnvelope(root, "accept-retry")
	first, err := svc.IngestEnvelope(context.Background(), project.ID, envelope)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.IngestEnvelope(context.Background(), project.ID, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Idempotent || first.Turn.ID != second.Turn.ID {
		t.Fatalf("retry = %#v", second)
	}
	assertExperienceCounts(t, db, 1, 1, 0, 0)
}

func TestAcceptanceRevisionBumpUpdatesSafely(t *testing.T) {
	db, project, root := newExperienceTestDB(t)
	svc := NewService(db)
	envelope := validEnvelope(root, "accept-revision")
	if _, err := svc.IngestEnvelope(context.Background(), project.ID, envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Turn.SourceRevision = "revision-2"
	envelope.Turn.AssistantText = "updated"
	result, err := svc.IngestEnvelope(context.Background(), project.ID, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.Turn.SourceRevision != "revision-2" {
		t.Fatalf("revision result = %#v", result)
	}
	assertExperienceCounts(t, db, 1, 1, 0, 0)
}

func TestAcceptanceSecretsNeverReachSinks(t *testing.T) {
	db, project, root := newExperienceTestDB(t)
	secret := "password=super-secret"
	envelope := validEnvelope(root, "accept-secret")
	envelope.Turn.UserText = secret
	envelope.Turn.AssistantText = "cookie=" + secret
	result, err := NewService(db, Config{KnownSecrets: []string{"super-secret"}}).IngestEnvelope(context.Background(), project.ID, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Turn.UserDigest, "super-secret") || strings.Contains(result.Turn.AssistantDigest, "super-secret") {
		t.Fatal("secret reached digest output")
	}
	var audit string
	if err := db.DB.QueryRow(`SELECT details_json FROM audit_events ORDER BY sequence DESC LIMIT 1`).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(audit, "super-secret") {
		t.Fatal("secret reached audit")
	}
}

func TestAcceptanceCursorAdvancesOnlyAfterCommit(t *testing.T) {
	db, project, root := newExperienceTestDB(t)
	svc := NewService(db, Config{Now: fixedClock()})
	svc.commitTx = func(*sql.Tx) error { return context.Canceled }
	input := &IngestInput{Envelope: validEnvelope(root, "accept-rollback"), SourceInstance: "fixture", CursorJSON: `{"offset":1}`, SourceOrder: 1}
	if _, err := svc.Ingest(context.Background(), project.ID, input); err == nil {
		t.Fatal("expected commit failure")
	}
	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM ingestion_cursors WHERE project_id = ?`, project.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("cursor count = %d, want 0", count)
	}
}
