package evidence

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// BlobStore provides content-addressable storage for evidence blobs.
// Blobs are stored under <dir>/sha256/<prefix>/<hash> where prefix is
// the first 2 characters of the hex-encoded SHA-256 digest.
type BlobStore struct {
	dir string
}

// Store opens (or creates) a blob store rooted at dir.
func Store(dir string) (*BlobStore, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("blob store: cannot resolve path: %w", err)
	}
	// Create the base directory.
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("blob store: cannot create directory: %w", err)
	}
	return &BlobStore{dir: abs}, nil
}

// Put stores content and returns its SHA-256 hex digest. The content is
// written atomically (temp file + rename).
func (b *BlobStore) Put(content []byte) (string, error) {
	if b == nil {
		return "", fmt.Errorf("blob store: nil store")
	}

	sum := sha256.Sum256(content)
	hash := fmt.Sprintf("%x", sum)

	prefix := hash[:2]
	blobDir := filepath.Join(b.dir, "sha256", prefix)
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		return "", fmt.Errorf("blob store: create prefix dir: %w", err)
	}

	blobPath := filepath.Join(blobDir, hash)

	// If already exists, skip write.
	if _, err := os.Stat(blobPath); err == nil {
		return hash, nil
	}

	// Atomic write: write to temp, then rename.
	tmpPath := blobPath + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0o644); err != nil {
		return "", fmt.Errorf("blob store: write temp: %w", err)
	}
	if err := os.Rename(tmpPath, blobPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("blob store: rename: %w", err)
	}

	return hash, nil
}

// validHashRe matches a valid hex SHA-256 string (64 lowercase hex chars).
var validHashRe = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Get retrieves the blob content by its SHA-256 hash.
func (b *BlobStore) Get(hash string) ([]byte, error) {
	if b == nil {
		return nil, fmt.Errorf("blob store: nil store")
	}
	if !validHashRe.MatchString(hash) {
		return nil, fmt.Errorf("blob store: invalid hash %q", hash)
	}

	blobPath := filepath.Join(b.dir, "sha256", hash[:2], hash)
	data, err := os.ReadFile(blobPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("blob not found: %s", hash)
		}
		return nil, fmt.Errorf("blob store: read: %w", err)
	}
	return data, nil
}

// Exists reports whether a blob with the given hash exists in the store.
func (b *BlobStore) Exists(hash string) bool {
	if b == nil {
		return false
	}
	if !validHashRe.MatchString(hash) {
		return false
	}
	blobPath := filepath.Join(b.dir, "sha256", hash[:2], hash)
	_, err := os.Stat(blobPath)
	return err == nil
}
