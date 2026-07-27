// experience claude-code — Hito 10 slice 10.6.
//
// Mirrors the opencode CLI shape (`experience opencode scan`) with the
// `claude_code` source label and the additive skipped_* counters that the
// JSONL adapter surfaces (docs/22 §3, docs/25 Hito 2 row).
//
// Flags:
//
//	--project-root <path>   root to scan; required, must be a real path
//	--fixture <path>        optional explicit JSONL path; bypasses discovery
//	--once                  present for forward compatibility with Hito 3
//
// Output: a single JSON object on stdout (see experienceClaudecodeScanOutput).
// Errors land on stderr through logging.WriteError with the project's stable
// error envelope. Exit codes follow domain.ErrorCode.ExitCode().

package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"sort"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
	"agent-royo-learn/internal/experience/claudecode"
	"agent-royo-learn/internal/project"
)

// runExperienceClaudecode dispatches the "experience claude-code" subcommand.
// Only "scan" is implemented in slice 10.6; --watch lands in Hito 3.
func runExperienceClaudecode(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeExperienceError(stderr, "invalid_argument", "experience claude-code: a subcommand is required: scan")
	}
	switch args[0] {
	case "scan":
		return runExperienceClaudecodeScan(args[1:], stdout, stderr)
	default:
		return writeExperienceError(stderr, "invalid_argument", "experience claude-code: unknown subcommand %q: must be scan", args[0])
	}
}

// experienceClaudecodeInstanceReport is the per-instance JSON shape produced
// by `experience claude-code scan --once`. Field names are stable and pinned
// by the Hito 10 contract.
type experienceClaudecodeInstanceReport struct {
	JSONLPath      string `json:"jsonl_path"`
	Status         string `json:"status"`
	Code           string `json:"code,omitempty"`
	Message        string `json:"message,omitempty"`
	IngestedTurns  int    `json:"ingested_turns"`
	Duplicates     int    `json:"duplicates"`
	SkippedIncomp  int    `json:"skipped_incomplete"`
	SkippedMalform int    `json:"skipped_malformed"`
	EnvelopesTotal int    `json:"envelopes_total"`
}

// experienceClaudecodeScanOutput is the top-level JSON shape. Schema is
// stable; every consumer that gates on these field names should be updated
// only with a versioned contract change.
type experienceClaudecodeScanOutput struct {
	Source         string                               `json:"source"`
	Status         string                               `json:"status"`
	Instances      []experienceClaudecodeInstanceReport `json:"instances"`
	IngestedTurns  int                                  `json:"ingested_turns"`
	Duplicates     int                                  `json:"duplicates"`
	SkippedIncomp  int                                  `json:"skipped_incomplete"`
	SkippedMalform int                                  `json:"skipped_malformed"`
	EnvelopesTotal int                                  `json:"envelopes_total"`
}

// runExperienceClaudecodeScan orchestrates one ingestion pass: discover
// Claude Code JSONL stores reachable from the project root, health-check
// each one, scan the healthy ones, and forward every emitted envelope to
// the core experience.Service for persistence.
func runExperienceClaudecodeScan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience claude-code scan", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root to scan for Claude Code JSONL stores")
	fixture := fs.String("fixture", "", "optional explicit JSONL path; bypasses discovery for tests")
	once := fs.Bool("once", true, "run a single scan and exit (default true)")
	_ = once
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience claude-code scan: %v", err)
	}
	if *projectRoot == "" {
		return writeExperienceError(stderr, "invalid_argument", "experience claude-code scan: --project-root is required")
	}

	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	adapter := claudecode.NewAdapter()
	ctx := context.Background()

	var instances []claudecode.SourceInstance
	if *fixture != "" {
		// --fixture replaces discovery. Validate the path so a symlinked
		// fixture cannot bypass the same symlink guard discover() applies.
		extra, err := buildClaudeCodeFixtureInstance(*projectRoot, *fixture)
		if err != nil {
			return writeExperienceDomainError(stderr, err)
		}
		instances = []claudecode.SourceInstance{extra}
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
		return encodeExperienceClaudecodeOutput(stdout, experienceClaudecodeScanOutput{
			Source:    string(domain.SourceClaudeCode),
			Status:    "ok",
			Instances: []experienceClaudecodeInstanceReport{},
		})
	}

	service := experience.NewService(db)
	report := make([]experienceClaudecodeInstanceReport, 0, len(instances))
	overallStatus := "ok"

	for _, instance := range instances {
		hr := adapter.Health(ctx, instance)
		if hr.Status != "ok" {
			report = append(report, experienceClaudecodeInstanceReport{
				JSONLPath: instance.JSONLPath,
				Status:    hr.Status,
				Code:      hr.Code,
				Message:   hr.Message,
			})
			if hr.Status == "error" {
				overallStatus = "error"
			} else if overallStatus != "error" {
				overallStatus = "degraded"
			}
			continue
		}
		scanResult, scanErr := adapter.Scan(ctx, claudecode.ScanRequest{
			ProjectRoot: *projectRoot,
			Instance:    instance,
		})
		if scanErr != nil {
			if code := writeExperienceDomainError(stderr, scanErr); code != exitSuccess {
				return code
			}
		}
		instReport := experienceClaudecodeInstanceReport{
			JSONLPath:      instance.JSONLPath,
			Status:         scanResult.Status,
			Code:           scanResult.Code,
			Message:        scanResult.Message,
			EnvelopesTotal: len(scanResult.Envelopes),
			SkippedIncomp:  scanResult.SkippedIncomplete,
			SkippedMalform: scanResult.SkippedMalformed,
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

	sort.Slice(report, func(i, j int) bool { return report[i].JSONLPath < report[j].JSONLPath })

	total := experienceClaudecodeScanOutput{
		Source:    string(domain.SourceClaudeCode),
		Status:    overallStatus,
		Instances: report,
	}
	for _, r := range report {
		total.EnvelopesTotal += r.EnvelopesTotal
		total.IngestedTurns += r.IngestedTurns
		total.Duplicates += r.Duplicates
		total.SkippedIncomp += r.SkippedIncomp
		total.SkippedMalform += r.SkippedMalform
	}
	return encodeExperienceClaudecodeOutput(stdout, total)
}

// buildClaudeCodeFixtureInstance validates a --fixture path and returns a
// single SourceInstance for it. Mirrors the opencode precedent
// (buildFixtureInstance in experience.go): no symlinks, inside project_root,
// JSONL extension only. Symlinks and outside-root paths are rejected with the
// same typed errors discover.go applies.
func buildClaudeCodeFixtureInstance(projectRoot, fixturePath string) (claudecode.SourceInstance, error) {
	canonicalRoot, err := project.Canonicalize(projectRoot)
	if err != nil {
		return claudecode.SourceInstance{}, err
	}
	info, err := os.Lstat(fixturePath)
	if err != nil {
		return claudecode.SourceInstance{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return claudecode.SourceInstance{}, domain.NewValidationError(domain.ErrSymlinkEscape,
			"experience claude-code scan: --fixture is a symlink")
	}
	canonicalPath, err := project.Canonicalize(fixturePath)
	if err != nil {
		return claudecode.SourceInstance{}, err
	}
	if !project.IsInsideRoot(canonicalPath, canonicalRoot) {
		return claudecode.SourceInstance{}, domain.NewValidationError(domain.ErrPathOutsideRoot,
			"experience claude-code scan: --fixture is outside the project root")
	}
	return claudecode.SourceInstance{
		Source:      domain.SourceClaudeCode,
		ProjectRoot: canonicalRoot,
		JSONLPath:   canonicalPath,
		Schema:      claudecode.SchemaTag,
	}, nil
}

func encodeExperienceClaudecodeOutput(stdout io.Writer, output experienceClaudecodeScanOutput) int {
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
