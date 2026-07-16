package portability

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"agent-royo-learn/internal/record"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
)

// ImportOptions controls how a bundle is applied.
type ImportOptions struct {
	// RecordsDir is where Markdown records are re-materialized on a real apply.
	RecordsDir string
	// BackupDir receives a consistent database snapshot taken before any write.
	BackupDir string
	// Apply switches from the default dry run to a real write. Dry run reports the
	// plan and writes nothing.
	Apply bool
}

// Conflict records an incoming record that cannot be applied without overwriting
// divergent local content. Import never resolves a conflict silently.
type Conflict struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// ImportResult reports what an import did, or (in dry run) what it would do.
type ImportResult struct {
	DryRun              bool       `json:"dry_run"`
	BackupPath          string     `json:"backup_path,omitempty"`
	LearningsInserted   int        `json:"learnings_inserted"`
	LearningsSkipped    int        `json:"learnings_skipped"`
	EvidenceInserted    int        `json:"evidence_inserted"`
	RelationsInserted   int        `json:"relations_inserted"`
	RecurrencesInserted int        `json:"recurrences_inserted"`
	Conflicts           []Conflict `json:"conflicts,omitempty"`
}

// Import validates and applies a bundle to db. By default it is a dry run: it
// classifies every record and reports the plan without writing. With Apply set,
// and only when there are no conflicts, it backs up the database, inserts the
// missing records in one transaction, and re-materializes their Markdown records
// (D6). A conflict — an existing learning with different content — is never
// overwritten; apply fails and the store is left untouched.
func Import(ctx context.Context, db *storage.DB, b *Bundle, opts ImportOptions) (*ImportResult, error) {
	if db == nil || db.DB == nil {
		return nil, fmt.Errorf("portability: import: nil database")
	}
	if b == nil {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "import: nil bundle")
	}
	if b.FormatVersion != BundleFormatVersion {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			fmt.Sprintf("import: unsupported format version %d (this build understands %d)",
				b.FormatVersion, BundleFormatVersion))
	}
	if b.Project == nil {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "import: bundle has no project")
	}

	res := &ImportResult{DryRun: !opts.Apply}

	// ---- Read pass: classify every record without mutating anything. --------
	plan, err := classify(ctx, db, b)
	if err != nil {
		return nil, err
	}
	res.LearningsInserted = len(plan.learnings)
	res.LearningsSkipped = plan.learningsSkipped
	res.EvidenceInserted = len(plan.evidence)
	res.RelationsInserted = len(plan.relations)
	res.RecurrencesInserted = len(plan.recurrences)
	res.Conflicts = plan.conflicts

	// A conflict blocks any write. In dry run we still surface it.
	if len(plan.conflicts) > 0 {
		if opts.Apply {
			return res, domain.NewConflictError(domain.ErrPublicationConflict,
				fmt.Sprintf("import: %d conflicting record(s); refusing to overwrite", len(plan.conflicts)))
		}
		return res, nil
	}

	if !opts.Apply {
		return res, nil // dry run: reported the plan, wrote nothing.
	}

	// ---- Backup before any write. ------------------------------------------
	backupPath, err := backupDatabase(ctx, db, opts.BackupDir)
	if err != nil {
		return res, fmt.Errorf("import: backup: %w", err)
	}
	res.BackupPath = backupPath

	// ---- Write pass: one transaction. --------------------------------------
	if err := storage.WithTx(ctx, db, func(tx *sql.Tx) error {
		if plan.insertProject {
			if err := storage.SaveProject(ctx, tx, b.Project); err != nil {
				return fmt.Errorf("save project: %w", err)
			}
		}
		for _, l := range plan.learnings {
			if err := storage.SaveLearning(ctx, tx, l); err != nil {
				return fmt.Errorf("save learning %s: %w", l.ID, err)
			}
		}
		for _, e := range plan.evidence {
			if err := storage.SaveEvidence(ctx, tx, e); err != nil {
				return fmt.Errorf("save evidence %s: %w", e.ID, err)
			}
		}
		for _, r := range plan.relations {
			if err := storage.SaveRelation(ctx, tx, r); err != nil {
				return fmt.Errorf("save relation %s: %w", r.ID, err)
			}
		}
		for _, r := range plan.recurrences {
			if err := storage.SaveRecurrenceRecord(ctx, tx, r); err != nil {
				return fmt.Errorf("save recurrence %s: %w", r.ID, err)
			}
		}
		return nil
	}); err != nil {
		return res, fmt.Errorf("import: apply: %w", err)
	}

	// ---- Re-materialize the derived Markdown records (D6). ------------------
	if opts.RecordsDir != "" {
		for _, l := range plan.learnings {
			if err := record.WriteRecord(opts.RecordsDir, l); err != nil {
				return res, fmt.Errorf("import: materialize record %s: %w", l.ID, err)
			}
		}
	}

	return res, nil
}

// importPlan is the outcome of the read pass: exactly what would be written.
type importPlan struct {
	insertProject    bool
	learnings        []*domain.Learning
	learningsSkipped int
	evidence         []*domain.Evidence
	relations        []*domain.LearningRelation
	recurrences      []*domain.RecurrenceRecord
	conflicts        []Conflict
}

func classify(ctx context.Context, db *storage.DB, b *Bundle) (*importPlan, error) {
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("import: begin read tx: %w", err)
	}
	defer tx.Rollback()

	plan := &importPlan{}

	if b.Project != nil {
		exists, perr := storage.ProjectExists(ctx, tx, b.Project.ID)
		if perr != nil {
			return nil, fmt.Errorf("import: check project: %w", perr)
		}
		plan.insertProject = !exists
	}

	for _, l := range b.Learnings {
		hash, found, gerr := storage.LearningHash(ctx, tx, l.ID)
		if gerr != nil {
			return nil, fmt.Errorf("import: check learning %s: %w", l.ID, gerr)
		}
		switch {
		case !found:
			plan.learnings = append(plan.learnings, l)
		case hash == l.NormalizedHash:
			plan.learningsSkipped++ // identical: idempotent re-import.
		default:
			plan.conflicts = append(plan.conflicts, Conflict{
				Kind:   "learning",
				ID:     string(l.ID),
				Reason: "a learning with this id already exists with different content",
			})
		}
	}

	for _, e := range b.Evidence {
		exists, gerr := storage.EvidenceExists(ctx, tx, e.ID)
		if gerr != nil {
			return nil, fmt.Errorf("import: check evidence %s: %w", e.ID, gerr)
		}
		if !exists {
			plan.evidence = append(plan.evidence, e)
		}
	}

	for _, r := range b.Relations {
		exists, gerr := storage.RelationExists(ctx, tx, r.ID)
		if gerr != nil {
			return nil, fmt.Errorf("import: check relation %s: %w", r.ID, gerr)
		}
		if !exists {
			plan.relations = append(plan.relations, r)
		}
	}

	for _, r := range b.Recurrences {
		exists, gerr := storage.RecurrenceExists(ctx, tx, r.ID)
		if gerr != nil {
			return nil, fmt.Errorf("import: check recurrence %s: %w", r.ID, gerr)
		}
		if !exists {
			plan.recurrences = append(plan.recurrences, r)
		}
	}

	return plan, nil
}

// backupDatabase writes a consistent snapshot of the live database into dir
// using SQLite's VACUUM INTO, which is safe against an open WAL connection.
func backupDatabase(ctx context.Context, db *storage.DB, dir string) (string, error) {
	if dir == "" {
		return "", nil // no backup requested
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir backup dir: %w", err)
	}
	name := fmt.Sprintf("pre-import-%s.db", time.Now().UTC().Format("20060102T150405Z"))
	path := filepath.Join(dir, name)
	// VACUUM INTO refuses to overwrite an existing file; make the name unique.
	if _, err := os.Stat(path); err == nil {
		name = fmt.Sprintf("pre-import-%d.db", time.Now().UTC().UnixNano())
		path = filepath.Join(dir, name)
	}
	if _, err := db.DB.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return "", fmt.Errorf("vacuum into %q: %w", path, err)
	}
	return path, nil
}
