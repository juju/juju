CREATE TABLE controller_config (
    "key" TEXT PRIMARY KEY,
    value TEXT
) STRICT;

CREATE TABLE controller (
    uuid TEXT PRIMARY KEY NOT NULL
) STRICT;

-- A unique constraint over a constant index ensures only 1 entry matching the 
-- condition can exist.
CREATE UNIQUE INDEX idx_singleton_controller ON controller ((1));

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
