CREATE TABLE model_migration (
    uuid TEXT PRIMARY KEY,
    attempt INT,
    target_controller_uuid TEXT NOT NULL,
    target_entity TEXT,
    target_password TEXT,
    target_macaroons TEXT,
    active INT,
    start_time TEXT,
    success_time TEXT,
    end_time TEXT,
    phase TEXT,
    phase_changed_time TEXT,
    status_message TEXT,
    CONSTRAINT fk_model_migration_target_controller
    FOREIGN KEY (target_controller_uuid)
    REFERENCES external_controller (uuid)
) STRICT;

CREATE TABLE model_migration_status (
    uuid TEXT PRIMARY KEY,
    start_time TEXT,
    success_time TEXT,
    end_time TEXT,
    phase TEXT,
    phase_changed_time TEXT,
    status TEXT
) STRICT;

CREATE TABLE model_migration_user (
    uuid TEXT PRIMARY KEY,
    --     user_uuid       TEXT NOT NULL,
    migration_uuid TEXT NOT NULL,
    permission TEXT,
    --     CONSTRAINT      fk_model_migration_user_XXX
    --         FOREIGN KEY (user_uuid)
    --         REFERENCES  XXX(uuid)
    CONSTRAINT fk_model_migration_user_model_migration
    FOREIGN KEY (migration_uuid)
    REFERENCES model_migration (uuid)
) STRICT;

CREATE TABLE model_migration_minion_sync (
    uuid TEXT PRIMARY KEY,
    migration_uuid TEXT NOT NULL,
    phase TEXT,
    entity_key TEXT,
    time TEXT,
    success INT,
    CONSTRAINT fk_model_migration_minion_sync_model_migration
    FOREIGN KEY (migration_uuid)
    REFERENCES model_migration (uuid)
) STRICT;
