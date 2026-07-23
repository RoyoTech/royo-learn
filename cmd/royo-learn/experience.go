package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
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
		return writeExperienceError(stderr, "invalid_argument", "experience: a subcommand is required: inject")
	}
	switch args[0] {
	case "inject":
		return runExperienceInject(args[1:], stdout, stderr)
	default:
		return writeExperienceError(stderr, "invalid_argument", "experience: unknown subcommand %q: must be inject", args[0])
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
