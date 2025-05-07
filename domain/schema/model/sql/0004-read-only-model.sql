-- The model table represents a readonly denormalised model data. The intended
-- use is to provide a read-only view of the model data for the purpose of
-- accessing common model data without the need to span multiple databases.
--
-- The model table primarily is used to drive the provider tracker. The model
-- table should *not* be changed in a patch/build release. The only time to make
-- changes to this table is during a major/minor release.
CREATE TABLE model (
    uuid TEXT NOT NULL PRIMARY KEY,
    controller_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    owner_name TEXT NOT NULL,
    owner_uuid TEXT NOT NULL,
    type TEXT NOT NULL,
    cloud TEXT NOT NULL,
    cloud_type TEXT NOT NULL,
    cloud_region TEXT,
    credential_owner TEXT,
    credential_name TEXT,
    is_controller_model BOOLEAN DEFAULT FALSE
);

-- A unique constraint over a constant index ensures only 1 entry matching the
-- condition can exist.
CREATE UNIQUE INDEX idx_singleton_model ON model ((1));

CREATE VIEW v_model_metrics AS
SELECT
    (SELECT COUNT(DISTINCT uuid) FROM application) AS application_count,
    (SELECT COUNT(DISTINCT uuid) FROM machine) AS machine_count,
    (SELECT COUNT(DISTINCT uuid) FROM unit) AS unit_count;
