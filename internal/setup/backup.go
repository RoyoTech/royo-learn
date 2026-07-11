package setup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// BackupConfig creates a timestamped backup of the given config file.
// The backup is placed in the same directory with a .backup-YYYYMMDDHHMMSS
// extension. Returns the path to the backup file.
func BackupConfig(configPath string) (string, error) {
	src, err := os.Open(configPath)
	if err != nil {
		return "", fmt.Errorf("setup: cannot open config for backup: %w", err)
	}
	defer src.Close()

	dir := filepath.Dir(configPath)
	base := filepath.Base(configPath)
	ts := time.Now().UTC().Format("20060102150405")
	backupName := base + ".backup-" + ts
	backupPath := filepath.Join(dir, backupName)

	dst, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("setup: cannot create backup: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(backupPath) // clean up partial copy
		return "", fmt.Errorf("setup: copy to backup: %w", err)
	}

	return backupPath, nil
}
