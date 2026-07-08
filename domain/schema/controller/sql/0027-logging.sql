CREATE TABLE logging_loki_config (
    uuid TEXT NOT NULL PRIMARY KEY,
    endpoint TEXT NOT NULL,
    ca_cert TEXT NOT NULL DEFAULT '',
    insecure_skip_verify BOOLEAN NULL,
    org_id TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX idx_singleton_logging_loki_config ON logging_loki_config ((1));
