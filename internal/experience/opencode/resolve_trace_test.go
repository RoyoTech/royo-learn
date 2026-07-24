package opencode

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-royo-learn/internal/domain"
)

// fixtureHash returns the hex-encoded SHA-256 of the file at path.
func fixtureHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// messageFixtureForTrace is a single messages row targeted by the trace
// tests. Times in unix milliseconds.
type messageFixtureForTrace struct {
	ID        string
	SessionID string
	Sequence  int64
	Role      string
	Content   string
	CreatedAt int64
}

// insertMessageForTrace appends one row to the messages table inside the
// opencode fixture. The caller is responsible for opening the test DB.
func insertMessageForTrace(t *testing.T, db *sql.DB, m messageFixtureForTrace) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO messages(id, session_id, sequence, role, content, created_at, complete) VALUES(?, ?, ?, ?, ?, ?, 1)`,
		m.ID, m.SessionID, m.Sequence, m.Role, m.Content, m.CreatedAt,
	)
	if err != nil {
		t.Fatalf("insert message %s: %v", m.ID, err)
	}
}

// insertSessionForTrace appends one row to the sessions table.
func insertSessionForTrace(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO sessions(id, project_id, started_at, updated_at) VALUES(?, ?, ?, ?)`,
		id, "project-root", 1700000000000, 1700000060000,
	)
	if err != nil {
		t.Fatalf("insert session %s: %v", id, err)
	}
}

// locatorFor builds a TranscriptLocator with the current fixture hash so
// the trace path is satisfied.
func locatorFor(t *testing.T, dbPath, turnID string) domain.TranscriptLocator {
	t.Helper()
	return domain.TranscriptLocator{
		Kind:       "sqlite",
		Path:       dbPath,
		SessionID:  "s-1",
		TurnID:     turnID,
		SourceHash: fixtureHash(t, dbPath),
	}
}

// TestResolveTrace_OK_ReturnsExcerpt verifies the happy path: a known
// turn returns its content (possibly redacted) and an empty Code.
func TestResolveTrace_OK_ReturnsExcerpt(t *testing.T) {
	dbPath := newFixtureDB(t, func(db *sql.DB) {
		insertSessionForTrace(t, db, "s-1")
		insertMessageForTrace(t, db, messageFixtureForTrace{
			ID: "t-1", SessionID: "s-1", Sequence: 1, Role: "user",
			Content: "hello world", CreatedAt: 1700000010000,
		})
	})

	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locatorFor(t, dbPath, "t-1"), TraceBounds{MaxBytes: 1024})
	if result.Code != "" {
		t.Fatalf("ResolveTrace Code = %q, want empty (result=%+v)", result.Code, result)
	}
	if !strings.Contains(result.Excerpt, "hello world") {
		t.Fatalf("ResolveTrace Excerpt = %q, want it to contain the original content", result.Excerpt)
	}
	if result.SourceChanged {
		t.Fatalf("ResolveTrace SourceChanged = true, want false on fresh locator")
	}
	if result.Redacted {
		t.Fatalf("ResolveTrace Redacted = true, want false on benign content")
	}
}

// TestResolveTrace_TruncatesToMaxBytes verifies the excerpt is capped at
// bounds.MaxBytes and ends with the truncation marker.
func TestResolveTrace_TruncatesToMaxBytes(t *testing.T) {
	big := strings.Repeat("A", 2048)
	dbPath := newFixtureDB(t, func(db *sql.DB) {
		insertSessionForTrace(t, db, "s-1")
		insertMessageForTrace(t, db, messageFixtureForTrace{
			ID: "t-1", SessionID: "s-1", Sequence: 1, Role: "user",
			Content: big, CreatedAt: 1700000010000,
		})
	})

	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locatorFor(t, dbPath, "t-1"), TraceBounds{MaxBytes: 256})
	if got := len(result.Excerpt); got > 256 {
		t.Fatalf("ResolveTrace Excerpt length = %d, want <= 256", got)
	}
	if !strings.HasSuffix(result.Excerpt, TraceExcerptSuffix) {
		t.Fatalf("ResolveTrace Excerpt = %q, want suffix %q", result.Excerpt, TraceExcerptSuffix)
	}
}

// TestResolveTrace_DefaultMaxBytes verifies that a zero MaxBytes falls
// back to the contract default (1 KiB).
func TestResolveTrace_DefaultMaxBytes(t *testing.T) {
	content := strings.Repeat("B", 4096)
	dbPath := newFixtureDB(t, func(db *sql.DB) {
		insertSessionForTrace(t, db, "s-1")
		insertMessageForTrace(t, db, messageFixtureForTrace{
			ID: "t-1", SessionID: "s-1", Sequence: 1, Role: "user",
			Content: content, CreatedAt: 1700000010000,
		})
	})

	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locatorFor(t, dbPath, "t-1"), TraceBounds{MaxBytes: 0})
	if got := len(result.Excerpt); got > defaultTraceMaxBytes {
		t.Fatalf("ResolveTrace Excerpt length = %d, want <= defaultTraceMaxBytes=%d", got, defaultTraceMaxBytes)
	}
}

// TestResolveTrace_RedactsSecrets verifies that obvious credential-like
// strings inside the content are scrubbed before the excerpt is returned.
func TestResolveTrace_RedactsSecrets(t *testing.T) {
	const secret = "sk-abc123def456ghi789jkl012mno345pq"
	content := "prefix " + secret + " suffix"
	dbPath := newFixtureDB(t, func(db *sql.DB) {
		insertSessionForTrace(t, db, "s-1")
		insertMessageForTrace(t, db, messageFixtureForTrace{
			ID: "t-1", SessionID: "s-1", Sequence: 1, Role: "user",
			Content: content, CreatedAt: 1700000010000,
		})
	})

	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locatorFor(t, dbPath, "t-1"), TraceBounds{MaxBytes: 1024})
	if !result.Redacted {
		t.Fatalf("ResolveTrace Redacted = false, want true when the content carried a secret")
	}
	if strings.Contains(result.Excerpt, secret) {
		t.Fatalf("ResolveTrace Excerpt still contains the raw secret %q", secret)
	}
}

// TestResolveTrace_SourceChanged verifies that a stale SourceHash on the
// locator surfaces as Code="trace_source_changed".
func TestResolveTrace_SourceChanged(t *testing.T) {
	dbPath := newFixtureDB(t, func(db *sql.DB) {
		insertSessionForTrace(t, db, "s-1")
		insertMessageForTrace(t, db, messageFixtureForTrace{
			ID: "t-1", SessionID: "s-1", Sequence: 1, Role: "user",
			Content: "hello", CreatedAt: 1700000010000,
		})
	})
	locator := locatorFor(t, dbPath, "t-1")
	locator.SourceHash = "stale-hash-does-not-match-the-current-file"

	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locator, TraceBounds{MaxBytes: 1024})
	if result.Code != "trace_source_changed" {
		t.Fatalf("ResolveTrace Code = %q, want trace_source_changed", result.Code)
	}
	if !result.SourceChanged {
		t.Fatalf("ResolveTrace SourceChanged = false, want true")
	}
	if result.Excerpt != "" {
		t.Fatalf("ResolveTrace Excerpt = %q, want empty when source changed", result.Excerpt)
	}
}

// TestResolveTrace_TurnNotFound verifies the "turn disappeared" case
// surfaces as Code="trace_source_unavailable".
func TestResolveTrace_TurnNotFound(t *testing.T) {
	dbPath := newFixtureDB(t, func(db *sql.DB) {
		insertSessionForTrace(t, db, "s-1")
		insertMessageForTrace(t, db, messageFixtureForTrace{
			ID: "t-1", SessionID: "s-1", Sequence: 1, Role: "user",
			Content: "hello", CreatedAt: 1700000010000,
		})
	})
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locatorFor(t, dbPath, "missing-turn"), TraceBounds{MaxBytes: 1024})
	if result.Code != "trace_source_unavailable" {
		t.Fatalf("ResolveTrace Code = %q, want trace_source_unavailable", result.Code)
	}
}

// TestResolveTrace_InvalidLocatorKind rejects locators whose Kind is
// not sqlite; the adapter only resolves OpenCode transcripts.
func TestResolveTrace_InvalidLocatorKind(t *testing.T) {
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind:   "jsonl",
		Path:   "/tmp/opencode.db",
		TurnID: "t-1",
	}, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceLocatorInvalid) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceLocatorInvalid)
	}
}

// TestResolveTrace_EmptyPath rejects locators whose Path is empty.
func TestResolveTrace_EmptyPath(t *testing.T) {
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind:   "sqlite",
		Path:   "",
		TurnID: "t-1",
	}, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceLocatorInvalid) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceLocatorInvalid)
	}
}

// TestResolveTrace_EmptyTurnID rejects locators whose TurnID is empty.
func TestResolveTrace_EmptyTurnID(t *testing.T) {
	dbPath := newFixtureDB(t, nil)
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind:   "sqlite",
		Path:   dbPath,
		TurnID: "",
	}, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceLocatorInvalid) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceLocatorInvalid)
	}
}

// TestResolveTrace_ContextCanceled verifies the contract: a cancelled
// context short-circuits before any source I/O.
func TestResolveTrace_ContextCanceled(t *testing.T) {
	dbPath := newFixtureDB(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	adapter := NewAdapter()
	result := adapter.ResolveTrace(ctx, locatorFor(t, dbPath, "t-1"), TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrTimeout) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrTimeout)
	}
}

// TestResolveTrace_MissingDB rejects locators whose Path no longer
// resolves to a readable file.
func TestResolveTrace_MissingDB(t *testing.T) {
	locator := domain.TranscriptLocator{
		Kind:   "sqlite",
		Path:   filepath.Join(t.TempDir(), "no-such.db"),
		TurnID: "t-1",
		// Empty SourceHash -> no hash comparison step; we go straight to
		// the open-source-failure path.
	}
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locator, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceSourceNotFound)
	}
}
