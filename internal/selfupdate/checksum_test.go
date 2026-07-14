package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestParseChecksums(t *testing.T) {
	archive := []byte("fake archive bytes")
	doc := sha256Hex(archive) + "  royo-learn-linux-amd64.tar.gz\n" +
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  royo-learn-windows-amd64.zip\n" +
		"\n" // trailing blank line must be tolerated

	sums, err := ParseChecksums([]byte(doc))
	if err != nil {
		t.Fatalf("ParseChecksums returned error: %v", err)
	}
	if got := sums["royo-learn-linux-amd64.tar.gz"]; got != sha256Hex(archive) {
		t.Fatalf("checksum for tar.gz = %q, want %q", got, sha256Hex(archive))
	}
	if got := sums["royo-learn-windows-amd64.zip"]; got != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("checksum for zip = %q", got)
	}
}

func TestParseChecksumsRejectsMalformedLine(t *testing.T) {
	if _, err := ParseChecksums([]byte("only-one-field\n")); err == nil {
		t.Fatal("ParseChecksums expected error for malformed line, got nil")
	}
}

func TestVerifyFileChecksumSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.tar.gz")
	content := []byte("archive payload")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := VerifyFileChecksum(path, sha256Hex(content)); err != nil {
		t.Fatalf("VerifyFileChecksum returned error for matching hash: %v", err)
	}
}

func TestVerifyFileChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.tar.gz")
	if err := os.WriteFile(path, []byte("archive payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	wrong := sha256Hex([]byte("something else"))
	err := VerifyFileChecksum(path, wrong)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("VerifyFileChecksum error = %v, want ErrChecksumMismatch", err)
	}
}
