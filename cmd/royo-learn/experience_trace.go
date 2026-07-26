package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/trace"
)

// runExperienceTrace implements `experience trace`.
// Hito 4 slice 4.2.
//
//	--project-root <path>  project root (required)
//	--id <learning-id>     learning id to trace (required)
//	--excerpt              include redacted excerpt (default false)
func runExperienceTrace(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience trace", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root (required)")
	id := fs.String("id", "", "learning id (required)")
	excerpt := fs.Bool("excerpt", false, "include redacted excerpt")
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience trace: %v", err)
	}
	if *projectRoot == "" {
		return writeExperienceError(stderr, "invalid_argument", "experience trace: --project-root is required")
	}
	if *id == "" {
		return writeExperienceError(stderr, "invalid_argument", "experience trace: --id is required")
	}

	_, db, _, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	svc := trace.NewService(db.DB)
	bounds := trace.TraceBounds{
		IncludeExcerpt:  *excerpt,
		MaxExcerptBytes: 1024, // default 1 KiB per threat model
	}
	result, err := svc.Trace(context.Background(), domain.LearningID(*id), bounds)
	if err != nil {
		return writeExperienceDomainError(stderr, err)
	}

	output := struct {
		Operation string             `json:"operation"`
		Status    string             `json:"status"`
		Result    *trace.TraceResult `json:"result"`
	}{
		Operation: "trace",
		Status:    "ok",
		Result:    result,
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return writeExperienceError(stderr, "internal_error", "experience trace: encode result: %v", err)
	}
	return exitSuccess
}
