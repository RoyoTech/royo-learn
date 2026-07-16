package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"agent-royo-learn/internal/coherence"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/logging"
	"agent-royo-learn/internal/portability"
	"agent-royo-learn/internal/record"
	"agent-royo-learn/internal/storage"
)

// ---------------------------------------------------------------------------
// export — versioned, portable snapshot of the store (docs/04-CLI-SPEC.md:356)
// ---------------------------------------------------------------------------

func runExport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	format := fs.String("format", "jsonl", "export format: jsonl|markdown")
	output := fs.String("output", "", "output file (jsonl) or directory (markdown); default stdout for jsonl")
	projectFilter := fs.String("project", "", "reserved: current project only for now")
	projectRoot := fs.String("project-root", "", "project root directory")
	if err := fs.Parse(args); err != nil {
		return writePortabilityError(stderr, "invalid_argument", "export: %v", err)
	}
	_ = projectFilter

	if *format != "jsonl" && *format != "markdown" {
		return writePortabilityError(stderr, "invalid_argument", "export: --format must be jsonl or markdown, got %q", *format)
	}

	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	ctx := context.Background()
	bundle, err := portability.Export(ctx, db, projectID)
	if err != nil {
		return writePortabilityError(stderr, "invalid_argument", "export: %v", err)
	}

	if *format == "markdown" {
		dir := *output
		if dir == "" {
			return writePortabilityError(stderr, "invalid_argument", "export: markdown format requires --output <dir>")
		}
		for _, l := range bundle.Learnings {
			if werr := record.WriteRecord(dir, l); werr != nil {
				return writePortabilityError(stderr, "invalid_argument", "export: write record %s: %v", l.ID, werr)
			}
		}
		_, _ = fmt.Fprintf(stdout, "exported %d record(s) to %s\n", len(bundle.Learnings), dir)
		return exitSuccess
	}

	// jsonl.
	var w io.Writer = stdout
	if *output != "" {
		f, ferr := os.Create(*output)
		if ferr != nil {
			return writePortabilityError(stderr, "invalid_argument", "export: create %s: %v", *output, ferr)
		}
		defer f.Close()
		w = f
	}
	if err := portability.EncodeJSONL(bundle, w); err != nil {
		return writePortabilityError(stderr, "invalid_argument", "export: encode: %v", err)
	}
	if *output != "" {
		_, _ = fmt.Fprintf(stdout, "exported %d learning(s) to %s\n", len(bundle.Learnings), *output)
	}
	return exitSuccess
}

// ---------------------------------------------------------------------------
// import — validate and apply a bundle (docs/04-CLI-SPEC.md:364)
// ---------------------------------------------------------------------------

func runImport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	file := fs.String("file", "", "bundle file to import (required)")
	dryRun := fs.Bool("dry-run", true, "validate and report the plan without writing (default)")
	apply := fs.Bool("apply", false, "actually write; equivalent to --dry-run=false")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")
	if err := fs.Parse(args); err != nil {
		return writePortabilityError(stderr, "invalid_argument", "import: %v", err)
	}
	if *file == "" {
		return writePortabilityError(stderr, "invalid_argument", "import: --file is required")
	}
	// --apply overrides the dry-run default; writing requires an explicit apply.
	writeIntended := *apply || !*dryRun

	f, ferr := os.Open(*file)
	if ferr != nil {
		return writePortabilityError(stderr, "invalid_argument", "import: open %s: %v", *file, ferr)
	}
	defer f.Close()
	bundle, derr := portability.DecodeJSONL(f)
	if derr != nil {
		return writePortabilityError(stderr, "invalid_argument", "import: decode %s: %v", *file, derr)
	}

	root, db, _, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	ctx := context.Background()
	res, err := portability.Import(ctx, db, bundle, portability.ImportOptions{
		RecordsDir: filepath.Join(root, ".royo-learn", "records"),
		BackupDir:  filepath.Join(root, ".royo-learn", "backups"),
		Apply:      writeIntended,
	})
	if err != nil {
		// A conflict is a real, reportable outcome, not a crash: emit the plan too.
		if res != nil && *jsonFlag {
			data, _ := json.MarshalIndent(importResultToMap(res), "", "  ")
			_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
		}
		return writePortabilityError(stderr, "publication_conflict", "import: %v", err)
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(importResultToMap(res), "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		mode := "would insert"
		if !res.DryRun {
			mode = "inserted"
		}
		_, _ = fmt.Fprintf(stdout, "%s %d learning(s), %d evidence, %d relation(s), %d recurrence(s); skipped %d; conflicts %d\n",
			mode, res.LearningsInserted, res.EvidenceInserted, res.RelationsInserted,
			res.RecurrencesInserted, res.LearningsSkipped, len(res.Conflicts))
		if res.DryRun && writeIntended {
			// Unreachable: writeIntended forces Apply. Kept defensively.
		}
		if res.DryRun {
			_, _ = fmt.Fprintf(stdout, "dry run: nothing was written. Re-run with --apply to write.\n")
		}
	}
	return exitSuccess
}

func importResultToMap(res *portability.ImportResult) map[string]any {
	conflicts := make([]map[string]any, 0, len(res.Conflicts))
	for _, c := range res.Conflicts {
		conflicts = append(conflicts, map[string]any{"kind": c.Kind, "id": c.ID, "reason": c.Reason})
	}
	return map[string]any{
		"dry_run":              res.DryRun,
		"backup_path":          res.BackupPath,
		"learnings_inserted":   res.LearningsInserted,
		"learnings_skipped":    res.LearningsSkipped,
		"evidence_inserted":    res.EvidenceInserted,
		"relations_inserted":   res.RelationsInserted,
		"recurrences_inserted": res.RecurrencesInserted,
		"conflicts":            conflicts,
	}
}

// ---------------------------------------------------------------------------
// rebuild-index — repair SQLite<->Markdown divergences (docs/04-CLI-SPEC.md:372)
// ---------------------------------------------------------------------------

func runRebuildIndex(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rebuild-index", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")
	if err := fs.Parse(args); err != nil {
		return writePortabilityError(stderr, "invalid_argument", "rebuild-index: %v", err)
	}

	root, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	ctx := context.Background()
	recordsDir := filepath.Join(root, ".royo-learn", "records")
	res, err := coherence.Repair(ctx, db, projectID, recordsDir)
	if err != nil {
		return writePortabilityError(stderr, "invalid_argument", "rebuild-index: %v", err)
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]any{
			"fts_rows":        res.FTSRows,
			"records_written": res.RecordsWritten,
			"orphans_removed": res.OrphansRemoved,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "rebuilt index: %d FTS row(s), %d record(s) re-materialized, %d orphan(s) removed\n",
			res.FTSRows, res.RecordsWritten, res.OrphansRemoved)
	}
	return exitSuccess
}

// ---------------------------------------------------------------------------
// review — the operator's inbox (docs/04-CLI-SPEC.md:346)
// ---------------------------------------------------------------------------

func runReview(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	limit := fs.Int("limit", 100, "max items per bucket")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")
	if err := fs.Parse(args); err != nil {
		return writePortabilityError(stderr, "invalid_argument", "review: %v", err)
	}

	_, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return writePortabilityError(stderr, "invalid_argument", "review: begin tx: %v", err)
	}
	defer tx.Rollback()

	candidates, err := storage.ListLearnings(ctx, tx, projectID, domain.LearningFilter{
		Status: []domain.LearningStatus{domain.StatusCaptured}, Limit: *limit,
	})
	if err != nil {
		return writePortabilityError(stderr, "invalid_argument", "review: candidates: %v", err)
	}
	needsEvidence, err := storage.ListLearnings(ctx, tx, projectID, domain.LearningFilter{
		Status: []domain.LearningStatus{domain.StatusNeedsEvidence}, Limit: *limit,
	})
	if err != nil {
		return writePortabilityError(stderr, "invalid_argument", "review: needs_evidence: %v", err)
	}
	approved, err := storage.ListLearnings(ctx, tx, projectID, domain.LearningFilter{
		Status: []domain.LearningStatus{domain.StatusApproved}, Limit: *limit,
	})
	if err != nil {
		return writePortabilityError(stderr, "invalid_argument", "review: approved: %v", err)
	}
	// Approved-not-published: approved learnings without a successful publication.
	approvedNotPublished := make([]*domain.Learning, 0, len(approved))
	for _, l := range approved {
		published, perr := hasSuccessfulPublication(ctx, tx, l.ID)
		if perr != nil {
			return writePortabilityError(stderr, "invalid_argument", "review: publications: %v", perr)
		}
		if !published {
			approvedNotPublished = append(approvedNotPublished, l)
		}
	}
	recurrences, err := storage.ListAllRecurrences(ctx, tx, projectID, *limit)
	if err != nil {
		return writePortabilityError(stderr, "invalid_argument", "review: recurrences: %v", err)
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]any{
			"candidates":             learningsBrief(candidates),
			"needs_evidence":         learningsBrief(needsEvidence),
			"approved_not_published": learningsBrief(approvedNotPublished),
			"recurrences":            len(recurrences),
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "Review:\n")
		_, _ = fmt.Fprintf(stdout, "  candidates:             %d\n", len(candidates))
		_, _ = fmt.Fprintf(stdout, "  needs_evidence:         %d\n", len(needsEvidence))
		_, _ = fmt.Fprintf(stdout, "  approved_not_published: %d\n", len(approvedNotPublished))
		_, _ = fmt.Fprintf(stdout, "  recurrences:            %d\n", len(recurrences))
	}
	return exitSuccess
}

func hasSuccessfulPublication(ctx context.Context, tx *sql.Tx, learningID domain.LearningID) (bool, error) {
	var one int
	err := tx.QueryRowContext(ctx,
		`SELECT 1 FROM publications WHERE learning_id = ? AND status = 'completed' LIMIT 1`,
		string(learningID)).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func learningsBrief(ls []*domain.Learning) []map[string]any {
	out := make([]map[string]any, 0, len(ls))
	for _, l := range ls {
		out = append(out, map[string]any{
			"id":         string(l.ID),
			"status":     string(l.Status),
			"title":      l.Title,
			"updated_at": l.UpdatedAt.Format(time.RFC3339),
		})
	}
	return out
}

func writePortabilityError(stderr io.Writer, code, format string, args ...interface{}) int {
	msg := fmt.Sprintf(format, args...)
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        code,
		Message:     msg,
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  `run "royo-learn export --help", "royo-learn import --help", "royo-learn rebuild-index --help" or "royo-learn review --help"`,
	})
	return domain.ErrorCode(code).ExitCode()
}
