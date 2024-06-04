-- The model table represents a readonly denormalised model data. The intended
-- use is to provide a read-only view of the model data for the purpose of
-- accessing common model data without the need to span multiple databases.
CREATE TABLE model (
    uuid                 TEXT PRIMARY KEY,
    controller_uuid      TEXT NOT NULL,
    name                 TEXT NOT NULL,
    type                 TEXT NOT NULL,
    target_agent_version TEXT NOT NULL,
    cloud                TEXT NOT NULL,
    cloud_type           TEXT NOT NULL,
    cloud_region         TEXT,
    credential_owner     TEXT,
    credential_name      TEXT
);

-- A unique constraint over a constant index ensures only 1 entry matching the
-- condition can exist.
CREATE UNIQUE INDEX idx_singleton_model ON model ((1));
