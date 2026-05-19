CREATE TABLE logging_loki_config (
    uuid TEXT NOT NULL PRIMARY KEY,
    endpoint TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_singleton_logging_loki_config ON logging_loki_config ((1));
