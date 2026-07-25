package opencode

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
)

// sessionFixture is one sessions row for tests that build Scan fixtures. Times
// are unix milliseconds; ClosedAt == 0 means the column is NULL.
type sessionFixture struct {
	ID        string
	StartedAt int64
	UpdatedAt int64
	ClosedAt  int64
}

// messageFixture is one messages row for tests that build Scan fixtures. Empty
// string columns are stored as NULL; Complete 0/1 maps to the integer column.
type messageFixture struct {
	ID        string
	SessionID string
	Sequence  int64
	Role      string
	Content   string
	Finish    string
	CreatedAt int64
	Complete  int
	Revision  string
}

// populateFixture inserts the given sessions and messages into db. It panics on
// error so a malformed fixture fails the test with a clear message.
func populateFixture(t *testing.T, db *sql.DB, sessions []sessionFixture, messages []messageFixture) {
	t.Helper()
	for _, s := range sessions {
		var closedAt any
		if s.ClosedAt != 0 {
			closedAt = s.ClosedAt
		}
		if _, err := db.Exec(
			"INSERT INTO sessions(id, project_id, started_at, updated_at, closed_at) VALUES(?, ?, ?, ?, ?)",
			s.ID, "project-fixture", s.StartedAt, s.UpdatedAt, closedAt,
		); err != nil {
			t.Fatalf("insert session %q: %v", s.ID, err)
		}
	}
	for _, m := range messages {
		var content, finish, revision any
		if m.Content != "" {
			content = m.Content
		}
		if m.Finish != "" {
			finish = m.Finish
		}
		if m.Revision != "" {
			revision = m.Revision
		}
		if _, err := db.Exec(
			"INSERT INTO messages(id, session_id, sequence, role, content, finish, created_at, complete, revision) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)",
			m.ID, m.SessionID, m.Sequence, m.Role, content, finish, m.CreatedAt, m.Complete, revision,
		); err != nil {
			t.Fatalf("insert message %q: %v", m.ID, err)
		}
	}
}

// sortedEnvelopePairs returns a stable representation of envelopes for
// comparison: a slice of (sessionID, turnID) pairs sorted lexicographically.
func sortedEnvelopePairs(envelopes []experience.ExperienceEnvelope) [][2]string {
	pairs := make([][2]string, 0, len(envelopes))
	for _, e := range envelopes {
		pairs = append(pairs, [2]string{e.Session.ExternalID, e.Turn.ExternalID})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] != pairs[j][0] {
			return pairs[i][0] < pairs[j][0]
		}
		return pairs[i][1] < pairs[j][1]
	})
	return pairs
}

// validScanInstance wraps a freshly-created fixture path into a SourceInstance
// suitable for the Scan adapter call.
func validScanInstance(dbPath string) SourceInstance {
	return SourceInstance{
		Source:      domain.SourceOpenCode,
		ProjectRoot: filepath.Dir(dbPath),
		DBPath:      dbPath,
		Schema:      SchemaTag,
		Discovered:  time.Unix(0, 0).UTC(),
	}
}

// fingerprintFromEnvelope builds the FingerprintInput a service would feed to
// experience.Fingerprint for the given envelope. The adapter does not compute
// this hash itself; tests use this helper to assert idempotency at the
// adapter boundary.
func fingerprintFromEnvelope(env experience.ExperienceEnvelope) experience.FingerprintInput {
	return experience.FingerprintInput{
		Source:          string(env.Source),
		ExternalSession: env.Session.ExternalID,
		ExternalTurn:    env.Turn.ExternalID,
		Sequence:        env.Turn.Sequence,
		UserDigest:      experience.DigestString(env.Turn.UserText),
		AssistantDigest: experience.DigestString(env.Turn.AssistantText),
		ToolCallsDigest: "",
		FinishReason:    env.Turn.FinishReason,
		Complete:        env.Turn.Complete,
		SourceRevision:  env.Turn.SourceRevision,
	}
}

// TestScan_OK_BuildsEnvelopesFromCompleteTurns verifies the canonical happy
// path: 1 session, 2 complete turns → 2 envelopes, status=ok, NextCursor set
// to the last emitted envelope.
func TestScan_OK_BuildsEnvelopesFromCompleteTurns(t *testing.T) {
	sessions := []sessionFixture{
		{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000, ClosedAt: 1700000120000},
	}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "hello", CreatedAt: 1700000010000, Complete: 1},
		{ID: "t2", SessionID: "s1", Sequence: 2, Role: "assistant", Content: "world", Finish: "stop", CreatedAt: 1700000020000, Complete: 1, Revision: "rev-1"},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	adapter.Now = func() time.Time { return time.Unix(0, 0).UTC() }
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("Scan Status = %q, want %q (result=%+v)", result.Status, "ok", result)
	}
	if result.Code != "" {
		t.Fatalf("Scan Code = %q, want empty on success", result.Code)
	}
	if got := len(result.Envelopes); got != 2 {
		t.Fatalf("Scan emitted %d envelopes, want 2", got)
	}
	if result.NextCursor == nil {
		t.Fatal("Scan NextCursor is nil, want a cursor for emitted envelopes")
	}
	if sid, _ := result.NextCursor["last_session_id"].(string); sid != "s1" {
		t.Fatalf("Scan NextCursor.last_session_id = %q, want %q", sid, "s1")
	}
	if seq, _ := result.NextCursor["last_sequence"].(int64); seq != 2 {
		t.Fatalf("Scan NextCursor.last_sequence = %d, want 2", seq)
	}

	e1 := result.Envelopes[0]
	if e1.SchemaVersion != experience.ExperienceEnvelopeSchemaVersion {
		t.Fatalf("envelope 0 SchemaVersion = %d, want %d", e1.SchemaVersion, experience.ExperienceEnvelopeSchemaVersion)
	}
	if e1.Session.ExternalID != "s1" || e1.Turn.ExternalID != "t1" {
		t.Fatalf("envelope 0 = session=%s turn=%s, want s1/t1", e1.Session.ExternalID, e1.Turn.ExternalID)
	}
	if e1.Turn.UserText != "hello" || e1.Turn.AssistantText != "" {
		t.Fatalf("envelope 0 text = (%q, %q), want user=hello assistant=empty", e1.Turn.UserText, e1.Turn.AssistantText)
	}
	if !e1.Turn.Complete {
		t.Fatal("envelope 0 Complete = false, want true (complete turn)")
	}
	if e1.Session.UpdatedAt.IsZero() {
		t.Fatal("envelope 0 Session.UpdatedAt is zero, want non-zero")
	}
	if e1.Session.Locator.SourceHash == "" {
		t.Fatal("envelope 0 Locator.SourceHash is empty, want DB-file SHA-256")
	}
	if e1.Session.Locator.Kind != "sqlite" {
		t.Fatalf("envelope 0 Locator.Kind = %q, want sqlite", e1.Session.Locator.Kind)
	}
	if e1.Session.Locator.TurnID != "t1" {
		t.Fatalf("envelope 0 Locator.TurnID = %q, want t1", e1.Session.Locator.TurnID)
	}

	e2 := result.Envelopes[1]
	if e2.Turn.ExternalID != "t2" || e2.Turn.FinishReason != "stop" {
		t.Fatalf("envelope 1 = turn=%s finish=%q, want t2/stop", e2.Turn.ExternalID, e2.Turn.FinishReason)
	}
	if e2.Turn.UserText != "" || e2.Turn.AssistantText != "world" {
		t.Fatalf("envelope 1 text = (%q, %q), want user=empty assistant=world", e2.Turn.UserText, e2.Turn.AssistantText)
	}
	if e2.Actor.Kind != "agent" || e2.Actor.Name != "opencode" {
		t.Fatalf("envelope 1 Actor = (%s/%s), want agent/opencode", e2.Actor.Kind, e2.Actor.Name)
	}
	if e2.Actor.SessionID != "s1" {
		t.Fatalf("envelope 1 Actor.SessionID = %q, want s1", e2.Actor.SessionID)
	}
	if e2.Turn.SourceRevision != "rev-1" {
		t.Fatalf("envelope 1 Turn.SourceRevision = %q, want rev-1", e2.Turn.SourceRevision)
	}
}

// TestScan_SkipsIncompleteTurns verifies that any message with complete=0 is
// not emitted as an envelope. The contract rule is "no incomplete turns",
// gated by the SQLite complete column.
func TestScan_SkipsIncompleteTurns(t *testing.T) {
	sessions := []sessionFixture{
		{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000},
	}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "first", CreatedAt: 1700000010000, Complete: 1},
		{ID: "t2", SessionID: "s1", Sequence: 2, Role: "assistant", Content: "second", CreatedAt: 1700000020000, Complete: 0},
		{ID: "t3", SessionID: "s1", Sequence: 3, Role: "user", Content: "third", CreatedAt: 1700000030000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if got := len(result.Envelopes); got != 2 {
		t.Fatalf("Scan emitted %d envelopes, want 2 (incomplete turn must be skipped)", got)
	}
	pairs := sortedEnvelopePairs(result.Envelopes)
	want := [][2]string{{"s1", "t1"}, {"s1", "t3"}}
	for i := range want {
		if pairs[i] != want[i] {
			t.Fatalf("envelope %d = %v, want %v", i, pairs[i], want[i])
		}
	}
	for _, e := range result.Envelopes {
		if !e.Turn.Complete {
			t.Fatalf("emitted envelope %s/%s has Complete=false; only complete turns are emitted", e.Session.ExternalID, e.Turn.ExternalID)
		}
	}
}

// TestScan_NoSourceSideEffects asserts that Scan never mutates the source
// database: the file mtime and SHA-256 digest must be identical before and
// after three Scan calls. The gate "cero side effects en la fuente OpenCode"
// depends on this property.
func TestScan_NoSourceSideEffects(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "hello", CreatedAt: 1700000010000, Complete: 1},
		{ID: "t2", SessionID: "s1", Sequence: 2, Role: "assistant", Content: "world", CreatedAt: 1700000020000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	beforeMtime := fileMtime(t, dbPath)
	beforeBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read source before Scan: %v", err)
	}
	beforeHash := sha256.Sum256(beforeBytes)

	adapter := NewAdapter()
	for i := 0; i < 3; i++ {
		result, err := adapter.Scan(context.Background(), ScanRequest{
			ProjectRoot: filepath.Dir(dbPath),
			Instance:    validScanInstance(dbPath),
		})
		if err != nil {
			t.Fatalf("Scan iteration %d error: %v", i, err)
		}
		if result.Status != "ok" {
			t.Fatalf("Scan iteration %d status = %q, want ok", i, result.Status)
		}
	}

	afterMtime := fileMtime(t, dbPath)
	if beforeMtime != afterMtime {
		t.Fatalf("Scan mutated source mtime: before=%d after=%d", beforeMtime, afterMtime)
	}
	afterBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read source after Scan: %v", err)
	}
	afterHash := sha256.Sum256(afterBytes)
	if beforeHash != afterHash {
		t.Fatalf("Scan mutated source contents: hash before=%s after=%s", hex.EncodeToString(beforeHash[:]), hex.EncodeToString(afterHash[:]))
	}
}

// TestScan_EmptyDatabase asserts that a fixture with no sessions yields zero
// envelopes with status=ok. The schema is present but the store is empty.
func TestScan_EmptyDatabase(t *testing.T) {
	dbPath := newFixtureDB(t, nil)

	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("Scan Status = %q, want %q (result=%+v)", result.Status, "ok", result)
	}
	if len(result.Envelopes) != 0 {
		t.Fatalf("Scan emitted %d envelopes on empty DB, want 0", len(result.Envelopes))
	}
	// No envelopes emitted: NextCursor carries the input forward, and the
	// input had no cursor, so the result is nil.
	if result.NextCursor != nil {
		t.Fatalf("Scan NextCursor = %v, want nil for empty DB with no input cursor", result.NextCursor)
	}
}

// TestScan_SortsDeterministically asserts the output ordering by
// (session.external_id, turn.sequence) when several sessions and turns are
// present. The contract requires stable order so dedup downstream is sound.
func TestScan_SortsDeterministically(t *testing.T) {
	sessions := []sessionFixture{
		{ID: "session-z", StartedAt: 1700000000000, UpdatedAt: 1700000060000},
		{ID: "session-a", StartedAt: 1700001000000, UpdatedAt: 1700001060000},
	}
	messages := []messageFixture{
		{ID: "z-3", SessionID: "session-z", Sequence: 3, Role: "user", Content: "z-3", CreatedAt: 1700000030000, Complete: 1},
		{ID: "z-1", SessionID: "session-z", Sequence: 1, Role: "user", Content: "z-1", CreatedAt: 1700000010000, Complete: 1},
		{ID: "z-2", SessionID: "session-z", Sequence: 2, Role: "user", Content: "z-2", CreatedAt: 1700000020000, Complete: 1},
		{ID: "a-2", SessionID: "session-a", Sequence: 2, Role: "assistant", Content: "a-2", CreatedAt: 1700001020000, Complete: 1},
		{ID: "a-3", SessionID: "session-a", Sequence: 3, Role: "assistant", Content: "a-3", CreatedAt: 1700001030000, Complete: 1},
		{ID: "a-1", SessionID: "session-a", Sequence: 1, Role: "assistant", Content: "a-1", CreatedAt: 1700001010000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if got := len(result.Envelopes); got != 6 {
		t.Fatalf("Scan emitted %d envelopes, want 6", got)
	}
	wantOrder := [][2]string{
		{"session-a", "a-1"},
		{"session-a", "a-2"},
		{"session-a", "a-3"},
		{"session-z", "z-1"},
		{"session-z", "z-2"},
		{"session-z", "z-3"},
	}
	gotPairs := make([][2]string, 0, len(result.Envelopes))
	for _, e := range result.Envelopes {
		gotPairs = append(gotPairs, [2]string{e.Session.ExternalID, e.Turn.ExternalID})
	}
	for i := range wantOrder {
		if gotPairs[i] != wantOrder[i] {
			t.Fatalf("envelope %d = %v, want %v (full order: %v)", i, gotPairs[i], wantOrder[i], gotPairs)
		}
	}
}

// TestScan_MissingDB asserts that Scan degrades gracefully when the database
// file is missing. Status="degraded", Code=source_not_found, no envelopes.
func TestScan_MissingDB(t *testing.T) {
	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: t.TempDir(),
		Instance:    validScanInstance(filepath.Join(t.TempDir(), "opencode.db")),
	})
	if err != nil {
		t.Fatalf("Scan returned error on missing DB: %v", err)
	}
	if result.Status != "degraded" {
		t.Fatalf("Scan Status = %q, want %q", result.Status, "degraded")
	}
	if result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("Scan Code = %q, want %q", result.Code, domain.ErrExperienceSourceNotFound)
	}
	if len(result.Envelopes) != 0 {
		t.Fatalf("Scan emitted %d envelopes on missing DB, want 0", len(result.Envelopes))
	}
}

// TestScan_WrongSource rejects a SourceInstance whose Source is not opencode.
// The adapter refuses to scan stores belonging to other adapters.
func TestScan_WrongSource(t *testing.T) {
	dbPath := newFixtureDB(t, nil)
	instance := validScanInstance(dbPath)
	instance.Source = domain.SourceClaudeCode

	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    instance,
	})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if result.Status != "error" {
		t.Fatalf("Scan Status = %q, want %q", result.Status, "error")
	}
	if result.Code != string(domain.ErrInvalidArgument) {
		t.Fatalf("Scan Code = %q, want %q", result.Code, domain.ErrInvalidArgument)
	}
}

// TestScan_EmptyDBPath rejects a SourceInstance without a DBPath. The adapter
// refuses to invent a path; the caller must run Discover first.
func TestScan_EmptyDBPath(t *testing.T) {
	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: t.TempDir(),
		Instance: SourceInstance{
			Source:      domain.SourceOpenCode,
			ProjectRoot: t.TempDir(),
			DBPath:      "",
			Schema:      SchemaTag,
		},
	})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if result.Status != "error" {
		t.Fatalf("Scan Status = %q, want %q", result.Status, "error")
	}
	if result.Code != string(domain.ErrInvalidArgument) {
		t.Fatalf("Scan Code = %q, want %q", result.Code, domain.ErrInvalidArgument)
	}
}

// TestScan_ContextCanceled verifies Scan surfaces context cancellation as a
// direct error so callers can detect a hung upstream DB.
func TestScan_ContextCanceled(t *testing.T) {
	dbPath := newFixtureDB(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	adapter := NewAdapter()
	if _, err := adapter.Scan(ctx, ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Scan with cancelled context error = %v, want context.Canceled", err)
	}
}

// TestScan_LocatorSourceHashStableAcrossReads asserts that the SourceHash
// stamped on every envelope's locator is deterministic across Scan calls.
// The hash is the SHA-256 of the DB file; repeated reads of the same file
// yield the same hash.
func TestScan_LocatorSourceHashStableAcrossReads(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "hi", CreatedAt: 1700000010000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	first, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan 1 error: %v", err)
	}
	if len(first.Envelopes) != 1 {
		t.Fatalf("Scan 1 emitted %d envelopes, want 1", len(first.Envelopes))
	}
	hash1 := first.Envelopes[0].Session.Locator.SourceHash

	second, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan 2 error: %v", err)
	}
	if len(second.Envelopes) != 1 {
		t.Fatalf("Scan 2 emitted %d envelopes, want 1", len(second.Envelopes))
	}
	hash2 := second.Envelopes[0].Session.Locator.SourceHash

	if hash1 == "" || hash2 == "" {
		t.Fatalf("Locator SourceHash empty: first=%q second=%q", hash1, hash2)
	}
	if hash1 != hash2 {
		t.Fatalf("Locator SourceHash differs across reads: first=%s second=%s", hash1, hash2)
	}
}

// TestScan_LocatorSourceHashChangesWhenDBReplaced asserts that swapping the DB
// contents produces a different SourceHash. The hash is the only signal that
// detects substitution; this property is the trust gate.
func TestScan_LocatorSourceHashChangesWhenDBReplaced(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "hi", CreatedAt: 1700000010000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	first, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan original error: %v", err)
	}
	originalHash := first.Envelopes[0].Session.Locator.SourceHash

	// Replace the file contents with different bytes (still a valid SQLite DB
	// with the same schema, but a different message row id, so the file hash
	// shifts).
	replacedSessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	replacedMessages := []messageFixture{
		{ID: "t1-different", SessionID: "s1", Sequence: 1, Role: "user", Content: "hi", CreatedAt: 1700000010000, Complete: 1},
	}
	replacedDBPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, replacedSessions, replacedMessages) })

	if err := copyFile(replacedDBPath, dbPath); err != nil {
		t.Fatalf("copy replacement DB: %v", err)
	}

	second, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan replaced error: %v", err)
	}
	newHash := second.Envelopes[0].Session.Locator.SourceHash
	if newHash == originalHash {
		t.Fatalf("Locator SourceHash unchanged after DB replacement: %s", newHash)
	}
}

// TestScan_EnvelopeSchemaVersionIsOne asserts every emitted envelope carries
// SchemaVersion=1, the only version the core service currently accepts.
func TestScan_EnvelopeSchemaVersionIsOne(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "hi", CreatedAt: 1700000010000, Complete: 1},
		{ID: "t2", SessionID: "s1", Sequence: 2, Role: "assistant", Content: "there", CreatedAt: 1700000020000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	for i, e := range result.Envelopes {
		if e.SchemaVersion != experience.ExperienceEnvelopeSchemaVersion {
			t.Fatalf("envelope %d SchemaVersion = %d, want %d", i, e.SchemaVersion, experience.ExperienceEnvelopeSchemaVersion)
		}
	}
}

// TestScan_NextCursorPersistsAcrossCalls verifies that the returned NextCursor
// lets a subsequent Scan identify the same checkpoint. When the second call
// uses the cursor together with a Since filter that excludes everything seen,
// no new envelopes are emitted.
func TestScan_NextCursorPersistsAcrossCalls(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "hi", CreatedAt: 1700000010000, Complete: 1},
		{ID: "t2", SessionID: "s1", Sequence: 2, Role: "assistant", Content: "there", CreatedAt: 1700000020000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	first, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan 1 error: %v", err)
	}
	if first.NextCursor == nil {
		t.Fatal("Scan 1 NextCursor is nil; the contract requires a cursor when envelopes are emitted")
	}

	// Since filter excludes everything the first scan saw. Together with the
	// cursor pointing at the last emitted envelope, the second scan emits
	// nothing new.
	since := time.UnixMilli(1700000020000 + 1)
	second, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
		Since:       &since,
		Cursor:      first.NextCursor,
	})
	if err != nil {
		t.Fatalf("Scan 2 error: %v", err)
	}
	if len(second.Envelopes) != 0 {
		t.Fatalf("Scan 2 emitted %d envelopes with the same cursor + Since filter, want 0", len(second.Envelopes))
	}
}

// TestScan_IdempotentAcrossRepeatedCalls asserts that three Scan calls against
// the same fixture produce byte-identical envelopes. The service uses envelope
// identity for idempotency; if the adapter drifts between calls, the dedupe
// layer breaks.
func TestScan_IdempotentAcrossRepeatedCalls(t *testing.T) {
	sessions := []sessionFixture{
		{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000, ClosedAt: 1700000120000},
		{ID: "s2", StartedAt: 1700001000000, UpdatedAt: 1700001060000},
	}
	messages := []messageFixture{
		{ID: "s1-t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "first", CreatedAt: 1700000010000, Complete: 1},
		{ID: "s1-t2", SessionID: "s1", Sequence: 2, Role: "assistant", Content: "second", Finish: "stop", CreatedAt: 1700000020000, Complete: 1},
		{ID: "s2-t1", SessionID: "s2", Sequence: 1, Role: "user", Content: "third", CreatedAt: 1700001010000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	var scans [][]experience.ExperienceEnvelope
	for i := 0; i < 3; i++ {
		result, err := adapter.Scan(context.Background(), ScanRequest{
			ProjectRoot: filepath.Dir(dbPath),
			Instance:    validScanInstance(dbPath),
		})
		if err != nil {
			t.Fatalf("Scan iteration %d error: %v", i, err)
		}
		scans = append(scans, result.Envelopes)
	}

	if len(scans[0]) != len(scans[1]) || len(scans[0]) != len(scans[2]) {
		t.Fatalf("envelope counts differ across scans: %d, %d, %d", len(scans[0]), len(scans[1]), len(scans[2]))
	}
	for i := range scans[0] {
		a, b, c := scans[0][i], scans[1][i], scans[2][i]
		if a.Session.ExternalID != b.Session.ExternalID || a.Session.ExternalID != c.Session.ExternalID {
			t.Fatalf("envelope %d session id drifted: %s %s %s", i, a.Session.ExternalID, b.Session.ExternalID, c.Session.ExternalID)
		}
		if a.Turn.ExternalID != b.Turn.ExternalID || a.Turn.ExternalID != c.Turn.ExternalID {
			t.Fatalf("envelope %d turn id drifted: %s %s %s", i, a.Turn.ExternalID, b.Turn.ExternalID, c.Turn.ExternalID)
		}
		if a.Session.Locator.SourceHash != b.Session.Locator.SourceHash || a.Session.Locator.SourceHash != c.Session.Locator.SourceHash {
			t.Fatalf("envelope %d SourceHash drifted", i)
		}
		if a.Turn.UserText != b.Turn.UserText || a.Turn.UserText != c.Turn.UserText {
			t.Fatalf("envelope %d UserText drifted: %q %q %q", i, a.Turn.UserText, b.Turn.UserText, c.Turn.UserText)
		}
		if a.Turn.AssistantText != b.Turn.AssistantText || a.Turn.AssistantText != c.Turn.AssistantText {
			t.Fatalf("envelope %d AssistantText drifted", i)
		}
		if a.Turn.SourceRevision != b.Turn.SourceRevision || a.Turn.SourceRevision != c.Turn.SourceRevision {
			t.Fatalf("envelope %d SourceRevision drifted: %q %q %q", i, a.Turn.SourceRevision, b.Turn.SourceRevision, c.Turn.SourceRevision)
		}
	}
}

// TestScan_ProducesStableTurnOrder asserts the order of envelopes is identical
// across calls. The contract requires stable ordering so downstream consumers
// can rely on a deterministic input stream.
func TestScan_ProducesStableTurnOrder(t *testing.T) {
	sessions := []sessionFixture{
		{ID: "session-a", StartedAt: 1700000000000, UpdatedAt: 1700000060000},
		{ID: "session-b", StartedAt: 1700001000000, UpdatedAt: 1700001060000},
	}
	messages := []messageFixture{
		{ID: "a-2", SessionID: "session-a", Sequence: 2, Role: "user", Content: "x", CreatedAt: 1700000020000, Complete: 1},
		{ID: "a-1", SessionID: "session-a", Sequence: 1, Role: "user", Content: "x", CreatedAt: 1700000010000, Complete: 1},
		{ID: "b-1", SessionID: "session-b", Sequence: 1, Role: "assistant", Content: "x", CreatedAt: 1700001010000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	first, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan 1 error: %v", err)
	}
	second, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan 2 error: %v", err)
	}

	pairs1 := sortedEnvelopePairs(first.Envelopes)
	pairs2 := sortedEnvelopePairs(second.Envelopes)
	if len(pairs1) != len(pairs2) {
		t.Fatalf("envelope counts differ: %d vs %d", len(pairs1), len(pairs2))
	}
	for i := range pairs1 {
		if pairs1[i] != pairs2[i] {
			t.Fatalf("envelope order differs at %d: %v vs %v", i, pairs1[i], pairs2[i])
		}
	}
}

// TestScan_FingerprintMatchesAcrossReads asserts that the FingerprintInput
// derived from each envelope is byte-identical across repeated scans. The core
// service feeds this input into experience.Fingerprint; if the input drifts,
// two identical turns produce different fingerprints and ingestion loses
// idempotency.
func TestScan_FingerprintMatchesAcrossReads(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "alpha", CreatedAt: 1700000010000, Complete: 1},
		{ID: "t2", SessionID: "s1", Sequence: 2, Role: "assistant", Content: "beta", Finish: "stop", CreatedAt: 1700000020000, Complete: 1, Revision: "rev-x"},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	first, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan 1 error: %v", err)
	}
	second, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan 2 error: %v", err)
	}

	if len(first.Envelopes) != len(second.Envelopes) {
		t.Fatalf("envelope counts differ: %d vs %d", len(first.Envelopes), len(second.Envelopes))
	}
	for i := range first.Envelopes {
		fp1 := experience.Fingerprint(fingerprintFromEnvelope(first.Envelopes[i]))
		fp2 := experience.Fingerprint(fingerprintFromEnvelope(second.Envelopes[i]))
		if fp1 != fp2 {
			t.Fatalf("fingerprint differs at %d: %s vs %s", i, fp1, fp2)
		}
		if fp1 == "" {
			t.Fatalf("fingerprint at %d is empty", i)
		}
	}
}

// copyFile overwrites dst with the contents of src using a single os.WriteFile
// round-trip. It is used to swap a fixture DB without re-running schema setup
// inside the test (the replacement DB already has the expected schema).
func copyFile(src, dst string) error {
	bytes, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, bytes, 0o600)
}

// TestScan_CursorSkipsEarlierSessionsKeepsLater asserts the cursor filters
// out turns at or before the checkpoint (session_id, sequence) regardless of
// which session they belong to. The second session's turns must still be
// emitted because they are lexicographically after the cursor's session id.
func TestScan_CursorSkipsEarlierSessionsKeepsLater(t *testing.T) {
	sessions := []sessionFixture{
		{ID: "s-a", StartedAt: 1700000000000, UpdatedAt: 1700000060000},
		{ID: "s-z", StartedAt: 1700001000000, UpdatedAt: 1700001060000},
	}
	messages := []messageFixture{
		{ID: "a-1", SessionID: "s-a", Sequence: 1, Role: "user", Content: "a", CreatedAt: 1700000010000, Complete: 1},
		{ID: "a-2", SessionID: "s-a", Sequence: 2, Role: "user", Content: "a", CreatedAt: 1700000020000, Complete: 1},
		{ID: "z-1", SessionID: "s-z", Sequence: 1, Role: "user", Content: "z", CreatedAt: 1700001010000, Complete: 1},
		{ID: "z-2", SessionID: "s-z", Sequence: 2, Role: "user", Content: "z", CreatedAt: 1700001020000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
		Cursor: map[string]any{
			"last_session_id": "s-a",
			"last_sequence":   int64(2),
		},
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(result.Envelopes) != 2 {
		t.Fatalf("Scan emitted %d envelopes, want 2 (only s-z turns after the cursor)", len(result.Envelopes))
	}
	for _, e := range result.Envelopes {
		if e.Session.ExternalID != "s-z" {
			t.Fatalf("emitted envelope belongs to %s, want only s-z after the cursor", e.Session.ExternalID)
		}
	}
	if sid, _ := result.NextCursor["last_session_id"].(string); sid != "s-z" {
		t.Fatalf("NextCursor.last_session_id = %q, want s-z", sid)
	}
	if seq, _ := result.NextCursor["last_sequence"].(int64); seq != 2 {
		t.Fatalf("NextCursor.last_sequence = %d, want 2", seq)
	}
}

// TestScan_CursorAcceptsFloat64Sequence asserts the cursor checkpoint parser
// tolerates float64 (a JSON-decoded numeric) in addition to int64. The
// service may persist the cursor as JSON, where numbers decode as float64.
func TestScan_CursorAcceptsFloat64Sequence(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "hi", CreatedAt: 1700000010000, Complete: 1},
		{ID: "t2", SessionID: "s1", Sequence: 2, Role: "user", Content: "there", CreatedAt: 1700000020000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
		Cursor: map[string]any{
			"last_session_id": "s1",
			"last_sequence":   float64(1),
		},
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(result.Envelopes) != 1 {
		t.Fatalf("Scan emitted %d envelopes, want 1 (cursor at s1/1 should skip t1, keep t2)", len(result.Envelopes))
	}
	if result.Envelopes[0].Turn.ExternalID != "t2" {
		t.Fatalf("Scan emitted envelope %s, want t2", result.Envelopes[0].Turn.ExternalID)
	}
}

// TestScan_PartialValidationFailure exercises the degraded-status path: at
// least one envelope is emitted, but at least one row fails envelope
// validation. The contract maps that combination to Status="degraded" with a
// bounded Message containing the validation error(s).
func TestScan_PartialValidationFailure(t *testing.T) {
	oversizedID := strings.Repeat("x", 512) // > MaxExperienceIDBytes (256) on purpose
	sessions := []sessionFixture{
		{ID: "s-ok", StartedAt: 1700000000000, UpdatedAt: 1700000060000},
		{ID: oversizedID, StartedAt: 1700001000000, UpdatedAt: 1700001060000},
	}
	messages := []messageFixture{
		{ID: "t-ok-1", SessionID: "s-ok", Sequence: 1, Role: "user", Content: "ok", CreatedAt: 1700000010000, Complete: 1},
		{ID: "t-bad-1", SessionID: oversizedID, Sequence: 1, Role: "user", Content: "bad", CreatedAt: 1700001010000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if result.Status != "degraded" {
		t.Fatalf("Scan Status = %q, want degraded (partial validation failure)", result.Status)
	}
	if !result.Degraded {
		t.Fatal("Scan Degraded = false, want true on partial validation failure")
	}
	if len(result.Envelopes) != 1 {
		t.Fatalf("Scan emitted %d envelopes, want 1 (only s-ok is valid)", len(result.Envelopes))
	}
	if result.Envelopes[0].Session.ExternalID != "s-ok" {
		t.Fatalf("Scan emitted envelope from %s, want s-ok", result.Envelopes[0].Session.ExternalID)
	}
	if result.Message == "" {
		t.Fatal("Scan Message is empty, want the bounded validation error message")
	}
	if !strings.Contains(result.Message, "session external id") {
		t.Fatalf("Scan Message = %q, want it to mention the rejected field", result.Message)
	}
}

// TestScan_BoundedMessageTruncatesAt1KB asserts the accumulated validation
// error message is truncated when it exceeds the 1KB cap, so a flood of
// invalid rows cannot grow the result unboundedly.
func TestScan_BoundedMessageTruncatesAt1KB(t *testing.T) {
	oversizedID := strings.Repeat("y", 512)
	const numBadSessions = 20
	sessions := make([]sessionFixture, 0, numBadSessions+1)
	sessions = append(sessions, sessionFixture{ID: "s-ok", StartedAt: 1700000000000, UpdatedAt: 1700000060000})
	messages := make([]messageFixture, 0, numBadSessions+1)
	messages = append(messages, messageFixture{ID: "t-ok-1", SessionID: "s-ok", Sequence: 1, Role: "user", Content: "ok", CreatedAt: 1700000010000, Complete: 1})
	for i := 0; i < numBadSessions; i++ {
		id := oversizedID + "-" + strconv.Itoa(i)
		sessions = append(sessions, sessionFixture{
			ID:        id,
			StartedAt: int64(1700010000000 + i*1000),
			UpdatedAt: int64(1700010006000 + i*1000),
		})
		messages = append(messages, messageFixture{
			ID:        "t-bad-" + strconv.Itoa(i),
			SessionID: id,
			Sequence:  1,
			Role:      "user",
			Content:   "bad",
			CreatedAt: int64(1700010010000 + i*1000),
			Complete:  1,
		})
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if result.Status != "degraded" {
		t.Fatalf("Scan Status = %q, want degraded", result.Status)
	}
	if got := len(result.Message); got > maxScanMessageBytes {
		t.Fatalf("Scan Message length = %d, want <= %d", got, maxScanMessageBytes)
	}
	if len(result.Message) < maxScanMessageBytes {
		t.Fatalf("Scan Message length = %d, want the cap (%d) to be reached with %d bad rows", len(result.Message), maxScanMessageBytes, numBadSessions)
	}
}
