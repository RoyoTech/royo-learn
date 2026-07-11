package recurrence

import (
	"context"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
)

// ComputeMetrics calculates recurrence metrics for a given fingerprint
// in the specified project.
func ComputeMetrics(ctx context.Context, db *storage.DB, projectID domain.ProjectID, fingerprint string) (*domain.RecurrenceMetrics, error) {
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("ComputeMetrics: begin tx: %w", err)
	}
	defer tx.Rollback()

	records, err := storage.ListRecurrenceRecords(ctx, tx, projectID, fingerprint, 1000)
	if err != nil {
		return nil, fmt.Errorf("ComputeMetrics: list records: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("ComputeMetrics: commit: %w", err)
	}

	m := &domain.RecurrenceMetrics{
		Fingerprint: fingerprint,
		Count:       len(records),
		Trend:       domain.TrendFirst,
	}

	if m.Count == 0 {
		return m, nil
	}

	// First and last seen.
	m.FirstSeen = records[len(records)-1].OccurredAt
	m.LastSeen = records[0].OccurredAt

	// Average interval.
	if m.Count >= 2 {
		totalSpan := m.LastSeen.Sub(m.FirstSeen)
		m.AvgInterval = totalSpan / time.Duration(m.Count-1)
	}

	// Trend.
	m.Trend = computeTrend(records)

	return m, nil
}

// computeTrend determines the recurrence trend from ordered records
// (newest first).
func computeTrend(records []*domain.RecurrenceRecord) domain.RecurrenceTrend {
	if len(records) <= 1 {
		return domain.TrendFirst
	}

	// Compare interval between first two (most recent) vs last two (oldest).
	// If recent intervals are shorter → increasing.
	if len(records) >= 4 {
		recentInterval := records[0].OccurredAt.Sub(records[1].OccurredAt)
		oldInterval := records[len(records)-1].OccurredAt.Sub(records[len(records)-2].OccurredAt)

		if recentInterval < oldInterval/2 {
			return domain.TrendIncreasing
		}
		if recentInterval > oldInterval*2 {
			return domain.TrendDecreasing
		}
		return domain.TrendStable
	}

	return domain.TrendSporadic
}
