package project

import (
	"path/filepath"
	"runtime"
	"strings"
)

// Stable error codes for path security.
const (
	ErrPathOutsideRoot = "path_outside_root"
	ErrSymlinkEscape   = "symlink_escape"
	ErrProtectedPath   = "protected_path"
)

// ProtectedPaths lists exact file or directory names that must never be
// accessed by the resolver.
var ProtectedPaths = []string{
	".git",
	".ssh",
	".env",
}

// ProtectedPrefixes lists directory prefixes that signal sensitive content.
var ProtectedPrefixes = []string{
	".git" + string(filepath.Separator),
	".ssh" + string(filepath.Separator),
}

// Canonicalize resolves path to its absolute canonical form.
// It rejects UNC, verbatim, and device paths on all platforms.
// Symlinks are resolved via filepath.EvalSymlinks.
func Canonicalize(path string) (string, error) {
	// Reject UNC, verbatim, and device paths.
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `\\.\`) || strings.HasPrefix(path, `\\?\`) {
		return "", &Error{Code: ErrPathOutsideRoot, Message: "forbidden UNC/device/verbatim path"}
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", &Error{Code: ErrPathOutsideRoot, Message: "cannot resolve absolute path", Err: err}
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// If the path doesn't exist, Abs + Clean is still safe.
		return filepath.Clean(abs), nil
	}

	return resolved, nil
}

// IsInsideRoot reports whether path is within root after canonicalization.
// Both arguments are canonicalized before comparison.
func IsInsideRoot(path, root string) bool {
	canonPath, err := Canonicalize(path)
	if err != nil {
		return false
	}
	canonRoot, err := Canonicalize(root)
	if err != nil {
		return false
	}

	// Normalize for case-insensitive comparison on Windows.
	canonPath = normalizePath(canonPath)
	canonRoot = normalizePath(canonRoot)

	if canonPath == canonRoot {
		return true
	}

	prefix := canonRoot
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(canonPath, prefix)
}

// normalizePath lowercases the path on case-insensitive platforms.
func normalizePath(p string) string {
	if isCaseInsensitiveFS() {
		return strings.ToLower(p)
	}
	return p
}

// isCaseInsensitiveFS reports whether the filesystem is case-insensitive
// (Windows and macOS by default, though macOS can be case-sensitive).
func isCaseInsensitiveFS() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin"
}

// IsProtectedPath reports whether path contains a protected component.
func IsProtectedPath(path string) bool {
	base := filepath.Base(path)
	baseLower := strings.ToLower(base)

	// Exact protected filenames.
	for _, p := range ProtectedPaths {
		if baseLower == p {
			return true
		}
	}

	// Credential-like files.
	credentialNames := []string{
		"credentials", ".credentials",
		".netrc", ".npmrc",
	}
	for _, c := range credentialNames {
		if baseLower == c || strings.HasPrefix(baseLower, c+".") {
			return true
		}
	}

	// Protected prefixes anywhere in the path.
	normalized := strings.ToLower(filepath.Clean(path))
	for _, prefix := range ProtectedPrefixes {
		if strings.Contains(normalized, prefix) {
			return true
		}
	}

	return false
}
