package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
	"agent-royo-learn/internal/experience/claudecode"
	experiencecli "agent-royo-learn/internal/experience/cli"
	"agent-royo-learn/internal/experience/codex"
	"agent-royo-learn/internal/experience/opencode"
	"agent-royo-learn/internal/logging"
	"agent-royo-learn/internal/project"
)

type experienceInjectOutput struct {
	SessionID   string `json:"session_id"`
	TurnID      string `json:"turn_id"`
	Fingerprint string `json:"fingerprint"`
	Duplicate   bool   `json:"duplicate"`
	Skipped     bool   `json:"skipped"`
}

func runExperience(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeExperienceError(stderr, "invalid_argument", "experience: a subcommand is required: detect, inject, scan, patterns, trace, jobs")
	}
	switch args[0] {
	case "inject":
		return runExperienceInject(args[1:], stdout, stderr)
	case "scan":
		return runExperienceUnified(args[1:], stdout, stderr)
	case "opencode":
		if experiencecli.ExperimentalCLICollapse() {
			return writeExperienceError(stderr, "invalid_argument", "experience: unknown subcommand %q", args[0])
		}
		return runExperienceLegacy(string(domain.SourceOpenCode), args[1:], stdout, stderr)
	case "claude-code", "claudecode":
		if experiencecli.ExperimentalCLICollapse() {
			return writeExperienceError(stderr, "invalid_argument", "experience: unknown subcommand %q", args[0])
		}
		return runExperienceLegacy(string(domain.SourceClaudeCode), args[1:], stdout, stderr)
	case "codex":
		if experiencecli.ExperimentalCLICollapse() {
			return writeExperienceError(stderr, "invalid_argument", "experience: unknown subcommand %q", args[0])
		}
		return runExperienceLegacy(string(domain.SourceCodex), args[1:], stdout, stderr)
	case "detect":
		return runExperienceDetect(args[1:], stdout, stderr)
	case "patterns":
		return runExperiencePatterns(args[1:], stdout, stderr)
	case "trace":
		return runExperienceTrace(args[1:], stdout, stderr)
	case "jobs":
		return runExperienceJobs(args[1:], stdout, stderr)
	default:
		return writeExperienceError(stderr, "invalid_argument", "experience: unknown subcommand %q: must be detect, inject, scan, patterns, trace, or jobs", args[0])
	}
}

func runExperienceUnified(args []string, stdout, stderr io.Writer) int {
	source, remaining, err := parseExperienceSource(args)
	if err != nil {
		return writeExperienceError(stderr, "invalid_argument", "%v; usage: experience scan --source=<opencode|claudecode|codex>", err)
	}
	return runExperienceSource(source, remaining, stdout, stderr)
}

func parseExperienceSource(args []string) (string, []string, error) {
	remaining := make([]string, 0, len(args))
	source := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case strings.HasPrefix(arg, "--source="):
			source = strings.TrimPrefix(arg, "--source=")
		case arg == "--source":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("experience scan: --source requires a value")
			}
			i++
			source = args[i]
		default:
			remaining = append(remaining, arg)
		}
	}
	if source == "" {
		return "", nil, fmt.Errorf("experience scan: --source is required")
	}
	return source, remaining, nil
}

func runExperienceLegacy(source string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "scan" {
		return writeExperienceError(stderr, "invalid_argument", "experience %s: a subcommand is required: scan", source)
	}
	_, _ = fmt.Fprintf(stderr, "DEPRECATED: use 'experience scan --source=%s' (legacy form removed in v1)\n", source)
	return runExperienceSource(source, args[1:], stdout, stderr)
}

func runExperienceSource(source string, args []string, stdout, stderr io.Writer) int {
	switch source {
	case string(domain.SourceOpenCode):
		_ = opencode.NewAdapter().Job()
		return runExperienceOpencodeScan(args, stdout, stderr)
	case "claudecode", string(domain.SourceClaudeCode):
		_ = claudecode.NewAdapter().Job()
		return runExperienceClaudecodeScan(args, stdout, stderr)
	case string(domain.SourceCodex):
		_ = codex.NewAdapter().Job()
		return runExperienceCodexScan(args, stdout, stderr)
	default:
		return writeExperienceError(stderr, "invalid_argument", "experience scan: invalid --source %q; allowed values: opencode, claudecode, codex", source)
	}
}

func runExperienceInject(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience inject", flag.ContinueOnError)
	envelopePath := fs.String("envelope", "", "path to an ExperienceEnvelope JSON file, or - for stdin")
	projectRoot := fs.String("project-root", "", "explicit project root")
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience inject: %v", err)
	}
	if *envelopePath == "" {
		return writeExperienceError(stderr, "invalid_argument", "experience inject: --envelope is required")
	}
	var input io.Reader = os.Stdin
	var file *os.File
	if *envelopePath != "-" {
		var err error
		file, err = os.Open(*envelopePath)
		if err != nil {
			return writeExperienceError(stderr, "invalid_argument", "experience inject: cannot open envelope: %v", err)
		}
		defer file.Close()
		input = file
	}
	decoder := json.NewDecoder(input)
	decoder.UseNumber()
	var envelope experience.ExperienceEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience inject: cannot parse envelope: %v", err)
	}
	if err := experience.ValidateEnvelope(&envelope); err != nil {
		return writeExperienceDomainError(stderr, err)
	}
	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()
	result, err := experience.NewService(db).IngestEnvelope(context.Background(), projectID, envelope)
	if err != nil {
		return writeExperienceDomainError(stderr, err)
	}
	output := experienceInjectOutput{SessionID: string(result.Session.ID), TurnID: string(result.Turn.ID), Fingerprint: result.Turn.Fingerprint, Duplicate: result.Idempotent, Skipped: false}
	if err := json.NewEncoder(stdout).Encode(output); err != nil {
		return exitFailure
	}
	return exitSuccess
}

func writeExperienceError(stderr io.Writer, code, format string, args ...interface{}) int {
	msg := fmt.Sprintf(format, args...)
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{Code: code, Message: msg, Recoverable: true, Details: map[string]any{}, NextAction: `run "royo-learn experience --help"`})
	return domain.ErrorCode(code).ExitCode()
}

func writeExperienceDomainError(stderr io.Writer, err error) int {
	if domainErr, ok := domain.AsDomainError(err); ok {
		return writeExperienceError(stderr, string(domainErr.Code), "%s", domainErr.Message)
	}
	return writeExperienceError(stderr, "invalid_argument", "experience inject: %v", err)
}

// runExperienceOpencode dispatches the "experience opencode" subcommand.
// Only "scan" is implemented in slice 2.6; --watch lands in Ola 2.
func runExperienceOpencode(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeExperienceError(stderr, "invalid_argument", "experience opencode: a subcommand is required: scan")
	}
	switch args[0] {
	case "scan":
		return runExperienceOpencodeScan(args[1:], stdout, stderr)
	default:
		return writeExperienceError(stderr, "invalid_argument", "experience opencode: unknown subcommand %q: must be scan", args[0])
	}
}

// experienceOpencodeInstanceReport is the per-instance JSON shape
// produced by `experience opencode scan --once`. Fields are stable and
// pinned by the Hito 2 contract.
type experienceOpencodeInstanceReport struct {
	DBPath         string `json:"db_path"`
	Status         string `json:"status"`
	Code           string `json:"code,omitempty"`
	Message        string `json:"message,omitempty"`
	IngestedTurns  int    `json:"ingested_turns"`
	Duplicates     int    `json:"duplicates"`
	SkippedIncomp  int    `json:"skipped_incomplete"`
	EnvelopesTotal int    `json:"envelopes_total"`
}

// experienceOpencodeScanOutput is the top-level JSON shape. Schema is
// stable: every consumer that gates on these field names should be
// updated only with a versioned contract change.
type experienceOpencodeScanOutput struct {
	Source         string                             `json:"source"`
	Status         string                             `json:"status"`
	Instances      []experienceOpencodeInstanceReport `json:"instances"`
	IngestedTurns  int                                `json:"ingested_turns"`
	Duplicates     int                                `json:"duplicates"`
	SkippedIncomp  int                                `json:"skipped_incomplete"`
	EnvelopesTotal int                                `json:"envelopes_total"`
}

// runExperienceOpencodeScan orchestrates one ingestion pass: discover
// OpenCode stores reachable from the project root, health-check each one,
// scan the healthy ones, and forward every emitted envelope to the core
// experience.Service for persistence.
//
// Flags:
//
//	--project-root <path>   root to scan; required, must be a real path
//	--fixture <path>        optional explicit opencode.db path; bypasses
//	                        discovery for test fixtures
//	--once                  present for forward compatibility; --watch is
//	                        not implemented in slice 2.6
//
// Output: a single JSON object on stdout (see experienceOpencodeScanOutput).
// Errors land on stderr through logging.WriteError with the project's
// stable error envelope. Exit codes follow domain.ErrorCode.ExitCode().
func runExperienceOpencodeScan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience opencode scan", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root to scan for OpenCode stores")
	fixture := fs.String("fixture", "", "optional explicit opencode.db path; bypasses discovery for tests")
	once := fs.Bool("once", true, "run a single scan and exit (default true)")
	_ = once
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience opencode scan: %v", err)
	}
	if *projectRoot == "" {
		return writeExperienceError(stderr, "invalid_argument", "experience opencode scan: --project-root is required")
	}

	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	adapter := opencode.NewAdapter()
	ctx := context.Background()

	var instances []opencode.SourceInstance
	if *fixture != "" {
		// --fixture replaces discovery. The test-and-demo path is explicit
		// and the core locator validation already constrains the fixture to
		// be inside projectRoot. Validate the path here so a symlinked
		// fixture cannot bypass the same symlink guard discover() applies.
		extra, err := buildFixtureInstance(*projectRoot, *fixture)
		if err != nil {
			return writeExperienceDomainError(stderr, err)
		}
		instances = []opencode.SourceInstance{extra}
	} else {
		discovered, discoverErr := adapter.Discover(ctx, *projectRoot)
		if discoverErr != nil {
			if code := writeExperienceDomainError(stderr, discoverErr); code != exitSuccess {
				return code
			}
		}
		instances = discovered
	}

	if len(instances) == 0 {
		output := experienceOpencodeScanOutput{
			Source:    string(domain.SourceOpenCode),
			Status:    "ok",
			Instances: []experienceOpencodeInstanceReport{},
		}
		return encodeExperienceOpencodeOutput(stdout, output)
	}

	service := experience.NewService(db)
	report := make([]experienceOpencodeInstanceReport, 0, len(instances))
	overallStatus := "ok"

	for _, instance := range instances {
		hr := adapter.Health(ctx, instance)
		if hr.Status != "ok" {
			report = append(report, experienceOpencodeInstanceReport{
				DBPath:  instance.DBPath,
				Status:  hr.Status,
				Code:    hr.Code,
				Message: hr.Message,
			})
			if hr.Status == "error" {
				overallStatus = "error"
			} else if overallStatus != "error" {
				overallStatus = "degraded"
			}
			continue
		}
		scanResult, scanErr := adapter.Scan(ctx, opencode.ScanRequest{
			ProjectRoot: *projectRoot,
			Instance:    instance,
		})
		if scanErr != nil {
			if code := writeExperienceDomainError(stderr, scanErr); code != exitSuccess {
				return code
			}
		}
		instReport := experienceOpencodeInstanceReport{
			DBPath:         instance.DBPath,
			Status:         scanResult.Status,
			Code:           scanResult.Code,
			Message:        scanResult.Message,
			EnvelopesTotal: len(scanResult.Envelopes),
			SkippedIncomp:  scanResult.SkippedIncomplete,
		}
		if scanResult.Status == "degraded" && overallStatus == "ok" {
			overallStatus = "degraded"
		}
		for _, env := range scanResult.Envelopes {
			res, err := service.IngestEnvelope(ctx, projectID, env)
			if err != nil {
				if code := writeExperienceDomainError(stderr, err); code != exitSuccess {
					return code
				}
				continue
			}
			if res.Idempotent {
				instReport.Duplicates++
			} else {
				instReport.IngestedTurns++
			}
		}
		report = append(report, instReport)
	}

	sort.Slice(report, func(i, j int) bool { return report[i].DBPath < report[j].DBPath })

	total := experienceOpencodeScanOutput{
		Source:    string(domain.SourceOpenCode),
		Status:    overallStatus,
		Instances: report,
	}
	for _, r := range report {
		total.EnvelopesTotal += r.EnvelopesTotal
		total.IngestedTurns += r.IngestedTurns
		total.Duplicates += r.Duplicates
		total.SkippedIncomp += r.SkippedIncomp
	}
	return encodeExperienceOpencodeOutput(stdout, total)
}

// buildFixtureInstance validates a --fixture path and returns a single
// SourceInstance for it. Rejects symlinks (ErrSymlinkEscape) and paths
// outside the canonical project root (ErrPathOutsideRoot). Mirrors the
// security checks discover.go applies, so --fixture cannot bypass the
// guard just because it skips the directory walk.
func buildFixtureInstance(projectRoot, fixturePath string) (opencode.SourceInstance, error) {
	canonicalRoot, err := project.Canonicalize(projectRoot)
	if err != nil {
		return opencode.SourceInstance{}, err
	}
	info, err := os.Lstat(fixturePath)
	if err != nil {
		return opencode.SourceInstance{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return opencode.SourceInstance{}, domain.NewValidationError(domain.ErrSymlinkEscape,
			"experience opencode scan: --fixture is a symlink")
	}
	canonicalPath, err := project.Canonicalize(fixturePath)
	if err != nil {
		return opencode.SourceInstance{}, err
	}
	if !project.IsInsideRoot(canonicalPath, canonicalRoot) {
		return opencode.SourceInstance{}, domain.NewValidationError(domain.ErrPathOutsideRoot,
			"experience opencode scan: --fixture is outside the project root")
	}
	return opencode.SourceInstance{
		Source:      domain.SourceOpenCode,
		ProjectRoot: canonicalRoot,
		DBPath:      canonicalPath,
		Schema:      opencode.SchemaTag,
	}, nil
}

func encodeExperienceOpencodeOutput(stdout io.Writer, output experienceOpencodeScanOutput) int {
	encoded, err := json.Marshal(output)
	if err != nil {
		return exitFailure
	}
	if _, err := stdout.Write(encoded); err != nil {
		return exitFailure
	}
	if _, err := stdout.Write([]byte("\n")); err != nil {
		return exitFailure
	}
	return exitSuccess
}
