// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"github.com/juju/juju/core/database/schema"
)

// ControllerDDL is used to create the controller database schema at bootstrap.
func ControllerDDL(nodeID uint64) *schema.Schema {
	patches := []func() schema.Patch{
		leaseSchema,
		changeLogSchema,
		changeLogControllerNamespaces,
		cloudSchema,
		changeLogTriggersForTable("cloud", "uuid", 7),
		changeLogTriggersForTable("cloud_credential", "uuid", 8),
		externalControllerSchema,
		changeLogTriggersForTable("external_controller", "uuid", 1),
		modelListSchema,
		controllerConfigSchema,
		changeLogTriggersForTable("controller_config", "key", 3),
		// These are broken up for 2 reasons:
		// 1. Bind variables do not work for multiple statements in one string.
		// 2. We want to insert the initial node before creating the change_log
		//    triggers as there is no need to produce a change stream event
		//    from what is a bootstrap activity.
		controllerNodeTable,
		controllerNodeEntry(nodeID),
		changeLogTriggersForTable("controller_node", "controller_id", 2),
		modelMigrationSchema,
		changeLogTriggersForTable("model_migration_status", "uuid", 4),
		changeLogTriggersForTable("model_migration_minion_sync", "uuid", 5),
		upgradeInfoSchema,
		changeLogTriggersForTable("upgrade_info", "uuid", 6),
		modelDefaults,
		autocertCacheSchema,
	}

	schema := schema.New()
	for _, fn := range patches {
		schema.Add(fn())
	}

	return schema
}

func leaseSchema() schema.Patch {
	return schema.MakePatch(`
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

func changeLogControllerNamespaces() schema.Patch {
	return schema.MakePatch(`
INSERT INTO change_log_namespace VALUES
    (1, 'external_controller', 'external controller changes based on the UUID'),
    (2, 'controller_node', 'controller node changes based on the controller ID'),
    (3, 'controller_config', 'controller config changes based on the key'),
    (4, 'model_migration_status', 'model migration status changes based on the UUID'),
    (5, 'model_migration_minion_sync', 'model migration minion sync changes based on the UUID'),
    (6, 'upgrade_info', 'upgrade info changes based on the UUID'),
    (7, 'cloud', 'cloud changes based on the UUID'),
    (8, 'cloud_credential', 'cloud credential changes based on the UUID'),
    (9, 'autocert_cache', 'autocert cache changes based on the UUID')
`)
}

func cloudSchema() schema.Patch {
	return schema.MakePatch(`
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
    name                TEXT NOT NULL UNIQUE,
    cloud_type_id       INT NOT NULL,
    endpoint            TEXT NOT NULL,
    identity_endpoint   TEXT,
    storage_endpoint    TEXT,
    skip_tls_verify     BOOLEAN NOT NULL,
    CONSTRAINT  chk_name_empty CHECK (name != ""),
    CONSTRAINT          fk_cloud_type
        FOREIGN KEY       (cloud_type_id)
        REFERENCES        cloud_type(id)
);

CREATE TABLE cloud_defaults (
    cloud_uuid TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT,
    PRIMARY KEY (cloud_uuid, key)
    CONSTRAINT chk_key_empty CHECK (key != ""),
    CONSTRAINT fk_cloud_uuid
        FOREIGN KEY (cloud_uuid)
        REFERENCES cloud(uuid)
);

CREATE TABLE cloud_auth_type (
    uuid              TEXT PRIMARY KEY,
    cloud_uuid        TEXT NOT NULL,
    auth_type_id      INT NOT NULL,
    CONSTRAINT        fk_cloud_auth_type_cloud
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

CREATE TABLE cloud_region_defaults (
    region_uuid     TEXT NOT NULL,
    key             TEXT NOT NULL,
    value           TEXT,
    PRIMARY KEY     (region_uuid, key),
    CONSTRAINT      chk_key_empty CHECK(key != ""),
    CONSTRAINT      fk_region_uuid
        FOREIGN KEY (region_uuid)
        REFERENCES  cloud_region(uuid)
);

CREATE TABLE cloud_ca_cert (
    uuid              TEXT PRIMARY KEY,
    cloud_uuid        TEXT NOT NULL,
    ca_cert           TEXT NOT NULL,
    CONSTRAINT        fk_cloud_ca_cert_cloud
        FOREIGN KEY       (cloud_uuid)
        REFERENCES        cloud(uuid)
);

CREATE UNIQUE INDEX idx_cloud_ca_cert_cloud_uuid_ca_cert
ON cloud_ca_cert (cloud_uuid, ca_cert);

CREATE TABLE cloud_credential (
        uuid                TEXT PRIMARY KEY,
        cloud_uuid          TEXT NOT NULL,
        auth_type_id        TEXT NOT NULL,
        owner_uuid          TEXT NOT NULL,
        name                TEXT NOT NULL,
        revoked             BOOLEAN,
        invalid             BOOLEAN,
        invalid_reason      TEXT,
        CONSTRAINT chk_name_empty CHECK (name != ""),
        CONSTRAINT          fk_cloud_credential_cloud
            FOREIGN KEY         (cloud_uuid)
            REFERENCES          cloud(uuid)
        CONSTRAINT          fk_cloud_credential_auth_type
            FOREIGN KEY         (auth_type_id)
            REFERENCES          auth_type(id)
--        CONSTRAINT          fk_cloud_credential_XXXX
--            FOREIGN KEY         (owner_uuid)
--            REFERENCES          XXXX(uuid)
);

CREATE UNIQUE INDEX idx_cloud_credential_cloud_uuid_owner_uuid
ON cloud_credential (cloud_uuid, owner_uuid, name);

CREATE TABLE cloud_credential_attributes (
    cloud_credential_uuid TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT,
    PRIMARY KEY (cloud_credential_uuid, key)
    CONSTRAINT chk_key_empty CHECK (key != ""),
    CONSTRAINT fk_cloud_credential_uuid
        FOREIGN KEY (cloud_credential_uuid)
        REFERENCES cloud_credential(uuid)
);
`)
}

func externalControllerSchema() schema.Patch {
	return schema.MakePatch(`
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
);`)
}

func modelListSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE model_list (
    uuid    TEXT PRIMARY KEY
);`)
}

func controllerConfigSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE controller_config (
    key     TEXT PRIMARY KEY,
    value   TEXT
);`)
}

func controllerNodeTable() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE controller_node (
    controller_id  TEXT PRIMARY KEY, 
    dqlite_node_id TEXT,              -- This is the uint64 from Dqlite NodeInfo, stored as text.
    bind_address   TEXT               -- IP address (no port) that Dqlite is bound to. 
);

CREATE UNIQUE INDEX idx_controller_node_dqlite_node
ON controller_node (dqlite_node_id);

CREATE UNIQUE INDEX idx_controller_node_bind_address
ON controller_node (bind_address);`)
}

func controllerNodeEntry(nodeID uint64) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(`
-- TODO (manadart 2023-06-06): At the time of writing, 
-- we have not yet modelled machines. 
-- Accordingly, the controller ID remains the ID of the machine, 
-- but it should probably become a UUID once machines have one.
-- While HA is not supported in K8s, this doesn't matter.
INSERT INTO controller_node (controller_id, dqlite_node_id, bind_address)
VALUES ('0', ?, '127.0.0.1');`, nodeID)
	}
}

func modelMigrationSchema() schema.Patch {
	return schema.MakePatch(`
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
);`)
}

func upgradeInfoSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE upgrade_info (
    uuid             TEXT PRIMARY KEY,
    previous_version TEXT NOT NULL,
    target_version   TEXT NOT NULL,
    created_at       TIMESTAMP NOT NULL,
    started_at       TIMESTAMP,
    completed_at     TIMESTAMP
);

-- A unique constraint over a contant index ensures only 1 entry matching the condition can exist
CREATE UNIQUE INDEX idx_singleton_active_upgrade ON upgrade_info ((1)) WHERE completed_at IS NULL;

CREATE TABLE upgrade_info_controller_node (
    uuid                      TEXT PRIMARY KEY,
    controller_node_id        TEXT NOT NULL,
    upgrade_info_uuid         TEXT NOT NULL,
    node_upgrade_completed_at TIMESTAMP,
    CONSTRAINT                fk_controller_node_id
        FOREIGN KEY               (controller_node_id)
        REFERENCES                controller_node(controller_id),
    CONSTRAINT                fk_upgrade_info
        FOREIGN KEY               (upgrade_info_uuid)
        REFERENCES                upgrade_info(uuid)
);

CREATE UNIQUE INDEX idx_upgrade_info_controller_node
ON upgrade_info_controller_node (controller_node_id, upgrade_info_uuid);
`)
}

func modelDefaults() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE model_defaults (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`)
}

func autocertCacheSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE autocert_cache (
    uuid 		TEXT PRIMARY KEY,
    name 		TEXT NOT NULL UNIQUE,
    data 		TEXT NOT NULL,
    encoding       	TEXT NOT NULL,
    CONSTRAINT 		fk_autocert_cache_encoding
        FOREIGN KEY 	    (encoding)
	REFERENCES 	    autocert_cache_encoding(id)
);

-- NOTE(nvinuesa): This table only populated with *one* hard-coded value
-- (x509) because golang's autocert cache doesn't provide encoding in it's
-- function signatures, and in juju we are only using x509 certs. The value
-- of this table is to correctly represent the domain and already have a
-- list of possible encodings when we update our code in the future.
CREATE TABLE autocert_cache_encoding (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL
);

INSERT INTO autocert_cache_encoding VALUES
    (0, 'x509');    -- Only x509 certs encoding supported today.
`)
}
