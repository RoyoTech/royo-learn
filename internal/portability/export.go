package portability

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
)

// Export reads a complete, portable snapshot of one project's operational truth
// from SQLite (D6). It reads everything in a single read-only transaction so the
// snapshot is internally consistent.
func Export(ctx context.Context, db *storage.DB, projectID domain.ProjectID) (*Bundle, error) {
	if db == nil || db.DB == nil {
		return nil, fmt.Errorf("portability: export: nil database")
	}
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("portability: export: begin tx: %w", err)
	}
	defer tx.Rollback()

	project, err := storage.GetProject(ctx, tx, projectID)
	if err != nil {
		return nil, fmt.Errorf("portability: export: get project: %w", err)
	}
	if project == nil {
		return nil, domain.NewNotFoundError(domain.ErrProjectNotFound, "project: "+string(projectID))
	}

	learnings, err := storage.ExportAllLearnings(ctx, tx, projectID)
	if err != nil {
		return nil, fmt.Errorf("portability: export: learnings: %w", err)
	}

	var evidence []*domain.Evidence
	var relations []*domain.LearningRelation
	for _, l := range learnings {
		evs, evErr := storage.ListEvidenceByLearning(ctx, tx, l.ID)
		if evErr != nil {
			return nil, fmt.Errorf("portability: export: evidence for %s: %w", l.ID, evErr)
		}
		evidence = append(evidence, evs...)

		rels, relErr := storage.ListRelationsBySource(ctx, tx, l.ID)
		if relErr != nil {
			return nil, fmt.Errorf("portability: export: relations for %s: %w", l.ID, relErr)
		}
		relations = append(relations, rels...)
	}

	recurrences, err := storage.ListAllRecurrences(ctx, tx, projectID, 1_000_000)
	if err != nil {
		return nil, fmt.Errorf("portability: export: recurrences: %w", err)
	}

	return &Bundle{
		FormatVersion: BundleFormatVersion,
		ExportedAt:    time.Now().UTC(),
		ProjectKey:    project.ProjectKey,
		Project:       project,
		Learnings:     learnings,
		Evidence:      evidence,
		Relations:     relations,
		Recurrences:   recurrences,
	}, nil
}
