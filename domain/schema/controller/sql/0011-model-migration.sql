CREATE TABLE model_migration (
    uuid TEXT NOT NULL PRIMARY KEY,
    attempt INT,
    target_controller_uuid TEXT NOT NULL,
    target_entity TEXT,
    target_password TEXT,
    target_macaroons TEXT,
    active BOOLEAN,
    start_time TIMESTAMP,
    success_time TIMESTAMP,
    end_time TIMESTAMP,
    phase TEXT,
    phase_changed_time TIMESTAMP,
    status_message TEXT,
    CONSTRAINT fk_model_migration_target_controller
    FOREIGN KEY (target_controller_uuid)
    REFERENCES external_controller (uuid)
);

CREATE TABLE model_migration_status (
    uuid TEXT NOT NULL PRIMARY KEY,
    start_time TIMESTAMP,
    success_time TIMESTAMP,
    end_time TIMESTAMP,
    phase TEXT,
    phase_changed_time TIMESTAMP,
    status TEXT
);

CREATE TABLE model_migration_user (
    uuid TEXT NOT NULL PRIMARY KEY,
    --     user_uuid       TEXT NOT NULL,
    migration_uuid TEXT NOT NULL,
    permission TEXT,
    --     CONSTRAINT      fk_model_migration_user_XXX
    --         FOREIGN KEY (user_uuid)
    --         REFERENCES  XXX(uuid)
    CONSTRAINT fk_model_migration_user_model_migration
    FOREIGN KEY (migration_uuid)
    REFERENCES model_migration (uuid)
);

CREATE TABLE model_migration_minion_sync (
    uuid TEXT NOT NULL PRIMARY KEY,
    migration_uuid TEXT NOT NULL,
    phase TEXT,
    entity_key TEXT,
    time TIMESTAMP,
    success BOOLEAN,
    CONSTRAINT fk_model_migration_minion_sync_model_migration
    FOREIGN KEY (migration_uuid)
    REFERENCES model_migration (uuid)
);
