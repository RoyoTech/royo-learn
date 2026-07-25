package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
	"agent-royo-learn/internal/experience/opencode"
	"agent-royo-learn/internal/logging"
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
		return writeExperienceError(stderr, "invalid_argument", "experience: a subcommand is required: inject, opencode")
	}
	switch args[0] {
	case "inject":
		return runExperienceInject(args[1:], stdout, stderr)
	case "opencode":
		return runExperienceOpencode(args[1:], stdout, stderr)
	default:
		return writeExperienceError(stderr, "invalid_argument", "experience: unknown subcommand %q: must be inject or opencode", args[0])
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

	instances, discoverErr := adapter.Discover(ctx, *projectRoot)
	if discoverErr != nil {
		if code := writeExperienceDomainError(stderr, discoverErr); code != exitSuccess {
			return code
		}
	}

	if *fixture != "" {
		instances = appendFixtureInstance(instances, *projectRoot, *fixture)
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

// appendFixtureInstance appends an instance for --fixture, deduplicating
// any DBPath that Discovery already returned.
func appendFixtureInstance(instances []opencode.SourceInstance, projectRoot, fixturePath string) []opencode.SourceInstance {
	for _, inst := range instances {
		if inst.DBPath == fixturePath {
			return instances
		}
	}
	return append(instances, opencode.SourceInstance{
		Source:      domain.SourceOpenCode,
		ProjectRoot: projectRoot,
		DBPath:      fixturePath,
		Schema:      opencode.SchemaTag,
	})
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
