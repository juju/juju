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

CREATE TABLE controller_config (
    "key" TEXT NOT NULL PRIMARY KEY,
    value TEXT
);

CREATE VIEW v_controller_config AS
    SELECT "key", value
    FROM controller_config
UNION ALL
    SELECT 'controller-uuid' AS "key", controller.uuid AS value
    FROM controller
UNION ALL
    SELECT 'api-port' AS "key", controller.api_port AS value
    FROM controller
    WHERE controller.api_port IS NOT NULL AND controller.api_port != '';
