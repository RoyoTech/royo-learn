package selfupdate

import (
	"errors"
	"fmt"
)

// ErrUnsupportedPlatform is returned when no release archive exists for
// the current GOOS/GOARCH combination.
var ErrUnsupportedPlatform = errors.New("unsupported platform")

// projectName mirrors project_name in .goreleaser.yml.
const projectName = "royo-learn"

// AssetName returns the release-archive file name that GoReleaser
// publishes for a platform, following the configured name template
// "{{ .ProjectName }}-{{ .Os }}-{{ .Arch }}" with tar.gz archives and a
// zip override for Windows.
func AssetName(goos, goarch string) (string, error) {
	switch goos {
	case "linux", "darwin", "windows":
	default:
		return "", fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, goos, goarch)
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, goos, goarch)
	}

	extension := "tar.gz"
	if goos == "windows" {
		extension = "zip"
	}
	return fmt.Sprintf("%s-%s-%s.%s", projectName, goos, goarch, extension), nil
}

// BinaryName returns the executable file name found inside the release
// archive for the given GOOS.
func BinaryName(goos string) string {
	if goos == "windows" {
		return projectName + ".exe"
	}
	return projectName
}
