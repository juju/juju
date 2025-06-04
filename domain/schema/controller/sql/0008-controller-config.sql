CREATE TABLE controller (
    uuid TEXT NOT NULL PRIMARY KEY,
    model_uuid TEXT NOT NULL
);

-- A unique constraint over a constant index ensures only 1 entry matching the
-- condition can exist.
CREATE UNIQUE INDEX idx_singleton_controller ON controller ((1));

CREATE TABLE controller_config (
    "key" TEXT NOT NULL PRIMARY KEY,
    value TEXT
);

CREATE VIEW v_controller_config AS
SELECT
    "key",
    value
FROM controller_config
UNION
SELECT
    'controller-uuid' AS "key",
    controller.uuid AS value
FROM controller;

-- Tracks the target binary version for the controller. Can only ever be at most
-- one reccord in the table by virtue of the controller table only supporting
-- one controller.
CREATE TABLE controller_version (
    controller_uuid TEXT NOT NULL PRIMARY KEY,
    target_version TEXT NOT NULL,
    FOREIGN KEY (controller_uuid)
    REFERENCES controller(uuid)
);
