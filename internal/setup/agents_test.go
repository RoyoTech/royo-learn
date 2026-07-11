package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAgent_AllSupportedKinds(t *testing.T) {
	for _, kind := range AllAgents {
		a, err := ResolveAgent(kind)
		if err != nil {
			t.Fatalf("ResolveAgent(%q): %v", kind, err)
		}
		if a.Kind() != kind {
			t.Errorf("Kind() = %q, want %q", a.Kind(), kind)
		}
		if a.DisplayName() == "" {
			t.Errorf("DisplayName() empty for %q", kind)
		}
	}
}

func TestResolveAgent_UnknownKindFails(t *testing.T) {
	_, err := ResolveAgent(AgentKind("bogus-agent"))
	if err == nil {
		t.Fatal("expected error for unknown agent kind")
	}
}

func TestHomeDir_ResolvesFromEnv(t *testing.T) {
	t.Setenv("HOME", "/tmp/fake-home")
	t.Setenv("USERPROFILE", "")
	if got := HomeDir(); got != "/tmp/fake-home" {
		t.Errorf("HomeDir() = %q, want /tmp/fake-home", got)
	}
}

func TestWriteFileAtomic_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := writeFileAtomic(path, []byte(`{"k":"v"}`), 0o644); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != `{"k":"v"}` {
		t.Errorf("content = %q", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("tmp file should be cleaned up; stat err = %v", err)
	}
}

func TestBinaryOnPath_Lookups(t *testing.T) {
	if !binaryOnPath("go") {
		t.Skip("go not on PATH in this environment")
	}
	if binaryOnPath("definitely-not-a-binary-name-12345") {
		t.Error("binaryOnPath returned true for non-existent binary")
	}
}
