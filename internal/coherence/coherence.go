// Package coherence reconciles the two representations of a learning store:
// SQLite (the operational source of truth, D6) and the derived Markdown records
// on disk. `doctor` DETECTS divergences (Audit); `rebuild-index` REPAIRS them
// (Repair). It never declares the two transactionally equivalent — it makes the
// derived side match the truth and reports what it had to change.
package coherence

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-royo-learn/internal/capture"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
)

// RepairResult reports what a repair did.
type RepairResult struct {
	FTSRows        int64 `json:"fts_rows"`
	RecordsWritten int   `json:"records_written"`
	OrphansRemoved int   `json:"orphans_removed"`
}

// Repair makes the derived representation match SQLite. It rebuilds the FTS
// index from the canonical tables, re-materializes a Markdown record for every
// learning, and removes orphan record files whose learning no longer exists.
// SQLite is the source of truth, so repair always flows truth -> derived.
func Repair(ctx context.Context, db *storage.DB, projectID domain.ProjectID, recordsDir string) (*RepairResult, error) {
	if db == nil || db.DB == nil {
		return nil, fmt.Errorf("coherence: repair: nil database")
	}
	res := &RepairResult{}

	// 1. Rebuild the search index from the canonical tables (deterministic).
	ftsRows, err := storage.RebuildSearchIndex(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("coherence: repair: rebuild index: %w", err)
	}
	res.FTSRows = ftsRows

	if recordsDir == "" {
		return res, nil
	}

	// 2. Re-materialize a record for every learning.
	learnings, err := loadLearnings(ctx, db, projectID)
	if err != nil {
		return nil, err
	}
	live := make(map[string]bool, len(learnings))
	for _, l := range learnings {
		live[string(l.ID)] = true
		if err := capture.WriteRecord(recordsDir, l); err != nil {
			return nil, fmt.Errorf("coherence: repair: write record %s: %w", l.ID, err)
		}
		res.RecordsWritten++
	}

	// 3. Remove orphan record files (a derived artifact with no source of truth).
	orphans, err := orphanRecords(recordsDir, live)
	if err != nil {
		return nil, err
	}
	for _, path := range orphans {
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("coherence: repair: remove orphan %s: %w", path, err)
		}
		res.OrphansRemoved++
	}

	return res, nil
}

func loadLearnings(ctx context.Context, db *storage.DB, projectID domain.ProjectID) ([]*domain.Learning, error) {
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("coherence: begin tx: %w", err)
	}
	defer tx.Rollback()
	learnings, err := storage.ExportAllLearnings(ctx, tx, projectID)
	if err != nil {
		return nil, fmt.Errorf("coherence: load learnings: %w", err)
	}
	return learnings, nil
}

// orphanRecords returns record files in dir whose learning id is not in live.
func orphanRecords(dir string, live map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("coherence: read records dir: %w", err)
	}
	var orphans []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".md")
		if !live[id] {
			orphans = append(orphans, filepath.Join(dir, e.Name()))
		}
	}
	return orphans, nil
}
