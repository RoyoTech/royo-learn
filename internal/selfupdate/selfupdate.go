package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrDevBuild is returned by Update when the running binary was built
// locally (buildinfo.Version == "dev") and no explicit --version was
// passed.
var ErrDevBuild = errors.New("development build; use the installer or pass --version")

// ---------------------------------------------------------------------------
// public types
// ---------------------------------------------------------------------------

// Config carries every external dependency so that Updater can be tested
// entirely with httptest servers.
type Config struct {
	// APIBaseURL is the root of the GitHub API (https://api.github.com).
	APIBaseURL string

	// CurrentVersion is the buildinfo.Version of the running binary.
	CurrentVersion string

	// ExecutablePath is the absolute path to the running binary.
	ExecutablePath string

	// GOOS / GOARCH identify the platform for asset resolution.
	GOOS   string
	GOARCH string

	// AllowInsecureHTTP permits http:// base URLs in tests.
	AllowInsecureHTTP bool
}

// Result is returned by Update after a successful binary replacement.
type Result struct {
	Updated        bool   `json:"updated"`
	CurrentVersion string `json:"current_version"`
	NewVersion     string `json:"new_version"`
	Path           string `json:"path"`
}

// CheckResult is returned by Check without touching the filesystem.
type CheckResult struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
}

// maxArtifactBytes caps both the downloaded release archive and the
// binary extracted from it so a malicious or corrupted stream cannot
// fill the disk. Real release assets are a few megabytes.
const maxArtifactBytes = 256 << 20

// Updater controls a single self-update operation.
type Updater struct {
	apiBase       string
	current       string
	execPath      string
	goos          string
	goarch        string
	allowInsecure bool
	httpClient    *http.Client
}

// New validates config and returns a ready-to-use Updater.
func New(cfg Config) (*Updater, error) {
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://api.github.com"
	}
	if !cfg.AllowInsecureHTTP && !strings.HasPrefix(cfg.APIBaseURL, "https://") {
		return nil, fmt.Errorf("API base URL must use HTTPS: %s", cfg.APIBaseURL)
	}
	if cfg.CurrentVersion == "" {
		return nil, errors.New("current version is required")
	}
	if cfg.ExecutablePath == "" {
		return nil, errors.New("executable path is required")
	}
	if cfg.GOOS == "" || cfg.GOARCH == "" {
		return nil, errors.New("GOOS and GOARCH are required")
	}

	allowInsecure := cfg.AllowInsecureHTTP
	return &Updater{
		apiBase:       strings.TrimRight(cfg.APIBaseURL, "/"),
		current:       cfg.CurrentVersion,
		execPath:      cfg.ExecutablePath,
		goos:          cfg.GOOS,
		goarch:        cfg.GOARCH,
		allowInsecure: allowInsecure,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errors.New("stopped after 10 redirects")
				}
				if !allowInsecure && req.URL.Scheme != "https" {
					return fmt.Errorf("redirect to non-HTTPS URL refused: %s", req.URL)
				}
				return nil
			},
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Check — read-only version comparison
// ---------------------------------------------------------------------------

// Check queries the latest GitHub release and compares it to the running
// version without downloading or modifying anything.
func (u *Updater) Check(ctx context.Context) (*CheckResult, error) {
	latestTag, err := u.fetchLatestTag(ctx)
	if err != nil {
		return nil, err
	}

	cmp, err := CompareVersions(latestTag, u.current)
	if err != nil {
		// If the running version is "dev" we still want to report that
		// an update is available. Treat "dev" as older than any semver.
		if isDevVersion(u.current) {
			return &CheckResult{
				CurrentVersion:  u.current,
				LatestVersion:   latestTag,
				UpdateAvailable: true,
			}, nil
		}
		return nil, fmt.Errorf("compare versions: %w", err)
	}

	return &CheckResult{
		CurrentVersion:  u.current,
		LatestVersion:   latestTag,
		UpdateAvailable: cmp > 0, // latest > current
	}, nil
}

// ---------------------------------------------------------------------------
// Update — full download + verify + replace
// ---------------------------------------------------------------------------

// Update fetches a release, downloads the platform asset, verifies its
// SHA-256 checksum, extracts the binary, and replaces the current
// executable.
//
// When explicitVersion is empty and the running version is already the
// latest, Update returns Result{Updated: false} without error.
//
// When explicitVersion is set it bypasses the dev-build guard and the
// implicit-downgrade refusal.
func (u *Updater) Update(ctx context.Context, explicitVersion string) (*Result, error) {
	// --- dev-build guard ---
	if isDevVersion(u.current) && explicitVersion == "" {
		// Do not even call the API for dev builds.
		return nil, ErrDevBuild
	}

	// --- resolve target version ---
	var (
		tag         string
		tagExplicit bool
	)
	if explicitVersion != "" {
		if !strings.HasPrefix(explicitVersion, "v") {
			explicitVersion = "v" + explicitVersion
		}
		tag = explicitVersion
		tagExplicit = true
	} else {
		latest, err := u.fetchLatestTag(ctx)
		if err != nil {
			return nil, err
		}
		tag = latest
	}

	// --- version comparison (implicit update only) ---
	if !tagExplicit && !isDevVersion(u.current) {
		cmp, err := CompareVersions(tag, u.current)
		if err != nil {
			// Fail closed: never install a tag we cannot compare
			// against the running version.
			return nil, fmt.Errorf("compare versions: %w", err)
		}
		if cmp == 0 {
			// Already up to date. Clean up a leftover .old
			// from a previous Windows update before returning.
			CleanupOldBinary(u.execPath)
			return &Result{
				Updated:        false,
				CurrentVersion: u.current,
				NewVersion:     tag,
				Path:           u.execPath,
			}, nil
		}
		if cmp < 0 {
			// latest < current — implicit downgrade refused.
			return nil, fmt.Errorf("release %s is older than the running version %s — use --version to install it explicitly", tag, u.current)
		}
	}

	// --- fetch release metadata (need asset URLs) ---
	release, err := u.fetchRelease(ctx, tag)
	if err != nil {
		return nil, err
	}

	assetName, err := AssetName(u.goos, u.goarch)
	if err != nil {
		return nil, err
	}

	assetURL, ok := release.Assets[assetName]
	if !ok {
		return nil, fmt.Errorf("release %s does not contain asset %q for %s/%s", tag, assetName, u.goos, u.goarch)
	}
	checksumURL, ok := release.Assets["checksums.txt"]
	if !ok {
		return nil, fmt.Errorf("release %s does not contain checksums.txt", tag)
	}

	// --- download assets to temp dir ---
	tmpDir, err := os.MkdirTemp("", "royo-learn-update-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, assetName)
	if err := u.download(ctx, assetURL, archivePath); err != nil {
		return nil, fmt.Errorf("download %s: %w", assetName, err)
	}

	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	if err := u.download(ctx, checksumURL, checksumPath); err != nil {
		return nil, fmt.Errorf("download checksums.txt: %w", err)
	}

	// --- verify checksum BEFORE touching the binary ---
	checksumData, err := os.ReadFile(checksumPath)
	if err != nil {
		return nil, fmt.Errorf("read checksums.txt: %w", err)
	}
	sums, err := ParseChecksums(checksumData)
	if err != nil {
		return nil, fmt.Errorf("parse checksums.txt: %w", err)
	}
	wantDigest, ok := sums[assetName]
	if !ok {
		return nil, fmt.Errorf("checksums.txt does not contain an entry for %s", assetName)
	}
	if err := VerifyFileChecksum(archivePath, wantDigest); err != nil {
		return nil, err
	}

	// --- extract binary from archive ---
	binaryName := BinaryName(u.goos)
	binaryPath := filepath.Join(tmpDir, binaryName)
	if err := extractBinary(archivePath, binaryName, binaryPath); err != nil {
		return nil, fmt.Errorf("extract %s from %s: %w", binaryName, assetName, err)
	}

	// --- cleanup stale old binary (Windows) ---
	CleanupOldBinary(u.execPath)

	// --- replace the current executable ---
	isWin := u.goos == "windows"
	if err := Replace(u.execPath, binaryPath, isWin); err != nil {
		return nil, fmt.Errorf("replace executable: %w", err)
	}

	return &Result{
		Updated:        true,
		CurrentVersion: u.current,
		NewVersion:     tag,
		Path:           u.execPath,
	}, nil
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// releaseData is the minimal subset of the GitHub release API used by the
// updater. The Assets map is keyed by asset name.
type releaseData struct {
	Assets map[string]string // name → browser_download_url
}

// fetchLatestTag returns the tag_name of the latest GitHub release.
func (u *Updater) fetchLatestTag(ctx context.Context) (string, error) {
	return u.fetchLatestTagRaw(ctx)
}

// addAuthHeader attaches GITHUB_TOKEN, when set, as a Bearer token so
// authenticated requests get the higher GitHub API rate limits the
// rate-limit error message advertises.
func addAuthHeader(req *http.Request) {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// copyLimited copies src into dst, failing when the stream exceeds
// maxArtifactBytes.
func copyLimited(dst io.Writer, src io.Reader) error {
	n, err := io.Copy(dst, io.LimitReader(src, maxArtifactBytes+1))
	if err != nil {
		return err
	}
	if n > maxArtifactBytes {
		return fmt.Errorf("stream exceeds the %d-byte artifact limit", int64(maxArtifactBytes))
	}
	return nil
}

// fetchLatestTagRaw hits the /releases/latest endpoint and returns tag_name.
func (u *Updater) fetchLatestTagRaw(ctx context.Context) (string, error) {
	url := u.apiBase + "/repos/RoyoTech/royo-learn/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "royo-learn (self-update)")
	addAuthHeader(req)

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return "", fmt.Errorf("GitHub API rate limit reached — try again later or set GITHUB_TOKEN")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode release: %w", err)
	}
	return payload.TagName, nil
}

// fetchRelease fetches a specific release by tag name (or "latest").
func (u *Updater) fetchRelease(ctx context.Context, tag string) (*releaseData, error) {
	url := u.apiBase + "/repos/RoyoTech/royo-learn/releases/tags/" + tag
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "royo-learn (self-update)")
	addAuthHeader(req)

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release %s: %w", tag, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("GitHub API rate limit reached — try again later or set GITHUB_TOKEN")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}

	assets := make(map[string]string, len(payload.Assets))
	for _, a := range payload.Assets {
		assets[a.Name] = a.BrowserDownloadURL
	}
	return &releaseData{Assets: assets}, nil
}

// download fetches url and writes the response body to destPath. In
// production mode (the same knob as the HTTPS check in New) the release
// JSON may only point at HTTPS download URLs.
func (u *Updater) download(ctx context.Context, url, destPath string) error {
	if !u.allowInsecure && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("download URL must use HTTPS: %s", url)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "royo-learn (self-update)")
	addAuthHeader(req)

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("download returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := copyLimited(f, resp.Body); err != nil {
		return err
	}
	return f.Close()
}

// extractBinary opens archive at srcPath and extracts the entry named
// binaryName to destPath with mode 0755.
func extractBinary(srcPath, binaryName, destPath string) error {
	switch {
	case strings.HasSuffix(srcPath, ".tar.gz"):
		return extractTarGz(srcPath, binaryName, destPath)
	case strings.HasSuffix(srcPath, ".zip"):
		return extractZip(srcPath, binaryName, destPath)
	default:
		return fmt.Errorf("unsupported archive format: %s", srcPath)
	}
}

func extractTarGz(srcPath, binaryName, destPath string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Name == binaryName {
			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				return err
			}
			defer out.Close()
			if err := copyLimited(out, tr); err != nil {
				return err
			}
			return out.Close()
		}
	}
	return fmt.Errorf("entry %q not found in %s", binaryName, srcPath)
}

func extractZip(srcPath, binaryName, destPath string) error {
	r, err := zip.OpenReader(srcPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name == binaryName {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				return err
			}
			defer out.Close()
			if err := copyLimited(out, rc); err != nil {
				return err
			}
			return out.Close()
		}
	}
	return fmt.Errorf("entry %q not found in %s", binaryName, srcPath)
}
