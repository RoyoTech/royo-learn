package storage

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

func TestExperienceMigrationSchema(t *testing.T) {
	db, _ := setupTestDB(t)

	wantColumns := map[string][]string{
		"experience_sessions": {"id", "project_id", "source", "external_session_id", "locator_json", "started_at", "updated_at", "closed_at", "metadata_sha256", "created_at"},
		"experience_turns":    {"id", "session_id", "external_turn_id", "sequence", "status", "fingerprint", "user_digest", "assistant_digest", "tool_calls_digest", "safe_summary", "occurred_at", "stable_at", "ingested_at", "source_revision", "redacted"},
		"experience_events":   {"id", "project_id", "turn_id", "kind", "summary", "observation", "outcome", "fingerprint", "evidence_json", "detector_json", "confidence", "created_at"},
		"ingestion_cursors":   {"project_id", "source", "source_instance", "cursor_json", "last_successful_at", "last_attempt_at", "last_error_code", "last_error_message", "input_digest", "revision"},
	}
	for table, want := range wantColumns {
		t.Run(table, func(t *testing.T) {
			rows, err := db.DB.Query("PRAGMA table_info(" + table + ")")
			if err != nil {
				t.Fatalf("table_info: %v", err)
			}
			defer rows.Close()
			var got []string
			for rows.Next() {
				var cid, notNull, primaryKey int
				var name, typ string
				var defaultValue any
				if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
					t.Fatalf("scan table_info: %v", err)
				}
				got = append(got, name)
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("table_info rows: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("columns = %v, want %v", got, want)
			}
		})
	}

	indexes := []string{
		"experience_turns_session_sequence",
		"experience_turns_fingerprint",
		"experience_events_project_kind",
		"experience_events_fingerprint",
		"experience_events_turn",
	}
	for _, index := range indexes {
		var count int
		if err := db.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&count); err != nil {
			t.Fatalf("query index %q: %v", index, err)
		}
		if count != 1 {
			t.Errorf("index %q count = %d, want 1", index, count)
		}
	}
	foreignKeys := map[string]int{
		"experience_sessions": 1,
		"experience_turns":    1,
		"experience_events":   2,
		"ingestion_cursors":   1,
	}
	for table, want := range foreignKeys {
		rows, err := db.DB.Query("PRAGMA foreign_key_list(" + table + ")")
		if err != nil {
			t.Fatalf("foreign_key_list(%s): %v", table, err)
		}
		count := 0
		for rows.Next() {
			count++
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close foreign keys for %s: %v", table, err)
		}
		if count != want {
			t.Errorf("foreign keys for %s = %d, want %d", table, count, want)
		}
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var applied int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 4`).Scan(&applied); err != nil {
		t.Fatalf("query migration 004: %v", err)
	}
	if applied != 1 {
		t.Fatalf("migration 004 rows = %d, want 1", applied)
	}
}

func TestOpenEnablesForeignKeysOnPooledConnections(t *testing.T) {
	db, _ := setupTestDB(t)
	db.DB.SetMaxOpenConns(2)
	db.DB.SetMaxIdleConns(2)
	ctx := context.Background()

	first, err := db.DB.Conn(ctx)
	if err != nil {
		t.Fatalf("first pooled connection: %v", err)
	}
	defer first.Close()
	second, err := db.DB.Conn(ctx)
	if err != nil {
		t.Fatalf("second pooled connection: %v", err)
	}
	defer second.Close()
	if got := db.DB.Stats().OpenConnections; got < 2 {
		t.Fatalf("open pooled connections = %d, want at least 2", got)
	}

	for name, conn := range map[string]*sql.Conn{"first": first, "second": second} {
		t.Run(name, func(t *testing.T) {
			var enabled int
			if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
				t.Fatalf("query foreign_keys: %v", err)
			}
			if enabled != 1 {
				t.Fatalf("foreign_keys = %d, want 1", enabled)
			}
		})
	}
}

func TestExperienceRepositoryPreservesNullTimestamps(t *testing.T) {
	db, project := setupTestDB(t)
	ctx := context.Background()
	session, turn, _, cursor := newExperienceBundle(project.ID, "nulls")
	session.StartedAt = nil
	session.ClosedAt = nil
	turn.StableAt = nil
	cursor.LastSuccessfulAt = nil
	cursor.LastAttemptAt = nil

	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		if err := SaveExperienceSession(ctx, tx, session); err != nil {
			return err
		}
		if err := SaveExperienceTurn(ctx, tx, turn); err != nil {
			return err
		}
		return SaveIngestionCursor(ctx, tx, cursor)
	}); err != nil {
		t.Fatalf("save nullable fields: %v", err)
	}
	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		gotSession, err := FindExperienceSession(ctx, tx, project.ID, session.Source, session.ExternalSessionID)
		if err != nil || !reflect.DeepEqual(gotSession, session) {
			t.Fatalf("nullable session = %#v, %v; want %#v", gotSession, err, session)
		}
		gotTurn, err := FindExperienceTurn(ctx, tx, session.ID, turn.ExternalTurnID)
		if err != nil || !reflect.DeepEqual(gotTurn, turn) {
			t.Fatalf("nullable turn = %#v, %v; want %#v", gotTurn, err, turn)
		}
		gotCursor, err := FindIngestionCursor(ctx, tx, project.ID, cursor.Source, cursor.SourceInstance)
		if err != nil || !reflect.DeepEqual(gotCursor, cursor) {
			t.Fatalf("nullable cursor = %#v, %v; want %#v", gotCursor, err, cursor)
		}
		return nil
	}); err != nil {
		t.Fatalf("read nullable fields: %v", err)
	}
}

func TestExperienceRepositoryRoundTripsAndUpdates(t *testing.T) {
	db, project := setupTestDB(t)
	ctx := context.Background()
	session, turn, event, cursor := newExperienceBundle(project.ID, "one")

	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		if err := SaveExperienceSession(ctx, tx, session); err != nil {
			return err
		}
		if err := SaveExperienceTurn(ctx, tx, turn); err != nil {
			return err
		}
		if err := SaveExperienceEvent(ctx, tx, event); err != nil {
			return err
		}
		return SaveIngestionCursor(ctx, tx, cursor)
	}); err != nil {
		t.Fatalf("save experience bundle: %v", err)
	}

	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		gotSession, err := FindExperienceSession(ctx, tx, project.ID, session.Source, session.ExternalSessionID)
		if err != nil || !reflect.DeepEqual(gotSession, session) {
			t.Fatalf("session = %#v, %v; want %#v", gotSession, err, session)
		}
		gotTurn, err := FindExperienceTurn(ctx, tx, session.ID, turn.ExternalTurnID)
		if err != nil || !reflect.DeepEqual(gotTurn, turn) {
			t.Fatalf("turn = %#v, %v; want %#v", gotTurn, err, turn)
		}
		gotEvent, err := FindExperienceEvent(ctx, tx, event.ID)
		if err != nil || !reflect.DeepEqual(gotEvent, event) {
			t.Fatalf("event = %#v, %v; want %#v", gotEvent, err, event)
		}
		gotCursor, err := FindIngestionCursor(ctx, tx, project.ID, cursor.Source, cursor.SourceInstance)
		if err != nil || !reflect.DeepEqual(gotCursor, cursor) {
			t.Fatalf("cursor = %#v, %v; want %#v", gotCursor, err, cursor)
		}
		return nil
	}); err != nil {
		t.Fatalf("read experience bundle: %v", err)
	}

	session.Locator.Offset++
	session.UpdatedAt = session.UpdatedAt.Add(time.Minute)
	session.MetadataSHA256 = "metadata-updated"
	turn.Status = domain.TurnSuperseded
	turn.Fingerprint = "fingerprint-updated"
	turn.SourceRevision = "revision-2"
	turn.StableAt = timePtr(turn.OccurredAt.Add(time.Minute))
	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		if err := UpdateExperienceSession(ctx, tx, session); err != nil {
			return err
		}
		return UpdateExperienceTurn(ctx, tx, turn)
	}); err != nil {
		t.Fatalf("update experience entities: %v", err)
	}
	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		gotSession, err := FindExperienceSession(ctx, tx, project.ID, session.Source, session.ExternalSessionID)
		if err != nil || !reflect.DeepEqual(gotSession, session) {
			t.Fatalf("updated session = %#v, %v; want %#v", gotSession, err, session)
		}
		gotTurn, err := FindExperienceTurn(ctx, tx, session.ID, turn.ExternalTurnID)
		if err != nil || !reflect.DeepEqual(gotTurn, turn) {
			t.Fatalf("updated turn = %#v, %v; want %#v", gotTurn, err, turn)
		}
		return nil
	}); err != nil {
		t.Fatalf("read updated entities: %v", err)
	}
}

func TestExperienceRepositoryConflicts(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *sql.Tx, domain.ProjectID) error
	}{
		{
			name: "session identity",
			run: func(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID) error {
				first, _, _, _ := newExperienceBundle(projectID, "duplicate")
				duplicate := *first
				duplicate.ID = domain.ExperienceSessionID(newUUID())
				if err := SaveExperienceSession(ctx, tx, first); err != nil {
					return err
				}
				return SaveExperienceSession(ctx, tx, &duplicate)
			},
		},
		{
			name: "turn identity",
			run: func(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID) error {
				session, first, _, _ := newExperienceBundle(projectID, "duplicate")
				duplicate := *first
				duplicate.ID = domain.ExperienceTurnID(newUUID())
				if err := SaveExperienceSession(ctx, tx, session); err != nil {
					return err
				}
				if err := SaveExperienceTurn(ctx, tx, first); err != nil {
					return err
				}
				return SaveExperienceTurn(ctx, tx, &duplicate)
			},
		},
		{
			name: "cursor identity",
			run: func(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID) error {
				_, _, _, first := newExperienceBundle(projectID, "duplicate")
				duplicate := *first
				if err := SaveIngestionCursor(ctx, tx, first); err != nil {
					return err
				}
				return SaveIngestionCursor(ctx, tx, &duplicate)
			},
		},
		{
			name: "event id",
			run: func(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID) error {
				session, turn, event, _ := newExperienceBundle(projectID, "duplicate")
				if err := SaveExperienceSession(ctx, tx, session); err != nil {
					return err
				}
				if err := SaveExperienceTurn(ctx, tx, turn); err != nil {
					return err
				}
				if err := SaveExperienceEvent(ctx, tx, event); err != nil {
					return err
				}
				return SaveExperienceEvent(ctx, tx, event)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, project := setupTestDB(t)
			err := WithTx(context.Background(), db, func(tx *sql.Tx) error {
				return tt.run(context.Background(), tx, project.ID)
			})
			assertDomainCode(t, err, domain.ErrExperienceRevisionConflict)
		})
	}
}

func TestSaveExperienceEventRejectsCrossProjectTurn(t *testing.T) {
	db, project := setupTestDB(t)
	ctx := context.Background()
	session, turn, event, _ := newExperienceBundle(project.ID, "provenance")

	otherProject := *project
	otherProject.ID = domain.ProjectID(newUUID())
	otherProject.ProjectKey = "other-project"
	otherProject.DisplayName = "Other Project"
	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		return SaveProject(ctx, tx, &otherProject)
	}); err != nil {
		t.Fatalf("save other project: %v", err)
	}
	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		if err := SaveExperienceSession(ctx, tx, session); err != nil {
			return err
		}
		return SaveExperienceTurn(ctx, tx, turn)
	}); err != nil {
		t.Fatalf("save session and turn: %v", err)
	}

	event.ProjectID = otherProject.ID
	err := WithTx(ctx, db, func(tx *sql.Tx) error {
		return SaveExperienceEvent(ctx, tx, event)
	})
	assertDomainCode(t, err, domain.ErrExperienceRevisionConflict)

	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM experience_events WHERE id = ?`, string(event.ID)).Scan(&count); err != nil {
		t.Fatalf("count rejected event: %v", err)
	}
	if count != 0 {
		t.Fatalf("rejected event count = %d, want 0", count)
	}
}

func TestUpdateIngestionCursorCompareAndSwap(t *testing.T) {
	db, project := setupTestDB(t)
	ctx := context.Background()
	_, _, _, cursor := newExperienceBundle(project.ID, "cursor")
	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		return SaveIngestionCursor(ctx, tx, cursor)
	}); err != nil {
		t.Fatalf("save cursor: %v", err)
	}

	updated := *cursor
	updated.CursorJSON = `{"offset":22}`
	updated.Revision = 2
	updated.LastSuccessfulAt = timePtr(time.Date(2026, 7, 21, 13, 0, 0, 456789000, time.UTC))
	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		return UpdateIngestionCursor(ctx, tx, &updated, 1)
	}); err != nil {
		t.Fatalf("update cursor: %v", err)
	}

	stale := updated
	stale.CursorJSON = `{"offset":999}`
	// Revision 2 is valid for expectedRevision 1, but the stored row is already at revision 2.
	err := WithTx(ctx, db, func(tx *sql.Tx) error {
		return UpdateIngestionCursor(ctx, tx, &stale, 1)
	})
	assertDomainCode(t, err, domain.ErrExperienceCursorConflict)

	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		got, err := FindIngestionCursor(ctx, tx, project.ID, updated.Source, updated.SourceInstance)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(got, &updated) {
			t.Fatalf("cursor after stale update = %#v, want %#v", got, &updated)
		}
		return nil
	}); err != nil {
		t.Fatalf("read cursor: %v", err)
	}
}

func TestExperienceWritesAreAtomic(t *testing.T) {
	tests := []struct {
		name       string
		finish     func(*sql.Tx) error
		wantCounts []int
	}{
		{name: "rollback", finish: (*sql.Tx).Rollback, wantCounts: []int{0, 0, 0, 0}},
		{name: "commit", finish: (*sql.Tx).Commit, wantCounts: []int{1, 1, 1, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, project := setupTestDB(t)
			ctx := context.Background()
			session, turn, event, cursor := newExperienceBundle(project.ID, tt.name)
			tx, err := db.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatalf("BeginTx: %v", err)
			}
			if err := SaveExperienceSession(ctx, tx, session); err != nil {
				t.Fatalf("save session: %v", err)
			}
			if err := SaveExperienceTurn(ctx, tx, turn); err != nil {
				t.Fatalf("save turn: %v", err)
			}
			if err := SaveExperienceEvent(ctx, tx, event); err != nil {
				t.Fatalf("save event: %v", err)
			}
			if err := SaveIngestionCursor(ctx, tx, cursor); err != nil {
				t.Fatalf("save cursor: %v", err)
			}
			if got := experienceTableCounts(t, db.DB); !reflect.DeepEqual(got, []int{0, 0, 0, 0}) {
				t.Fatalf("uncommitted table counts = %v, want all zero", got)
			}
			if err := tt.finish(tx); err != nil {
				t.Fatalf("finish transaction: %v", err)
			}
			got := experienceTableCounts(t, db.DB)
			if !reflect.DeepEqual(got, tt.wantCounts) {
				t.Fatalf("table counts = %v, want %v", got, tt.wantCounts)
			}
		})
	}
}

func TestExperienceRepositoriesValidateBeforeWriting(t *testing.T) {
	db, project := setupTestDB(t)
	tests := []struct {
		name string
		save func(context.Context, *sql.Tx) error
	}{
		{name: "session", save: func(ctx context.Context, tx *sql.Tx) error {
			return SaveExperienceSession(ctx, tx, &domain.ExperienceSession{})
		}},
		{name: "turn", save: func(ctx context.Context, tx *sql.Tx) error {
			return SaveExperienceTurn(ctx, tx, &domain.ExperienceTurn{})
		}},
		{name: "event", save: func(ctx context.Context, tx *sql.Tx) error {
			return SaveExperienceEvent(ctx, tx, &domain.ExperienceEvent{})
		}},
		{name: "cursor", save: func(ctx context.Context, tx *sql.Tx) error {
			return SaveIngestionCursor(ctx, tx, &domain.IngestionCursor{ProjectID: project.ID, Source: "invalid", SourceInstance: "source", CursorJSON: `{}`, Revision: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WithTx(context.Background(), db, func(tx *sql.Tx) error {
				return tt.save(context.Background(), tx)
			})
			var validation *domain.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %T %v, want ValidationError", err, err)
			}
		})
	}
}

func newExperienceBundle(projectID domain.ProjectID, suffix string) (*domain.ExperienceSession, *domain.ExperienceTurn, *domain.ExperienceEvent, *domain.IngestionCursor) {
	started := time.Date(2026, 7, 21, 10, 0, 0, 123456000, time.UTC)
	closed := started.Add(2 * time.Hour)
	stable := started.Add(30 * time.Minute)
	lastAttempt := closed.Add(time.Minute)
	session := &domain.ExperienceSession{
		ID:                domain.ExperienceSessionID(newUUID()),
		ProjectID:         projectID,
		Source:            domain.SourceOpenCode,
		ExternalSessionID: "external-session-" + suffix,
		Locator: domain.TranscriptLocator{
			Kind: "sqlite", Path: "C:/safe/sessions.db", SessionID: "native-session-" + suffix,
			TurnID: "native-turn-" + suffix, Offset: 42, SourceHash: "source-hash",
		},
		StartedAt:      &started,
		UpdatedAt:      closed,
		ClosedAt:       &closed,
		MetadataSHA256: "metadata-sha256",
		CreatedAt:      started.Add(-time.Minute),
	}
	turn := &domain.ExperienceTurn{
		ID: domain.ExperienceTurnID(newUUID()), SessionID: session.ID, ExternalTurnID: "external-turn-" + suffix,
		Sequence: 7, Status: domain.TurnIngested, Fingerprint: "turn-fingerprint", UserDigest: "user-digest",
		AssistantDigest: "assistant-digest", ToolCallsDigest: "tools-digest", SafeSummary: "Safe summary.",
		OccurredAt: started, StableAt: &stable, IngestedAt: closed, SourceRevision: "revision-1", Redacted: true,
	}
	event := &domain.ExperienceEvent{
		ID: domain.ExperienceEventID(newUUID()), ProjectID: projectID, TurnID: turn.ID,
		Kind: domain.EventSuccessfulProcedure, Summary: "Procedure succeeded.", Observation: "Tests passed.",
		Outcome: "success", Fingerprint: "event-fingerprint", EvidenceJSON: `[{"kind":"test"}]`,
		Detector:   domain.DetectorIdentity{Kind: "deterministic", Name: "test-outcome", Version: "1.0.0"},
		Confidence: domain.ConfidenceHigh, CreatedAt: closed,
	}
	cursor := &domain.IngestionCursor{
		ProjectID: projectID, Source: domain.SourceOpenCode, SourceInstance: "instance-" + suffix,
		CursorJSON: `{"offset":21}`, LastSuccessfulAt: &closed, LastAttemptAt: &lastAttempt,
		LastErrorCode: "", LastErrorMessage: "", InputDigest: "input-digest", Revision: 1,
	}
	return session, turn, event, cursor
}

func experienceTableCounts(t *testing.T, db *sql.DB) []int {
	t.Helper()
	tables := []string{"experience_sessions", "experience_turns", "experience_events", "ingestion_cursors"}
	counts := make([]int, len(tables))
	for i, table := range tables {
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&counts[i]); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
	}
	return counts
}

func assertDomainCode(t *testing.T, err error, want domain.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	got, ok := domain.AsDomainError(err)
	if !ok || got.Code != want {
		t.Fatalf("error = %T %v, code = %q; want %q", err, err, gotCode(got), want)
	}
}

func gotCode(err *domain.DomainError) domain.ErrorCode {
	if err == nil {
		return ""
	}
	return err.Code
}

func timePtr(value time.Time) *time.Time { return &value }
