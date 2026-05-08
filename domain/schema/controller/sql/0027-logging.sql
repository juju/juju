CREATE TABLE logging_loki_config (
    uuid TEXT NOT NULL PRIMARY KEY,
    endpoint TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_logging_loki_config_endpoint ON logging_loki_config (endpoint);
