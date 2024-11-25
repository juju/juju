
CREATE TABLE agent_version (
    target_version TEXT NOT NULL
);

-- A unique constraint over a constant index 
-- ensures only 1 row can exist.
CREATE UNIQUE INDEX idx_singleton_agent_version ON agent_version ((1));
