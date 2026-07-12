package storage

import (
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a *sql.DB connection with royo-learn-specific lifecycle helpers.
type DB struct {
	DB   *sql.DB
	path string
	mu   sync.Mutex
}

// Open opens a SQLite database at path and applies the required pragmas.
// It returns a ready-to-use *DB connection.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("storage: open %q: %w", path, err)
	}

	// Apply pragmas.
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	for _, p := range pragmas {
		if _, execErr := conn.Exec(p); execErr != nil {
			conn.Close()
			return nil, fmt.Errorf("storage: %s: %w", p, execErr)
		}
	}

	return &DB{DB: conn, path: path}, nil
}

// Close closes the underlying database connection.
//
// On Windows, modernc.org/sqlite in WAL mode may leave -wal and -shm sidecar
// files with lingering OS file handles for a few milliseconds after Close
// returns. This breaks t.TempDir() cleanup ("The directory is not empty")
// under -race, which slows handle release and widens the timing window. To
// stay reproducible we explicitly remove the sidecars on Windows after a
// successful Close, retrying briefly while the OS releases the handles.
func (db *DB) Close() error {
	if db.DB == nil {
		return nil
	}

	// Checkpoint the WAL into the main database so sidecar files can be
	// cleaned up. If checkpointing fails we still close and remove sidecars;
	// the WAL will be checkpointed on the next open.
	if _, cerr := db.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); cerr != nil {
		// Non-fatal: proceed to close regardless.
	}

	err := db.DB.Close()
	if err != nil {
		return fmt.Errorf("storage: close %q: %w", db.path, err)
	}

	if runtime.GOOS == "windows" {
		removeSidecarWithRetry(db.path + "-wal")
		removeSidecarWithRetry(db.path + "-shm")
		// Wait for the OS to release the main DB file handle. On Windows,
		// modernc/sqlite may hold the file handle briefly after Close
		// returns, causing t.TempDir() RemoveAll to fail with "directory
		// is not empty". We poll with os.Stat until the handle settles.
		waitForFileHandleRelease(db.path)
	}

	return nil
}

// removeSidecarWithRetry attempts to remove a SQLite sidecar file (-wal, -shm)
// with a short retry loop on Windows. The OS may hold a handle briefly after
// DB.Close() returns; without retry, os.Remove fails with "file in use".
// Returns nil if the file was removed or never existed.
func removeSidecarWithRetry(path string) {
	for attempt := 0; attempt < 5; attempt++ {
		err := os.Remove(path)
		if err == nil || os.IsNotExist(err) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitForFileHandleRelease waits for the OS to release the main DB file handle
// on Windows. After db.Close(), modernc/sqlite may hold the handle briefly,
// causing t.TempDir() RemoveAll to fail ("directory is not empty"). A brief
// bounded sleep is the most reliable workaround — polling with os.Rename or
// os.Stat does not detect the handle release because modernc opens files with
// FILE_SHARE_DELETE (rename succeeds while the handle is still held). The
// sleep is 100ms, sufficient for the OS to finalize handle release in
// practice. This runs only on Windows and only during DB.Close().
func waitForFileHandleRelease(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return // file already gone, nothing to wait for
	}
	time.Sleep(100 * time.Millisecond)
}
