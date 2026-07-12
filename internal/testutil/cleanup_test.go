package testutil

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRemoveAllWithRetryExistingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "remove-me")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := RemoveAllWithRetry(dir); err != nil {
		t.Fatalf("RemoveAllWithRetry(%q): %v", dir, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Stat(%q) error = %v, want not exist", dir, err)
	}
}

func TestRemoveAllWithRetryAbsentDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")

	if err := RemoveAllWithRetry(dir); err != nil {
		t.Fatalf("RemoveAllWithRetry(%q): %v", dir, err)
	}
}

func TestRemoveAllWithRetryReturnsFinalError(t *testing.T) {
	wantErr := errors.New("directory remains locked")
	const dir = `C:\locked\test-directory`
	attempts := 0

	err := removeAllWithRetry(dir, func(gotDir string) error {
		attempts++
		if gotDir != dir {
			t.Fatalf("removeAll dir = %q, want %q", gotDir, dir)
		}
		return wantErr
	}, func(time.Duration) {})

	if err == nil {
		t.Fatal("removeAllWithRetry error = nil, want failure")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("removeAllWithRetry error = %v, want wrapped %v", err, wantErr)
	}
	if attempts != removeAllAttempts {
		t.Fatalf("removeAllWithRetry attempts = %d, want %d", attempts, removeAllAttempts)
	}
	if !strings.Contains(err.Error(), strconv.Quote(dir)) || !strings.Contains(err.Error(), "20 attempts") {
		t.Fatalf("removeAllWithRetry error = %q, want directory and attempt count", err)
	}
}
