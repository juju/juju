-- Lookup of export-side migration phases. Mirrors core/migration/phase.go,
-- minus UNKNOWN/NONE (code-only sentinels never written to the DB) and
-- PROCESSRELATIONS (vestigial phase with no actions attached).
CREATE TABLE model_migration_phase (
    id INT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_model_migration_phase_name
ON model_migration_phase (name);

INSERT INTO model_migration_phase VALUES
(1, 'quiesce'),
(2, 'import'),
(3, 'validation'),
(4, 'success'),
(5, 'log-transfer'),
(6, 'reap'),
(7, 'reap-failed'),
(8, 'done'),
(9, 'abort'),
(10, 'abort-done');


-- Lookup of import-claim phases. A FK to this table replaces an inline CHECK
-- so that adding a future phase is a single INSERT rather than a SQLite
-- table rebuild.
CREATE TABLE model_migration_import_phase_type (
    id INT NOT NULL PRIMARY KEY,
    type TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_model_migration_import_phase_type
ON model_migration_import_phase_type (type);

INSERT INTO model_migration_import_phase_type VALUES
(0, 'importing'),
(1, 'activating'),
(2, 'aborting');

CREATE TABLE model_migration_import (
    uuid TEXT NOT NULL PRIMARY KEY,
    model_uuid TEXT NOT NULL,
    source_migration_uuid TEXT NOT NULL,
    phase_type_id INT NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT (DATETIME('now', 'utc')),
    CONSTRAINT fk_model_migration_import_phase_type
    FOREIGN KEY (phase_type_id)
    REFERENCES model_migration_import_phase_type (id)
);

CREATE UNIQUE INDEX idx_model_migration_import ON model_migration_import (model_uuid);

CREATE INDEX idx_model_migration_import_phase_updated
ON model_migration_import (phase_type_id, updated_at);

-- Durable handoff from Import to Activate for cross-model relation offers.
CREATE TABLE model_migration_import_offer (
    migration_uuid TEXT NOT NULL,
    offer_uuid TEXT NOT NULL,
    PRIMARY KEY (migration_uuid, offer_uuid),
    CONSTRAINT fk_model_migration_import_offer_migration
    FOREIGN KEY (migration_uuid)
    REFERENCES model_migration_import (uuid)
);

-- Durable handoff from Import to Activate for third-party external controller
-- model mappings. Activate reads these rows to set the offerer controller
-- for each offerer model without depending on the original import envelope
-- still being in memory after a controller restart.
CREATE TABLE model_migration_import_external_controller_model (
    migration_uuid TEXT NOT NULL,
    offerer_model_uuid TEXT NOT NULL,
    controller_uuid TEXT NOT NULL,
    PRIMARY KEY (migration_uuid, offerer_model_uuid),
    CONSTRAINT fk_model_migration_import_ecm_migration
    FOREIGN KEY (migration_uuid)
    REFERENCES model_migration_import (uuid),
    CONSTRAINT fk_model_migration_import_ecm_external_controller
    FOREIGN KEY (controller_uuid)
    REFERENCES external_controller (uuid)
);

CREATE INDEX idx_model_migration_import_ecm_controller
ON model_migration_import_external_controller_model (controller_uuid);

-- One row per export migration attempt for a model. There is no attempt
-- counter: a retry is just a new migration with a new uuid.
-- model_uuid deliberately has no FK to model because source REAP deletes
-- the model row while export history remains available for diagnostics.
--
-- current_phase_id and updated_at are denormalised from
-- model_migration_export_phase so that watchers and "is this migration
-- active?" queries do not have to aggregate the history table on every read.
-- The history table remains the source of truth.
CREATE TABLE model_migration_export (
    uuid TEXT NOT NULL PRIMARY KEY,
    model_uuid TEXT NOT NULL,
    target_controller_uuid TEXT NOT NULL,
    current_phase_id INT NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    start_time TIMESTAMP NOT NULL,
    CONSTRAINT fk_model_migration_export_target_controller
    FOREIGN KEY (target_controller_uuid)
    REFERENCES external_controller (uuid),
    CONSTRAINT fk_model_migration_export_current_phase
    FOREIGN KEY (current_phase_id)
    REFERENCES model_migration_phase (id)
);

-- At most one active export migration per model. Persisted terminal phase ids
-- are reap-failed (7), done (8), and abort-done (10).
CREATE UNIQUE INDEX idx_model_migration_export_active_model
ON model_migration_export (model_uuid)
WHERE current_phase_id NOT IN (7, 8, 10);

CREATE INDEX idx_model_migration_export_model
ON model_migration_export (model_uuid);

CREATE INDEX idx_model_migration_export_target_controller
ON model_migration_export (target_controller_uuid);

-- Hosted offer UUIDs captured before the source model DB is purged. REAP uses
-- these rows to delete source-controller offer permissions after the source
-- model row and model DB namespace have gone.
CREATE TABLE model_migration_export_offer (
    migration_uuid TEXT NOT NULL,
    offer_uuid TEXT NOT NULL,
    PRIMARY KEY (migration_uuid, offer_uuid),
    CONSTRAINT fk_model_migration_export_offer_migration
    FOREIGN KEY (migration_uuid)
    REFERENCES model_migration_export (uuid)
);

-- Standalone redirect snapshot for a model that has completed REAP. This table
-- deliberately has no FK to model, model_migration_export, or
-- external_controller because those rows may be removed while offline agents
-- still need redirect information from this source controller.
CREATE TABLE model_migration_redirect (
    model_uuid TEXT NOT NULL PRIMARY KEY,
    source_migration_uuid TEXT NOT NULL,
    target_controller_uuid TEXT NOT NULL,
    target_controller_alias TEXT,
    target_addresses TEXT NOT NULL,
    target_ca_cert TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP
);

-- Captured model access at migration time. The snapshot is intentionally
-- independent of the live permission rows, which are removed during REAP.
CREATE TABLE model_migration_redirect_user (
    model_uuid TEXT NOT NULL,
    user_uuid TEXT NOT NULL,
    user_name TEXT NOT NULL,
    access TEXT NOT NULL,
    PRIMARY KEY (model_uuid, user_uuid),
    CONSTRAINT fk_model_migration_redirect_user_redirect
    FOREIGN KEY (model_uuid)
    REFERENCES model_migration_redirect (model_uuid)
);

-- Per-migration authentication material for connecting to the target
-- controller. Held separately from external_controller because the material
-- is per-migration (it may be rotated, scoped, or JIMM-issued) and
-- external_controller is shared with the cross-model relations domain.
--
-- The dual FK (migration + external_controller) makes the relationship
-- explicit: a row here means "auth material for this migration to connect
-- to that external controller".
CREATE TABLE model_migration_export_target_auth (
    migration_uuid TEXT NOT NULL PRIMARY KEY,
    external_controller_uuid TEXT NOT NULL,
    target_user TEXT NOT NULL,
    target_macaroons TEXT,
    target_token TEXT,
    target_skip_user_checks BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT fk_model_migration_export_target_auth_migration
    FOREIGN KEY (migration_uuid)
    REFERENCES model_migration_export (uuid),
    CONSTRAINT fk_model_migration_export_target_auth_external_controller
    FOREIGN KEY (external_controller_uuid)
    REFERENCES external_controller (uuid)
);

CREATE INDEX idx_model_migration_export_target_auth_external_controller
ON model_migration_export_target_auth (external_controller_uuid);

-- Time-ordered record of which phases an export migration has entered.
-- Each phase is entered at most once per migration, so (migration_uuid,
-- phase_id) is the natural key.
--
-- model_uuid is denormalised from the parent model_migration_export row so the
-- changestream trigger can emit the model-scoped key watched for phase changes.
CREATE TABLE model_migration_export_phase (
    migration_uuid TEXT NOT NULL,
    model_uuid TEXT NOT NULL,
    phase_id INT NOT NULL,
    changed_at TIMESTAMP NOT NULL,
    PRIMARY KEY (migration_uuid, phase_id),
    CONSTRAINT fk_model_migration_export_phase_migration
    FOREIGN KEY (migration_uuid)
    REFERENCES model_migration_export (uuid),
    CONSTRAINT fk_model_migration_export_phase_phase
    FOREIGN KEY (phase_id)
    REFERENCES model_migration_phase (id)
);

CREATE INDEX idx_model_migration_export_phase_changed_at
ON model_migration_export_phase (migration_uuid, changed_at);

-- Current free-form status message reported by the migration master.
CREATE TABLE model_migration_export_status (
    migration_uuid TEXT NOT NULL PRIMARY KEY,
    message TEXT NOT NULL,
    recorded_at TIMESTAMP NOT NULL,
    CONSTRAINT fk_model_migration_export_status_migration
    FOREIGN KEY (migration_uuid)
    REFERENCES model_migration_export (uuid)
);

CREATE INDEX idx_model_migration_export_status_recorded_at
ON model_migration_export_status (migration_uuid, recorded_at);

-- Status messages are intentionally not changelogged. They can update often
-- during migration and should not amplify model migration watcher traffic.

-- Reports submitted by minion agents (machines/units) confirming or
-- failing the work for a given phase. entity_key is the agent's tag-like
-- identifier (e.g. "machine-0", "unit-foo-0").
CREATE TABLE model_migration_export_minion_sync (
    migration_uuid TEXT NOT NULL,
    phase_id INT NOT NULL,
    entity_key TEXT NOT NULL,
    success BOOLEAN NOT NULL,
    reported_at TIMESTAMP NOT NULL,
    PRIMARY KEY (migration_uuid, phase_id, entity_key),
    CONSTRAINT fk_model_migration_export_minion_sync_migration
    FOREIGN KEY (migration_uuid)
    REFERENCES model_migration_export (uuid),
    CONSTRAINT fk_model_migration_export_minion_sync_phase
    FOREIGN KEY (phase_id)
    REFERENCES model_migration_phase (id)
);
