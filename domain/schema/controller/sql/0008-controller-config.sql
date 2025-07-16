CREATE TABLE controller_config (
    "key" TEXT NOT NULL PRIMARY KEY,
    value TEXT
);

CREATE VIEW v_controller_config AS
SELECT
    "key",
    value
FROM controller_config
UNION ALL
SELECT
    'controller-uuid' AS "key",
    controller.uuid AS value
FROM controller
UNION ALL
SELECT
    'api-port' AS "key",
    controller.api_port AS value
FROM controller
WHERE controller.api_port IS NOT NULL AND controller.api_port != '';
