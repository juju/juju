-- agent_stream defines the recognised streams available in the model for
-- fetching agent binaries.
CREATE TABLE agent_stream (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_agent_stream_name
ON agent_stream (name);

INSERT INTO agent_stream VALUES
(0, 'released'),
(1, 'proposed'),
(2, 'testing'),
(3, 'devel');

CREATE TABLE agent_version (
    stream_id INT NOT NULL,
    target_version TEXT NOT NULL,
    FOREIGN KEY (stream_id)
    REFERENCES agent_stream(id)
);

-- A unique constraint over a constant index
-- ensures only 1 row can exist.
CREATE UNIQUE INDEX idx_singleton_agent_version ON agent_version ((1));
