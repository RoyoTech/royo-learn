// experience jobs — Hito 8 slices 8.3 and 8.4.
//
// The subcommand dispatches to register, list, run-due, and recover.
// Every variant reads the canonical experience store for --project-root
// and forwards every operation through the jobs.Service.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"

	"agent-royo-learn/internal/experience/jobs"
)

func runExperienceJobs(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeExperienceError(stderr, "invalid_argument",
			"experience jobs: a subcommand is required: list, register, run-due, recover")
	}
	switch args[0] {
	case "list":
		return runExperienceJobsList(args[1:], stdout, stderr)
	case "register":
		return runExperienceJobsRegister(args[1:], stdout, stderr)
	case "run-due":
		return runExperienceJobsRunDue(args[1:], stdout, stderr)
	case "recover":
		return runExperienceJobsRecover(args[1:], stdout, stderr)
	default:
		return writeExperienceError(stderr, "invalid_argument",
			"experience jobs: unknown subcommand %q: must be list, register, run-due, or recover", args[0])
	}
}

// runExperienceJobsList lists all job states for a project.
func runExperienceJobsList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience jobs list", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root (required)")
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience jobs list: %v", err)
	}
	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	svc := jobs.NewServiceWithDefaults(db.DB)
	ctx := context.Background()

	states, err := svc.ListStates(ctx, projectID)
	if err != nil {
		return writeExperienceDomainError(stderr, err)
	}

	type outputItem struct {
		JobName       string `json:"job_name"`
		Status        string `json:"status"`
		RetryCount    int    `json:"retry_count"`
		LastErrorCode string `json:"last_error_code,omitempty"`
		LeaseOwner    string `json:"lease_owner,omitempty"`
	}
	output := make([]outputItem, 0, len(states))
	for _, s := range states {
		output = append(output, outputItem{
			JobName:       s.JobName,
			Status:        string(s.Status),
			RetryCount:    s.RetryCount,
			LastErrorCode: s.LastErrorCode,
			LeaseOwner:    s.LeaseOwner,
		})
	}
	if err := json.NewEncoder(stdout).Encode(output); err != nil {
		return exitFailure
	}
	return exitSuccess
}

// runExperienceJobsRegister registers or updates a job.
func runExperienceJobsRegister(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience jobs register", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root (required)")
	name := fs.String("name", "", "job name (required)")
	description := fs.String("description", "", "job description")
	intervalSec := fs.Int("interval-sec", 3600, "default interval in seconds")
	maxRetries := fs.Int("max-retries", 3, "max retries")
	enabledFlag := fs.Bool("enabled", true, "whether the job is enabled")
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience jobs register: %v", err)
	}
	if *name == "" {
		return writeExperienceError(stderr, "invalid_argument", "experience jobs register: --name is required")
	}

	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	svc := jobs.NewServiceWithDefaults(db.DB)
	ctx := context.Background()

	entry := jobs.JobRegistryEntry{
		JobName:            *name,
		Description:        *description,
		DefaultIntervalSec: *intervalSec,
		DefaultMaxRetries:  *maxRetries,
		Enabled:            *enabledFlag,
	}
	if err := svc.Register(ctx, projectID, entry); err != nil {
		return writeExperienceDomainError(stderr, err)
	}

	output := map[string]string{"status": "registered", "job_name": *name}
	if err := json.NewEncoder(stdout).Encode(output); err != nil {
		return exitFailure
	}
	return exitSuccess
}

// runExperienceJobsRunDue runs all due jobs for a project.
func runExperienceJobsRunDue(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience jobs run-due", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root (required)")
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience jobs run-due: %v", err)
	}

	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	svc := jobs.NewServiceWithDefaults(db.DB)
	ctx := context.Background()

	// Run-due without registered functions reports which jobs would run.
	// In a real deployment, the registered functions are wired at startup.
	results, err := svc.RunDue(ctx, projectID, "cli-runner", nil)
	if err != nil {
		return writeExperienceDomainError(stderr, err)
	}

	type outputItem struct {
		Status  string `json:"status"`
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
	}
	output := make([]outputItem, 0, len(results))
	for _, r := range results {
		output = append(output, outputItem{
			Status:  string(r.Status),
			Code:    r.Code,
			Message: r.Message,
		})
	}
	if err := json.NewEncoder(stdout).Encode(output); err != nil {
		return exitFailure
	}
	return exitSuccess
}

// runExperienceJobsRecover clears stale leases.
func runExperienceJobsRecover(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience jobs recover", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root (required)")
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience jobs recover: %v", err)
	}

	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	svc := jobs.NewServiceWithDefaults(db.DB)
	ctx := context.Background()

	recovered, err := svc.RecoverStaleLeases(ctx, projectID)
	if err != nil {
		return writeExperienceDomainError(stderr, err)
	}

	output := map[string]interface{}{
		"status":    "ok",
		"recovered": recovered,
	}
	if err := json.NewEncoder(stdout).Encode(output); err != nil {
		return exitFailure
	}
	return exitSuccess
}
