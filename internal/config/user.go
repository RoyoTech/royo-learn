package config

import (
	"os"
	"path/filepath"
)

// UserConfigPath returns the OS-specific user configuration file path.
// On Unix-like systems it uses os.UserConfigDir() (respecting XDG_CONFIG_HOME).
// On Windows it also uses os.UserConfigDir() (typically %LocalAppData%).
func UserConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "royo-learn", "config.yaml"), nil
}
