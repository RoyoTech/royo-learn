// experience codex — Hito 10 slice 10.6.
//
// Mirrors the Claude Code CLI shape with the `codex` source label and the
// rollout-specific path field (docs/22 §3, docs/25 Hito 10 row).
//
// Flags:
//
//	--project-root <path>   root to scan; required, must be a real path
//	--fixture <path>        optional explicit rollout path; bypasses discovery
//	--once                  present for forward compatibility with Ola 2
//
// Output: a single JSON object on stdout (see experienceCodexScanOutput).
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
	"agent-royo-learn/internal/experience/codex"
	"agent-royo-learn/internal/project"
)

// runExperienceCodex dispatches the "experience codex" subcommand.
// Only "scan" is implemented in slice 10.6; --watch lands in Ola 2.
func runExperienceCodex(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeExperienceError(stderr, "invalid_argument", "experience codex: a subcommand is required: scan")
	}
	switch args[0] {
	case "scan":
		return runExperienceCodexScan(args[1:], stdout, stderr)
	default:
		return writeExperienceError(stderr, "invalid_argument", "experience codex: unknown subcommand %q: must be scan", args[0])
	}
}

// experienceCodexInstanceReport is the per-instance JSON shape produced
// by `experience codex scan --once`. Field names are stable and pinned
// by the Hito 10 contract.
type experienceCodexInstanceReport struct {
	RolloutPath    string `json:"rollout_path"`
	Status         string `json:"status"`
	Code           string `json:"code,omitempty"`
	Message        string `json:"message,omitempty"`
	IngestedTurns  int    `json:"ingested_turns"`
	Duplicates     int    `json:"duplicates"`
	SkippedIncomp  int    `json:"skipped_incomplete"`
	SkippedMalform int    `json:"skipped_malformed"`
	EnvelopesTotal int    `json:"envelopes_total"`
}

// experienceCodexScanOutput is the top-level JSON shape. Schema is
// stable; every consumer that gates on these field names should be updated
// only with a versioned contract change.
type experienceCodexScanOutput struct {
	Source         string                          `json:"source"`
	Status         string                          `json:"status"`
	Instances      []experienceCodexInstanceReport `json:"instances"`
	IngestedTurns  int                             `json:"ingested_turns"`
	Duplicates     int                             `json:"duplicates"`
	SkippedIncomp  int                             `json:"skipped_incomplete"`
	SkippedMalform int                             `json:"skipped_malformed"`
	EnvelopesTotal int                             `json:"envelopes_total"`
}

// runExperienceCodexScan orchestrates one ingestion pass: discover Codex
// rollout JSONL stores reachable from the project root, health-check each
// one, scan the healthy ones, and forward every emitted envelope to the
// core experience.Service for persistence.
func runExperienceCodexScan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience codex scan", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root to scan for Codex rollout JSONL stores")
	fixture := fs.String("fixture", "", "optional explicit rollout path; bypasses discovery for tests")
	once := fs.Bool("once", true, "run a single scan and exit (default true)")
	_ = once
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience codex scan: %v", err)
	}
	if *projectRoot == "" {
		return writeExperienceError(stderr, "invalid_argument", "experience codex scan: --project-root is required")
	}

	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	adapter := codex.NewAdapter()
	ctx := context.Background()

	var instances []codex.SourceInstance
	if *fixture != "" {
		extra, err := buildCodexFixtureInstance(*projectRoot, *fixture)
		if err != nil {
			return writeExperienceDomainError(stderr, err)
		}
		instances = []codex.SourceInstance{extra}
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
		return encodeExperienceCodexOutput(stdout, experienceCodexScanOutput{
			Source:    string(domain.SourceCodex),
			Status:    "ok",
			Instances: []experienceCodexInstanceReport{},
		})
	}

	service := experience.NewService(db)
	report := make([]experienceCodexInstanceReport, 0, len(instances))
	overallStatus := "ok"

	for _, instance := range instances {
		hr := adapter.Health(ctx, instance)
		if hr.Status != "ok" {
			report = append(report, experienceCodexInstanceReport{
				RolloutPath: instance.RolloutPath,
				Status:      hr.Status,
				Code:        hr.Code,
				Message:     hr.Message,
			})
			if hr.Status == "error" {
				overallStatus = "error"
			} else if overallStatus != "error" {
				overallStatus = "degraded"
			}
			continue
		}
		scanResult, scanErr := adapter.Scan(ctx, codex.ScanRequest{
			ProjectRoot: *projectRoot,
			Instance:    instance,
		})
		if scanErr != nil {
			if code := writeExperienceDomainError(stderr, scanErr); code != exitSuccess {
				return code
			}
		}
		instReport := experienceCodexInstanceReport{
			RolloutPath:    instance.RolloutPath,
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

	sort.Slice(report, func(i, j int) bool { return report[i].RolloutPath < report[j].RolloutPath })

	total := experienceCodexScanOutput{
		Source:    string(domain.SourceCodex),
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
	return encodeExperienceCodexOutput(stdout, total)
}

// buildCodexFixtureInstance validates a --fixture path and returns a
// single SourceInstance for it. Mirrors the claudecode precedent: no
// symlinks, inside project_root, rollout JSONL extension only. Symlinks
// and outside-root paths are rejected with the same typed errors
// discover.go applies.
func buildCodexFixtureInstance(projectRoot, fixturePath string) (codex.SourceInstance, error) {
	canonicalRoot, err := project.Canonicalize(projectRoot)
	if err != nil {
		return codex.SourceInstance{}, err
	}
	info, err := os.Lstat(fixturePath)
	if err != nil {
		return codex.SourceInstance{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return codex.SourceInstance{}, domain.NewValidationError(domain.ErrSymlinkEscape,
			"experience codex scan: --fixture is a symlink")
	}
	canonicalPath, err := project.Canonicalize(fixturePath)
	if err != nil {
		return codex.SourceInstance{}, err
	}
	if !project.IsInsideRoot(canonicalPath, canonicalRoot) {
		return codex.SourceInstance{}, domain.NewValidationError(domain.ErrPathOutsideRoot,
			"experience codex scan: --fixture is outside the project root")
	}
	return codex.SourceInstance{
		Source:      domain.SourceCodex,
		ProjectRoot: canonicalRoot,
		RolloutPath: canonicalPath,
		Schema:      codex.SchemaTag,
	}, nil
}

func encodeExperienceCodexOutput(stdout io.Writer, output experienceCodexScanOutput) int {
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
