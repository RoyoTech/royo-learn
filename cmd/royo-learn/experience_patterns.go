// experience patterns — Hito 6 slice 6.4.
//
// Slice 6.4 adds three subcommands to `experience patterns`:
//
//   - list   — list patterns filtered by status / kind / limit.
//   - get    — fetch one pattern by id, including membership.
//   - dismiss — dismiss a pattern by id with a typed reason.
//
// Flags:
//
//	--project-root <path>   project root (required; resolves project id)
//	--status <status>       one of observed|qualified|dismissed|promoted|stale
//	--kind <kind>           filter by event kind
//	--limit <n>             cap the list response
//	--id <pattern-id>       pattern id for get / dismiss
//	--reason <reason>       dismissal reason (one_off|not_reusable|...)
//	--note <text>           optional reviewer note (bounded)
//	--actor <json>          optional actor JSON (defaults to system)
//
// Output: a single JSON object on stdout. Errors land on stderr
// through the project-wide WriteError helper. Exit codes follow the
// domain.ErrorCode.ExitCode() mapping.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience/patterns"
)

// experiencePatternsOutput is the top-level JSON shape produced by
// the patterns subcommands. Schema is pinned by Hito 6 slice 6.4;
// consumers should only update on a versioned contract change.
type experiencePatternsOutput struct {
	Operation string                       `json:"operation"`
	Status    string                       `json:"status"`
	Patterns  []patterns.ExperiencePattern `json:"patterns,omitempty"`
	Pattern   *patterns.ExperiencePattern  `json:"pattern,omitempty"`
	Members   []patterns.Membership        `json:"members,omitempty"`
	Total     int                          `json:"total"`
}

// runExperiencePatterns dispatches the `experience patterns`
// subcommand.
func runExperiencePatterns(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns: a subcommand is required: list, get, dismiss")
	}
	switch args[0] {
	case "list":
		return runExperiencePatternsList(args[1:], stdout, stderr)
	case "get":
		return runExperiencePatternsGet(args[1:], stdout, stderr)
	case "dismiss":
		return runExperiencePatternsDismiss(args[1:], stdout, stderr)
	default:
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns: unknown subcommand %q: must be list, get, or dismiss", args[0])
	}
}

// runExperiencePatternsList implements `experience patterns list`.
func runExperiencePatternsList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience patterns list", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root to scope the list (required)")
	status := fs.String("status", string(patterns.PatternObserved), "status filter: observed|qualified|dismissed|promoted|stale")
	kind := fs.String("kind", "", "event-kind filter (e.g. test_failure)")
	limit := fs.Int("limit", 0, "maximum number of patterns to return (0 = no limit)")
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience patterns list: %v", err)
	}
	if *projectRoot == "" {
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns list: --project-root is required")
	}

	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	svc := patterns.NewService(db)
	filter := patterns.ListerFilter{
		Project: projectID,
		Status:  patterns.PatternStatus(*status),
		Kind:    domain.ExperienceEventKind(*kind),
		Limit:   *limit,
	}
	out, err := svc.List(context.Background(), filter)
	if err != nil {
		return writeExperienceDomainError(stderr, err)
	}

	output := experiencePatternsOutput{
		Operation: "list",
		Status:    "ok",
		Patterns:  out,
		Total:     len(out),
	}
	return encodeExperiencePatternsOutput(stdout, output)
}

// runExperiencePatternsGet implements `experience patterns get`.
func runExperiencePatternsGet(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience patterns get", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root to scope the lookup (required)")
	id := fs.String("id", "", "pattern id (required)")
	withMembers := fs.Bool("with-members", true, "include the membership list in the response")
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience patterns get: %v", err)
	}
	if *projectRoot == "" {
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns get: --project-root is required")
	}
	if *id == "" {
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns get: --id is required")
	}

	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	svc := patterns.NewService(db)
	got, err := svc.Get(context.Background(), domain.ExperiencePatternID(*id))
	if err != nil {
		return writeExperienceDomainError(stderr, err)
	}

	output := experiencePatternsOutput{
		Operation: "get",
		Status:    "ok",
		Pattern:   got,
		Total:     1,
	}
	if *withMembers {
		members, mErr := patterns.NewRepository(db).Members(context.Background(), got.ID)
		if mErr != nil {
			return writeExperienceDomainError(stderr, mErr)
		}
		output.Members = members
	}
	_ = projectID // scoping is enforced by the project resolver
	return encodeExperiencePatternsOutput(stdout, output)
}

// runExperiencePatternsDismiss implements `experience patterns dismiss`.
func runExperiencePatternsDismiss(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience patterns dismiss", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root to scope the dismissal (required)")
	id := fs.String("id", "", "pattern id (required)")
	reason := fs.String("reason", "", "dismissal reason: one_off|not_reusable|already_covered|contradicted|insufficient_evidence|private_or_sensitive|false_cluster")
	note := fs.String("note", "", "optional reviewer note (bounded)")
	actorJSON := fs.String("actor", "", "optional actor JSON; defaults to {kind:system,name:cli}")
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience patterns dismiss: %v", err)
	}
	if *projectRoot == "" {
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns dismiss: --project-root is required")
	}
	if *id == "" {
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns dismiss: --id is required")
	}
	if *reason == "" {
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns dismiss: --reason is required")
	}

	_, db, _, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	actor := domain.Actor{Kind: "system", Name: "cli"}
	if *actorJSON != "" {
		if err := json.Unmarshal([]byte(*actorJSON), &actor); err != nil {
			return writeExperienceError(stderr, "invalid_argument",
				"experience patterns dismiss: --actor is not valid JSON: %v", err)
		}
	}

	svc := patterns.NewService(db)
	err := svc.Dismiss(context.Background(),
		domain.ExperiencePatternID(*id),
		patterns.DismissalReason(*reason),
		patterns.DismissalDetails{
			Reason: patterns.DismissalReason(*reason),
			Note:   *note,
			Actor:  actor,
		})
	if err != nil {
		return writeExperienceDomainError(stderr, err)
	}

	// Re-fetch so the caller sees the updated status + reason.
	got, gErr := svc.Get(context.Background(), domain.ExperiencePatternID(*id))
	if gErr != nil {
		return writeExperienceDomainError(stderr, gErr)
	}
	output := experiencePatternsOutput{
		Operation: "dismiss",
		Status:    "ok",
		Pattern:   got,
		Total:     1,
	}
	return encodeExperiencePatternsOutput(stdout, output)
}

// encodeExperiencePatternsOutput writes the JSON envelope to stdout.
func encodeExperiencePatternsOutput(stdout io.Writer, out experiencePatternsOutput) int {
	if out.Patterns == nil {
		out.Patterns = []patterns.ExperiencePattern{}
	}
	if out.Members == nil {
		out.Members = []patterns.Membership{}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		// Marshal failures on the typed output are internal errors;
		// stderr is in scope from the caller but writeExperienceError
		// receives it explicitly, so we surface via the io.Writer.
		return writeEncodeFailure(stdout)
	}
	if _, err := stdout.Write(encoded); err != nil {
		return exitFailure
	}
	if _, err := stdout.Write([]byte("\n")); err != nil {
		return exitFailure
	}
	return exitSuccess
}

// writeEncodeFailure collapses the rare json.Marshal failure on the
// typed output. We deliberately do not call writeExperienceError here
// because the caller passes stderr only as part of the larger
// orchestration, and this branch is the only one where stdout itself
// is no longer trustworthy. Returning exitFailure matches the rest
// of the project (cmd/royo-learn/experience_detect.go).
func writeEncodeFailure(stdout io.Writer) int {
	_, _ = stdout.Write([]byte(`{"status":"error","code":"internal_error"}` + "\n"))
	return exitFailure
}
