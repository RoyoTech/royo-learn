package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// SaveOccurrence inserts a new occurrence record.
func SaveOccurrence(ctx context.Context, tx *sql.Tx, o *domain.Occurrence) error {
	learningID := (*string)(nil)
	if o.LearningID != nil {
		s := string(*o.LearningID)
		learningID = &s
	}
	evidenceJSON := marshalAny(o.Evidence)

	_, err := tx.ExecContext(ctx, `
		INSERT INTO occurrences (id, learning_id, project_id, fingerprint, summary, evidence_json, learning_was_retrieved, skill_was_activated, outcome, occurred_at, actor_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		string(o.ID),
		learningID,
		string(o.ProjectID),
		o.Fingerprint,
		o.Summary,
		evidenceJSON,
		o.LearningWasRetrieved,
		o.SkillWasActivated,
		string(o.Outcome),
		o.OccurredAt.Format(time.RFC3339),
		o.Actor.ActorJSON(),
	)
	if err != nil {
		return fmt.Errorf("SaveOccurrence: %w", err)
	}
	return nil
}

// ListOccurrences returns occurrences for a project, ordered by occurred_at DESC.
func ListOccurrences(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, limit int) ([]*domain.Occurrence, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, learning_id, project_id, fingerprint, summary, evidence_json, learning_was_retrieved, skill_was_activated, outcome, occurred_at, actor_json
		FROM occurrences
		WHERE project_id = ?
		ORDER BY occurred_at DESC
		LIMIT ?
	`, string(projectID), limit)
	if err != nil {
		return nil, fmt.Errorf("ListOccurrences: %w", err)
	}
	defer rows.Close()

	var out []*domain.Occurrence
	for rows.Next() {
		o := &domain.Occurrence{}
		var occurredAt, actorJSON, evidenceJSON string
		var learningIDScan *string
		if err := rows.Scan(
			(*string)(&o.ID),
			&learningIDScan,
			(*string)(&o.ProjectID),
			&o.Fingerprint,
			&o.Summary,
			&evidenceJSON,
			&o.LearningWasRetrieved,
			&o.SkillWasActivated,
			(*string)(&o.Outcome),
			&occurredAt,
			&actorJSON,
		); err != nil {
			return nil, fmt.Errorf("ListOccurrences scan: %w", err)
		}
		if learningIDScan != nil {
			lid := domain.LearningID(*learningIDScan)
			o.LearningID = &lid
		}
		o.Evidence = unmarshalEvidenceRefs(evidenceJSON)
		o.OccurredAt, _ = time.Parse(time.RFC3339, occurredAt)
		o.Actor = parseActor(actorJSON)
		out = append(out, o)
	}
	return out, rows.Err()
}

// unmarshalEvidenceRefs parses a JSON array of EvidenceRef.
func unmarshalEvidenceRefs(raw string) []domain.EvidenceRef {
	if raw == "" || raw == "[]" {
		return nil
	}
	var out []domain.EvidenceRef
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
