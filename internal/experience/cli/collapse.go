package cli

import (
	"os"
	"strconv"
	"strings"
)

// CollapseFlag is set at build time with -X. Empty means enabled so ordinary
// builds expose the unified experience CLI by default.
var CollapseFlag = "true"

// ExperimentalCLICollapse reports whether the unified CLI is authoritative.
// ROYO_LEARN_EXPERIMENTAL_CLI_COLLAPSE overrides the build-time value.
func ExperimentalCLICollapse() bool {
	value := CollapseFlag
	if override := os.Getenv("ROYO_LEARN_EXPERIMENTAL_CLI_COLLAPSE"); override != "" {
		value = override
	}
	enabled, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return true
	}
	return enabled
}
