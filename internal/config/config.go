// Package config loads and validates compiled, user, and project configuration.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Stable error codes used by this package. They must stay in sync with
// docs/17-ERROR-CODES.md.
const (
	ErrInvalidConfig   = "invalid_config"
	ErrPathOutsideRoot = "path_outside_root"
	ErrSymlinkEscape   = "symlink_escape"
	ErrProtectedPath   = "protected_path"
)

// Compiled default values.
const (
	DefaultSchemaVersion       = 1
	DefaultRecordFormatVersion = 1
	DefaultMigrationLevel      = 1

	DefaultProjectConfigFilename = ".royo-learn/config.yaml"
	DefaultDatabaseFilename      = "royo-learn.db"
	DefaultRecordsDir            = ".royo-learn/records"
	DefaultEvidenceDir           = ".royo-learn/evidence"
	DefaultMaxPayloadBytes       = 1024 * 1024

	MaxConfigFileBytes = 1 << 20 // 1 MiB

	// MaxPayloadBytesUpperBound is the largest byte limit the loader will
	// accept for any user-configurable size guardrail. It keeps accidental
	// values like terabytes from exhausting process memory.
	MaxPayloadBytesUpperBound = 1 << 30 // 1 GiB
)

// Warning records a non-fatal degradation that callers can surface through
// logs or reports. Warnings are never serialized to YAML.
type Warning struct {
	Code    string
	Message string
}

// Config is the merged runtime configuration.
type Config struct {
	Version int `yaml:"version,omitempty"`

	Project    Project    `yaml:"project,omitempty"`
	Engram     Engram     `yaml:"engram,omitempty"`
	GentleAI   GentleAI   `yaml:"gentle_ai,omitempty"`
	Publishing Publishing `yaml:"publishing,omitempty"`

	// Legacy/direct paths. These are kept alongside the structured project
	// block so callers can choose the layout that fits their use case.
	ProjectRoot string `yaml:"project_root,omitempty"`
	SharedRoot  string `yaml:"shared_root,omitempty"`

	Database Database `yaml:"database,omitempty"`
	Records  Records  `yaml:"records,omitempty"`
	Evidence Evidence `yaml:"evidence,omitempty"`
	Limits   Limits   `yaml:"limits,omitempty"`

	// Warnings collects non-fatal problems encountered while loading.
	Warnings []Warning `yaml:"-"`
}

// Project groups project-level directory and naming settings.
type Project struct {
	Name         string `yaml:"name,omitempty"`
	RecordsRoot  string `yaml:"records_root,omitempty"`
	EvidenceRoot string `yaml:"evidence_root,omitempty"`
	BackupRoot   string `yaml:"backup_root,omitempty"`
}

// Engram holds the optional local Engram memory integration settings.
type Engram struct {
	Enabled        bool   `yaml:"enabled,omitempty"`
	BaseURL        string `yaml:"base_url,omitempty"`
	TimeoutSeconds int    `yaml:"timeout_seconds,omitempty"`
	WriteReference bool   `yaml:"write_reference,omitempty"`
}

// GentleAI holds the optional Gentle-AI skill registry integration settings.
type GentleAI struct {
	Enabled                          bool `yaml:"enabled,omitempty"`
	RefreshSkillRegistryAfterPublish bool `yaml:"refresh_skill_registry_after_publish,omitempty"`
}

// Publishing controls publication safety defaults.
type Publishing struct {
	DryRunDefault                bool       `yaml:"dry_run_default,omitempty"`
	BlockDirtyTargets            bool       `yaml:"block_dirty_targets,omitempty"`
	RequireHumanForAgentsMD      bool       `yaml:"require_human_for_agents_md,omitempty"`
	RequireHumanForShared        bool       `yaml:"require_human_for_shared,omitempty"`
	RequireHumanForExistingSkill bool       `yaml:"require_human_for_existing_skill,omitempty"`
	AllowedRoots                 []string   `yaml:"allowed_roots,omitempty"`
	CommandAllowlist             [][]string `yaml:"command_allowlist,omitempty"`
}

// Database holds SQLite storage settings.
type Database struct {
	Path string `yaml:"path,omitempty"`
}

// Records holds the learned-record materialization directory.
type Records struct {
	Dir string `yaml:"dir,omitempty"`
}

// Evidence holds the evidence-blob storage directory.
type Evidence struct {
	Dir string `yaml:"dir,omitempty"`
}

// Limits holds runtime size and rate guardrails.
type Limits struct {
	MaxPayloadBytes    int64 `yaml:"max_payload_bytes,omitempty"`
	RequestBytes       int64 `yaml:"request_bytes,omitempty"`
	ResponseBytes      int64 `yaml:"response_bytes,omitempty"`
	EvidenceBytes      int64 `yaml:"evidence_bytes,omitempty"`
	CommandOutputBytes int64 `yaml:"command_output_bytes,omitempty"`
}

// Error is a typed configuration error carrying a stable error code.
type Error struct {
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error for errors.Is/errors.As chains.
func (e *Error) Unwrap() error { return e.Err }

// DefaultConfig returns the compiled defaults. It never reads files or env.
func DefaultConfig() *Config {
	return &Config{
		Version: DefaultSchemaVersion,
		Project: Project{
			RecordsRoot:  DefaultRecordsDir,
			EvidenceRoot: DefaultEvidenceDir,
			BackupRoot:   ".royo-learn/backups",
		},
		Publishing: Publishing{DryRunDefault: true},
		Database:   Database{Path: DefaultDatabaseFilename},
		Records:    Records{Dir: DefaultRecordsDir},
		Evidence:   Evidence{Dir: DefaultEvidenceDir},
		Limits:     Limits{MaxPayloadBytes: DefaultMaxPayloadBytes},
	}
}

// Load merges compiled defaults with user config and project config.
// Precedence: compiled defaults < user config < project config.
// Explicit flags and environment variables are intentionally outside this
// function so callers can override the returned config before use.
//
// A malformed user config is treated as missing and surfaced through
// cfg.Warnings so the caller can decide how to report the degradation.
// A malformed project config still returns an error because the project
// explicitly opted into that configuration.
func Load(projectRoot string) (*Config, error) {
	cfg := DefaultConfig()

	userPath, err := UserConfigPath()
	if err != nil {
		return nil, &Error{Code: ErrInvalidConfig, Message: "cannot resolve user config dir", Err: err}
	}
	if userCfg, err := loadFile(userPath); err != nil {
		cfg.Warnings = append(cfg.Warnings, Warning{
			Code:    ErrInvalidConfig,
			Message: fmt.Sprintf("user config is malformed; using compiled defaults: %v", err),
		})
	} else if userCfg != nil {
		cfg.Merge(userCfg)
	}

	if projectRoot != "" {
		projectPath := filepath.Join(projectRoot, DefaultProjectConfigFilename)
		if projectCfg, err := loadFile(projectPath); err != nil {
			return nil, err
		} else if projectCfg != nil {
			cfg.Merge(projectCfg)
		}
	}

	return cfg, nil
}

// LoadAndValidate loads the configuration for projectRoot and validates that
// all configured paths remain inside trustedRoots. It is the recommended entry
// point for callers that need a guaranteed-safe configuration.
func LoadAndValidate(projectRoot string, trustedRoots []string) (*Config, error) {
	cfg, err := Load(projectRoot)
	if err != nil {
		return cfg, err
	}
	if cfg.ProjectRoot == "" {
		cfg.ProjectRoot = projectRoot
	}
	if err := cfg.Validate(trustedRoots); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// loadFile reads and parses one YAML config file. A missing file is not an
// error and returns nil. Malformed, oversized, aliased, or unknown-key files
// return typed *Error values.
func loadFile(path string) (*Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, &Error{Code: ErrInvalidConfig, Message: fmt.Sprintf("cannot stat %s", path), Err: err}
	}
	if info.IsDir() {
		return nil, &Error{Code: ErrInvalidConfig, Message: fmt.Sprintf("config path is a directory: %s", path)}
	}
	if info.Size() > MaxConfigFileBytes {
		return nil, &Error{Code: ErrInvalidConfig, Message: fmt.Sprintf("config file exceeds %d bytes: %s", MaxConfigFileBytes, path)}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &Error{Code: ErrInvalidConfig, Message: fmt.Sprintf("cannot read %s", path), Err: err}
	}

	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, &Error{Code: ErrInvalidConfig, Message: fmt.Sprintf("cannot parse %s", path), Err: err}
	}
	if hasAlias(&node) {
		return nil, &Error{Code: ErrInvalidConfig, Message: fmt.Sprintf("YAML aliases are not allowed: %s", path)}
	}

	var cfg Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return &cfg, nil
		}
		return nil, &Error{Code: ErrInvalidConfig, Message: fmt.Sprintf("cannot decode %s", path), Err: err}
	}
	return &cfg, nil
}

// hasAlias reports whether the YAML document contains any alias node.
func hasAlias(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	if n.Kind == yaml.AliasNode {
		return true
	}
	for _, child := range n.Content {
		if hasAlias(child) {
			return true
		}
	}
	return false
}

// Merge overlays other onto cfg. Non-zero scalar values and non-empty strings
// from other replace cfg values. Nested structs are merged field-by-field.
func (c *Config) Merge(other *Config) {
	if other == nil {
		return
	}
	if other.Version != 0 {
		c.Version = other.Version
	}
	mergeProject(&c.Project, other.Project)
	mergeEngram(&c.Engram, other.Engram)
	mergeGentleAI(&c.GentleAI, other.GentleAI)
	mergePublishing(&c.Publishing, other.Publishing)
	if other.ProjectRoot != "" {
		c.ProjectRoot = other.ProjectRoot
	}
	if other.SharedRoot != "" {
		c.SharedRoot = other.SharedRoot
	}
	if other.Database.Path != "" {
		c.Database.Path = other.Database.Path
	}
	if other.Records.Dir != "" {
		c.Records.Dir = other.Records.Dir
	}
	if other.Evidence.Dir != "" {
		c.Evidence.Dir = other.Evidence.Dir
	}
	if other.Limits.MaxPayloadBytes != 0 {
		c.Limits.MaxPayloadBytes = other.Limits.MaxPayloadBytes
	}
	if other.Limits.RequestBytes != 0 {
		c.Limits.RequestBytes = other.Limits.RequestBytes
	}
	if other.Limits.ResponseBytes != 0 {
		c.Limits.ResponseBytes = other.Limits.ResponseBytes
	}
	if other.Limits.EvidenceBytes != 0 {
		c.Limits.EvidenceBytes = other.Limits.EvidenceBytes
	}
	if other.Limits.CommandOutputBytes != 0 {
		c.Limits.CommandOutputBytes = other.Limits.CommandOutputBytes
	}
}

func mergeProject(dst *Project, src Project) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.RecordsRoot != "" {
		dst.RecordsRoot = src.RecordsRoot
	}
	if src.EvidenceRoot != "" {
		dst.EvidenceRoot = src.EvidenceRoot
	}
	if src.BackupRoot != "" {
		dst.BackupRoot = src.BackupRoot
	}
}

func mergeEngram(dst *Engram, src Engram) {
	if src.Enabled {
		dst.Enabled = true
	}
	if src.BaseURL != "" {
		dst.BaseURL = src.BaseURL
	}
	if src.TimeoutSeconds != 0 {
		dst.TimeoutSeconds = src.TimeoutSeconds
	}
	if src.WriteReference {
		dst.WriteReference = true
	}
}

func mergeGentleAI(dst *GentleAI, src GentleAI) {
	if src.Enabled {
		dst.Enabled = true
	}
	if src.RefreshSkillRegistryAfterPublish {
		dst.RefreshSkillRegistryAfterPublish = true
	}
}

func mergePublishing(dst *Publishing, src Publishing) {
	if src.DryRunDefault {
		dst.DryRunDefault = true
	}
	if src.BlockDirtyTargets {
		dst.BlockDirtyTargets = true
	}
	if src.RequireHumanForAgentsMD {
		dst.RequireHumanForAgentsMD = true
	}
	if src.RequireHumanForShared {
		dst.RequireHumanForShared = true
	}
	if src.RequireHumanForExistingSkill {
		dst.RequireHumanForExistingSkill = true
	}
	if len(src.AllowedRoots) > 0 {
		dst.AllowedRoots = src.AllowedRoots
	}
	if len(src.CommandAllowlist) > 0 {
		dst.CommandAllowlist = src.CommandAllowlist
	}
}

// Validate checks that configured paths stay within trustedRoots and that
// size guardrails are sane. Each root must be an absolute, clean path.
// Relative project roots and storage paths are resolved against the current
// working directory or c.ProjectRoot when it is set.
func (c *Config) Validate(trustedRoots []string) error {
	if len(trustedRoots) == 0 {
		return &Error{Code: ErrInvalidConfig, Message: "trustedRoots is empty"}
	}

	for i, root := range trustedRoots {
		if root == "" {
			return &Error{Code: ErrInvalidConfig, Message: fmt.Sprintf("trusted root at index %d is empty", i)}
		}
		if !filepath.IsAbs(root) {
			return &Error{Code: ErrInvalidConfig, Message: fmt.Sprintf("trusted root is not absolute: %s", root)}
		}
	}

	if err := c.validateLimits(); err != nil {
		return err
	}

	for _, check := range []struct {
		name string
		path string
	}{
		{"project_root", c.ProjectRoot},
		{"shared_root", c.SharedRoot},
		{"database.path", c.Database.Path},
		{"records.dir", c.Records.Dir},
		{"evidence.dir", c.Evidence.Dir},
		{"project.records_root", c.Project.RecordsRoot},
		{"project.evidence_root", c.Project.EvidenceRoot},
		{"project.backup_root", c.Project.BackupRoot},
	} {
		if check.path == "" {
			continue
		}
		if err := validatePathInsideRoots(check.name, check.path, c.ProjectRoot, trustedRoots); err != nil {
			return err
		}
	}

	return nil
}

func (c *Config) validateLimits() error {
	check := func(name string, val int64) error {
		if val <= 0 {
			return &Error{Code: ErrInvalidConfig, Message: fmt.Sprintf("%s must be positive", name)}
		}
		if val > MaxPayloadBytesUpperBound {
			return &Error{Code: ErrInvalidConfig, Message: fmt.Sprintf("%s exceeds %d bytes", name, MaxPayloadBytesUpperBound)}
		}
		return nil
	}
	if err := check("limits.max_payload_bytes", c.Limits.MaxPayloadBytes); err != nil {
		return err
	}
	if c.Limits.RequestBytes != 0 {
		if err := check("limits.request_bytes", c.Limits.RequestBytes); err != nil {
			return err
		}
	}
	if c.Limits.ResponseBytes != 0 {
		if err := check("limits.response_bytes", c.Limits.ResponseBytes); err != nil {
			return err
		}
	}
	if c.Limits.EvidenceBytes != 0 {
		if err := check("limits.evidence_bytes", c.Limits.EvidenceBytes); err != nil {
			return err
		}
	}
	if c.Limits.CommandOutputBytes != 0 {
		if err := check("limits.command_output_bytes", c.Limits.CommandOutputBytes); err != nil {
			return err
		}
	}
	return nil
}

func validatePathInsideRoots(field, value, projectRoot string, trustedRoots []string) error {
	if value == "" {
		return nil
	}

	// Reject UNC, verbatim (\\?\) and device (\\.\) paths before any stat
	// call that could trigger network or device I/O.
	if strings.HasPrefix(value, `\\`) {
		return &Error{Code: ErrPathOutsideRoot, Message: fmt.Sprintf("%s uses a forbidden UNC/device/verbatim path: %s", field, value)}
	}

	isSymlink := false
	if info, err := os.Lstat(value); err == nil && info.Mode()&os.ModeSymlink != 0 {
		isSymlink = true
	}

	resolved, err := resolveConfigPath(field, value, projectRoot)
	if err != nil {
		return err
	}

	for _, root := range trustedRoots {
		rootClean := filepath.Clean(root)
		if resolved == rootClean {
			return nil
		}
		prefix := rootClean
		if !strings.HasSuffix(prefix, string(filepath.Separator)) {
			prefix += string(filepath.Separator)
		}
		if strings.HasPrefix(resolved, prefix) {
			return nil
		}
	}
	if isSymlink {
		return &Error{Code: ErrSymlinkEscape, Message: fmt.Sprintf("%s escapes trusted roots via symlink: %s", field, value)}
	}
	return &Error{Code: ErrPathOutsideRoot, Message: fmt.Sprintf("%s is outside trusted roots: %s", field, value)}
}

func resolveConfigPath(field, value, projectRoot string) (string, error) {
	// Reject UNC, verbatim (\\?\) and device (\\.\) paths on any OS.
	if strings.HasPrefix(value, `\\`) {
		return "", &Error{Code: ErrPathOutsideRoot, Message: fmt.Sprintf("%s uses a forbidden UNC/device/verbatim path: %s", field, value)}
	}

	var abs string
	switch {
	case filepath.IsAbs(value):
		abs = value
	case projectRoot != "":
		abs = filepath.Join(projectRoot, value)
	default:
		var err error
		abs, err = filepath.Abs(value)
		if err != nil {
			return "", &Error{Code: ErrInvalidConfig, Message: fmt.Sprintf("cannot resolve %s", field), Err: err}
		}
	}

	resolved, err := resolveExistingSymlinks(abs)
	if err != nil {
		return "", &Error{Code: ErrSymlinkEscape, Message: fmt.Sprintf("cannot resolve symlinks for %s: %s", field, value), Err: err}
	}
	return resolved, nil
}

// resolveExistingSymlinks walks the path resolving symlinks for the deepest
// existing prefix. Non-existing suffixes are appended cleaned but unresolved,
// which lets Validate reason about paths that init will create later.
func resolveExistingSymlinks(path string) (string, error) {
	path = filepath.Clean(path)
	if _, err := os.Lstat(path); err == nil {
		return filepath.EvalSymlinks(path)
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	for dir != path {
		if _, err := os.Lstat(dir); err == nil {
			resolvedDir, err := filepath.EvalSymlinks(dir)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolvedDir, base), nil
		}
		path = dir
		dir = filepath.Dir(path)
		base = filepath.Join(filepath.Base(path), base)
	}
	return path, nil
}
