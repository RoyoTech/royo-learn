package opencode

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

// validOpenCodeInstance wraps a freshly-created fixture path into a
// SourceInstance suitable for the Health adapter call.
func validOpenCodeInstance(t *testing.T, dbPath string) SourceInstance {
	t.Helper()
	return SourceInstance{
		Source:      domain.SourceOpenCode,
		ProjectRoot: filepath.Dir(dbPath),
		DBPath:      dbPath,
		Schema:      SchemaTag,
		Discovered:  time.Unix(0, 0).UTC(),
	}
}

// TestHealth_OK verifies the canonical happy path: a freshly-created
// OpenCode-shaped SQLite database yields Status="ok" with both flags set.
func TestHealth_OK(t *testing.T) {
	dbPath := newFixtureDB(t, nil)
	adapter := NewAdapter()
	adapter.Now = func() time.Time { return time.Unix(0, 0).UTC() }

	result := adapter.Health(context.Background(), validOpenCodeInstance(t, dbPath))
	if result.Status != "ok" {
		t.Fatalf("Health Status = %q, want %q (result=%+v)", result.Status, "ok", result)
	}
	if !result.Readable || !result.SchemaOK {
		t.Fatalf("Health Readable=%v SchemaOK=%v, want both true", result.Readable, result.SchemaOK)
	}
	if result.Code != "" {
		t.Fatalf("Health Code = %q, want empty on success", result.Code)
	}
	if result.DBPath != dbPath {
		t.Fatalf("Health DBPath = %q, want %q", result.DBPath, dbPath)
	}
	if result.CheckedAt.IsZero() {
		t.Fatalf("Health CheckedAt is zero")
	}
}

// TestHealth_NoSourceSideEffects verifies that Health did not mutate the
// source database. The gate "cero side effects en la fuente OpenCode"
// depends on this property.
func TestHealth_NoSourceSideEffects(t *testing.T) {
	dbPath := newFixtureDB(t, nil)
	before := fileMtime(t, dbPath)

	adapter := NewAdapter()
	for i := 0; i < 3; i++ {
		result := adapter.Health(context.Background(), validOpenCodeInstance(t, dbPath))
		if result.Status != "ok" {
			t.Fatalf("Health iteration %d status = %q, want ok (result=%+v)", i, result.Status, result)
		}
	}
	after := fileMtime(t, dbPath)
	if before != after {
		t.Fatalf("Health mutated source: mtime before=%d after=%d", before, after)
	}
}

// TestHealth_MissingDB returns ErrExperienceSourceNotFound when the file
// does not exist. The caller treats this as "source disappeared" and
// reports degraded ingestion, never as a fatal error.
func TestHealth_MissingDB(t *testing.T) {
	adapter := NewAdapter()
	result := adapter.Health(context.Background(), validOpenCodeInstance(t, "/no/such/path.db"))
	if result.Status != "degraded" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "degraded")
	}
	if result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrExperienceSourceNotFound)
	}
	if result.Readable || result.SchemaOK {
		t.Fatalf("Health flags readable=%v schemaOK=%v, want both false", result.Readable, result.SchemaOK)
	}
}

// TestHealth_PathIsDirectory rejects a directory at the opencode.db path.
// Health must not open a directory as a database; this signals "source
// layout changed" and degrades gracefully.
func TestHealth_PathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	if err := os.MkdirAll(dbPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	adapter := NewAdapter()
	result := adapter.Health(context.Background(), validOpenCodeInstance(t, dbPath))
	if result.Status != "degraded" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "degraded")
	}
	if result.Code != string(domain.ErrExperienceSourceNotFound) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrExperienceSourceNotFound)
	}
}

// TestHealth_UnsupportedSchema rejects a SQLite database at the opencode.db
// path that does not contain the expected OpenCode tables. The fixture
// creates a database with unrelated tables; Health detects the missing
// "sessions" and "messages" tables and reports ErrExperienceSchemaUnsupported.
func TestHealth_UnsupportedSchema(t *testing.T) {
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
	result := adapter.Health(context.Background(), validOpenCodeInstance(t, dbPath))
	if result.Status != "degraded" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "degraded")
	}
	if result.Code != string(domain.ErrExperienceSchemaUnsupported) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrExperienceSchemaUnsupported)
	}
	if !result.Readable {
		t.Fatalf("Health Readable = false, want true (file did open)")
	}
	if result.SchemaOK {
		t.Fatalf("Health SchemaOK = true, want false on unsupported schema")
	}
}

// TestHealth_NonSQLiteFile rejects a file that is not a SQLite database.
// The read-only open fails and Health degrades with source_not_found.
func TestHealth_NonSQLiteFile(t *testing.T) {
	dbPath := writeRawDB(t, []byte("not a sqlite database"))
	adapter := NewAdapter()
	result := adapter.Health(context.Background(), validOpenCodeInstance(t, dbPath))
	if result.Status != "degraded" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "degraded")
	}
	if result.Readable {
		t.Fatalf("Health Readable = true, want false on non-SQLite file")
	}
	if result.SchemaOK {
		t.Fatalf("Health SchemaOK = true, want false on non-SQLite file")
	}
}

// TestHealth_RejectsWrongSource rejects an instance whose Source is not
// opencode. The adapter refuses to health-check stores belonging to other
// adapters; the typed error is ErrInvalidArgument.
func TestHealth_RejectsWrongSource(t *testing.T) {
	dbPath := newFixtureDB(t, nil)
	instance := validOpenCodeInstance(t, dbPath)
	instance.Source = domain.SourceClaudeCode
	adapter := NewAdapter()
	result := adapter.Health(context.Background(), instance)
	if result.Status != "error" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "error")
	}
	if result.Code != string(domain.ErrInvalidArgument) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrInvalidArgument)
	}
}

// TestHealth_RejectsEmptyDBPath rejects an instance without a DBPath. The
// caller must have run Discover first to obtain a SourceInstance; bare
// invocations cannot be trusted.
func TestHealth_RejectsEmptyDBPath(t *testing.T) {
	adapter := NewAdapter()
	result := adapter.Health(context.Background(), SourceInstance{
		Source:      domain.SourceOpenCode,
		ProjectRoot: t.TempDir(),
		DBPath:      "",
		Schema:      SchemaTag,
	})
	if result.Status != "error" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "error")
	}
	if result.Code != string(domain.ErrInvalidArgument) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrInvalidArgument)
	}
}

// TestHealth_ContextCanceled verifies that Health surfaces context
// cancellation as ErrTimeout. The contract requires every method to
// honor cancellation.
func TestHealth_ContextCanceled(t *testing.T) {
	dbPath := newFixtureDB(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	adapter := NewAdapter()
	result := adapter.Health(ctx, validOpenCodeInstance(t, dbPath))
	if result.Status != "error" {
		t.Fatalf("Health Status = %q, want %q", result.Status, "error")
	}
	if result.Code != string(domain.ErrTimeout) {
		t.Fatalf("Health Code = %q, want %q", result.Code, domain.ErrTimeout)
	}
}

// TestHealth_CheckedAtUTC verifies the CheckedAt timestamp is in UTC so
// downstream JSON consumers never have to deal with local-time drift.
func TestHealth_CheckedAtUTC(t *testing.T) {
	dbPath := newFixtureDB(t, nil)
	adapter := NewAdapter()
	adapter.Now = func() time.Time {
		loc, _ := time.LoadLocation("America/Buenos_Aires")
		return time.Date(2026, 7, 24, 12, 0, 0, 0, loc)
	}
	result := adapter.Health(context.Background(), validOpenCodeInstance(t, dbPath))
	if result.CheckedAt.Location() != time.UTC {
		t.Fatalf("Health CheckedAt location = %v, want UTC", result.CheckedAt.Location())
	}
}
