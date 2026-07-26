-- 006_trace: Learning↔Event join table for progressive trace (Hito 4).
--
-- Links a promoted Learning to the source ExperienceEvents that form its
-- provenance chain. Each row carries a relationship_type so trace consumers
-- can distinguish direct evidence from derived or referenced events.
--
-- The (learning_id, event_id) pair is the primary key and is unique: the same
-- event cannot be linked to the same learning more than once.

CREATE TABLE IF NOT EXISTS learning_events (
    learning_id       TEXT NOT NULL,
    event_id          TEXT NOT NULL,
    relationship_type TEXT NOT NULL CHECK(relationship_type IN ('source','derived','referenced')),
    created_at        TEXT NOT NULL,
    PRIMARY KEY (learning_id, event_id),
    FOREIGN KEY (learning_id) REFERENCES learnings(id) ON DELETE CASCADE,
    FOREIGN KEY (event_id)   REFERENCES experience_events(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_learning_events_event
    ON learning_events(event_id);
