package evidence

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"agent-royo-learn/internal/project"
)

// ResolvePath canonicalizes candidate and confines it beneath root.
func ResolvePath(root, candidate string) (string, error) {
	if root == "" || candidate == "" {
		return "", fmt.Errorf("evidence path: root and path are required")
	}
	if strings.IndexByte(root, 0) >= 0 || strings.IndexByte(candidate, 0) >= 0 {
		return "", fmt.Errorf("evidence path: NUL byte is forbidden")
	}
	if strings.HasPrefix(candidate, `\\`) {
		return "", fmt.Errorf("evidence path: UNC, device, and verbatim paths are forbidden")
	}
	if hasParentReference(candidate) {
		return "", fmt.Errorf("evidence path: parent traversal is forbidden")
	}
	if hasWindowsVolume(candidate) && (runtime.GOOS != "windows" || !filepath.IsAbs(candidate)) {
		return "", fmt.Errorf("evidence path: unsafe Windows path")
	}
	canonicalRoot, err := project.Canonicalize(root)
	if err != nil {
		return "", fmt.Errorf("evidence path: canonicalize root: %w", err)
	}
	target := candidate
	if !filepath.IsAbs(target) {
		target = filepath.Join(canonicalRoot, target)
	}
	canonicalTarget, err := project.Canonicalize(target)
	if err != nil {
		return "", fmt.Errorf("evidence path: canonicalize target: %w", err)
	}
	if !project.IsInsideRoot(canonicalTarget, canonicalRoot) {
		return "", fmt.Errorf("evidence path: target escapes allowed root")
	}
	if project.IsProtectedPath(canonicalTarget) {
		return "", fmt.Errorf("evidence path: protected target")
	}
	return canonicalTarget, nil
}

func hasParentReference(path string) bool {
	for _, part := range strings.Split(strings.ReplaceAll(path, `\`, "/"), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func hasWindowsVolume(path string) bool {
	return len(path) >= 2 && ((path[0] >= 'A' && path[0] <= 'Z') ||
		(path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':'
}
