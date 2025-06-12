CREATE TABLE controller (
    uuid TEXT NOT NULL PRIMARY KEY,
    model_uuid TEXT NOT NULL,
    target_version TEXT NOT NULL
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
