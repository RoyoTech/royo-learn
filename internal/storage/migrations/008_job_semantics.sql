-- 008_job_semantics: semantic taxonomy columns on job_registry + job_run_log
-- audit table for the symmetric Job engine (Hito 11, PR #13).
--
-- The runner at internal/storage/migrate.go is forward-only. There is no
-- reverse migration file: emergency rollback is performed manually by the
-- operator with the SQL recipe documented in
-- openspec/changes/hito11-semantic/tasks.md (Phase 1 "Manual rollback
-- recipe" block). Bumping the schema on the applied DB must always ship
-- as a new migration (009_*) instead of editing 008 — the runner computes
-- a SHA-256 of this file and refuses to apply when the stored checksum
-- disagrees with the current one.

PRAGMA foreign_keys = ON;

-- 1. Add the three taxonomy columns to job_registry. The DEFAULT values
--    satisfy the NOT NULL constraint on every existing row without
--    requiring a backfill UPDATE; the engine re-stamps the values
--    authoritatively at every UpsertRegistryEntry call.
ALTER TABLE job_registry ADD COLUMN intent     TEXT NOT NULL DEFAULT 'ingest';
ALTER TABLE job_registry ADD COLUMN scope      TEXT NOT NULL DEFAULT 'project';
ALTER TABLE job_registry ADD COLUMN risk_class TEXT NOT NULL DEFAULT 'low';

-- 2. job_run_log: one row per engine-driven run. The PK is a deterministic
--    UUIDv7 minted by jobs.Service.RunOne (not by the per-adapter code)
--    so the same run_id is shared across the four lifecycle events in
--    audit_events. The FK to job_registry enforces the "every run is for
--    a registered job" invariant; historical rows survive a future
--    registry rename because the FK has no ON DELETE CASCADE.
CREATE TABLE IF NOT EXISTS job_run_log (
    run_id        TEXT    PRIMARY KEY,
    job_name      TEXT    NOT NULL REFERENCES job_registry(job_name),
    state         TEXT    NOT NULL CHECK(state IN
                  ('pending','running','succeeded','failed','lease_held')),
    started_at    TEXT    NOT NULL,
    finished_at   TEXT,
    error_code    TEXT    NOT NULL DEFAULT '',
    error_message TEXT    NOT NULL DEFAULT '',
    attempt       INTEGER NOT NULL DEFAULT 0
);

-- 3. Indexes for the two read paths: the operator query "what did ingest
--    do over the last 24 h" (job_name, started_at) and the audit hook
--    join "fetch the four events for a run" (run_id).
CREATE INDEX IF NOT EXISTS idx_job_run_log_job_started
    ON job_run_log(job_name, started_at);
CREATE INDEX IF NOT EXISTS idx_job_run_log_run_id
    ON job_run_log(run_id);
