CREATE TABLE controller (
    uuid TEXT NOT NULL PRIMARY KEY,
    model_uuid TEXT NOT NULL,
    target_version TEXT NOT NULL,
    api_port TEXT,
    cert TEXT,
    private_key TEXT,
    ca_private_key TEXT,
    system_identity TEXT
);

-- A unique constraint over a constant index ensures only 1 entry matching the
-- condition can exist.
CREATE UNIQUE INDEX idx_singleton_controller ON controller ((1));
