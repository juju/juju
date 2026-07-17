CREATE TABLE charm_tracing_config (
    "key" TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE INDEX idx_charm_tracing_config_key_value
ON charm_tracing_config ("key", value);
