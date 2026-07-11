package evidence

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBlobStorePutAndGet(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := Store(dir)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	content := []byte("Hello, this is evidence content for testing.")
	hash, err := store.Put(content)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if hash == "" {
		t.Fatal("Put returned empty hash")
	}
	if len(hash) != 64 {
		t.Errorf("Hash length = %d, want 64", len(hash))
	}

	got, err := store.Get(hash)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Error("Get returned different content")
	}
}

func TestBlobStorePutSameContentSameHash(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := Store(dir)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	content := []byte("deterministic hash test")
	hash1, err := store.Put(content)
	if err != nil {
		t.Fatalf("Put 1: %v", err)
	}
	hash2, err := store.Put(content)
	if err != nil {
		t.Fatalf("Put 2: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("Same content produced different hashes: %q vs %q", hash1, hash2)
	}
}

func TestBlobStorePutDifferentContentDifferentHash(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := Store(dir)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	hash1, _ := store.Put([]byte("content A"))
	hash2, _ := store.Put([]byte("content B"))

	if hash1 == hash2 {
		t.Error("Different content produced same hash")
	}
}

func TestBlobStoreExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := Store(dir)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	hash, _ := store.Put([]byte("test content"))
	if !store.Exists(hash) {
		t.Error("Exists returned false for stored blob")
	}
	if store.Exists("a" + hash[1:]) {
		t.Error("Exists returned true for non-existent blob")
	}
}

func TestBlobStoreGetNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := Store(dir)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	_, err = store.Get("0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("Get for non-existent blob: expected error")
	}
}

func TestBlobStoreGetInvalidHash(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := Store(dir)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	_, err = store.Get("../../../etc/passwd")
	if err == nil {
		t.Fatal("Get with path traversal: expected error")
	}
}

func TestBlobStoreStorageLayout(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := Store(dir)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	hash, _ := store.Put([]byte("layout test"))
	prefix := hash[:2]
	expectedPath := filepath.Join(dir, "sha256", prefix, hash)

	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("blob file %q not found: %v", expectedPath, err)
	}
}

func TestBlobStorePutEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := Store(dir)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	hash, err := store.Put([]byte{})
	if err != nil {
		t.Fatalf("Put empty: %v", err)
	}
	got, err := store.Get(hash)
	if err != nil {
		t.Fatalf("Get empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty bytes, got %d bytes", len(got))
	}
}

func TestBlobStoreNilStore(t *testing.T) {
	t.Parallel()

	var store *BlobStore
	_, err := store.Put([]byte("content"))
	if err == nil {
		t.Fatal("Put on nil store: expected error")
	}
	_, err = store.Get("abc")
	if err == nil {
		t.Fatal("Get on nil store: expected error")
	}
}

func TestBlobStoreEnforcesConfiguredSizeBoundary(t *testing.T) {
	store, err := StoreWithinRoot(t.TempDir(), "blobs", 4)
	if err != nil {
		t.Fatalf("StoreWithinRoot: %v", err)
	}
	if _, err := store.Put([]byte("1234")); err != nil {
		t.Fatalf("Put at limit: %v", err)
	}
	if _, err := store.Put([]byte("12345")); err == nil {
		t.Fatal("Put above limit succeeded")
	}
}

func TestStoreWithinRootRejectsTraversalAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := StoreWithinRoot(root, "../outside", 10); err == nil {
		t.Fatal("StoreWithinRoot accepted traversal")
	}

	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := StoreWithinRoot(root, filepath.Join("link", "blobs"), 10); err == nil {
		t.Fatal("StoreWithinRoot accepted symlink escape")
	}
}
