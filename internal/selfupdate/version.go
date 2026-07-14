package selfupdate

import (
	"fmt"
	"strconv"
	"strings"
)

// devVersion is the placeholder version compiled into local builds that
// were not produced by a tagged release.
const devVersion = "dev"

// CompareVersions compares two semantic versions numerically by their
// X.Y.Z components. Leading "v" prefixes are stripped and pre-release or
// build-metadata suffixes ("-rc1", "+meta") are ignored. It returns
// 1 when a > b, -1 when a < b, and 0 when they are equal.
func CompareVersions(a, b string) (int, error) {
	av, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	bv, err := parseVersion(b)
	if err != nil {
		return 0, err
	}
	for i := range av {
		switch {
		case av[i] > bv[i]:
			return 1, nil
		case av[i] < bv[i]:
			return -1, nil
		}
	}
	return 0, nil
}

// parseVersion converts "v1.2.3" (or "1.2", "1.2.3-rc1", ...) into its
// numeric [major, minor, patch] components. Missing components are zero.
func parseVersion(version string) ([3]int, error) {
	var out [3]int

	trimmed := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if idx := strings.IndexAny(trimmed, "-+"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	if trimmed == "" {
		return out, fmt.Errorf("invalid version %q: empty after normalization", version)
	}

	parts := strings.Split(trimmed, ".")
	if len(parts) > 3 {
		return out, fmt.Errorf("invalid version %q: more than three components", version)
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, fmt.Errorf("invalid version %q: component %q is not a non-negative integer", version, part)
		}
		out[i] = n
	}
	return out, nil
}

// isDevVersion reports whether the running binary is a local development
// build without release provenance.
func isDevVersion(version string) bool {
	return version == devVersion
}
