// experience patterns — Hito 6 slice 6.4 + Hito 7 slice 7.4.
//
// Slice 6.4 added three subcommands:
//
//   - list   — list patterns filtered by status / kind / limit.
//   - get    — fetch one pattern by id, including membership.
//   - dismiss — dismiss a pattern by id with a typed reason.
//
// Slice 7.4 adds the admin wrap that bridges a qualified pattern to a
// persisted Learning via promotion.Service.Promote:
//
//   - promote — promote a qualified pattern, idempotent on the
//     (pattern_id, fingerprint) idempotency key. Admin-only in MCP;
//     the CLI is the operator escape hatch for the same flow.
//
// Flags:
//
//	--project-root <path>   project root (required; resolves project id)
//	--status <status>       one of observed|qualified|dismissed|promoted|stale
//	--kind <kind>           filter by event kind
//	--limit <n>             cap the list response
//	--id <pattern-id>       pattern id for get / dismiss / promote
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
	"path/filepath"

	"agent-royo-learn/internal/capture"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/evidence"
	"agent-royo-learn/internal/experience/patterns"
	"agent-royo-learn/internal/experience/promotion"
)

// experiencePatternsOutput is the top-level JSON shape produced by
// the patterns subcommands. Schema is pinned by Hito 6 slice 6.4;
// consumers should only update on a versioned contract change. Slice
// 7.4 adds the optional Result envelope so the `promote` subcommand
// can surface the canonical promotion.PromotionResult without breaking
// the existing typed shape of the list/get/dismiss envelopes.
type experiencePatternsOutput struct {
	Operation string                       `json:"operation"`
	Status    string                       `json:"status"`
	Patterns  []patterns.ExperiencePattern `json:"patterns,omitempty"`
	Pattern   *patterns.ExperiencePattern  `json:"pattern,omitempty"`
	Members   []patterns.Membership        `json:"members,omitempty"`
	Total     int                          `json:"total"`
	Result    *promotion.PromotionResult   `json:"result,omitempty"`
}

// runExperiencePatterns dispatches the `experience patterns`
// subcommand.
func runExperiencePatterns(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns: a subcommand is required: list, get, dismiss, promote")
	}
	switch args[0] {
	case "list":
		return runExperiencePatternsList(args[1:], stdout, stderr)
	case "get":
		return runExperiencePatternsGet(args[1:], stdout, stderr)
	case "dismiss":
		return runExperiencePatternsDismiss(args[1:], stdout, stderr)
	case "promote":
		return runExperiencePatternsPromote(args[1:], stdout, stderr)
	default:
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns: unknown subcommand %q: must be list, get, dismiss, or promote", args[0])
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

// runExperiencePatternsPromote implements `experience patterns promote`.
// Hito 7 slice 7.4. The flag surface mirrors the dismiss subcommand so
// operators can rely on the same JSON envelope from the CLI:
//
//	--project-root <path>   project root (required)
//	--id <pattern-id>       pattern id (required)
//	--note <text>           optional reviewer note (bounded to MaxPromotionNoteBytes)
//	--actor <json>          optional actor JSON; defaults to {kind:system,name:cli}
//
// The handler runs the slice 7.3 idempotent pipeline end-to-end:
// patterns.Service.LookupPromotionState gates the well-known
// "already-promoted" case before capture.Service.Capture runs, the
// two-phase Promote pipeline runs the rest, and the updated pattern is
// re-fetched so the response includes the new status, revision and
// proposed_learning_id. The PromotionResult envelope is the canonical
// shape the audit row promised in docs/23-PATTERN-MINING.md §8.
func runExperiencePatternsPromote(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience patterns promote", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root to scope the promotion (required)")
	id := fs.String("id", "", "pattern id (required)")
	note := fs.String("note", "", "optional reviewer note (bounded to MaxPromotionNoteBytes)")
	actorJSON := fs.String("actor", "", "optional actor JSON; defaults to {kind:system,name:cli}")
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience patterns promote: %v", err)
	}
	if *projectRoot == "" {
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns promote: --project-root is required")
	}
	if *id == "" {
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns promote: --id is required")
	}
	// Reject oversized notes at the CLI surface so the operator gets a
	// typed error before the service runs. The same cap is enforced by
	// promotion.PromotionInput.Validate, but failing fast here yields a
	// more actionable message.
	if len(*note) > promotion.MaxPromotionNoteBytes {
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns promote: --note exceeds the permitted byte limit")
	}

	resolvedRoot, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	actor := domain.Actor{Kind: "system", Name: "cli"}
	if *actorJSON != "" {
		if err := json.Unmarshal([]byte(*actorJSON), &actor); err != nil {
			return writeExperienceError(stderr, "invalid_argument",
				"experience patterns promote: --actor is not valid JSON: %v", err)
		}
	}

	// Wire the capture service the same way the rest of the CLI does
	// (cmd/royo-learn/main.go: runCapture). The promotion pipeline
	// cross-references the capture pipeline, so the evidence layer must
	// be wired too. We tolerate the evidence init failure with a typed
	// error so the operator gets a clear message instead of a panic.
	recordsDir := filepath.Join(resolvedRoot, ".royo-learn", "records")
	evidenceSvc, err := evidence.NewService(resolvedRoot, nil)
	if err != nil {
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns promote: init evidence: %v", err)
	}
	captureSvc := capture.NewServiceWithEvidence(db, recordsDir, evidenceSvc)
	patternSvc := patterns.NewService(db)
	promotionSvc, err := promotion.NewService(captureSvc, patternSvc, db)
	if err != nil {
		return writeExperienceError(stderr, "invalid_argument",
			"experience patterns promote: init promotion service: %v", err)
	}

	input := &promotion.PromotionInput{
		PatternID: domain.ExperiencePatternID(*id),
		Actor:     actor,
		Note:      *note,
	}
	result, err := promotionSvc.Promote(context.Background(), projectID, input)
	if err != nil {
		return writeExperienceDomainError(stderr, err)
	}

	// Re-fetch so the caller sees the updated status, revision and
	// proposed_learning_id. The Get is cheap and is the only way the
	// caller can confirm the two-phase pipeline committed Phase 2.
	got, gErr := patternSvc.Get(context.Background(), domain.ExperiencePatternID(*id))
	if gErr != nil {
		return writeExperienceDomainError(stderr, gErr)
	}
	output := experiencePatternsOutput{
		Operation: "promote",
		Status:    "ok",
		Pattern:   got,
		Result:    result,
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
