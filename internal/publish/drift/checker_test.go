// Tests for Checker.Check (Hito 12, T12.3). The four outcomes
// (ok, drifted, target_missing, target_unreadable) plus the context
// cancellation branch and the SHA-256 hex invariant are covered here.
//
// The contract tests for read-only enforcement and forbidden write APIs
// live in contract_test.go (T12.4) so the harness of TestRunOne-style
// static greps is colocated with the rest of the package guards.

package drift

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-royo-learn/internal/testutil"
)

// sha256OfHex returns the hex-encoded SHA-256 of the byte slice.
func sha256OfHex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// writeFile writes the given bytes to a fresh file under dir and returns
// the absolute path.
func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", p, err)
	}
	return p
}

// TestChecker_OKOnHashMatch asserts the ok outcome when the file's
// SHA-256 matches expectedHash. The Result carries ActualHash equal to
// the expected hash and a nil error.
func TestChecker_OKOnHashMatch(t *testing.T) {
	dir := testutil.TempDir(t)
	data := []byte("hello world")
	target := writeFile(t, dir, "hello.txt", data)

	c := NewChecker()
	res, err := c.Check(context.Background(), target, sha256OfHex(data))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Status != StatusOK {
		t.Errorf("Status = %q, want %q", res.Status, StatusOK)
	}
	if res.ActualHash != sha256OfHex(data) {
		t.Errorf("ActualHash = %q, want %q", res.ActualHash, sha256OfHex(data))
	}
	if res.Err != nil {
		t.Errorf("Err = %v, want nil", res.Err)
	}
}

// TestChecker_DriftedOnHashMismatch asserts the drifted outcome when the
// on-disk content does NOT match expectedHash. The Result carries the
// actual SHA-256 (different from expectedHash) and no error.
func TestChecker_DriftedOnHashMismatch(t *testing.T) {
	dir := testutil.TempDir(t)
	data := []byte("v1 contents")
	target := writeFile(t, dir, "thing.bin", data)

	expected := sha256OfHex([]byte("v0 contents that no longer exist"))

	c := NewChecker()
	res, err := c.Check(context.Background(), target, expected)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Status != StatusDrifted {
		t.Errorf("Status = %q, want %q", res.Status, StatusDrifted)
	}
	if res.ActualHash != sha256OfHex(data) {
		t.Errorf("ActualHash = %q, want %q (the on-disk hash)", res.ActualHash, sha256OfHex(data))
	}
	if res.ActualHash == expected {
		t.Error("ActualHash unexpectedly equals the (wrong) expected hash")
	}
	if res.Err != nil {
		t.Errorf("Err = %v, want nil", res.Err)
	}
}

// TestChecker_TargetMissingReturnsErrTargetMissing asserts that the
// target_missing outcome wraps ErrTargetMissing so callers can branch
// on it with errors.Is. The test uses a path under testutil.TempDir that
// does not exist.
func TestChecker_TargetMissingReturnsErrTargetMissing(t *testing.T) {
	dir := testutil.TempDir(t)
	missing := filepath.Join(dir, "absent.jsonl")

	c := NewChecker()
	res, err := c.Check(context.Background(), missing, "anything")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Status != StatusTargetMissing {
		t.Errorf("Status = %q, want %q", res.Status, StatusTargetMissing)
	}
	if !errors.Is(res.Err, ErrTargetMissing) {
		t.Errorf("res.Err = %v, want errors.Is(_, ErrTargetMissing) = true", res.Err)
	}
}

// TestChecker_TargetUnreadableWrapsUnderlying asserts that an os.Open
// error after a successful stat (here injected via Checker.openFn)
// produces target_unreadable with ErrTargetUnreadable wrapped via
// errors.Join alongside the underlying cause.
func TestChecker_TargetUnreadableWrapsUnderlying(t *testing.T) {
	dir := testutil.TempDir(t)
	target := writeFile(t, dir, "perm.bin", []byte("locked"))

	underlying := errors.New("synthetic open failure")
	c := &Checker{
		openFn: func(name string) (*os.File, error) {
			return nil, underlying
		},
	}
	res, err := c.Check(context.Background(), target, "ignored")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Status != StatusTargetUnreadable {
		t.Errorf("Status = %q, want %q", res.Status, StatusTargetUnreadable)
	}
	if !errors.Is(res.Err, ErrTargetUnreadable) {
		t.Errorf("res.Err = %v, want errors.Is(_, ErrTargetUnreadable) = true", res.Err)
	}
	if !errors.Is(res.Err, underlying) {
		t.Errorf("res.Err = %v, want errors.Is(_, underlying) = true (errors.Join)", res.Err)
	}
	if res.ActualHash != "" {
		t.Errorf("ActualHash = %q, want empty on unreadable", res.ActualHash)
	}
}

// TestChecker_RespectsContextCancellation asserts that a pre-cancelled
// context short-circuits to target_unreadable without performing I/O.
func TestChecker_RespectsContextCancellation(t *testing.T) {
	dir := testutil.TempDir(t)
	target := writeFile(t, dir, "ctx.bin", []byte("never read"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	c := NewChecker()
	res, err := c.Check(ctx, target, sha256OfHex([]byte("never read")))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Status != StatusTargetUnreadable {
		t.Errorf("Status = %q, want %q", res.Status, StatusTargetUnreadable)
	}
	if !errors.Is(res.Err, ErrTargetUnreadable) {
		t.Errorf("res.Err = %v, want errors.Is(_, ErrTargetUnreadable) = true", res.Err)
	}
	if res.ActualHash != "" {
		t.Errorf("ActualHash = %q, want empty on cancelled ctx", res.ActualHash)
	}
}

// TestChecker_ActualHashIsHex asserts the ActualHash is exactly 64 hex
// characters long for both ok and drifted outcomes. SHA-256 produces 32
// bytes which encode to 64 lowercase hex characters.
func TestChecker_ActualHashIsHex(t *testing.T) {
	cases := []struct {
		name       string
		data       []byte
		expected   string
		wantStatus Status
	}{
		{"ok", []byte("ok content"), sha256OfHex([]byte("ok content")), StatusOK},
		{"drifted", []byte("real content"), sha256OfHex([]byte("other")), StatusDrifted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempDir(t)
			target := writeFile(t, dir, "f.bin", tc.data)

			c := NewChecker()
			res, err := c.Check(context.Background(), target, tc.expected)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if res.Status != tc.wantStatus {
				t.Fatalf("Status = %q, want %q", res.Status, tc.wantStatus)
			}
			if len(res.ActualHash) != 64 {
				t.Errorf("len(ActualHash) = %d, want 64", len(res.ActualHash))
			}
			if _, err := hex.DecodeString(res.ActualHash); err != nil {
				t.Errorf("ActualHash %q is not valid hex: %v", res.ActualHash, err)
			}
		})
	}
}

// TestChecker_LargeFileStreamingHash asserts the streaming hash matches
// the reference SHA-256 for an 8 KiB random payload. The intent is to
// verify the io.Copy loop streams rather than buffering the entire
// file (the 8 KiB size is below any sane buffer threshold but exercises
// the io.Reader path).
func TestChecker_LargeFileStreamingHash(t *testing.T) {
	dir := testutil.TempDir(t)

	// 8 KiB random data
	payload := make([]byte, 8*1024)
	if _, err := io.ReadFull(rand.Reader, payload); err != nil {
		t.Fatalf("rand.ReadFull: %v", err)
	}
	target := writeFile(t, dir, "big.bin", payload)

	c := NewChecker()
	res, err := c.Check(context.Background(), target, sha256OfHex(payload))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Status != StatusOK {
		t.Errorf("Status = %q, want %q", res.Status, StatusOK)
	}
	if res.ActualHash != sha256OfHex(payload) {
		t.Errorf("ActualHash = %q, want %q", res.ActualHash, sha256OfHex(payload))
	}
}

// TestChecker_OpenFnNilUsesDefault asserts that a Checker with a nil
// openFn falls back to os.Open (production-default code path).
func TestChecker_OpenFnNilUsesDefault(t *testing.T) {
	dir := testutil.TempDir(t)
	data := []byte("nil-open-fn default")
	target := writeFile(t, dir, "f.bin", data)

	c := &Checker{openFn: nil}
	res, err := c.Check(context.Background(), target, sha256OfHex(data))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Status != StatusOK {
		t.Errorf("Status = %q, want %q", res.Status, StatusOK)
	}
}

// TestChecker_StatErrorOnNonEnotExentIsUnreadable asserts that an
// os.Stat error other than ENOENT (here simulated by feeding a path
// whose parent is a file, which yields ENOTDIR on most platforms) is
// classified as target_unreadable, not target_missing. This protects
// against a regression that would treat every stat error as missing.
func TestChecker_StatErrorOnNonEnotExentIsUnreadable(t *testing.T) {
	dir := testutil.TempDir(t)
	// Create a regular file that is also a "directory" of a sibling path.
	// Stat("/some_file/child") returns ENOTDIR, not ENOENT.
	asFile := writeFile(t, dir, "asfile", []byte("not a dir"))
	bogus := filepath.Join(asFile, "child")

	c := NewChecker()
	res, err := c.Check(context.Background(), bogus, "ignored")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Status != StatusTargetUnreadable {
		t.Errorf("Status = %q, want %q (ENOTDIR should be unreadable, not missing)", res.Status, StatusTargetUnreadable)
	}
	if !errors.Is(res.Err, ErrTargetUnreadable) {
		t.Errorf("res.Err = %v, want errors.Is(_, ErrTargetUnreadable) = true", res.Err)
	}
}

// TestChecker_DriftedStatusStringStable pins the literal status values
// so the JSON envelope comparator and the SQLite CHECK constraint do
// not silently diverge across refactors.
func TestChecker_DriftedStatusStringStable(t *testing.T) {
	cases := []struct {
		got  Status
		want string
	}{
		{StatusOK, "ok"},
		{StatusDrifted, "drifted"},
		{StatusTargetMissing, "target_missing"},
		{StatusTargetUnreadable, "target_unreadable"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("Status literal = %q, want %q", string(c.got), c.want)
		}
	}
}

// TestChecker_SentinelErrorsAreDistinct asserts the two sentinel errors
// are distinct values (not aliases of each other) so errors.Is routing
// in the CLI/MCP surfaces the right user-facing message.
func TestChecker_SentinelErrorsAreDistinct(t *testing.T) {
	if errors.Is(ErrTargetMissing, ErrTargetUnreadable) {
		t.Error("ErrTargetMissing must not satisfy errors.Is(_, ErrTargetUnreadable)")
	}
	if errors.Is(ErrTargetUnreadable, ErrTargetMissing) {
		t.Error("ErrTargetUnreadable must not satisfy errors.Is(_, ErrTargetMissing)")
	}
	if !strings.Contains(ErrTargetMissing.Error(), "missing") {
		t.Errorf("ErrTargetMissing message = %q, want it to mention 'missing'", ErrTargetMissing.Error())
	}
	if !strings.Contains(ErrTargetUnreadable.Error(), "unreadable") {
		t.Errorf("ErrTargetUnreadable message = %q, want it to mention 'unreadable'", ErrTargetUnreadable.Error())
	}
}
