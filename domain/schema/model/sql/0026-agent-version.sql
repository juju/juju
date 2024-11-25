
CREATE TABLE agent_version (
    current_version TEXT NOT NULL,
    target_version TEXT NOT NULL
);

-- A unique constraint over a constant index 
-- ensures only 1 row can exist.
CREATE UNIQUE INDEX idx_singleton_agent_version ON agent_version ((1));
