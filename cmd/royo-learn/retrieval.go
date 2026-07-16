package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"agent-royo-learn/internal/config"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/engram"
	"agent-royo-learn/internal/logging"
	"agent-royo-learn/internal/recurrence"
	"agent-royo-learn/internal/storage"
)

// ---------------------------------------------------------------------------
// get — retrieve a single learning by ID (docs/04-CLI-SPEC.md:166)
// ---------------------------------------------------------------------------

func runGet(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return writeRetrievalError(stderr, "invalid_argument", "get: a learning id is required as the first argument")
	}
	learningID := args[0]

	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	includeEvidence := fs.Bool("include-evidence", false, "include attached evidence records")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")
	if err := fs.Parse(args[1:]); err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "get: %v", err)
	}

	root, db, _, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "get: begin tx: %v", err)
	}
	learning, err := storage.GetLearning(ctx, tx, domain.LearningID(learningID))
	tx.Rollback()
	if err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "get: %v", err)
	}
	if learning == nil {
		return writeRetrievalError(stderr, "learning_not_found", "get: learning %q not found", learningID)
	}

	out := learningToOutputMap(learning)

	if *includeEvidence {
		svc, evErr := newEvidenceCaptureSvc(root, db)
		if evErr != nil {
			return writeRetrievalError(stderr, "invalid_argument", "get: %v", evErr)
		}
		records, evErr := svc.ListEvidence(ctx, domain.LearningID(learningID))
		if evErr != nil {
			return writeRetrievalError(stderr, "invalid_argument", "get: %v", evErr)
		}
		items := make([]map[string]any, 0, len(records))
		for _, r := range records {
			items = append(items, map[string]any{
				"id":       string(r.ID),
				"kind":     string(r.Kind),
				"summary":  r.Summary,
				"source":   r.URI,
				"sha256":   r.SHA256,
				"redacted": r.Redacted,
			})
		}
		out["evidence"] = items
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(out, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "%s [%s] %s\n", learning.ID, learning.Status, learning.Title)
	}
	return exitSuccess
}

// ---------------------------------------------------------------------------
// search — full-text search over learnings (docs/04-CLI-SPEC.md:189)
// ---------------------------------------------------------------------------

func runSearch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	query := fs.String("query", "", "search query (or the first positional argument)")
	limit := fs.Int("limit", 50, "max results")
	statusFilter := fs.String("status", "", "filter by learning status")
	includeEngram := fs.Bool("include-engram", false, "also search Engram memory when enabled")
	allProjects := fs.Bool("all-projects", false, "search across all projects (reserved)")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	// Accept the query as a leading positional argument (docs/04: `search <query>`).
	rest := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		*query = args[0]
		rest = args[1:]
	}
	if err := fs.Parse(rest); err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "search: %v", err)
	}
	_ = allProjects // reserved for Hito 2; declared so the flag is accepted.

	if *query == "" {
		return writeRetrievalError(stderr, "invalid_argument", "search: a query is required")
	}

	root, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	ctx := context.Background()
	learnings, err := storage.Search(ctx, db, projectID, *query)
	if err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "search: %v", err)
	}

	results := make([]map[string]any, 0, len(learnings))
	for _, l := range learnings {
		if *statusFilter != "" && string(l.Status) != *statusFilter {
			continue
		}
		m := learningToOutputMap(l)
		m["source"] = "royo_learn"
		results = append(results, m)
	}

	// Merge Engram results when explicitly requested and enabled (D9 folds
	// engram-search under `search --include-engram`). Disabled Engram is not an
	// error: local results still stand.
	if *includeEngram {
		if merged := searchEngram(ctx, root, *query); merged != nil {
			results = append(results, merged...)
		}
	}

	if *limit > 0 && *limit < len(results) {
		results = results[:*limit]
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(results, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		for _, r := range results {
			_, _ = fmt.Fprintf(stdout, "[%s] %v: %v\n", r["source"], r["id"], r["title"])
		}
		if len(results) == 0 {
			_, _ = fmt.Fprintf(stdout, "No results.\n")
		}
	}
	return exitSuccess
}

// searchEngram returns Engram results as source-labeled maps, or nil when Engram
// is disabled or unreachable (degraded, never fatal).
func searchEngram(ctx context.Context, root, query string) []map[string]any {
	cfg, err := config.Load(root)
	if err != nil || !cfg.Engram.Enabled {
		return nil
	}
	url := cfg.Engram.BaseURL
	if url == "" {
		url = "http://localhost:8765"
	}
	degraded := engram.NewDegradedClient(engram.NewHTTPClient(url))
	hits, err := degraded.Search(ctx, query)
	if err != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		out = append(out, map[string]any{
			"source": "engram",
			"title":  h.Title,
			"score":  h.Score,
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// list — list learnings with optional filters (docs/04-CLI-SPEC.md:175)
// ---------------------------------------------------------------------------

func runList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	statusFilter := fs.String("status", "", "filter by learning status")
	typeFilter := fs.String("type", "", "filter by learning type")
	scopeFilter := fs.String("scope", "", "filter by scope guess")
	projectFilter := fs.String("project", "", "reserved: filter by project (current project only for now)")
	limit := fs.Int("limit", 50, "max results")
	offset := fs.Int("offset", 0, "results to skip")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "list: %v", err)
	}
	_ = projectFilter // reserved for Hito 2 cross-project listing; declared so the flag is accepted.

	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	filter := domain.LearningFilter{Limit: *limit, Offset: *offset}
	if *statusFilter != "" {
		filter.Status = []domain.LearningStatus{domain.LearningStatus(*statusFilter)}
	}
	if *typeFilter != "" {
		filter.Type = []domain.LearningType{domain.LearningType(*typeFilter)}
	}
	if *scopeFilter != "" {
		filter.Scope = []domain.Scope{domain.Scope(*scopeFilter)}
	}

	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "list: begin tx: %v", err)
	}
	learnings, err := storage.ListLearnings(ctx, tx, projectID, filter)
	tx.Rollback()
	if err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "list: %v", err)
	}

	results := make([]map[string]any, 0, len(learnings))
	for _, l := range learnings {
		results = append(results, learningToOutputMap(l))
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(results, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		for _, l := range learnings {
			_, _ = fmt.Fprintf(stdout, "%s [%s] %s\n", l.ID, l.Status, l.Title)
		}
		if len(learnings) == 0 {
			_, _ = fmt.Fprintf(stdout, "No learnings.\n")
		}
	}
	return exitSuccess
}

// ---------------------------------------------------------------------------
// status — report a learning's lifecycle status (mirrors learning_status)
// ---------------------------------------------------------------------------

func runStatus(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return writeRetrievalError(stderr, "invalid_argument", "status: a learning id is required as the first argument")
	}
	learningID := args[0]

	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")
	if err := fs.Parse(args[1:]); err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "status: %v", err)
	}

	_, db, _, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "status: begin tx: %v", err)
	}
	learning, err := storage.GetLearning(ctx, tx, domain.LearningID(learningID))
	tx.Rollback()
	if err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "status: %v", err)
	}
	if learning == nil {
		return writeRetrievalError(stderr, "learning_not_found", "status: learning %q not found", learningID)
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]any{
			"learning_id": string(learning.ID),
			"status":      string(learning.Status),
			"type":        string(learning.Type),
			"title":       learning.Title,
			"revision":    learning.Revision,
			"updated_at":  learning.UpdatedAt.Format(time.RFC3339),
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "%s: %s (%s, revision %d)\n",
			learning.ID, learning.Status, learning.Type, learning.Revision)
	}
	return exitSuccess
}

// ---------------------------------------------------------------------------
// occurrence — record a recurrence of a learning's pattern (docs/04:261)
// ---------------------------------------------------------------------------

func runOccurrence(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("occurrence", flag.ContinueOnError)
	learningID := fs.String("learning-id", "", "learning whose pattern recurred (required)")
	fingerprint := fs.String("fingerprint", "", "override the derived recurrence fingerprint")
	summary := fs.String("summary", "", "what recurred")
	outcome := fs.String("outcome", "", "outcome of the recurrence")
	retrieved := fs.String("retrieved", "", "whether the learning was retrieved: true|false")
	skillActivated := fs.String("skill-activated", "", "whether the skill was activated: true|false")
	evidence := fs.String("evidence-file", "", "reference to evidence for this occurrence")
	idempotencyKey := fs.String("idempotency-key", "", "the same key on a retry does not create a second record (D5)")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "occurrence: %v", err)
	}
	if *learningID == "" {
		return writeRetrievalError(stderr, "invalid_argument", "occurrence: --learning-id is required")
	}
	retrievedBool, err := parseTriState(*retrieved)
	if err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "occurrence: --retrieved %v", err)
	}
	skillBool, err := parseTriState(*skillActivated)
	if err != nil {
		return writeRetrievalError(stderr, "invalid_argument", "occurrence: --skill-activated %v", err)
	}

	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	ctx := context.Background()
	tx, txErr := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if txErr != nil {
		return writeRetrievalError(stderr, "invalid_argument", "occurrence: begin tx: %v", txErr)
	}
	learning, getErr := storage.GetLearning(ctx, tx, domain.LearningID(*learningID))
	tx.Rollback()
	if getErr != nil {
		return writeRetrievalError(stderr, "invalid_argument", "occurrence: %v", getErr)
	}
	if learning == nil {
		return writeRetrievalError(stderr, "learning_not_found", "occurrence: learning %q not found", *learningID)
	}

	rec, isNew, recErr := recurrence.RecordOccurrence(ctx, db, projectID, learning, recurrence.OccurrenceInput{
		Summary:        *summary,
		Fingerprint:    *fingerprint,
		Outcome:        *outcome,
		Retrieved:      retrievedBool,
		SkillActivated: skillBool,
		Evidence:       *evidence,
		Actor:          domain.Actor{Kind: "human", Name: "cli-user"},
		IdempotencyKey: *idempotencyKey,
	})
	if recErr != nil {
		return writeRetrievalError(stderr, "invalid_argument", "occurrence: %v", recErr)
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]any{
			"recurrence_id":   string(rec.ID),
			"learning_id":     string(rec.LearningID),
			"fingerprint":     rec.RecurrenceFingerprint,
			"occurred_at":     rec.OccurredAt.Format(time.RFC3339),
			"outcome":         rec.Outcome,
			"retrieved":       rec.Retrieved,
			"skill_activated": rec.SkillActivated,
			"new":             isNew,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		verb := "recorded"
		if !isNew {
			verb = "deduplicated (existing)"
		}
		_, _ = fmt.Fprintf(stdout, "%s occurrence %s for learning %s\n", verb, rec.ID, rec.LearningID)
	}
	return exitSuccess
}

// parseTriState parses "", "true" or "false" into a bool. Empty means false.
func parseTriState(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "false", "0", "no":
		return false, nil
	case "true", "1", "yes":
		return true, nil
	default:
		return false, fmt.Errorf("must be true or false, got %q", v)
	}
}

// learningToOutputMap renders a learning for CLI JSON output. It mirrors the MCP
// server's projection so both interfaces present the same shape.
func learningToOutputMap(l *domain.Learning) map[string]any {
	return map[string]any{
		"id":              string(l.ID),
		"project_id":      string(l.ProjectID),
		"status":          string(l.Status),
		"type":            string(l.Type),
		"title":           l.Title,
		"context":         l.Context,
		"observation":     l.Observation,
		"reusable_lesson": l.ReusableLesson,
		"scope_guess":     string(l.ScopeGuess),
		"confidence":      string(l.Confidence),
		"evidence_level":  string(l.EvidenceLevel),
		"created_at":      l.CreatedAt.Format(time.RFC3339),
		"updated_at":      l.UpdatedAt.Format(time.RFC3339),
	}
}

func writeRetrievalError(stderr io.Writer, code, format string, args ...interface{}) int {
	msg := fmt.Sprintf(format, args...)
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        code,
		Message:     msg,
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  `run "royo-learn get <id>", "royo-learn search <query>" or "royo-learn occurrence --learning-id <id>"`,
	})
	return domain.ErrorCode(code).ExitCode()
}
