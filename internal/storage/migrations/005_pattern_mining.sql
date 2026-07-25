-- 005_pattern_mining: persistent pattern-mining tables.
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS experience_patterns (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    status TEXT NOT NULL CHECK (status IN ('observed', 'qualified', 'dismissed', 'promoted', 'stale')),
    kind TEXT NOT NULL CHECK (kind IN ('user_correction', 'command_failure', 'test_failure', 'test_success', 'successful_procedure', 'retry_corrected', 'tool_limitation', 'architecture_decision', 'preference', 'unknown')),
    fingerprint TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    distinct_sessions INTEGER NOT NULL DEFAULT 0,
    distinct_days INTEGER NOT NULL DEFAULT 0,
    occurrence_count INTEGER NOT NULL DEFAULT 0,
    first_seen_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    proposed_learning_id TEXT REFERENCES learnings(id),
    detector_version TEXT NOT NULL DEFAULT '',
    input_digest TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    revision INTEGER NOT NULL DEFAULT 1,
    dismissal_reason TEXT NOT NULL DEFAULT '' CHECK (dismissal_reason IN ('', 'one_off', 'not_reusable', 'already_covered', 'contradicted', 'insufficient_evidence', 'private_or_sensitive', 'false_cluster')),
    UNIQUE(project_id, fingerprint)
);

CREATE INDEX IF NOT EXISTS experience_patterns_project_status
    ON experience_patterns(project_id, status);
CREATE INDEX IF NOT EXISTS experience_patterns_kind
    ON experience_patterns(kind);
CREATE INDEX IF NOT EXISTS experience_patterns_last_seen
    ON experience_patterns(project_id, last_seen_at);

CREATE TABLE IF NOT EXISTS experience_pattern_members (
    pattern_id TEXT NOT NULL REFERENCES experience_patterns(id),
    event_id TEXT NOT NULL REFERENCES experience_events(id),
    similarity_kind TEXT NOT NULL CHECK (length(similarity_kind) > 0),
    similarity_score REAL NOT NULL CHECK (similarity_score >= 0 AND similarity_score <= 1),
    added_at TEXT NOT NULL,
    PRIMARY KEY(pattern_id, event_id)
);

CREATE INDEX IF NOT EXISTS experience_pattern_members_pattern
    ON experience_pattern_members(pattern_id);
CREATE INDEX IF NOT EXISTS experience_pattern_members_event
    ON experience_pattern_members(event_id);