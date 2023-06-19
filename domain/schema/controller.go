// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import "github.com/juju/juju/core/database"

// ControllerDDL is used to create the controller database schema at bootstrap.
func ControllerDDL(nodeID uint64) []database.Delta {
	schemas := []func() database.Delta{
		leaseSchema,
		changeLogSchema,
		changeLogControllerNamespaces,
		cloudSchema,
		externalControllerSchema,
		modelListSchema,
		controllerConfigSchema,
		// These are broken up for 2 reasons:
		// 1. Bind variables do not work for multiple statements in one string.
		// 2. We want to insert the initial node before creating the change_log
		//    triggers as there is no need to produce a change stream event
		//    from what is a bootstrap activity.
		controllerNodeTable,
		controllerNodeEntry(nodeID),
		controllerNodeTriggers,
		modelMigrationSchema,
		upgradeInfoSchema,
	}

	var deltas []database.Delta
	for _, fn := range schemas {
		deltas = append(deltas, fn())
	}

	return deltas
}

func leaseSchema() database.Delta {
	return database.MakeDelta(`
CREATE TABLE lease_type (
    id   INT PRIMARY KEY,
    type TEXT
);

CREATE UNIQUE INDEX idx_lease_type_type
ON lease_type (type);

INSERT INTO lease_type VALUES
    (0, 'singular-controller'),    -- The controller running singular controller/model workers.
    (1, 'application-leadership'); -- The unit that holds leadership for an application.

CREATE TABLE lease (
    uuid            TEXT PRIMARY KEY,
    lease_type_id   INT NOT NULL,
    model_uuid      TEXT,
    name            TEXT,
    holder          TEXT,
    start           TIMESTAMP,
    expiry          TIMESTAMP,
    CONSTRAINT      fk_lease_lease_type
        FOREIGN KEY (lease_type_id)
        REFERENCES  lease_type(id)
);

CREATE UNIQUE INDEX idx_lease_model_type_name
ON lease (model_uuid, lease_type_id, name);

CREATE INDEX idx_lease_expiry
ON lease (expiry);

CREATE TABLE lease_pin (
    -- The presence of entries in this table for a particular lease_uuid
    -- implies that the lease in question is pinned and cannot expire.
    uuid       TEXT PRIMARY KEY,
    lease_uuid TEXT,
    entity_id  TEXT,
    CONSTRAINT      fk_lease_pin_lease
        FOREIGN KEY (lease_uuid)
        REFERENCES  lease(uuid)
);

CREATE UNIQUE INDEX idx_lease_pin_lease_entity
ON lease_pin (lease_uuid, entity_id);

CREATE INDEX idx_lease_pin_lease
ON lease_pin (lease_uuid);`)
}

func changeLogSchema() database.Delta {
	return database.MakeDelta(`
CREATE TABLE change_log_edit_type (
    id        INT PRIMARY KEY,
    edit_type TEXT
);

CREATE UNIQUE INDEX idx_change_log_edit_type_edit_type
ON change_log_edit_type (edit_type);

-- The change log type values are bitmasks, so that multiple types can be
-- expressed when looking for changes.
INSERT INTO change_log_edit_type VALUES
    (1, 'create'),
    (2, 'update'),
    (4, 'delete');

CREATE TABLE change_log_namespace (
    id        INT PRIMARY KEY,
    namespace TEXT
);

CREATE UNIQUE INDEX idx_change_log_namespace_namespace
ON change_log_namespace (namespace);

CREATE TABLE change_log (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    edit_type_id        INT NOT NULL,
    namespace_id        INT NOT NULL,
    changed_uuid        TEXT NOT NULL,
    created_at          DATETIME NOT NULL DEFAULT(STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    CONSTRAINT          fk_change_log_edit_type
            FOREIGN KEY (edit_type_id)
            REFERENCES  change_log_edit_type(id),
    CONSTRAINT          fk_change_log_namespace
            FOREIGN KEY (namespace_id)
            REFERENCES  change_log_namespace(id)
);

-- The change log witness table is used to track which nodes have seen
-- which change log entries. This is used to determine when a change log entry
-- can be deleted.
-- We'll delete all change log entries that are older than the lower_bound
-- change log entry that has been seen by all controllers.
CREATE TABLE change_log_witness (
    controller_id       TEXT PRIMARY KEY,
    lower_bound         INT NOT NULL DEFAULT(-1),
    upper_bound         INT NOT NULL DEFAULT(-1),
    updated_at          DATETIME NOT NULL DEFAULT(STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc'))
);`)
}

func changeLogControllerNamespaces() database.Delta {
	return database.MakeDelta(`
INSERT INTO change_log_namespace VALUES
    (1, 'external_controller'),
    (2, 'controller_node'),
    (3, 'controller_config'),
    (4, 'model_migration_status'),
    (5, 'model_migration_minion_sync'),
    (6, 'upgrade_info');
`)
}

func cloudSchema() database.Delta {
	return database.MakeDelta(`
CREATE TABLE cloud_type (
    id   INT PRIMARY KEY,
    type TEXT
);

CREATE UNIQUE INDEX idx_cloud_type_type
ON cloud_type (type);

-- The list of all the cloud types that are supported for this release. This
-- doesn't indicate whether the cloud type is supported for the current
-- controller, but rather the cloud type is supported in general.
INSERT INTO cloud_type VALUES
    (0, 'kubernetes'),
    (1, 'lxd'),
    (2, 'maas'),
    (3, 'manual'),
    (4, 'azure'),
    (5, 'ec2'),
    (6, 'equinix'),
    (7, 'gce'),
    (8, 'oci'),
    (9, 'openstack'),
    (10, 'vsphere');

CREATE TABLE auth_type (
    id   INT PRIMARY KEY,
    type TEXT
);

CREATE UNIQUE INDEX idx_auth_type_type
ON auth_type (type);

INSERT INTO auth_type VALUES
    (0, 'access-key'),
    (1, 'instance-role'),
    (2, 'userpass'),
    (3, 'oauth1'),
    (4, 'oauth2'),
    (5, 'jsonfile'),
    (6, 'clientcertificate'),
    (7, 'httpsig'),
    (8, 'interactive'),
    (9, 'empty'),
    (10, 'certificate'),
    (11, 'oauth2withcert');

CREATE TABLE cloud (
    uuid                TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    cloud_type_id       INT NOT NULL,
    endpoint            TEXT NOT NULL,
    identity_endpoint   TEXT,
    storage_endpoint    TEXT,
    skip_tls_verify     BOOLEAN NOT NULL,
    CONSTRAINT          fk_cloud_type
        FOREIGN KEY       (cloud_type_id)
        REFERENCES        cloud_type(id)
);

CREATE TABLE cloud_auth_type (
    uuid              TEXT PRIMARY KEY,
    cloud_uuid        TEXT NOT NULL,
    auth_type_id      INT NOT NULL,
    CONSTRAINT		  fk_cloud_auth_type_cloud
        FOREIGN KEY       (cloud_uuid)
        REFERENCES        cloud(uuid),
    CONSTRAINT        fk_cloud_auth_type_auth_type
        FOREIGN KEY       (auth_type_id)
        REFERENCES        auth_type(id)
);

CREATE UNIQUE INDEX idx_cloud_auth_type_cloud_uuid_auth_type_id
ON cloud_auth_type (cloud_uuid, auth_type_id);

CREATE TABLE cloud_region (
    uuid                TEXT PRIMARY KEY,
    cloud_uuid          TEXT NOT NULL,
    name                TEXT NOT NULL,
    endpoint            TEXT,
    identity_endpoint   TEXT,
    storage_endpoint    TEXT,
    CONSTRAINT          fk_cloud_region_cloud
        FOREIGN KEY         (cloud_uuid)
        REFERENCES          cloud(uuid)
);

CREATE UNIQUE INDEX idx_cloud_region_cloud_uuid_name
ON cloud_region (cloud_uuid, name);

CREATE INDEX idx_cloud_region_cloud_uuid
ON cloud_region (cloud_uuid);

CREATE TABLE cloud_ca_cert (
    uuid              TEXT PRIMARY KEY,
    cloud_uuid        TEXT NOT NULL,
    ca_cert           TEXT NOT NULL,
    CONSTRAINT        fk_cloud_ca_cert_cloud
        FOREIGN KEY       (cloud_uuid)
        REFERENCES        cloud(uuid)
);

CREATE UNIQUE INDEX idx_cloud_ca_cert_cloud_uuid_ca_cert
ON cloud_ca_cert (cloud_uuid, ca_cert);`)
}

func externalControllerSchema() database.Delta {
	return database.MakeDelta(`
CREATE TABLE external_controller (
    uuid            TEXT PRIMARY KEY,
    alias           TEXT,
    ca_cert         TEXT NOT NULL
);

CREATE TABLE external_controller_address (
    uuid               TEXT PRIMARY KEY,
    controller_uuid    TEXT NOT NULL,
    address            TEXT NOT NULL,
    CONSTRAINT         fk_external_controller_address_external_controller_uuid
        FOREIGN KEY         (controller_uuid)
        REFERENCES          external_controller(uuid)
);

CREATE UNIQUE INDEX idx_external_controller_address
ON external_controller_address (controller_uuid, address);

CREATE TABLE external_model (
    uuid                TEXT PRIMARY KEY,
    controller_uuid     TEXT NOT NULL,
    CONSTRAINT          fk_external_model_external_controller_uuid
        FOREIGN KEY         (controller_uuid)
        REFERENCES          external_controller(uuid)
);

CREATE TRIGGER trg_log_external_controller_insert
AFTER INSERT ON external_controller FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at) 
    VALUES (1, 1, NEW.uuid, DATETIME('now'));
END;
CREATE TRIGGER trg_log_external_controller_update
AFTER UPDATE ON external_controller FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at) 
    VALUES (2, 1, OLD.uuid, DATETIME('now'));
END;
CREATE TRIGGER trg_log_external_controller_delete
AFTER DELETE ON external_controller FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at) 
    VALUES (4, 1, OLD.uuid, DATETIME('now'));
END;`)
}

func modelListSchema() database.Delta {
	return database.MakeDelta(`
CREATE TABLE model_list (
    uuid    TEXT PRIMARY KEY
);`)
}

func controllerConfigSchema() database.Delta {
	return database.MakeDelta(`
CREATE TABLE controller_config (
    key     TEXT PRIMARY KEY,
    value   TEXT
);

CREATE TRIGGER trg_log_controller_config_insert
AFTER INSERT ON controller_config FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at) 
    VALUES (1, 3, NEW.key, DATETIME('now'));
END;
CREATE TRIGGER trg_log_controller_config_update
AFTER UPDATE ON controller_config FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at) 
    VALUES (2, 3, OLD.key, DATETIME('now'));
END;
CREATE TRIGGER trg_log_controller_config_delete
AFTER DELETE ON controller_config FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at) 
    VALUES (4, 3, OLD.key, DATETIME('now'));
END;`)
}

func controllerNodeTable() database.Delta {
	return database.MakeDelta(`
CREATE TABLE controller_node (
    controller_id  TEXT PRIMARY KEY, 
    dqlite_node_id INT,               -- This is the uint64 from Dqlite NodeInfo.
    bind_address   TEXT               -- IP address (no port) that Dqlite is bound to. 
);

CREATE UNIQUE INDEX idx_controller_node_dqlite_node
ON controller_node (dqlite_node_id);

CREATE UNIQUE INDEX idx_controller_node_bind_address
ON controller_node (bind_address);`)
}

func controllerNodeEntry(nodeID uint64) func() database.Delta {
	return func() database.Delta {
		return database.MakeDelta(`
-- TODO (manadart 2023-06-06): At the time of writing, 
-- we have not yet modelled machines. 
-- Accordingly, the controller ID remains the ID of the machine, 
-- but it should probably become a UUID once machines have one.
-- While HA is not supported in K8s, this doesn't matter.
INSERT INTO controller_node (controller_id, dqlite_node_id, bind_address)
VALUES ('0', ?, '127.0.0.1');`, nodeID)
	}
}

func controllerNodeTriggers() database.Delta {
	return database.MakeDelta(`
CREATE TRIGGER trg_changelog_controller_node_insert
AFTER INSERT ON controller_node FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at)
    VALUES (1, 2, NEW.controller_id, DATETIME('now'));
END;

CREATE TRIGGER trg_changelog_controller_node_update
AFTER UPDATE ON controller_node FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at)
    VALUES (2, 2, OLD.controller_id, DATETIME('now'));
END;

CREATE TRIGGER trg_changelog_controller_node_delete
AFTER DELETE ON controller_node FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at)
    VALUES (4, 2, OLD.controller_id, DATETIME('now'));
END;`)
}

func modelMigrationSchema() database.Delta {
	return database.MakeDelta(`
CREATE TABLE model_migration (
    uuid                    TEXT PRIMARY KEY,
    attempt                 INT,
    target_controller_uuid  TEXT NOT NULL,
    target_entity           TEXT,
    target_password         TEXT,
    target_macaroons        TEXT,
    active                  BOOLEAN,
    start_time              TIMESTAMP,
    success_time            TIMESTAMP,
    end_time                TIMESTAMP,
    phase                   TEXT,
    phase_changed_time      TIMESTAMP,
    status_message          TEXT,
    CONSTRAINT              fk_model_migration_target_controller
        FOREIGN KEY         (target_controller_uuid)
        REFERENCES          external_controller(uuid)
);

CREATE TABLE model_migration_status (
    uuid                TEXT PRIMARY KEY,
    start_time          TIMESTAMP,
    success_time        TIMESTAMP,
    end_time            TIMESTAMP,
    phase               TEXT,
    phase_changed_time  TIMESTAMP,
    status              TEXT
);

CREATE TRIGGER trg_log_model_migration_status_insert
AFTER INSERT ON model_migration_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at) 
    VALUES (1, 4, NEW.key, DATETIME('now'));
END;
CREATE TRIGGER trg_log_model_migration_status_update
AFTER UPDATE ON model_migration_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at) 
    VALUES (2, 4, OLD.key, DATETIME('now'));
END;
CREATE TRIGGER trg_log_model_migration_status_delete
AFTER DELETE ON model_migration_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at) 
    VALUES (4, 4, OLD.key, DATETIME('now'));
END;

CREATE TABLE model_migration_user (
    uuid            TEXT PRIMARY KEY,
--     user_uuid       TEXT NOT NULL,
    migration_uuid  TEXT NOT NULL,
    permission      TEXT,
--     CONSTRAINT      fk_model_migration_user_XXX
--         FOREIGN KEY (user_uuid)
--         REFERENCES  XXX(uuid)
    CONSTRAINT      fk_model_migration_user_model_migration
        FOREIGN KEY (migration_uuid)
        REFERENCES  model_migration(uuid)    
);

CREATE TABLE model_migration_minion_sync (
    uuid            TEXT PRIMARY KEY,
    migration_uuid  TEXT NOT NULL,
    phase           TEXT,
    entity_key      TEXT,
    time            TIMESTAMP,
    success         BOOLEAN,
    CONSTRAINT      fk_model_migration_minion_sync_model_migration
        FOREIGN KEY (migration_uuid)
        REFERENCES  model_migration(uuid)
);

CREATE TRIGGER trg_log_model_migration_minion_sync_insert
AFTER INSERT ON model_migration_minion_sync FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at) 
    VALUES (1, 5, NEW.key, DATETIME('now'));
END;
CREATE TRIGGER trg_log_model_migration_minion_sync_update
AFTER UPDATE ON model_migration_minion_sync FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at) 
    VALUES (2, 5, OLD.key, DATETIME('now'));
END;
CREATE TRIGGER trg_log_model_migration_minion_sync_delete
AFTER DELETE ON model_migration_minion_sync FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at) 
    VALUES (4, 5, OLD.key, DATETIME('now'));
END;`)
}

func upgradeInfoSchema() database.Delta {
	return database.MakeDelta(`
CREATE TABLE upgrade_info (
    uuid             TEXT PRIMARY KEY,
    previous_version TEXT NOT NULL,
    target_version   TEXT NOT NULL,
    init_time        TIMESTAMP NOT NULL,
    start_time       TIMESTAMP,
    completion_time  TIMESTAMP
);

CREATE TABLE upgrade_node_status (
    id     INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO upgrade_node_status VALUES
    (0, "ready"),
    (1, "done");

CREATE TABLE upgrade_info_controller_node (
    uuid                      TEXT PRIMARY KEY,
    controller_node_id        TEXT NOT NULL,
    upgrade_info_uuid         TEXT NOT NULL,
    upgrade_node_status_id    INT NOT NULL,
    CONSTRAINT                fk_controller_node_id
        FOREIGN KEY               (controller_node_id)
        REFERENCES                controller_node(controller_id),
    CONSTRAINT                fk_upgrade_info
        FOREIGN KEY               (upgrade_info_uuid)
        REFERENCES                upgrade_info(uuid),
    CONSTRAINT                fk_node_status
        FOREIGN KEY               (upgrade_node_status_id)
        REFERENCES                upgrade_node_status(id)
);

CREATE UNIQUE INDEX idx_upgrade_info_controller_node
ON upgrade_info_controller_node (controller_node_id, upgrade_info_uuid, upgrade_node_status_id);

CREATE TRIGGER trg_changelog_upgradeinfo_insert
AFTER INSERT ON upgrade_info FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at)
    VALUES (1, 4, NEW.uuid, DATETIME('now'));
END;

CREATE TRIGGER trg_changelog_upgradeinfo_update
AFTER UPDATE ON upgrade_info FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at)
    VALUES (2, 4, OLD.uuid, DATETIME('now'));
END;

CREATE TRIGGER trg_changelog_upgradeinfo_delete
AFTER DELETE ON upgrade_info FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed_uuid, created_at)
    VALUES (4, 4, OLD.uuid, DATETIME('now'));
END;
`)
}
