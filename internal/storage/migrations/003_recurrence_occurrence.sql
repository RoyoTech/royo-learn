-- Migration 002: occurrence detail for recurrence records.
--
-- Recorrido E / D17 pulls `royo-learn occurrence` and `learning_report_occurrence`
-- into Hito 1. The report records the fields the plan 4.4 enumerates. These
-- columns are additive and nullable/defaulted, so existing rows and readers are
-- unaffected. `idempotency_key` implements the D5 technical-retry guard.

ALTER TABLE recurrence_records ADD COLUMN outcome TEXT NOT NULL DEFAULT '';
ALTER TABLE recurrence_records ADD COLUMN retrieved INTEGER NOT NULL DEFAULT 0;
ALTER TABLE recurrence_records ADD COLUMN skill_activated INTEGER NOT NULL DEFAULT 0;
ALTER TABLE recurrence_records ADD COLUMN evidence TEXT NOT NULL DEFAULT '';
ALTER TABLE recurrence_records ADD COLUMN actor_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE recurrence_records ADD COLUMN actor_name TEXT NOT NULL DEFAULT '';
ALTER TABLE recurrence_records ADD COLUMN idempotency_key TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_recurrence_idempotency
    ON recurrence_records(project_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
