CREATE TABLE workload_tracing_config (
    "key" TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE INDEX idx_workload_tracing_config_key_value
ON workload_tracing_config ("key", value);
