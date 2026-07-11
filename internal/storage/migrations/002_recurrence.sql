-- 002_recurrence: tracks recurrence of learning patterns across captures.
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS recurrence_records (
    id TEXT PRIMARY KEY,
    recurrence_fingerprint TEXT NOT NULL,
    learning_id TEXT NOT NULL REFERENCES learnings(id),
    project_id TEXT NOT NULL REFERENCES projects(id),
    summary TEXT NOT NULL,
    occurred_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_recurrence_fp ON recurrence_records(recurrence_fingerprint);
CREATE INDEX IF NOT EXISTS idx_recurrence_learning ON recurrence_records(learning_id);
CREATE INDEX IF NOT EXISTS idx_recurrence_project ON recurrence_records(project_id);
