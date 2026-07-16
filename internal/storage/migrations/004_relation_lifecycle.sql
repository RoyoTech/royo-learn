-- Migration 004: propose/confirm lifecycle for learning relations (plan 4.5).
--
-- The agent proposes a relation; curation confirms it. These columns are
-- additive. Existing rows predate the lifecycle, so they are backfilled to
-- `proposed` with their original actor as the proposer and NO confirmer: the
-- migration must never fabricate a confirmation for a relation nobody confirmed.

ALTER TABLE learning_relations ADD COLUMN status TEXT NOT NULL DEFAULT 'proposed';
ALTER TABLE learning_relations ADD COLUMN proposed_by_json TEXT NOT NULL DEFAULT '';
ALTER TABLE learning_relations ADD COLUMN confirmed_by_json TEXT;
ALTER TABLE learning_relations ADD COLUMN confirmed_at TEXT;

UPDATE learning_relations
SET proposed_by_json = actor_json
WHERE proposed_by_json = '';
