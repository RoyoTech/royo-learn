// Package buildinfo provides stable, build-time and runtime metadata.
package buildinfo

import (
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/debug"
)

const (
	SchemaVersion       = 1
	MigrationLevel      = 1
	RecordFormatVersion = 1
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Metadata is the stable JSON contract returned by the version command.
type Metadata struct {
	Version                string `json:"version"`
	Commit                 string `json:"commit"`
	BuildDate              string `json:"build_date"`
	GoVersion              string `json:"go_version"`
	SchemaVersion          int    `json:"schema_version"`
	MCPSDKVersion          string `json:"mcp_sdk_version"`
	DatabaseMigrationLevel int    `json:"database_migration_level"`
	RecordFormatVersion    int    `json:"record_format_version"`
}

// Current returns metadata without consulting Git, files, or database state.
func Current() Metadata {
	return Metadata{
		Version:                Version,
		Commit:                 Commit,
		BuildDate:              BuildDate,
		GoVersion:              runtime.Version(),
		SchemaVersion:          SchemaVersion,
		MCPSDKVersion:          mcpSDKVersion(),
		DatabaseMigrationLevel: MigrationLevel,
		RecordFormatVersion:    RecordFormatVersion,
	}
}

// HumanString returns a newline-terminated, human-readable version summary for
// interactive CLI use. Machine clients should use VersionJSON instead.
func HumanString() string {
	m := Current()
	return fmt.Sprintf(
		"royo-learn %s\ncommit:  %s\nbuilt:   %s\ngo:      %s\n",
		m.Version, m.Commit, m.BuildDate, m.GoVersion,
	)
}

// VersionJSON returns one newline-terminated JSON document for machine clients.
func VersionJSON() (string, error) {
	document, err := json.Marshal(Current())
	if err != nil {
		return "", err
	}
	return string(document) + "\n", nil
}

func mcpSDKVersion() string {
	build, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dependency := range build.Deps {
		if dependency.Path == "github.com/modelcontextprotocol/go-sdk" {
			return dependency.Version
		}
	}
	return "unknown"
}
