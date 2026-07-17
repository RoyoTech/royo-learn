package publish

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var errDirectorySyncUnsupported = errors.New("directory sync is unsupported on this platform")

func directorySyncAvailable() bool { return runtime.GOOS != "windows" }

func fileModeIdentity(mode os.FileMode) uint32 {
	if runtime.GOOS == "windows" {
		if mode.Perm()&0o200 == 0 {
			return 0o444
		}
		return 0o666
	}
	return uint32(mode.Perm())
}

func secureRelativePath(root, relative, label string, createParents bool) (string, error) {
	if root == "" {
		return "", fmt.Errorf("%s root is required", label)
	}
	if unsafePathForm(relative) || filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" {
		return "", fmt.Errorf("%s path %q is not a safe relative path", label, relative)
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == "" || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s path %q escapes its root", label, relative)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("%s root: %w", label, err)
	}
	absRoot = filepath.Clean(absRoot)
	if unsafeRootForm(absRoot) {
		return "", fmt.Errorf("%s root %q uses a forbidden path form", label, root)
	}
	rootHandle, err := openRootNoFollow(absRoot)
	if err != nil {
		return "", fmt.Errorf("%s root: %w", label, err)
	}
	defer rootHandle.Close()
	full := filepath.Join(absRoot, clean)
	rel, err := filepath.Rel(absRoot, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%s path %q escapes root %q", label, relative, absRoot)
	}
	if createParents {
		parent := filepath.Dir(clean)
		if err := rejectRootSymlinks(rootHandle, parent, true); err != nil {
			return "", fmt.Errorf("%s parent: %w", label, err)
		}
		if err := rootHandle.MkdirAll(parent, 0o755); err != nil {
			return "", fmt.Errorf("%s create parent: %w", label, err)
		}
	}
	if err := rejectRootSymlinks(rootHandle, filepath.Dir(clean), !createParents); err != nil {
		return "", fmt.Errorf("%s parent: %w", label, err)
	}
	if info, err := rootHandle.Lstat(clean); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s path is a symlink: %s", label, full)
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("%s lstat: %w", label, err)
	}
	return full, nil
}

func openRootNoFollow(root string) (*os.Root, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("root must be a non-symlink directory: %s", root)
	}
	return os.OpenRoot(root)
}

func cleanRootName(relative, label string) (string, error) {
	if unsafePathForm(relative) || filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" {
		return "", fmt.Errorf("%s path %q is not a safe relative path", label, relative)
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == "" || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s path %q escapes its root", label, relative)
	}
	return clean, nil
}

func rejectRootSymlinks(root *os.Root, relative string, allowMissing bool) error {
	clean := filepath.Clean(relative)
	if clean == "." || clean == "" {
		return nil
	}
	current := ""
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if err != nil {
			if allowMissing && os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink component is not allowed: %s", current)
		}
	}
	return nil
}

func secureAbsoluteWithin(root, path, label string) (string, error) {
	if path == "" || unsafeRootForm(path) {
		return "", fmt.Errorf("%s path is required and must not use a forbidden path form", label)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("%s root: %w", label, err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%s path: %w", label, err)
	}
	rel, err := filepath.Rel(filepath.Clean(absRoot), filepath.Clean(absPath))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%s path %q escapes root %q", label, path, absRoot)
	}
	return secureRelativePath(absRoot, rel, label, false)
}

func unsafePathForm(path string) bool {
	if unsafeRootForm(path) {
		return true
	}
	return len(path) >= 2 && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) && path[1] == ':'
}

func unsafeRootForm(path string) bool {
	if path == "" || strings.IndexByte(path, 0) >= 0 {
		return true
	}
	normalized := strings.ReplaceAll(path, "/", `\`)
	lower := strings.ToLower(normalized)
	return strings.HasPrefix(lower, `\\`) || strings.HasPrefix(lower, `\??\`) || strings.HasPrefix(lower, `\\?\`) || strings.HasPrefix(lower, `\\.\`)
}

func rejectSymlinkComponents(path string, allowMissing bool) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(abs)
	base := volume + string(filepath.Separator)
	if volume == "" {
		base = string(filepath.Separator)
	}
	rel := strings.TrimPrefix(abs, base)
	current := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			if allowMissing && os.IsNotExist(statErr) {
				return nil
			}
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink component is not allowed: %s", current)
		}
	}
	return nil
}

func syncParentDirectory(path string) (bool, error) {
	if !directorySyncAvailable() {
		return false, errDirectorySyncUnsupported
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return true, err
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return true, err
	}
	if err := dir.Close(); err != nil {
		return true, err
	}
	return true, nil
}

func syncParentDirectoryRequired(path string) (bool, error) {
	supported, err := syncParentDirectory(path)
	if errors.Is(err, errDirectorySyncUnsupported) {
		return false, nil
	}
	if err != nil {
		return supported, fmt.Errorf("sync parent directory: %w", err)
	}
	return supported, nil
}
