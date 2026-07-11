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
