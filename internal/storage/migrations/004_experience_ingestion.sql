-- 004_experience_ingestion: persists bounded, already-safe experience data.
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS experience_sessions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    source TEXT NOT NULL,
    external_session_id TEXT NOT NULL,
    locator_json TEXT NOT NULL,
    started_at TEXT,
    updated_at TEXT NOT NULL,
    closed_at TEXT,
    metadata_sha256 TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(project_id, source, external_session_id)
);

CREATE TABLE IF NOT EXISTS experience_turns (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES experience_sessions(id),
    external_turn_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    status TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    user_digest TEXT NOT NULL,
    assistant_digest TEXT NOT NULL,
    tool_calls_digest TEXT NOT NULL,
    safe_summary TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    stable_at TEXT,
    ingested_at TEXT NOT NULL,
    source_revision TEXT NOT NULL,
    redacted INTEGER NOT NULL,
    UNIQUE(session_id, external_turn_id)
);

CREATE INDEX IF NOT EXISTS experience_turns_session_sequence
    ON experience_turns(session_id, sequence);
CREATE INDEX IF NOT EXISTS experience_turns_fingerprint
    ON experience_turns(fingerprint);

CREATE TABLE IF NOT EXISTS experience_events (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    turn_id TEXT NOT NULL REFERENCES experience_turns(id),
    kind TEXT NOT NULL,
    summary TEXT NOT NULL,
    observation TEXT NOT NULL,
    outcome TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    evidence_json TEXT NOT NULL,
    detector_json TEXT NOT NULL,
    confidence TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS experience_events_project_kind
    ON experience_events(project_id, kind);
CREATE INDEX IF NOT EXISTS experience_events_fingerprint
    ON experience_events(fingerprint);
CREATE INDEX IF NOT EXISTS experience_events_turn
    ON experience_events(turn_id);

CREATE TABLE IF NOT EXISTS ingestion_cursors (
    project_id TEXT NOT NULL REFERENCES projects(id),
    source TEXT NOT NULL,
    source_instance TEXT NOT NULL,
    cursor_json TEXT NOT NULL,
    last_successful_at TEXT,
    last_attempt_at TEXT,
    last_error_code TEXT NOT NULL,
    last_error_message TEXT NOT NULL,
    input_digest TEXT NOT NULL,
    revision INTEGER NOT NULL,
    source_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(project_id, source, source_instance)
);
