package claudecode

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/storage/storagetest"
)

func TestCursorCheckpoint_AcceptsTurnUUIDVariants(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "string", value: "7", want: "7"},
		{name: "int", value: int(8), want: "8"},
		{name: "int64", value: int64(9), want: "9"},
		{name: "float64", value: float64(10), want: "10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionID, turnUUID, ok := cursorCheckpoint(map[string]any{
				"last_session_id": "session-001",
				"last_turn_uuid":  tt.value,
			})
			if !ok || sessionID != "session-001" || turnUUID != tt.want {
				t.Fatalf("cursorCheckpoint(%T(%v)) = %q, %q, %t; want session-001, %q, true", tt.value, tt.value, sessionID, turnUUID, ok, tt.want)
			}
		})
	}

	if _, _, ok := cursorCheckpoint(map[string]any{
		"last_session_id": "session-001",
		"last_turn_uuid":  true,
	}); ok {
		t.Fatal("cursorCheckpoint accepted an unsupported turn UUID type")
	}
}

func TestScan_RescanAfterIngestIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db, _ := storagetest.OpenMemory(t)
	req := fixtureScanRequest(t)
	project := saveTestProject(t, db, req.ProjectRoot)
	adapter := NewAdapter()
	service := experience.NewService(db)

	first, err := adapter.Scan(ctx, req)
	if err != nil {
		t.Fatalf("first Scan: %v", err)
	}
	if len(first.Envelopes) != 2 {
		t.Fatalf("first Scan envelopes = %d, want 2", len(first.Envelopes))
	}
	for i, envelope := range first.Envelopes {
		result, err := service.IngestEnvelope(ctx, project.ID, envelope)
		if err != nil {
			t.Fatalf("first ingest envelope %d: %v", i, err)
		}
		if !result.Created {
			t.Fatalf("first ingest envelope %d = %#v, want Created", i, result)
		}
	}

	second, err := adapter.Scan(ctx, req)
	if err != nil {
		t.Fatalf("second Scan: %v", err)
	}
	if len(second.Envelopes) != len(first.Envelopes) {
		t.Fatalf("second Scan envelopes = %d, want %d", len(second.Envelopes), len(first.Envelopes))
	}
	ingestedTurns := 0
	for i, envelope := range second.Envelopes {
		if envelope.Session.ExternalID != first.Envelopes[i].Session.ExternalID || envelope.Turn.ExternalID != first.Envelopes[i].Turn.ExternalID {
			t.Fatalf("envelope %d identity drifted: first=%s/%s second=%s/%s", i,
				first.Envelopes[i].Session.ExternalID, first.Envelopes[i].Turn.ExternalID,
				envelope.Session.ExternalID, envelope.Turn.ExternalID)
		}
		result, err := service.IngestEnvelope(ctx, project.ID, envelope)
		if err != nil {
			t.Fatalf("second ingest envelope %d: %v", i, err)
		}
		if !result.Idempotent {
			t.Fatalf("second ingest envelope %d = %#v, want Idempotent", i, result)
		}
		if result.Created {
			ingestedTurns++
		}
	}
	if ingestedTurns != 0 {
		t.Fatalf("second ingest created %d turns, want 0", ingestedTurns)
	}
}

func TestScan_CursorPersistsAcrossDatabaseRestart(t *testing.T) {
	ctx := context.Background()
	req := fixtureScanRequest(t)
	dbPath := filepath.Join(t.TempDir(), "experience.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
		}
	})
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	project := saveTestProject(t, db, req.ProjectRoot)
	first, err := NewAdapter().Scan(ctx, req)
	if err != nil {
		t.Fatalf("first Scan: %v", err)
	}
	if !reflect.DeepEqual(first.NextCursor, map[string]any{
		"last_session_id": "session-001",
		"last_turn_uuid":  "turn-user-001",
	}) {
		t.Fatalf("first NextCursor = %#v", first.NextCursor)
	}

	service := experience.NewService(db)
	for i, envelope := range first.Envelopes {
		cursorJSON := marshalCursor(t, cursorForEnvelope(envelope))
		result, err := service.Ingest(ctx, project.ID, &experience.IngestInput{
			Envelope:       envelope,
			SourceInstance: req.Instance.JSONLPath,
			CursorJSON:     cursorJSON,
			SourceOrder:    int64(i + 1),
		})
		if err != nil {
			t.Fatalf("ingest envelope %d with cursor: %v", i, err)
		}
		if result.Cursor == nil || result.Cursor.CursorJSON != cursorJSON {
			t.Fatalf("ingest envelope %d cursor = %#v, want %s", i, result.Cursor, cursorJSON)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	db = nil
	db, err = storage.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}

	var persistedCursor string
	if err := storage.WithTx(ctx, db, func(tx *sql.Tx) error {
		cursor, err := storage.FindIngestionCursor(ctx, tx, project.ID, domain.SourceClaudeCode, req.Instance.JSONLPath)
		if err != nil {
			return err
		}
		if cursor != nil {
			persistedCursor = cursor.CursorJSON
		}
		return nil
	}); err != nil {
		t.Fatalf("load persisted cursor: %v", err)
	}
	var cursor map[string]any
	if err := json.Unmarshal([]byte(persistedCursor), &cursor); err != nil {
		t.Fatalf("decode persisted cursor %q: %v", persistedCursor, err)
	}
	if !reflect.DeepEqual(cursor, first.NextCursor) {
		t.Fatalf("persisted cursor = %#v, want %#v", cursor, first.NextCursor)
	}

	req.Cursor = cursor
	second, err := NewAdapter().Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan after restart: %v", err)
	}
	if len(second.Envelopes) != 0 {
		t.Fatalf("Scan after restart emitted %d envelopes, want 0", len(second.Envelopes))
	}
}

func fixtureScanRequest(t *testing.T) ScanRequest {
	t.Helper()
	path, err := filepath.Abs("testdata/fixtures/session-001.jsonl")
	if err != nil {
		t.Fatalf("fixture path: %v", err)
	}
	root := filepath.Dir(path)
	return ScanRequest{
		ProjectRoot: root,
		Instance: SourceInstance{
			Source:      domain.SourceClaudeCode,
			ProjectRoot: root,
			JSONLPath:   path,
			Schema:      SchemaTag,
		},
	}
}

func saveTestProject(t *testing.T, db *storage.DB, root string) *domain.Project {
	t.Helper()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	project := &domain.Project{
		ID:            "claudecode-idempotency",
		ProjectKey:    "claudecode-idempotency",
		DisplayName:   "Claude Code Idempotency",
		CanonicalPath: root,
		Fingerprint:   "claudecode-idempotency-fixture",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := storage.WithTx(context.Background(), db, func(tx *sql.Tx) error {
		return storage.SaveProject(context.Background(), tx, project)
	}); err != nil {
		t.Fatalf("save project: %v", err)
	}
	return project
}

func cursorForEnvelope(envelope experience.ExperienceEnvelope) map[string]any {
	return map[string]any{
		"last_session_id": envelope.Session.ExternalID,
		"last_turn_uuid":  envelope.Turn.ExternalID,
	}
}

func marshalCursor(t *testing.T, cursor map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(cursor)
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}
	return string(encoded)
}
