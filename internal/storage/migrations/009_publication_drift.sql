-- 009_publication_drift: per-(publication, target) drift state captured by
-- the publication_drift_check job (Hito 12, PR #1).
--
-- The runner at internal/storage/migrate.go is forward-only. There is no
-- reverse migration file: emergency rollback is performed manually by the
-- operator with the SQL recipe documented in
-- openspec/changes/hito12-drift-release/tasks.md (Phase 1 "Manual rollback
-- recipe" block). Bumping the schema on the applied DB must always ship as
-- a new migration (010_*) instead of editing 009 — the runner computes a
-- SHA-256 of this file and refuses to apply when the stored checksum
-- disagrees with the current one.
--
-- The CHECK constraints enforce the four-value outcome enum (status) and
-- the three-value source enum (opencode / claudecode / codex). Both
-- constraints fail the insert at write time so a stale or future value
-- cannot silently land in publication_drift_state.

PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS publication_drift_state (
    publication_id TEXT NOT NULL REFERENCES publications(id),
    source         TEXT NOT NULL CHECK(source IN ('opencode','claudecode','codex')),
    target_path    TEXT NOT NULL,
    expected_hash  TEXT NOT NULL,
    actual_hash    TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL CHECK(status IN
                    ('ok','drifted','target_missing','target_unreadable')),
    checked_at     TEXT NOT NULL,
    run_id         TEXT NOT NULL,
    PRIMARY KEY (publication_id, target_path)
);

CREATE INDEX IF NOT EXISTS idx_drift_status_checked
    ON publication_drift_state(status, checked_at);
CREATE INDEX IF NOT EXISTS idx_drift_run_id
    ON publication_drift_state(run_id);
CREATE INDEX IF NOT EXISTS idx_drift_publication
    ON publication_drift_state(publication_id);