package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseWorkflowRequiresSuccessfulCIForTaggedSHA(t *testing.T) {
	root := repositoryRoot(t)
	ci := readRepositoryFile(t, root, ".github", "workflows", "ci.yml")
	release := readRepositoryFile(t, root, ".github", "workflows", "release.yml")
	for _, required := range []string{"tags:", "'v*'", "Coverage gates", "./internal/domain", "./internal/storage", "./internal/publish", "80", "90"} {
		if !strings.Contains(ci, required) {
			t.Errorf("CI workflow missing %q", required)
		}
	}
	for _, required := range []string{"workflow_run:", "workflows: [CI]", "github.event.workflow_run.head_sha", "github.event.workflow_run.conclusion == 'success'"} {
		if !strings.Contains(release, required) {
			t.Errorf("release workflow missing %q", required)
		}
	}
	if strings.Contains(release, "goreleaser/goreleaser-action@v") || strings.Contains(release, "version: latest") || strings.Contains(release, "curl -sSfL") {
		t.Fatal("release workflow executes an unpinned downloaded tool")
	}
}

func TestInstallersRequireChecksumVersionAndRollback(t *testing.T) {
	root := repositoryRoot(t)
	unix := readRepositoryFile(t, root, "install.sh")
	powershell := readRepositoryFile(t, root, "install.ps1")
	for _, required := range []string{"checksum entry not found", "checksum mismatch", "version mismatch", "ROYO_LEARN_INSTALL_DIR", "rollback"} {
		if !strings.Contains(unix, required) {
			t.Errorf("install.sh missing fail-closed contract %q", required)
		}
	}
	for _, required := range []string{"checksum entry not found", "checksum mismatch", "version mismatch", "ROYO_LEARN_INSTALL_ROOT", "Restore-PreviousBinary"} {
		if !strings.Contains(powershell, required) {
			t.Errorf("install.ps1 missing fail-closed contract %q", required)
		}
	}
	for _, forbidden := range []string{"skipping verification", "version check skipped"} {
		if strings.Contains(unix, forbidden) || strings.Contains(powershell, forbidden) {
			t.Errorf("installer still contains soft-pass text %q", forbidden)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readRepositoryFile(t *testing.T, root string, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
