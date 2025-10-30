CREATE TABLE controller (
    uuid TEXT NOT NULL PRIMARY KEY,
    model_uuid TEXT NOT NULL,
    target_version TEXT NOT NULL,
    api_port TEXT,
    cert BLOB,
    ca_cert BLOB,
    private_key BLOB,
    ca_private_key BLOB,
    system_identity BLOB
) STRICT;

-- A unique constraint over a constant index ensures only 1 entry matching the
-- condition can exist.
CREATE UNIQUE INDEX idx_singleton_controller ON controller ((1));
