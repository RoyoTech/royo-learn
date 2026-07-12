// Package testutil provides test helpers shared across packages.
package testutil

import (
	"fmt"
	"os"
	"testing"
	"time"
)

const removeAllAttempts = 20

// TempDir creates a temporary directory whose cleanup retries RemoveAll.
// Unlike testing.T.TempDir, this tolerates briefly lingering Windows file
// handles from SQLite and other asynchronous test resources.
func TempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "royo-test-*")
	if err != nil {
		t.Fatalf("testutil.TempDir: MkdirTemp: %v", err)
	}
	t.Cleanup(func() {
		if err := RemoveAllWithRetry(dir); err != nil {
			t.Errorf("testutil.TempDir: clean up %q: %v", dir, err)
		}
	})
	return dir
}

// RemoveAllWithRetry retries os.RemoveAll on directories where file handles
// may linger briefly after Close (Windows + modernc/sqlite). Adaptive: waits
// only as long as needed, unlike a fixed sleep. On Unix, the first attempt
// always succeeds, so there is zero overhead.
func RemoveAllWithRetry(dir string) error {
	return removeAllWithRetry(dir, os.RemoveAll, time.Sleep)
}

func removeAllWithRetry(dir string, removeAll func(string) error, sleep func(time.Duration)) error {
	var lastErr error
	for attempt := 1; attempt <= removeAllAttempts; attempt++ {
		lastErr = removeAll(dir)
		if lastErr == nil || os.IsNotExist(lastErr) {
			return nil
		}
		if attempt < removeAllAttempts {
			sleep(50 * time.Millisecond)
		}
	}
	return fmt.Errorf("testutil: remove directory %q after %d attempts: %w", dir, removeAllAttempts, lastErr)
}
