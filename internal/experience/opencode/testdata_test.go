package opencode

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// opencodeTestSchema is the minimal OpenCode schema the Health and Scan
// code paths expect to find. Real OpenCode databases may carry additional
// tables; the adapter only gates on these two.
//
// Bumping this schema is a breaking change for any persisted fixture or
// production data; update docs/22-ADAPTER-CONTRACT.md §7 (opencode/sqlite-v1)
// in the same commit.
const opencodeTestSchema = `
CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    started_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    closed_at   INTEGER
);
CREATE TABLE messages (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL,
    sequence    INTEGER NOT NULL,
    role        TEXT NOT NULL,
    content     TEXT,
    finish      TEXT,
    created_at  INTEGER NOT NULL,
    complete    INTEGER NOT NULL DEFAULT 1,
    revision    TEXT
);
CREATE INDEX idx_messages_session ON messages(session_id, sequence);
`

// newFixtureDB writes an OpenCode-shaped SQLite database inside a fresh
// temp directory and returns its canonical absolute path. The caller may
// pass a populate function to insert rows after schema creation; nil is
// fine for tests that only exercise the empty database.
//
// The file lives under t.TempDir() so the test framework removes it on
// cleanup; the test never mutates a real OpenCode store on the host.
func newFixtureDB(t *testing.T, populate func(*sql.DB)) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	if _, err := db.Exec(opencodeTestSchema); err != nil {
		_ = db.Close()
		t.Fatalf("create schema: %v", err)
	}
	if populate != nil {
		populate(db)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	return dbPath
}

// writeRawDB writes an arbitrary byte payload as a file inside a temp dir
// and returns its canonical path. Used to simulate non-SQLite files at
// the opencode.db path.
func writeRawDB(t *testing.T, contents []byte) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	if err := os.WriteFile(dbPath, contents, 0o600); err != nil {
		t.Fatalf("write raw db: %v", err)
	}
	return dbPath
}

// fileMtime returns the modification time of the file at path. Tests use
// it to assert that Health did not touch the source database.
func fileMtime(t *testing.T, path string) (mtime int64) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.ModTime().UnixNano()
}
