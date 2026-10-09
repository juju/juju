-- log_transfer_progress stores the latest log timestamp received through the
-- migration log-transfer endpoint. MigrationTarget uses this checkpoint to
-- resume transfers without treating agent logs as migrated logs.
-- The row is keyed by the model uuid, and each model has its own database,
-- so the table contains at most one row.
CREATE TABLE log_transfer_progress (
    uuid TEXT NOT NULL PRIMARY KEY,
    last_time TIMESTAMP NOT NULL,
    CONSTRAINT fk_uuid_model_uuid
    FOREIGN KEY (uuid)
    REFERENCES model (uuid)
);
