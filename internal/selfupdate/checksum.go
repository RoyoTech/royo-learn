package selfupdate

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ErrChecksumMismatch is returned when a downloaded archive does not
// match its published SHA-256 checksum.
var ErrChecksumMismatch = errors.New("checksum mismatch")

// ParseChecksums parses a GoReleaser checksums.txt document. Each
// non-empty line has the form "<hex>  <filename>" (an optional "*"
// binary-mode marker before the filename is tolerated). It returns a map
// from filename to lowercase hex digest.
func ParseChecksums(data []byte) (map[string]string, error) {
	sums := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("checksums.txt line %d: expected \"<hex>  <filename>\", got %q", lineNumber, line)
		}
		digest := strings.ToLower(fields[0])
		if _, err := hex.DecodeString(digest); err != nil || len(digest) != sha256.Size*2 {
			return nil, fmt.Errorf("checksums.txt line %d: %q is not a SHA-256 hex digest", lineNumber, fields[0])
		}
		name := strings.TrimPrefix(fields[1], "*")
		sums[name] = digest
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksums.txt: %w", err)
	}
	return sums, nil
}

// VerifyFileChecksum streams the file at path through SHA-256 and
// compares the digest with wantHex (case-insensitive). It returns an
// error wrapping ErrChecksumMismatch when the digests differ.
func VerifyFileChecksum(path, wantHex string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s for checksum verification: %w", path, err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("hash %s: %w", path, err)
	}
	gotHex := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(gotHex, wantHex) {
		return fmt.Errorf("%w for %s: expected %s, got %s", ErrChecksumMismatch, path, strings.ToLower(wantHex), gotHex)
	}
	return nil
}
