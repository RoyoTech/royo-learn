// Package storagetest provides a test helper for creating temporary SQLite
// databases with migrations applied. It uses os.MkdirTemp (not t.TempDir)
// and registers a cleanup that retries RemoveAll to handle Windows file
// handle lingering after DB.Close().
package storagetest

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/testutil"
)

// OpenTemp creates a temporary SQLite database with migrations applied.
// The cleanup (registered via t.Cleanup) calls db.Close() and then
// testutil.RemoveAllWithRetry(dir) to handle Windows handle-release timing.
func OpenTemp(t *testing.T) *storage.DB {
	t.Helper()
	dir, err := os.MkdirTemp("", "royo-test-*")
	if err != nil {
		t.Fatalf("storagetest.OpenTemp: MkdirTemp: %v", err)
	}
	path := filepath.Join(dir, "test.db")
	db, err := storage.Open(path)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("storagetest.OpenTemp: Open: %v", err)
	}
	if err := storage.Migrate(db); err != nil {
		db.Close()
		os.RemoveAll(dir)
		t.Fatalf("storagetest.OpenTemp: Migrate: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("storagetest.OpenTemp: close database %q: %v", path, err)
		}
		if err := testutil.RemoveAllWithRetry(dir); err != nil {
			t.Errorf("storagetest.OpenTemp: clean up database directory %q: %v", dir, err)
		}
	})
	return db
}

var memoryCounter atomic.Uint64

// OpenMemory creates an in-memory SQLite database with migrations applied.
// The cleanup (registered via t.Cleanup) calls db.Close(). Use this when
// tests need maximum speed and don't require file-based persistence. Returns
// the DB and the DSN string (for Config.DBPath in mcpserver tests).
func OpenMemory(t *testing.T) (*storage.DB, string) {
	t.Helper()
	dsn := fmt.Sprintf("file:test-memory-%d?mode=memory&cache=shared", memoryCounter.Add(1))
	db, err := storage.Open(dsn)
	if err != nil {
		t.Fatalf("storagetest.OpenMemory: Open: %v", err)
	}
	if err := storage.Migrate(db); err != nil {
		db.Close()
		t.Fatalf("storagetest.OpenMemory: Migrate: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("storagetest.OpenMemory: close database %q: %v", dsn, err)
		}
	})
	return db, dsn
}
