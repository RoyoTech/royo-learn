package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Database.Path != DefaultDatabaseFilename || cfg.Records.Dir != ".royo-learn/records" ||
		cfg.Evidence.Dir != ".royo-learn/evidence" || cfg.Limits.MaxPayloadBytes != 1024*1024 {
		t.Errorf("unexpected defaults: %+v", *cfg)
	}
}

func TestLoad(t *testing.T) {
	userDir, projectDir, emptyDir := t.TempDir(), t.TempDir(), t.TempDir()
	withUserConfigDir(t, userDir)
	writeFile(t, filepath.Join(userDir, "royo-learn", "config.yaml"), "database:\n  path: user.db\nlimits:\n  max_payload_bytes: 2000000\n")
	writeFile(t, filepath.Join(projectDir, ".royo-learn", "config.yaml"), "database:\n  path: project.db\nlimits:\n  max_payload_bytes: 3000000\n")

	cases := []struct {
		name      string
		root      string
		wantDB    string
		wantLimit int64
	}{
		{"user config when no project config", emptyDir, "user.db", 2000000},
		{"project config overrides user config", projectDir, "project.db", 3000000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load(tc.root)
			if err != nil || cfg.Database.Path != tc.wantDB || cfg.Limits.MaxPayloadBytes != tc.wantLimit {
				t.Fatalf("got err=%v cfg=%+v, want db=%q limit=%d", err, *cfg, tc.wantDB, tc.wantLimit)
			}
		})
	}
}

func TestLoadRejectsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	withUserConfigDir(t, t.TempDir())
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"unknown key", "unknown_key: 1\n", "invalid_config"},
		{"yaml alias", "defaults: &defaults\n  database:\n    path: alias.db\nconfig: *defaults\n", "invalid_config"},
		{"oversized file", strings.Repeat("# padding\n", 200000), "invalid_config"},
		{"crlf accepted", "database:\r\n  path: crlf.db\r\nlimits:\r\n  max_payload_bytes: 1234\r\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeFile(t, filepath.Join(dir, ".royo-learn", "config.yaml"), tc.content)
			cfg, err := Load(dir)
			if tc.want == "" {
				if err != nil || cfg.Database.Path != "crlf.db" || cfg.Limits.MaxPayloadBytes != 1234 {
					t.Fatalf("got err=%v cfg=%+v", err, *cfg)
				}
				return
			}
			assertErrorCode(t, err, tc.want)
		})
	}
}

func TestValidatePaths(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	nested := filepath.Join(root, "nested", "project")
	os.MkdirAll(nested, 0o755)
	cases := []struct {
		name    string
		project string
		shared  string
		want    string
	}{
		{"inside trusted root", nested, "", ""},
		{"outside trusted root", outside, "", "path_outside_root"},
		{"traversal escape", filepath.Join(nested, "..", "..", "..", "outside"), "", "path_outside_root"},
		{"shared root outside", root, outside, "path_outside_root"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.ProjectRoot = tc.project
			cfg.SharedRoot = tc.shared
			err := cfg.Validate([]string{root})
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			assertErrorCode(t, err, tc.want)
		})
	}
}

func TestUserConfigPath(t *testing.T) {
	userDir := t.TempDir()
	withUserConfigDir(t, userDir)
	got, err := UserConfigPath()
	if err != nil || got != filepath.Join(userDir, "royo-learn", "config.yaml") {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func withUserConfigDir(t *testing.T, dir string) {
	t.Helper()
	old := os.Getenv("XDG_CONFIG_HOME")
	if runtime.GOOS == "windows" {
		old = os.Getenv("AppData")
		t.Cleanup(func() { os.Setenv("AppData", old) })
		os.Setenv("AppData", dir)
		return
	}
	t.Cleanup(func() { os.Setenv("XDG_CONFIG_HOME", old) })
	os.Setenv("XDG_CONFIG_HOME", dir)
}

func assertErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	var cErr *Error
	if !errors.As(err, &cErr) || cErr.Code != want {
		t.Fatalf("got %v, want code %q", err, want)
	}
}

func TestExampleConfigParses(t *testing.T) {
	withUserConfigDir(t, t.TempDir())
	dir := t.TempDir()
	example, err := os.ReadFile(filepath.Join("..", "..", "examples", "config.project.yaml"))
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	writeFile(t, filepath.Join(dir, ".royo-learn", "config.yaml"), string(example))

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load example: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("version=%d want 1", cfg.Version)
	}
	if cfg.Project.RecordsRoot != ".royo-learn/records" {
		t.Errorf("project.records_root=%q", cfg.Project.RecordsRoot)
	}
	if cfg.Project.EvidenceRoot != ".royo-learn/evidence" {
		t.Errorf("project.evidence_root=%q", cfg.Project.EvidenceRoot)
	}
	if cfg.Project.BackupRoot != ".royo-learn/backups" {
		t.Errorf("project.backup_root=%q", cfg.Project.BackupRoot)
	}
	if !cfg.Engram.Enabled {
		t.Errorf("engram.enabled=false")
	}
	if cfg.Engram.BaseURL != "http://127.0.0.1:7437" {
		t.Errorf("engram.base_url=%q", cfg.Engram.BaseURL)
	}
	if cfg.Engram.TimeoutSeconds != 2 {
		t.Errorf("engram.timeout_seconds=%d", cfg.Engram.TimeoutSeconds)
	}
	if cfg.Engram.WriteReference {
		t.Errorf("engram.write_reference=true")
	}
	if !cfg.GentleAI.Enabled {
		t.Errorf("gentle_ai.enabled=false")
	}
	if !cfg.GentleAI.RefreshSkillRegistryAfterPublish {
		t.Errorf("gentle_ai.refresh_skill_registry_after_publish=false")
	}
	if !cfg.Publishing.DryRunDefault {
		t.Errorf("publishing.dry_run_default=false")
	}
	if len(cfg.Publishing.AllowedRoots) != 1 || cfg.Publishing.AllowedRoots[0] != "." {
		t.Errorf("publishing.allowed_roots=%v", cfg.Publishing.AllowedRoots)
	}
	if len(cfg.Publishing.CommandAllowlist) != 3 {
		t.Errorf("publishing.command_allowlist len=%d", len(cfg.Publishing.CommandAllowlist))
	}
	if cfg.Limits.RequestBytes != 524288 {
		t.Errorf("limits.request_bytes=%d", cfg.Limits.RequestBytes)
	}
	if cfg.Limits.ResponseBytes != 786432 {
		t.Errorf("limits.response_bytes=%d", cfg.Limits.ResponseBytes)
	}
	if cfg.Limits.EvidenceBytes != 10485760 {
		t.Errorf("limits.evidence_bytes=%d", cfg.Limits.EvidenceBytes)
	}
	if cfg.Limits.CommandOutputBytes != 1048576 {
		t.Errorf("limits.command_output_bytes=%d", cfg.Limits.CommandOutputBytes)
	}
}

func TestValidateChecksAllConfiguredPaths(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	os.MkdirAll(project, 0o755)
	outside := t.TempDir()

	cases := []struct {
		name  string
		setup func(*Config)
		want  string
	}{
		{"defaults inside project root", func(c *Config) { c.ProjectRoot = project }, ""},
		{"project root outside", func(c *Config) { c.ProjectRoot = outside }, ErrPathOutsideRoot},
		{"shared root outside", func(c *Config) { c.ProjectRoot = project; c.SharedRoot = outside }, ErrPathOutsideRoot},
		{"database path outside", func(c *Config) { c.ProjectRoot = project; c.Database.Path = outside }, ErrPathOutsideRoot},
		{"records dir outside", func(c *Config) { c.ProjectRoot = project; c.Records.Dir = outside }, ErrPathOutsideRoot},
		{"evidence dir outside", func(c *Config) { c.ProjectRoot = project; c.Evidence.Dir = outside }, ErrPathOutsideRoot},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.setup(cfg)
			err := cfg.Validate([]string{root})
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			assertErrorCode(t, err, tc.want)
		})
	}
}

func TestValidateChecksProjectRoots(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	os.MkdirAll(project, 0o755)
	outside := t.TempDir()

	cases := []struct {
		name  string
		setup func(*Config)
		want  string
	}{
		{"records_root outside", func(c *Config) { c.ProjectRoot = project; c.Project.RecordsRoot = outside }, ErrPathOutsideRoot},
		{"evidence_root outside", func(c *Config) { c.ProjectRoot = project; c.Project.EvidenceRoot = outside }, ErrPathOutsideRoot},
		{"backup_root outside", func(c *Config) { c.ProjectRoot = project; c.Project.BackupRoot = outside }, ErrPathOutsideRoot},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.setup(cfg)
			err := cfg.Validate([]string{root})
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			assertErrorCode(t, err, tc.want)
		})
	}
}

func TestValidateAcceptsFilesystemRoot(t *testing.T) {
	var root, nested, child string
	if runtime.GOOS == "windows" {
		root = `C:\`
		nested = `C:\tmp`
		child = `C:\tmp\foo`
	} else {
		root = "/"
		nested = "/tmp"
		child = "/tmp/foo"
	}

	cases := []struct {
		name string
		path string
	}{
		{"filesystem root prefix", root + "foo"},
		{"exact filesystem root", root},
		{"nested root prefix", child},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.ProjectRoot = tc.path
			cfg.SharedRoot = ""
			cfg.Database.Path = ""
			cfg.Records.Dir = ""
			cfg.Evidence.Dir = ""
			cfg.Project.RecordsRoot = ""
			cfg.Project.EvidenceRoot = ""
			cfg.Project.BackupRoot = ""
			var trusted []string
			if tc.name == "nested root prefix" {
				trusted = []string{nested}
			} else {
				trusted = []string{root}
			}
			err := cfg.Validate(trusted)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateRejectsEmptyTrustedRoots(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ProjectRoot = t.TempDir()
	err := cfg.Validate(nil)
	assertErrorCode(t, err, ErrInvalidConfig)
}

func TestValidateRejectsMaxPayloadBytesBounds(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name string
		val  int64
		want string
	}{
		{"zero", 0, ErrInvalidConfig},
		{"negative", -1, ErrInvalidConfig},
		{"too large", MaxPayloadBytesUpperBound + 1, ErrInvalidConfig},
		{"at upper bound", MaxPayloadBytesUpperBound, ""},
		{"default", DefaultMaxPayloadBytes, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.ProjectRoot = root
			cfg.Limits.MaxPayloadBytes = tc.val
			err := cfg.Validate([]string{root})
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			assertErrorCode(t, err, tc.want)
		})
	}
}

func TestValidateRejectsWindowsDangerousPaths(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name string
		path string
	}{
		{"UNC", "\\\\server\\share\\project"},
		{"verbatim", "\\\\?\\C:\\project"},
		{"device", "\\\\.\\C:\\project"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.ProjectRoot = tc.path
			err := cfg.Validate([]string{root})
			assertErrorCode(t, err, ErrPathOutsideRoot)
		})
	}
}

func TestValidateResolvesSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("cannot create symlink: ", err)
	}

	cfg := DefaultConfig()
	cfg.ProjectRoot = link
	err := cfg.Validate([]string{root})
	assertErrorCode(t, err, ErrSymlinkEscape)
}

func TestLoadDegradesMalformedUserConfig(t *testing.T) {
	userDir := t.TempDir()
	withUserConfigDir(t, userDir)
	writeFile(t, filepath.Join(userDir, "royo-learn", "config.yaml"), "invalid yaml: [")

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load returned error for malformed user config: %v", err)
	}
	if len(cfg.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(cfg.Warnings))
	}
	if cfg.Warnings[0].Code != ErrInvalidConfig {
		t.Fatalf("warning code=%q want %q", cfg.Warnings[0].Code, ErrInvalidConfig)
	}
	if cfg.Database.Path != DefaultDatabaseFilename {
		t.Fatalf("defaults not used: database.path=%q", cfg.Database.Path)
	}
}

func TestLoadAndValidate(t *testing.T) {
	userDir, projectDir := t.TempDir(), t.TempDir()
	withUserConfigDir(t, userDir)
	writeFile(t, filepath.Join(userDir, "royo-learn", "config.yaml"), "limits:\n  max_payload_bytes: 2000000\n")
	writeFile(t, filepath.Join(projectDir, ".royo-learn", "config.yaml"), "limits:\n  max_payload_bytes: 3000000\n")

	cfg, err := LoadAndValidate(projectDir, []string{projectDir})
	if err != nil {
		t.Fatalf("LoadAndValidate: %v", err)
	}
	if cfg.Limits.MaxPayloadBytes != 3000000 {
		t.Fatalf("limit=%d want 3000000", cfg.Limits.MaxPayloadBytes)
	}

	_, err = LoadAndValidate(projectDir, nil)
	assertErrorCode(t, err, ErrInvalidConfig)
}
