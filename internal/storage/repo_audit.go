package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// RecordEvent inserts an append-only audit event. Audit events are never
// updated or deleted by application code.
func RecordEvent(ctx context.Context, db *sql.DB, evt *domain.AuditEvent) error {
	return recordEvent(ctx, db, evt)
}

// RecordEventTx inserts an audit event in the caller's state transaction.
func RecordEventTx(ctx context.Context, tx *sql.Tx, evt *domain.AuditEvent) error {
	return recordEvent(ctx, tx, evt)
}

type auditExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func recordEvent(ctx context.Context, exec auditExecer, evt *domain.AuditEvent) error {
	prevJSON := "{}"
	if evt.PreviousState != nil {
		prevJSON = *evt.PreviousState
	}
	newJSON := "{}"
	if evt.NewState != nil {
		newJSON = *evt.NewState
	}
	detailsJSON := marshalAny(evt.Details)

	_, err := exec.ExecContext(ctx, `
		INSERT INTO audit_events (id, occurred_at, actor_json, operation, entity_type, entity_id, previous_state, new_state, payload_sha256, result, error_code, details_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		string(evt.ID),
		evt.OccurredAt.Format(time.RFC3339),
		evt.Actor.ActorJSON(),
		evt.Operation,
		evt.EntityType,
		evt.EntityID,
		nilCoalesce(prevJSON),
		nilCoalesce(newJSON),
		evt.PayloadSHA256,
		evt.Result,
		evt.ErrorCode,
		detailsJSON,
	)
	if err != nil {
		return fmt.Errorf("RecordEvent: %w", err)
	}
	return nil
}

// AuditEventFilter provides filtering options for listing audit events.
type AuditEventFilter struct {
	EntityType string
	EntityID   string
	Operation  string
	Limit      int
	Offset     int
}

// ListEvents returns audit events matching the filter.
func ListEvents(ctx context.Context, db *sql.DB, filter AuditEventFilter) ([]*domain.AuditEvent, error) {
	where := "1=1"
	var args []interface{}

	if filter.EntityType != "" {
		where += " AND entity_type = ?"
		args = append(args, filter.EntityType)
	}
	if filter.EntityID != "" {
		where += " AND entity_id = ?"
		args = append(args, filter.EntityID)
	}
	if filter.Operation != "" {
		where += " AND operation = ?"
		args = append(args, filter.Operation)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	query := fmt.Sprintf(`
		SELECT sequence, id, occurred_at, actor_json, operation, entity_type, entity_id, previous_state, new_state, payload_sha256, result, error_code, details_json
		FROM audit_events
		WHERE %s
		ORDER BY sequence DESC
		LIMIT ? OFFSET ?
	`, where)

	args = append(args, limit, filter.Offset)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListEvents: %w", err)
	}
	defer rows.Close()

	var out []*domain.AuditEvent
	for rows.Next() {
		evt := &domain.AuditEvent{}
		var occurredAt, actorJSON, detailsJSON string
		if err := rows.Scan(
			&evt.Sequence,
			(*string)(&evt.ID),
			&occurredAt,
			&actorJSON,
			&evt.Operation,
			&evt.EntityType,
			&evt.EntityID,
			&evt.PreviousState,
			&evt.NewState,
			&evt.PayloadSHA256,
			&evt.Result,
			&evt.ErrorCode,
			&detailsJSON,
		); err != nil {
			return nil, fmt.Errorf("ListEvents scan: %w", err)
		}
		evt.OccurredAt, _ = time.Parse(time.RFC3339, occurredAt)
		evt.Actor = parseActor(actorJSON)
		if detailsJSON != "" && detailsJSON != "{}" {
			evt.Details = make(map[string]any)
			_, _ = fmt.Sscanf(detailsJSON, "%v", &evt.Details) // best-effort
		}
		out = append(out, evt)
	}
	return out, rows.Err()
}

// nilCoalesce returns the value or "{}" if it's empty.
func nilCoalesce(v string) string {
	if v == "" {
		return "{}"
	}
	return v
}
