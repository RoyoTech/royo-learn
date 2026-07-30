package opencode

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
	"agent-royo-learn/internal/experience/semantic"
)

// depthOfCoverage covers the two empty-string branches the public Discover
// path never exercises; pinned here so a future refactor cannot regress the
// zero-depth case.
func TestDepthOf_EmptyAndDotVariants(t *testing.T) {
	if got := depthOf(""); got != 0 {
		t.Fatalf("depthOf(\"\") = %d, want 0", got)
	}
	if got := depthOf("."); got != 0 {
		t.Fatalf("depthOf(\".\") = %d, want 0", got)
	}
	if got := depthOf("a"); got != 1 {
		t.Fatalf("depthOf(\"a\") = %d, want 1", got)
	}
	if got := depthOf(filepath.Join("a", "b", "c")); got != 3 {
		t.Fatalf("depthOf(\"a/b/c\") = %d, want 3", got)
	}
}

// cursorAtOrBeforeLexLess covers the lexicographically-less branch: when the
// current session id is strictly less than the cursor checkpoint, the cursor
// filter short-circuits without touching the sequence.
func TestCursorAtOrBeforeLexLess(t *testing.T) {
	if !cursorAtOrBefore("aaa", 999, "bbb", 1) {
		t.Fatal("cursorAtOrBefore(aaa, 999, bbb, 1) = false, want true (lex less)")
	}
	if cursorAtOrBefore("ccc", 1, "bbb", 999) {
		t.Fatal("cursorAtOrBefore(ccc, 1, bbb, 999) = true, want false (lex greater)")
	}
}

// TestResolveTrace_SchemaMismatch exercises the schema-verify branch in
// ResolveTrace. The fixture is a valid SQLite file that lacks the required
// OpenCode tables.
func TestResolveTrace_SchemaMismatch(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE unrelated (id INTEGER)"); err != nil {
		t.Fatalf("create unrelated: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind:   "sqlite",
		Path:   dbPath,
		TurnID: "t-1",
	}, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceSchemaUnsupported) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceSchemaUnsupported)
	}
}

// TestScan_SchemaMismatch exercises the schema-verify branch in Scan. The
// fixture is a valid SQLite file that lacks the required OpenCode tables.
func TestScan_SchemaMismatch(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE unrelated (id INTEGER)"); err != nil {
		t.Fatalf("create unrelated: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: dir,
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if result.Status != "degraded" {
		t.Fatalf("Scan Status = %q, want degraded", result.Status)
	}
	if result.Code != string(domain.ErrExperienceSchemaUnsupported) {
		t.Fatalf("Scan Code = %q, want %q", result.Code, domain.ErrExperienceSchemaUnsupported)
	}
}

// TestScan_DirAsDB exercises the non-NotExist branch of hashFileContents:
// os.Open on a directory succeeds in unix, but io.Copy fails with "is a
// directory". Scan reports ErrExperienceSourceNotFound.
func TestScan_DirAsDB(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "opencode.db"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: dir,
		Instance:    validScanInstance(filepath.Join(dir, "opencode.db")),
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if result.Status != "degraded" {
		t.Fatalf("Scan Status = %q, want degraded", result.Status)
	}
	if result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("Scan Code = %q, want %q", result.Code, domain.ErrExperienceSourceNotFound)
	}
}

// TestResolveTrace_DirAsDB exercises the same io.Copy failure branch in
// ResolveTrace: a directory at the locator path causes hashFileContents to
// fail past the os.ErrNotExist gate.
func TestResolveTrace_DirAsDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	if err := os.MkdirAll(dbPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind:   "sqlite",
		Path:   dbPath,
		TurnID: "t-1",
	}, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceSourceNotFound)
	}
}

// TestScan_SkippedIncompleteCounter verifies the SkippedIncomplete counter is
// surfaced from collectEnvelopes to the ScanResult. This goes through the
// `if complete == 0 { skippedIncomplete++; continue }` branch.
func TestScan_SkippedIncompleteCounter(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "a", CreatedAt: 1700000010000, Complete: 1},
		{ID: "t2", SessionID: "s1", Sequence: 2, Role: "user", Content: "b", CreatedAt: 1700000020000, Complete: 0},
		{ID: "t3", SessionID: "s1", Sequence: 3, Role: "user", Content: "c", CreatedAt: 1700000030000, Complete: 0},
		{ID: "t4", SessionID: "s1", Sequence: 4, Role: "user", Content: "d", CreatedAt: 1700000040000, Complete: 1},
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
	if result.SkippedIncomplete != 2 {
		t.Fatalf("SkippedIncomplete = %d, want 2", result.SkippedIncomplete)
	}
	if len(result.Envelopes) != 2 {
		t.Fatalf("Envelopes = %d, want 2", len(result.Envelopes))
	}
}

// TestScan_TruncatesOversizedMessage verifies the buildEnvelope path that
// truncates content larger than maxMessageBytes (1 MiB). The adapter never
// forwards a runaway message verbatim.
func TestScan_TruncatesOversizedMessage(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	big := strings.Repeat("X", maxMessageBytes+1024)
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: big, CreatedAt: 1700000010000, Complete: 1},
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
	if len(result.Envelopes) != 1 {
		t.Fatalf("Envelopes = %d, want 1", len(result.Envelopes))
	}
	if len(result.Envelopes[0].Turn.UserText) != maxMessageBytes {
		t.Fatalf("UserText length = %d, want %d (truncated)", len(result.Envelopes[0].Turn.UserText), maxMessageBytes)
	}
}

// TestDiscover_ProtectedPathDir exercises the IsProtectedPath branch when
// the walker encounters a protected directory (e.g. ".git"). The walker
// must skip the subtree and still return any other opencode.db at the root.
func TestDiscover_ProtectedPathDir(t *testing.T) {
	root := fixtureTree(t, map[string]string{
		filepath.Join(".git", "opencode.db"): "FILE",
		"opencode.db":                        "FILE",
	})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("Discover emitted %d instances, want 1 (only root opencode.db; .git subtree must be skipped)", len(instances))
	}
}

// TestDiscover_ProtectedPathFile exercises the IsProtectedPath branch when
// the walker encounters a protected file (e.g. ".gitignore"). The file
// must be skipped silently.
func TestDiscover_ProtectedPathFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		// ".gitignore" is only protected on POSIX; verify the named branch
		// only matters on POSIX. On Windows we still cover the existing
		// root-level opencode.db path.
		root := fixtureTree(t, map[string]string{
			"opencode.db":                     "FILE",
			filepath.Join("a", "opencode.db"): "FILE",
		})
		instances, err := NewAdapter().Discover(context.Background(), root)
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if len(instances) != 2 {
			t.Fatalf("Discover emitted %d instances, want 2", len(instances))
		}
		return
	}
	root := fixtureTree(t, map[string]string{
		"opencode.db": "FILE",
		".gitignore":  "FILE",
	})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("Discover emitted %d instances, want 1 (only opencode.db; .gitignore is protected)", len(instances))
	}
}

// TestDiscover_DepthLimit exercises the depth-cap branch. The walker must
// skip subtrees deeper than maxDiscoveryDepth (8): a directory at depth 9
// satisfies the strict-greater test and is skipped.
func TestDiscover_DepthLimit(t *testing.T) {
	tree := map[string]string{}
	// Build maxDiscoveryDepth+1 nested directories (so the dir is at depth
	// maxDiscoveryDepth+1 = 9, which is > 8), then place an opencode.db
	// inside.
	deep := make([]string, 0, maxDiscoveryDepth+1)
	for i := 0; i < maxDiscoveryDepth+1; i++ {
		part := "d" + string(rune('a'+i))
		deep = append(deep, part)
	}
	rel := filepath.Join(append(deep, "opencode.db")...)
	tree[rel] = "FILE"
	tree[filepath.Join("opencode.db")] = "FILE"
	root := fixtureTree(t, tree)
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("Discover emitted %d instances, want 1 (only root opencode.db; deep subtree must be skipped)", len(instances))
	}
}

// TestDiscover_SymlinkOpenCodeDBOutsideRoot exercises the IsInsideRoot
// branch for a symlink named opencode.db whose canonical target lies
// outside the project root. The adapter must reject the candidate.
func TestDiscover_SymlinkOpenCodeDBOutsideRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink-based coverage exercised on POSIX; Windows symlinks require admin")
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "opencode.db"), []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	root := fixtureTree(t, map[string]string{
		"opencode.db": "SYMLINK<-" + filepath.Join(outside, "opencode.db"),
	})
	instances, err := NewAdapter().Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("Discover returned %d instances, want 0 (symlink to outside root must be rejected)", len(instances))
	}
}

// TestDiscover_WalkErrorSubdir exercises the walkErr-with-d.IsDir branch.
// The walker hits a permission-denied subdir, sees the walkErr, and skips
// the subtree without surfacing the error to the caller.
func TestDiscover_WalkErrorSubdir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions only; chmod 000 is a no-op on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses chmod 000; cannot exercise walkErr on root")
	}
	tmp := t.TempDir()
	subdir := filepath.Join(tmp, "locked")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	if err := os.Chmod(subdir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(subdir, 0o755) })

	if err := os.WriteFile(filepath.Join(tmp, "opencode.db"), []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write root opencode.db: %v", err)
	}
	instances, err := NewAdapter().Discover(context.Background(), tmp)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("Discover emitted %d instances, want 1 (locked subdir should be skipped)", len(instances))
	}
}

// TestVerifyOpenCodeSchema_CancelledContext exercises the
// verifyOpenCodeSchema short-circuit when the context is already cancelled.
func TestVerifyOpenCodeSchema_CancelledContext(t *testing.T) {
	dbPath := newFixtureDB(t, nil)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ok := verifyOpenCodeSchema(ctx, db); ok {
		t.Fatal("verifyOpenCodeSchema on cancelled context = true, want false")
	}
}

// TestJobs_FuncWrongSourceInstanceType exercises the type-assertion-fail
// branch in the Func returned by Job. The semantic result is empty and
// the error is nil.
func TestJobs_FuncWrongSourceInstanceType(t *testing.T) {
	result, err := NewAdapter().Job().Func(context.Background(), semantic.Deps{SourceInstance: "not-a-SourceInstance"})
	if err != nil {
		t.Fatalf("Func error: %v", err)
	}
	if len(result.Envelopes) != 0 {
		t.Fatalf("Result.Envelopes = %d, want 0", len(result.Envelopes))
	}
	if result.ErrorCode != "" {
		t.Fatalf("Result.ErrorCode = %q, want empty", result.ErrorCode)
	}
}

// TestJobs_FuncUnwrapsEnvelopes exercises the envelope-unwrap branch and
// the missing "last_message_id" key in the cursor map. The cursor key
// only matches the Codex-style carriers; opencode uses last_session_id
// + last_sequence, so NextCursor must end up empty.
func TestJobs_FuncUnwrapsEnvelopes(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "hi", CreatedAt: 1700000010000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	path := dbPath
	result, err := NewAdapter().Job().Func(context.Background(), semantic.Deps{SourceInstance: validScanInstance(path)})
	if err != nil {
		t.Fatalf("Func: %v", err)
	}
	if len(result.Envelopes) != 1 {
		t.Fatalf("Result.Envelopes = %d, want 1", len(result.Envelopes))
	}
	if result.NextCursor != "" {
		t.Fatalf("Result.NextCursor = %q, want empty (opencode cursor uses different keys)", result.NextCursor)
	}
	if _, ok := result.Envelopes[0].(experience.ExperienceEnvelope); !ok {
		t.Fatalf("Envelope[0] type = %T, want experience.ExperienceEnvelope", result.Envelopes[0])
	}
}

// TestScan_UpdatedAtPopulated verifies the Session.UpdatedAt field is
// stamped on every emitted envelope. The session fixture has a populated
// updated_at; the envelope must reflect it.
func TestScan_UpdatedAtPopulated(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "hi", CreatedAt: 1700000010000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 1 {
		t.Fatalf("Envelopes = %d, want 1", len(result.Envelopes))
	}
	wantUpdated := time.UnixMilli(1700000060000).UTC()
	if !result.Envelopes[0].Session.UpdatedAt.Equal(wantUpdated) {
		t.Fatalf("UpdatedAt = %v, want %v", result.Envelopes[0].Session.UpdatedAt, wantUpdated)
	}
}

// TestScan_ZeroClosedAt verifies the closedAt-Valid branch: a session with
// closed_at=0 is treated as "still open" and never stamps ClosedAt on the
// envelope.
func TestScan_ZeroClosedAt(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000, ClosedAt: 0}}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "hi", CreatedAt: 1700000010000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 1 {
		t.Fatalf("Envelopes = %d, want 1", len(result.Envelopes))
	}
	if result.Envelopes[0].Session.ClosedAt != nil {
		t.Fatalf("ClosedAt = %v, want nil (closed_at=0 means still open)", result.Envelopes[0].Session.ClosedAt)
	}
}

// TestScan_NextCursorPreservedWhenEmpty verifies the branch that copies the
// input cursor forward when no envelopes are emitted (regression for the
// persisted-cursor contract).
func TestScan_NextCursorPreservedWhenEmpty(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "user", Content: "hi", CreatedAt: 1700000010000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	inputCursor := map[string]any{"last_session_id": "s1", "last_sequence": int64(1)}
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
		Cursor:      inputCursor,
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 0 {
		t.Fatalf("Envelopes = %d, want 0 (cursor + Since excludes everything)", len(result.Envelopes))
	}
	if result.NextCursor == nil {
		t.Fatal("NextCursor = nil, want the input cursor passed forward")
	}
}

// TestScan_RoleDefaultsToNeither exercises the role-default branch in
// buildEnvelope: a role other than "user"/"assistant" leaves both text
// fields blank. The session still produces an envelope.
func TestScan_RoleDefaultsToNeither(t *testing.T) {
	sessions := []sessionFixture{{ID: "s1", StartedAt: 1700000000000, UpdatedAt: 1700000060000}}
	messages := []messageFixture{
		{ID: "t1", SessionID: "s1", Sequence: 1, Role: "system", Content: "x", CreatedAt: 1700000010000, Complete: 1},
	}
	dbPath := newFixtureDB(t, func(db *sql.DB) { populateFixture(t, db, sessions, messages) })

	adapter := NewAdapter()
	result, err := adapter.Scan(context.Background(), ScanRequest{
		ProjectRoot: filepath.Dir(dbPath),
		Instance:    validScanInstance(dbPath),
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Envelopes) != 1 {
		t.Fatalf("Envelopes = %d, want 1", len(result.Envelopes))
	}
	if result.Envelopes[0].Turn.UserText != "" || result.Envelopes[0].Turn.AssistantText != "" {
		t.Fatalf("system-role envelope leaked text: user=%q assistant=%q", result.Envelopes[0].Turn.UserText, result.Envelopes[0].Turn.AssistantText)
	}
}

// TestResolveTrace_QueryError exercises the non-NoRows, non-Canceled query
// branch. We trigger it by closing the DB before calling ResolveTrace, so
// the query fails with "sql: database is closed".
func TestResolveTrace_QueryErrorClosedDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE sessions (id TEXT PRIMARY KEY, project_id TEXT, started_at INTEGER, updated_at INTEGER)"); err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE messages (id TEXT PRIMARY KEY, session_id TEXT, sequence INTEGER, role TEXT, content TEXT, created_at INTEGER, complete INTEGER, revision TEXT)"); err != nil {
		t.Fatalf("create messages: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := os.Remove(dbPath); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind:   "sqlite",
		Path:   dbPath,
		TurnID: "t-1",
	}, TraceBounds{MaxBytes: 1024})
	// The DB is unreadable as SQLite; resolveTrace first hits hashFileContents
	// (which returns the hash of the raw bytes), then sql.Open/Ping fails.
	if result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceSourceNotFound)
	}
}

// TestResolveTrace_NonSQLitePingError exercises the PingContext error
// branch in ResolveTrace. A file that is not a valid SQLite database opens
// the connection pool but the ping fails; the adapter must map this to
// ErrExperienceSourceNotFound.
func TestResolveTrace_NonSQLitePingError(t *testing.T) {
	dbPath := writeRawDB(t, []byte("not a sqlite database"))
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), domain.TranscriptLocator{
		Kind:   "sqlite",
		Path:   dbPath,
		TurnID: "t-1",
	}, TraceBounds{MaxBytes: 1024})
	if result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("ResolveTrace Code = %q, want %q", result.Code, domain.ErrExperienceSourceNotFound)
	}
}

// TestScan_NonSQLitePingError exercises the PingContext error branch in
// Scan. A non-SQLite file triggers hashFileContents success (raw bytes)
// followed by sql.Open/Ping failure.
func TestScan_NonSQLitePingError(t *testing.T) {
	dbPath := writeRawDB(t, []byte("not a sqlite database"))
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
	if result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("Scan Code = %q, want %q", result.Code, domain.ErrExperienceSourceNotFound)
	}
}

// TestResolveTrace_MaxBytesBelowSuffix exercises the redactExcerpt
// limit<0 branch: a MaxBytes below the TraceExcerptSuffix length (3) yields
// a zero limit; the only output is the suffix itself ("...").
func TestResolveTrace_MaxBytesBelowSuffix(t *testing.T) {
	dbPath := newFixtureDB(t, func(db *sql.DB) {
		insertSessionForTrace(t, db, "s-1")
		insertMessageForTrace(t, db, messageFixtureForTrace{
			ID: "t-1", SessionID: "s-1", Sequence: 1, Role: "user",
			Content: "hello", CreatedAt: 1700000010000,
		})
	})
	adapter := NewAdapter()
	result := adapter.ResolveTrace(context.Background(), locatorFor(t, dbPath, "t-1"), TraceBounds{MaxBytes: 1})
	if got := len(result.Excerpt); got > 3 {
		t.Fatalf("ResolveTrace Excerpt length = %d, want <= 3 (suffix only)", got)
	}
	if result.Code != "" {
		t.Fatalf("ResolveTrace Code = %q, want empty on success", result.Code)
	}
}

// TestJobRegistryEntry_PopulatedFields verifies the IngestJobRegistryEntry
// helper returns an entry with all required fields populated, including a
// UTC-normalized CreatedAt.
func TestJobRegistryEntry_PopulatedFields(t *testing.T) {
	loc, err := time.LoadLocation("America/Buenos_Aires")
	if err != nil {
		t.Skip("no tzdata available")
	}
	createdAt := time.Date(2026, 7, 24, 12, 0, 0, 0, loc)
	entry := newIngestJobRegistryEntry(createdAt)
	if entry.JobName != JobName {
		t.Fatalf("JobName = %q, want %q", entry.JobName, JobName)
	}
	if entry.Description == "" {
		t.Fatal("Description is empty")
	}
	if entry.DefaultIntervalSec <= 0 {
		t.Fatalf("DefaultIntervalSec = %d, want > 0", entry.DefaultIntervalSec)
	}
	if entry.DefaultMaxRetries <= 0 {
		t.Fatalf("DefaultMaxRetries = %d, want > 0", entry.DefaultMaxRetries)
	}
	if entry.CreatedAt.Location() != time.UTC {
		t.Fatalf("CreatedAt location = %v, want UTC", entry.CreatedAt.Location())
	}
	if entry.Intent == "" {
		t.Fatal("Intent is empty")
	}
	if entry.Scope == "" {
		t.Fatal("Scope is empty")
	}
	if entry.RiskClass == "" {
		t.Fatal("RiskClass is empty")
	}
}

// TestJob_BindingsFromRegistryEntry verifies the Job returned by the
// Accessor copies every field from the IngestJobRegistryEntry. Source must
// be the opencode source constant.
func TestJob_BindingsFromRegistryEntry(t *testing.T) {
	adapter := NewAdapter()
	adapter.Now = func() time.Time { return time.Unix(0, 0).UTC() }
	job := adapter.Job()
	if job.Name != JobName {
		t.Fatalf("Name = %q, want %q", job.Name, JobName)
	}
	if job.DefaultIntervalSec <= 0 {
		t.Fatalf("DefaultIntervalSec = %d, want > 0", job.DefaultIntervalSec)
	}
	if job.DefaultMaxRetries <= 0 {
		t.Fatalf("DefaultMaxRetries = %d, want > 0", job.DefaultMaxRetries)
	}
}

// TestSemanticResult_NoMessageID exercises the branch in semanticResult
// where the cursor map has no "last_message_id" key. NextCursor must be
// empty.
func TestSemanticResult_NoMessageID(t *testing.T) {
	result := semanticResult(nil, 0, 0, map[string]any{"last_session_id": "s1", "last_sequence": int64(1)})
	if result.NextCursor != "" {
		t.Fatalf("NextCursor = %q, want empty (no last_message_id key)", result.NextCursor)
	}
	if len(result.Envelopes) != 0 {
		t.Fatalf("Envelopes = %d, want 0", len(result.Envelopes))
	}
}

// TestHashFileContents_Directory exercises the io.Copy error branch inside
// hashFileContents. A directory opens without "not exist" but fails to
// read.
func TestHashFileContents_Directory(t *testing.T) {
	dir := t.TempDir()
	if _, err := hashFileContents(dir); err == nil {
		t.Fatal("hashFileContents(dir) error = nil, want an error")
	} else if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("hashFileContents(dir) error = %v, want non-NotExist", err)
	}
}

// TestDepthOf_TrailingSlash covers the slash-counting branch. A trailing
// separator still counts as one of the separators in the relative path.
func TestDepthOf_TrailingSlash(t *testing.T) {
	if got := depthOf("a/b/"); got != 3 {
		t.Fatalf("depthOf(\"a/b/\") = %d, want 3", got)
	}
}
