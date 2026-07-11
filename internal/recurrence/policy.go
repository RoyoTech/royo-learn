package recurrence

import (
	"context"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
)

const (
	// DefaultStaleThresholdDays is the number of days after which a learning
	// that hasn't recurred is flagged for review.
	DefaultStaleThresholdDays = 90
)

// ReviewStatus indicates whether a learning needs review and why.
type ReviewStatus struct {
	NeedsReview  bool
	Reason       string
	LastRecurred *time.Time
}

// CheckNeedsReview evaluates whether a learning should be flagged for review
// based on its recurrence pattern. A learning needs review if:
// 1. It has recurrence records but the most recent is older than the stale threshold.
// 2. It has no recurrence records at all (not yet classified as "needs review" — only after first capture ages out).
func CheckNeedsReview(ctx context.Context, db *storage.DB, projectID domain.ProjectID, learning *domain.Learning) (*ReviewStatus, error) {
	if learning == nil {
		return &ReviewStatus{NeedsReview: false}, nil
	}

	fp := RecurrenceFingerprint(learning)
	if fp == "" {
		return &ReviewStatus{NeedsReview: false}, nil
	}

	metrics, err := ComputeMetrics(ctx, db, projectID, fp)
	if err != nil {
		return nil, fmt.Errorf("CheckNeedsReview: %w", err)
	}

	status := &ReviewStatus{}

	if metrics.Count == 0 {
		status.NeedsReview = false
		status.Reason = "no recurrence records yet"
		return status, nil
	}

	status.LastRecurred = &metrics.LastSeen

	staleThreshold := time.Now().UTC().AddDate(0, 0, -DefaultStaleThresholdDays)
	if metrics.LastSeen.Before(staleThreshold) {
		status.NeedsReview = true
		daysSince := int(time.Since(metrics.LastSeen).Hours() / 24)
		status.Reason = fmt.Sprintf("last recurred %d days ago, exceeds %d-day threshold",
			daysSince, DefaultStaleThresholdDays)
	} else {
		status.Reason = fmt.Sprintf("last recurred within %d days", DefaultStaleThresholdDays)
	}

	return status, nil
}
