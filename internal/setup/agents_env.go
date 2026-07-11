package setup

import "os"

// getenv is the os.LookupEnv equivalent, isolated here so other files
// in this package do not need to import os directly for env reads.
func getenv(key string) (string, bool) {
	return os.LookupEnv(key)
}

// writeFile is os.WriteFile exposed at package scope so writeFileAtomic
// (and agent implementations) can use it without re-importing os.
func writeFile(path string, data []byte, perm uint32) error {
	return os.WriteFile(path, data, os.FileMode(perm))
}

// renameFile is os.Rename exposed at package scope for atomic writes.
func renameFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// removeFile is os.Remove exposed at package scope.
func removeFile(path string) error {
	return os.Remove(path)
}
