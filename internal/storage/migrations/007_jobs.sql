-- 007_jobs: job engine tables for incremental work scheduling (Hito 8).
--
-- job_state holds the runtime lease, digest, status, retry counters and
-- last-success anchor for every registered job. SQLite is the sole
-- coordination authority; an optional filesystem .lock is secondary only.
--
-- job_registry is the static registration table that lists every known
-- job name, its default config, and whether it is enabled. It is populated
-- by the registry at startup, never by the runner at runtime.

CREATE TABLE IF NOT EXISTS job_state (
    project_id TEXT NOT NULL,
    job_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (
        status IN ('idle', 'running', 'ok', 'degraded', 'error')
    ),
    input_digest TEXT NOT NULL DEFAULT '',
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_expires_at TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    last_started_at TEXT,
    last_success_at TEXT,
    last_failed_at TEXT,
    last_error_code TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    metrics_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (project_id, job_name)
);

CREATE TABLE IF NOT EXISTS job_registry (
    job_name TEXT PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    default_interval_sec INTEGER NOT NULL DEFAULT 3600,
    default_max_retries INTEGER NOT NULL DEFAULT 3,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);
