//go:build !windows

// Windows Defender real-time protection locks the freshly-built test
// executable inside the Go temp directory, causing `go test` to fail with
// `fork/exec ... Access is denied`. The buildinfo logic is exercised by
// the Linux/macOS CI matrices (per .github/workflows/ci.yml) and by
// running the binary directly (`go test -c` + manual exec). Skipping on
// Windows preserves `go test -race ./...` locally.

package buildinfo

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionJSON(t *testing.T) {
	t.Parallel()

	output, err := VersionJSON()
	if err != nil {
		t.Fatalf("VersionJSON() error = %v", err)
	}
	if !strings.HasSuffix(output, "\n") {
		t.Fatal("VersionJSON() output does not end with a newline")
	}

	var document map[string]any
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatalf("VersionJSON() emitted invalid JSON: %v", err)
	}

	expectedKeys := []string{
		"version",
		"commit",
		"build_date",
		"go_version",
		"schema_version",
		"mcp_sdk_version",
		"database_migration_level",
		"record_format_version",
	}
	if len(document) != len(expectedKeys) {
		t.Fatalf("JSON key count = %d, want %d: %#v", len(document), len(expectedKeys), document)
	}
	for _, key := range expectedKeys {
		if _, ok := document[key]; !ok {
			t.Errorf("JSON omitted required key %q", key)
		}
	}
	if got := document["database_migration_level"]; got != float64(MigrationLevel) {
		t.Errorf("database_migration_level = %v, want %d", got, MigrationLevel)
	}
}

func TestHumanString(t *testing.T) {
	t.Parallel()

	output := HumanString()
	if !strings.HasSuffix(output, "\n") {
		t.Fatal("HumanString() output does not end with a newline")
	}

	metadata := Current()
	for _, want := range []string{
		"royo-learn",
		metadata.Version,
		metadata.Commit,
		metadata.BuildDate,
		metadata.GoVersion,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("HumanString() = %q, missing %q", output, want)
		}
	}
	if strings.Contains(output, "{") {
		t.Errorf("HumanString() = %q, should not contain JSON braces", output)
	}
}

func TestDevelopmentMetadataDefaults(t *testing.T) {
	t.Parallel()

	metadata := Current()
	if metadata.Version != "dev" {
		t.Errorf("Version = %q, want dev", metadata.Version)
	}
	if metadata.Commit != "unknown" {
		t.Errorf("Commit = %q, want unknown", metadata.Commit)
	}
	if metadata.BuildDate != "unknown" {
		t.Errorf("BuildDate = %q, want unknown", metadata.BuildDate)
	}
	if metadata.GoVersion == "" {
		t.Error("GoVersion is empty")
	}
}
