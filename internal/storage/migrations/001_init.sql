PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    checksum TEXT NOT NULL,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    project_key TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    canonical_path TEXT NOT NULL,
    git_remote TEXT NOT NULL DEFAULT '',
    fingerprint TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS learnings (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    status TEXT NOT NULL,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    context TEXT NOT NULL,
    observation TEXT NOT NULL,
    reusable_lesson TEXT NOT NULL,
    recommended_procedure_json TEXT NOT NULL DEFAULT '[]',
    limits_text TEXT NOT NULL DEFAULT '',
    scope_guess TEXT NOT NULL,
    approved_scope TEXT,
    confidence TEXT NOT NULL,
    evidence_level TEXT NOT NULL,
    proposed_destination TEXT NOT NULL DEFAULT 'none',
    approved_destination_json TEXT,
    retrieval_terms_text TEXT NOT NULL DEFAULT '',
    fingerprint TEXT NOT NULL,
    normalized_hash TEXT NOT NULL,
    idempotency_key TEXT,
    actor_json TEXT NOT NULL,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(project_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_learnings_project_status
ON learnings(project_id, status);

CREATE INDEX IF NOT EXISTS idx_learnings_normalized_hash
ON learnings(project_id, normalized_hash);

CREATE INDEX IF NOT EXISTS idx_learnings_fingerprint
ON learnings(fingerprint);

CREATE TABLE IF NOT EXISTS learning_revisions (
    id TEXT PRIMARY KEY,
    learning_id TEXT NOT NULL REFERENCES learnings(id),
    revision INTEGER NOT NULL,
    payload_json TEXT NOT NULL,
    payload_sha256 TEXT NOT NULL,
    created_at TEXT NOT NULL,
    created_by_json TEXT NOT NULL,
    UNIQUE(learning_id, revision)
);

CREATE TABLE IF NOT EXISTS evidence (
    id TEXT PRIMARY KEY,
    learning_id TEXT NOT NULL REFERENCES learnings(id),
    kind TEXT NOT NULL,
    uri TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL,
    sha256 TEXT NOT NULL DEFAULT '',
    command_json TEXT,
    exit_code INTEGER,
    redacted INTEGER NOT NULL DEFAULT 0,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    collected_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_evidence_learning ON evidence(learning_id);

CREATE TABLE IF NOT EXISTS learning_relations (
    id TEXT PRIMARY KEY,
    source_learning_id TEXT NOT NULL REFERENCES learnings(id),
    target_learning_id TEXT NOT NULL REFERENCES learnings(id),
    relation TEXT NOT NULL,
    confidence REAL,
    rationale TEXT NOT NULL DEFAULT '',
    actor_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(source_learning_id, target_learning_id, relation),
    CHECK(source_learning_id <> target_learning_id)
);

CREATE TABLE IF NOT EXISTS curations (
    id TEXT PRIMARY KEY,
    learning_id TEXT NOT NULL REFERENCES learnings(id),
    decision TEXT NOT NULL,
    rationale TEXT NOT NULL,
    destination_json TEXT,
    validation_json TEXT NOT NULL DEFAULT '[]',
    acceptance_checks_json TEXT NOT NULL DEFAULT '[]',
    rollback_condition TEXT NOT NULL,
    actor_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS publication_previews (
    id TEXT PRIMARY KEY,
    learning_id TEXT NOT NULL REFERENCES learnings(id),
    plan_json TEXT NOT NULL,
    preview_hash TEXT NOT NULL UNIQUE,
    risk TEXT NOT NULL,
    requires_approval INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    invalidated_at TEXT
);

CREATE TABLE IF NOT EXISTS approvals (
    id TEXT PRIMARY KEY,
    learning_id TEXT NOT NULL REFERENCES learnings(id),
    preview_hash TEXT NOT NULL,
    approved_by TEXT NOT NULL,
    reason TEXT NOT NULL,
    approval_evidence TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT,
    revoked_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_approvals_preview ON approvals(preview_hash);

CREATE TABLE IF NOT EXISTS publications (
    id TEXT PRIMARY KEY,
    learning_id TEXT NOT NULL REFERENCES learnings(id),
    preview_hash TEXT NOT NULL,
    approval_id TEXT REFERENCES approvals(id),
    targets_json TEXT NOT NULL,
    verification_json TEXT NOT NULL,
    rollback_json TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    error_code TEXT,
    error_message TEXT
);

CREATE TABLE IF NOT EXISTS occurrences (
    id TEXT PRIMARY KEY,
    learning_id TEXT REFERENCES learnings(id),
    project_id TEXT NOT NULL REFERENCES projects(id),
    fingerprint TEXT NOT NULL,
    summary TEXT NOT NULL,
    evidence_json TEXT NOT NULL DEFAULT '[]',
    learning_was_retrieved INTEGER,
    skill_was_activated INTEGER,
    outcome TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    actor_json TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_occurrences_learning ON occurrences(learning_id);
CREATE INDEX IF NOT EXISTS idx_occurrences_fingerprint ON occurrences(fingerprint);

CREATE TABLE IF NOT EXISTS audit_events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE,
    occurred_at TEXT NOT NULL,
    actor_json TEXT NOT NULL,
    operation TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    previous_state TEXT,
    new_state TEXT,
    payload_sha256 TEXT NOT NULL,
    result TEXT NOT NULL,
    error_code TEXT,
    details_json TEXT NOT NULL DEFAULT '{}'
);

CREATE VIRTUAL TABLE IF NOT EXISTS learnings_fts USING fts5(
    learning_id UNINDEXED,
    project_key,
    title,
    context,
    observation,
    reusable_lesson,
    retrieval_terms,
    tokenize = 'unicode61'
);

CREATE TRIGGER IF NOT EXISTS learnings_ai AFTER INSERT ON learnings BEGIN
    INSERT INTO learnings_fts(
        learning_id, project_key, title, context, observation, reusable_lesson, retrieval_terms
    )
    SELECT
        new.id, p.project_key, new.title, new.context, new.observation,
        new.reusable_lesson, new.retrieval_terms_text
    FROM projects p WHERE p.id = new.project_id;
END;

CREATE TRIGGER IF NOT EXISTS learnings_au AFTER UPDATE ON learnings BEGIN
    DELETE FROM learnings_fts WHERE learning_id = old.id;
    INSERT INTO learnings_fts(
        learning_id, project_key, title, context, observation, reusable_lesson, retrieval_terms
    )
    SELECT
        new.id, p.project_key, new.title, new.context, new.observation,
        new.reusable_lesson, new.retrieval_terms_text
    FROM projects p WHERE p.id = new.project_id;
END;

CREATE TRIGGER IF NOT EXISTS learnings_ad AFTER DELETE ON learnings BEGIN
    DELETE FROM learnings_fts WHERE learning_id = old.id;
END;
