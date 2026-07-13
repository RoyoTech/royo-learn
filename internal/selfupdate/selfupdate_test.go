package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// makeTarGz builds an in-memory .tar.gz archive containing a single file.
func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(content)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// makeZip builds an in-memory .zip archive containing a single file.
func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// releaseServer is a fake GitHub API + download host for one release.
type releaseServer struct {
	server        *httptest.Server
	tag           string
	assets        map[string][]byte
	downloadCount atomic.Int64
	apiCount      atomic.Int64
}

// newReleaseServer serves release metadata for tag under both the "latest"
// and "tags/{tag}" endpoints, plus asset downloads whose
// browser_download_url values point back at the test server itself.
func newReleaseServer(t *testing.T, tag string, assets map[string][]byte) *releaseServer {
	t.Helper()
	rs := &releaseServer{tag: tag, assets: assets}

	mux := http.NewServeMux()
	releaseHandler := func(w http.ResponseWriter, r *http.Request) {
		rs.apiCount.Add(1)
		if r.Header.Get("User-Agent") == "" {
			http.Error(w, "User-Agent header required", http.StatusBadRequest)
			return
		}
		type asset struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		}
		payload := struct {
			TagName string  `json:"tag_name"`
			Assets  []asset `json:"assets"`
		}{TagName: rs.tag}
		for name := range rs.assets {
			payload.Assets = append(payload.Assets, asset{
				Name:               name,
				BrowserDownloadURL: rs.server.URL + "/download/" + name,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}
	mux.HandleFunc("/repos/RoyoTech/royo-learn/releases/latest", releaseHandler)
	mux.HandleFunc("/repos/RoyoTech/royo-learn/releases/tags/"+tag, releaseHandler)
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		rs.downloadCount.Add(1)
		if r.Header.Get("User-Agent") == "" {
			http.Error(w, "User-Agent header required", http.StatusBadRequest)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/download/")
		content, ok := rs.assets[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(content)
	})

	rs.server = httptest.NewServer(mux)
	t.Cleanup(rs.server.Close)
	return rs
}

// newTestUpdater wires an Updater at the fake server with test overrides.
func newTestUpdater(t *testing.T, rs *releaseServer, currentVersion, execPath, goos, goarch string) *Updater {
	t.Helper()
	u, err := New(Config{
		APIBaseURL:        rs.server.URL,
		CurrentVersion:    currentVersion,
		ExecutablePath:    execPath,
		GOOS:              goos,
		GOARCH:            goarch,
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return u
}

func TestUpdateFullFlowTarGz(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "royo-learn")
	writeFileOrFatal(t, execPath, []byte("old executable"))

	newContent := []byte("new executable v0.2.0")
	archive := makeTarGz(t, "royo-learn", newContent)
	assetName := "royo-learn-linux-amd64.tar.gz"
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(archive), assetName)

	rs := newReleaseServer(t, "v0.2.0", map[string][]byte{
		assetName:       archive,
		"checksums.txt": []byte(checksums),
	})

	u := newTestUpdater(t, rs, "0.1.0", execPath, "linux", "amd64")
	result, err := u.Update(context.Background(), "")
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if !result.Updated {
		t.Fatal("result.Updated = false, want true")
	}
	if result.CurrentVersion != "0.1.0" {
		t.Fatalf("result.CurrentVersion = %q, want 0.1.0", result.CurrentVersion)
	}
	if result.NewVersion != "v0.2.0" {
		t.Fatalf("result.NewVersion = %q, want v0.2.0", result.NewVersion)
	}
	if result.Path != execPath {
		t.Fatalf("result.Path = %q, want %q", result.Path, execPath)
	}
	if got := string(readFileOrFatal(t, execPath)); got != string(newContent) {
		t.Fatalf("executable content = %q, want %q", got, newContent)
	}
}

func TestUpdateFullFlowZipWindows(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "royo-learn.exe")
	writeFileOrFatal(t, execPath, []byte("old executable"))

	newContent := []byte("new executable v0.2.0 for windows")
	archive := makeZip(t, "royo-learn.exe", newContent)
	assetName := "royo-learn-windows-amd64.zip"
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(archive), assetName)

	rs := newReleaseServer(t, "v0.2.0", map[string][]byte{
		assetName:       archive,
		"checksums.txt": []byte(checksums),
	})

	u := newTestUpdater(t, rs, "0.1.0", execPath, "windows", "amd64")
	result, err := u.Update(context.Background(), "")
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if !result.Updated {
		t.Fatal("result.Updated = false, want true")
	}
	if got := string(readFileOrFatal(t, execPath)); got != string(newContent) {
		t.Fatalf("executable content = %q, want %q", got, newContent)
	}
	// Windows-style replacement parks the previous binary as <name>.old.
	if got := string(readFileOrFatal(t, execPath+oldBinarySuffix)); got != "old executable" {
		t.Fatalf("%s content = %q, want the previous binary", oldBinarySuffix, got)
	}
}

func TestUpdateAlreadyUpToDate(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "royo-learn")
	writeFileOrFatal(t, execPath, []byte("current executable"))

	rs := newReleaseServer(t, "v0.1.8", map[string][]byte{})

	u := newTestUpdater(t, rs, "0.1.8", execPath, "linux", "amd64")
	result, err := u.Update(context.Background(), "")
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if result.Updated {
		t.Fatal("result.Updated = true, want false")
	}
	if got := string(readFileOrFatal(t, execPath)); got != "current executable" {
		t.Fatalf("executable content changed: %q", got)
	}
	if rs.downloadCount.Load() != 0 {
		t.Fatalf("downloads = %d, want 0 when already up to date", rs.downloadCount.Load())
	}
}

func TestUpdateRefusesImplicitDowngrade(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "royo-learn")
	writeFileOrFatal(t, execPath, []byte("current executable"))

	rs := newReleaseServer(t, "v0.2.0", map[string][]byte{})

	u := newTestUpdater(t, rs, "0.3.0", execPath, "linux", "amd64")
	_, err := u.Update(context.Background(), "")
	if err == nil {
		t.Fatal("Update expected error when latest release is older, got nil")
	}
	if !strings.Contains(err.Error(), "--version") {
		t.Fatalf("error %q should mention --version for explicit downgrade", err)
	}
	if got := string(readFileOrFatal(t, execPath)); got != "current executable" {
		t.Fatalf("executable content changed: %q", got)
	}
	if rs.downloadCount.Load() != 0 {
		t.Fatalf("downloads = %d, want 0 when downgrade is refused", rs.downloadCount.Load())
	}
}

func TestUpdateRefusesDevBuildWithoutExplicitVersion(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "royo-learn")
	writeFileOrFatal(t, execPath, []byte("dev executable"))

	rs := newReleaseServer(t, "v0.2.0", map[string][]byte{})

	u := newTestUpdater(t, rs, "dev", execPath, "linux", "amd64")
	_, err := u.Update(context.Background(), "")
	if !errors.Is(err, ErrDevBuild) {
		t.Fatalf("Update error = %v, want ErrDevBuild", err)
	}
	if !strings.Contains(err.Error(), "development build") {
		t.Fatalf("error %q should mention development build", err)
	}
	if rs.apiCount.Load() != 0 {
		t.Fatalf("API calls = %d, want 0 when dev build is refused", rs.apiCount.Load())
	}
	if got := string(readFileOrFatal(t, execPath)); got != "dev executable" {
		t.Fatalf("executable content changed: %q", got)
	}
}

func TestUpdateDevBuildAllowedWithExplicitVersion(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "royo-learn")
	writeFileOrFatal(t, execPath, []byte("dev executable"))

	newContent := []byte("released executable")
	archive := makeTarGz(t, "royo-learn", newContent)
	assetName := "royo-learn-linux-amd64.tar.gz"
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(archive), assetName)

	rs := newReleaseServer(t, "v0.2.0", map[string][]byte{
		assetName:       archive,
		"checksums.txt": []byte(checksums),
	})

	u := newTestUpdater(t, rs, "dev", execPath, "linux", "amd64")
	result, err := u.Update(context.Background(), "v0.2.0")
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if !result.Updated {
		t.Fatal("result.Updated = false, want true")
	}
	if got := string(readFileOrFatal(t, execPath)); got != string(newContent) {
		t.Fatalf("executable content = %q, want %q", got, newContent)
	}
}

func TestUpdateExplicitVersionAllowsDowngrade(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "royo-learn")
	writeFileOrFatal(t, execPath, []byte("newer executable"))

	oldContent := []byte("older pinned executable")
	archive := makeTarGz(t, "royo-learn", oldContent)
	assetName := "royo-learn-linux-amd64.tar.gz"
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(archive), assetName)

	rs := newReleaseServer(t, "v0.2.0", map[string][]byte{
		assetName:       archive,
		"checksums.txt": []byte(checksums),
	})

	u := newTestUpdater(t, rs, "0.3.0", execPath, "linux", "amd64")
	result, err := u.Update(context.Background(), "v0.2.0")
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if !result.Updated {
		t.Fatal("result.Updated = false, want true")
	}
	if result.NewVersion != "v0.2.0" {
		t.Fatalf("result.NewVersion = %q, want v0.2.0", result.NewVersion)
	}
	if got := string(readFileOrFatal(t, execPath)); got != string(oldContent) {
		t.Fatalf("executable content = %q, want %q", got, oldContent)
	}
}

// TestUpdateChecksumMismatchLeavesBinaryUntouched is the critical safety
// test: a corrupted download must never brick the installed CLI.
func TestUpdateChecksumMismatchLeavesBinaryUntouched(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "royo-learn")
	original := []byte("original executable — must survive")
	writeFileOrFatal(t, execPath, original)

	archive := makeTarGz(t, "royo-learn", []byte("tampered payload"))
	assetName := "royo-learn-linux-amd64.tar.gz"
	wrongChecksums := fmt.Sprintf("%s  %s\n", sha256Hex([]byte("not the archive")), assetName)

	rs := newReleaseServer(t, "v0.2.0", map[string][]byte{
		assetName:       archive,
		"checksums.txt": []byte(wrongChecksums),
	})

	u := newTestUpdater(t, rs, "0.1.0", execPath, "linux", "amd64")
	_, err := u.Update(context.Background(), "")
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Update error = %v, want ErrChecksumMismatch", err)
	}

	if got := string(readFileOrFatal(t, execPath)); got != string(original) {
		t.Fatalf("executable was modified after checksum mismatch: %q", got)
	}
	if _, statErr := os.Stat(execPath + oldBinarySuffix); !os.IsNotExist(statErr) {
		t.Fatalf("checksum mismatch must not leave a %s file, stat err = %v", oldBinarySuffix, statErr)
	}
}

func TestUpdateCleansLeftoverOldBinary(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "royo-learn")
	writeFileOrFatal(t, execPath, []byte("current executable"))
	writeFileOrFatal(t, execPath+oldBinarySuffix, []byte("leftover from previous update"))

	rs := newReleaseServer(t, "v0.1.8", map[string][]byte{})

	u := newTestUpdater(t, rs, "0.1.8", execPath, "linux", "amd64")
	if _, err := u.Update(context.Background(), ""); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if _, err := os.Stat(execPath + oldBinarySuffix); !os.IsNotExist(err) {
		t.Fatalf("leftover %s file must be removed at command start, stat err = %v", oldBinarySuffix, err)
	}
}

func TestCheckReportsUpdateAvailableWithoutDownloading(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "royo-learn")
	writeFileOrFatal(t, execPath, []byte("current executable"))

	rs := newReleaseServer(t, "v0.2.0", map[string][]byte{
		"royo-learn-linux-amd64.tar.gz": []byte("archive"),
		"checksums.txt":                 []byte("irrelevant"),
	})

	u := newTestUpdater(t, rs, "0.1.0", execPath, "linux", "amd64")
	check, err := u.Check(context.Background())
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if check.CurrentVersion != "0.1.0" {
		t.Fatalf("check.CurrentVersion = %q, want 0.1.0", check.CurrentVersion)
	}
	if check.LatestVersion != "v0.2.0" {
		t.Fatalf("check.LatestVersion = %q, want v0.2.0", check.LatestVersion)
	}
	if !check.UpdateAvailable {
		t.Fatal("check.UpdateAvailable = false, want true")
	}
	if rs.downloadCount.Load() != 0 {
		t.Fatalf("downloads = %d, want 0 for --check", rs.downloadCount.Load())
	}
	if got := string(readFileOrFatal(t, execPath)); got != "current executable" {
		t.Fatalf("executable content changed during check: %q", got)
	}
}

func TestCheckReportsUpToDate(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "royo-learn")
	writeFileOrFatal(t, execPath, []byte("current executable"))

	rs := newReleaseServer(t, "v0.1.8", map[string][]byte{})

	u := newTestUpdater(t, rs, "0.1.8", execPath, "linux", "amd64")
	check, err := u.Check(context.Background())
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if check.UpdateAvailable {
		t.Fatal("check.UpdateAvailable = true, want false")
	}
}

func TestRateLimitErrorIsReadable(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
			})
			server := httptest.NewServer(mux)
			defer server.Close()

			dir := t.TempDir()
			execPath := filepath.Join(dir, "royo-learn")
			writeFileOrFatal(t, execPath, []byte("current executable"))

			u, err := New(Config{
				APIBaseURL:        server.URL,
				CurrentVersion:    "0.1.0",
				ExecutablePath:    execPath,
				GOOS:              "linux",
				GOARCH:            "amd64",
				AllowInsecureHTTP: true,
			})
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}

			_, err = u.Check(context.Background())
			if err == nil {
				t.Fatal("Check expected rate-limit error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "rate limit") {
				t.Fatalf("error %q should mention the rate limit", err)
			}
		})
	}
}

func TestNewRejectsInsecureBaseURLInProductionMode(t *testing.T) {
	_, err := New(Config{
		APIBaseURL:     "http://api.github.com",
		CurrentVersion: "0.1.0",
	})
	if err == nil {
		t.Fatal("New expected error for non-HTTPS base URL without AllowInsecureHTTP")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "https") {
		t.Fatalf("error %q should mention HTTPS", err)
	}
}

func TestUpdateUnsupportedPlatform(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "royo-learn")
	writeFileOrFatal(t, execPath, []byte("current executable"))

	rs := newReleaseServer(t, "v0.2.0", map[string][]byte{})

	u := newTestUpdater(t, rs, "0.1.0", execPath, "plan9", "amd64")
	_, err := u.Update(context.Background(), "")
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Update error = %v, want ErrUnsupportedPlatform", err)
	}
	if got := string(readFileOrFatal(t, execPath)); got != "current executable" {
		t.Fatalf("executable content changed: %q", got)
	}
}
