// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"github.com/juju/juju/core/database/schema"
)

type tableNamespaceID int

const (
	tableExternalController tableNamespaceID = iota
	tableControllerNode
	tableControllerConfig
	tableModelMigrationStatus
	tableModelMigrationMinionSync
	tableUpgradeInfo
	tableCloud
	tableCloudCredential
	tableAutocertCache
	tableUpgradeInfoControllerNode
	tableObjectStoreMetadata
	tableSecretBackendRotation
	tableModelMetadata
)

// ControllerDDL is used to create the controller database schema at bootstrap.
func ControllerDDL() *schema.Schema {
	patches := []func() schema.Patch{
		namespaceSchema,
		lifeSchema,
		leaseSchema,
		changeLogSchema,
		changeLogControllerNamespacesSchema,
		cloudSchema,
		externalControllerSchema,
		modelSchema,
		modelAgentSchema,
		controllerConfigSchema,
		controllerNodeTableSchema,
		modelMigrationSchema,
		upgradeInfoSchema,
		autocertCacheSchema,
		objectStoreMetadataSchema,
		userSchema,
		modelLastLogin,
		flagSchema,
		userPermissionSchema,
		secretBackendSchema,
	}

	patches = append(patches,
		changeLogTriggersForTable("cloud", "uuid", tableCloud),
		changeLogTriggersForTable("cloud_credential", "uuid", tableCloudCredential),
		changeLogTriggersForTable("external_controller", "uuid", tableExternalController),
		changeLogTriggersForTable("controller_config", "key", tableControllerConfig),
		changeLogTriggersForTable("controller_node", "controller_id", tableControllerNode),
		changeLogTriggersForTable("model_migration_status", "uuid", tableModelMigrationStatus),
		changeLogTriggersForTable("model_migration_minion_sync", "uuid", tableModelMigrationMinionSync),
		changeLogTriggersForTable("upgrade_info", "uuid", tableUpgradeInfo),
		changeLogTriggersForTable("upgrade_info_controller_node", "upgrade_info_uuid", tableUpgradeInfoControllerNode),
		changeLogTriggersForTable("object_store_metadata_path", "path", tableObjectStoreMetadata),
		changeLogTriggersForTable("secret_backend_rotation", "backend_uuid", tableSecretBackendRotation),
		changeLogTriggersForTable("model", "uuid", tableModelMetadata),

		// We need to ensure that the internal and kubernetes backends are immutable after
		// they are created by the controller during bootstrap time.
		// 0 is 'controller', 1 is 'kubernetes'.
		triggersForImmutableTable("secret_backend",
			"OLD.backend_type_id IN (0, 1)",
			"secret backends with type controller or kubernetes are immutable"),
	)

	ctrlSchema := schema.New()
	for _, fn := range patches {
		ctrlSchema.Add(fn())
	}

	return ctrlSchema
}

func namespaceSchema() schema.Patch {
	return schema.MakePatch(`
-- namespace_list maintains a list of tracked dqlite namespaces for the
-- controller.
CREATE TABLE namespace_list (
    namespace TEXT PRIMARY KEY
);
`)
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

func changeLogControllerNamespacesSchema() schema.Patch {
	// Note: These should match exactly the values of the tableNamespaceID
	// constants above.
	return schema.MakePatch(`
INSERT INTO change_log_namespace VALUES
    (0, 'external_controller', 'external controller changes based on the UUID'),
    (1, 'controller_node', 'controller node changes based on the controller ID'),
    (2, 'controller_config', 'controller config changes based on the key'),
    (3, 'model_migration_status', 'model migration status changes based on the UUID'),
    (4, 'model_migration_minion_sync', 'model migration minion sync changes based on the UUID'),
    (5, 'upgrade_info', 'upgrade info changes based on the UUID'),
    (6, 'cloud', 'cloud changes based on the UUID'),
    (7, 'cloud_credential', 'cloud credential changes based on the UUID'),
    (8, 'autocert_cache', 'autocert cache changes based on the UUID'),
    (9, 'upgrade_info_controller_node', 'upgrade info controller node changes based on the upgrade info UUID'),
    (10, 'object_store_metadata_path', 'object store metadata path changes based on the path'),
    (11, 'secret_backend_rotation', 'secret backend rotation changes based on the backend UUID and next rotation time'),
    (12, 'model', 'model changes based on the model UUID');
`)
}

func cloudSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE cloud_type (
    id   INT PRIMARY KEY,
    type TEXT NOT NULL
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
    (11, 'oauth2withcert'),
    (12, 'service-principal-secret');

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

-- v_cloud is used to fetch well constructed information about a cloud. This
-- view also includes information on whether the cloud is the controller
-- model's cloud.
CREATE VIEW v_cloud
AS
-- This selects the controller model's cloud uuid. We use this when loading
-- clouds to know if the cloud is the controllers cloud.
WITH controllers AS (
    SELECT m.cloud_uuid
    FROM model m
    INNER JOIN user u ON u.uuid = m.owner_uuid
    WHERE m.name = "controller"
    AND u.name = "admin"
    AND m.finalised = true
)
SELECT c.uuid,
       c.name,
       c.cloud_type_id,
       ct.type AS cloud_type,
       c.endpoint,
       c.identity_endpoint,
       c.storage_endpoint,
       c.skip_tls_verify,
       IIF(controllers.cloud_uuid IS NULL, false, true) AS is_controller_cloud
FROM cloud c
INNER JOIN cloud_type ct ON c.cloud_type_id = ct.id
LEFT JOIN controllers ON controllers.cloud_uuid = c.uuid;

-- v_cloud_auth is a connivance view similar to v_cloud but includes a row for
-- each cloud and auth type pair.
CREATE VIEW v_cloud_auth
AS
SELECT c.uuid,
       c.name,
       c.cloud_type_id,
       c.cloud_type,
       c.endpoint,
       c.identity_endpoint,
       c.storage_endpoint,
       c.skip_tls_verify,
       c.is_controller_cloud,
       at.id                  AS auth_type_id,
       at.type                AS auth_type
FROM v_cloud c
LEFT JOIN cloud_auth_type cat ON c.uuid = cat.cloud_uuid
JOIN auth_type at ON at.id = cat.auth_type_id;

CREATE TABLE cloud_defaults (
    cloud_uuid TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT,
    PRIMARY KEY (cloud_uuid, key),
    CONSTRAINT chk_key_empty CHECK (key != ""),
    CONSTRAINT fk_cloud_uuid
        FOREIGN KEY (cloud_uuid)
        REFERENCES cloud(uuid)
);

CREATE TABLE cloud_auth_type (
    cloud_uuid        TEXT NOT NULL,
    auth_type_id      INT NOT NULL,
    CONSTRAINT        fk_cloud_auth_type_cloud
        FOREIGN KEY       (cloud_uuid)
        REFERENCES        cloud(uuid),
    CONSTRAINT        fk_cloud_auth_type_auth_type
        FOREIGN KEY       (auth_type_id)
        REFERENCES        auth_type(id),
    PRIMARY KEY (cloud_uuid, auth_type_id)
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
    cloud_uuid        TEXT NOT NULL,
    ca_cert           TEXT NOT NULL,
    CONSTRAINT        fk_cloud_ca_cert_cloud
        FOREIGN KEY       (cloud_uuid)
        REFERENCES        cloud(uuid),
    PRIMARY KEY (cloud_uuid, ca_cert)
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
            REFERENCES          cloud(uuid),
        CONSTRAINT          fk_cloud_credential_auth_type
            FOREIGN KEY         (auth_type_id)
            REFERENCES          auth_type(id),
        CONSTRAINT          fk_cloud_credential_user
            FOREIGN KEY         (owner_uuid)
            REFERENCES          user(uuid)
);

CREATE UNIQUE INDEX idx_cloud_credential_cloud_uuid_owner_uuid
ON cloud_credential (cloud_uuid, owner_uuid, name);

-- view_cloud_credential provides a convenience view for accessing a
-- credentials uuid baseD on the natural key used to display the credential to
-- users.
CREATE VIEW v_cloud_credential
AS
SELECT cc.uuid,
       cc.cloud_uuid,
       c.name             AS cloud_name,
       cc.auth_type_id,
       at.type            AS auth_type,
       cc.owner_uuid,
       cc.name,
       cc.revoked,
       cc.invalid,
       cc.invalid_reason,
       c.name             AS cloud_name,
       u.name             AS owner_name
FROM cloud_credential cc
INNER JOIN cloud c ON c.uuid = cc.cloud_uuid
INNER JOIN user u ON u.uuid = cc.owner_uuid
INNER JOIN auth_type at ON cc.auth_type_id = at.id;

CREATE TABLE cloud_credential_attributes (
    cloud_credential_uuid TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT,
    PRIMARY KEY (cloud_credential_uuid, key),
    CONSTRAINT chk_key_empty CHECK (key != ""),
    CONSTRAINT fk_cloud_credential_uuid
        FOREIGN KEY (cloud_credential_uuid)
        REFERENCES cloud_credential(uuid)
);

-- v_cloud_credential_attributes is responsible for return a view of all cloud
-- credentials and their attributes repeated for every attribute.
CREATE VIEW v_cloud_credential_attributes
AS
SELECT uuid,
       cloud_uuid,
       auth_type_id,
       auth_type,
       owner_uuid,
       name,
       revoked,
       invalid,
       invalid_reason,
       cloud_name,
       owner_name,
       cca.key         AS attribute_key,
       cca.value       AS attribute_value
FROM v_cloud_credential
INNER JOIN cloud_credential_attributes cca ON uuid = cca.cloud_credential_uuid;
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

func modelSchema() schema.Patch {
	return schema.MakePatch(`
-- model_namespace is a mapping table from models to the corresponding dqlite
-- namespace database.
CREATE TABLE model_namespace (
    namespace  TEXT NOT NULL,
    model_uuid TEXT UNIQUE NOT NULL,
    CONSTRAINT fk_model_uuid
        FOREIGN KEY (model_uuid)
        REFERENCES  model(uuid)
);

CREATE UNIQUE INDEX idx_namespace_model_uuid ON model_namespace (namespace, model_uuid);

CREATE TABLE model_type (
    id INT PRIMARY KEY,
    type TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_model_type_type
ON model_type (type);

INSERT INTO model_type VALUES
    (0, 'iaas'),
    (1, 'caas');

CREATE TABLE model (
    uuid                  TEXT PRIMARY KEY,
-- finalised tells us if the model creation process has been completed and
-- we can use this model. The reason for this is model creation still happens
-- over several transactions with any one of them possibly failing. We write true
-- to this field when we are happy that the model can safely be used after all
-- operations have been completed.
    finalised             BOOLEAN DEFAULT FALSE NOT NULL,
    cloud_uuid            TEXT NOT NULL,
    cloud_region_uuid     TEXT,
    cloud_credential_uuid TEXT,
    model_type_id         INT NOT NULL,
    life_id               INT NOT NULL,
    name                  TEXT NOT NULL,
    owner_uuid            TEXT NOT NULL,
    CONSTRAINT            fk_model_cloud
        FOREIGN KEY           (cloud_uuid)
        REFERENCES            cloud(uuid),
    CONSTRAINT            fk_model_cloud_region
        FOREIGN KEY           (cloud_region_uuid)
        REFERENCES            cloud_region(uuid),
    CONSTRAINT            fk_model_cloud_credential
        FOREIGN KEY           (cloud_credential_uuid)
        REFERENCES            cloud_credential(uuid),
    CONSTRAINT            fk_model_model_type_id
        FOREIGN KEY           (model_type_id)
        REFERENCES            model_type(id)
    CONSTRAINT            fk_model_owner_uuid
        FOREIGN KEY           (owner_uuid)
        REFERENCES            user(uuid)
    CONSTRAINT            fk_model_life_id
        FOREIGN KEY           (life_id)
        REFERENCES            life(id)
);

-- idx_model_name_owner established an index that stops models being created
-- with the same name for a given owner.
CREATE UNIQUE INDEX idx_model_name_owner ON model (name, owner_uuid);
CREATE INDEX idx_model_finalised ON model (finalised);

--- v_model purpose is to provide an easy access mechanism for models in the
--- system. It will only show models that have been finalised so the caller does
--- not have to worry about retrieving half complete models.
CREATE VIEW v_model AS
SELECT m.uuid AS uuid,
       m.cloud_uuid,
       c.name            AS cloud_name,
       c.uuid            AS cloud_uuid,
       ct.type           AS cloud_type,
       c.endpoint        AS cloud_endpoint,
       c.skip_tls_verify AS cloud_skip_tls_verify,
       cr.uuid           AS cloud_region_uuid,
       cr.name           AS cloud_region_name,
       cc.uuid           AS cloud_credential_uuid,
       cc.name           AS cloud_credential_name,
       ccc.name          AS cloud_credential_cloud_name,
       cco.uuid          AS cloud_credential_owner_uuid,
       cco.name          AS cloud_credential_owner_name,
       m.model_type_id,
       mt.type          AS model_type,
       m.name AS name,
       m.owner_uuid,
       o.name           AS owner_name,
       l.value          AS life
FROM model m
INNER JOIN cloud c ON m.cloud_uuid = c.uuid
INNER JOIN cloud_type ct ON c.cloud_type_id = ct.id
LEFT JOIN cloud_region cr ON m.cloud_region_uuid = cr.uuid
LEFT JOIN cloud_credential cc ON m.cloud_credential_uuid = cc.uuid
LEFT JOIN cloud ccc ON cc.cloud_uuid = ccc.uuid
LEFT JOIN user cco ON cc.owner_uuid = cco.uuid
INNER JOIN model_type mt ON m.model_type_id = mt.id
INNER JOIN user o ON m.owner_uuid = o.uuid
INNER JOIN life l on m.life_id = l.id
WHERE m.finalised = true;
`)
}

func modelAgentSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE model_agent (
    model_uuid TEXT PRIMARY KEY,

    -- previous_version describes the agent version that was in use before the
    -- the current target_version.
    previous_version TEXT NOT NULL,

    -- target_version describes the desired agent version that should be
    -- being run in this model. It should not be considered "the" version that
    -- is being run for every agent as each agent needs to upgrade to this
    -- version.
    target_version TEXT NOT NULL,
    CONSTRAINT            fk_model_agent_model
        FOREIGN KEY           (model_uuid)
        REFERENCES            model(uuid)
);`)
}

func controllerConfigSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE controller_config (
    key     TEXT PRIMARY KEY,
    value   TEXT
);`)
}

func controllerNodeTableSchema() schema.Patch {
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

func modelLastLogin() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE model_last_login (
    model_uuid TEXT NOT NULL,
    user_uuid TEXT NOT NULL,
    time TIMESTAMP,
    PRIMARY KEY (model_uuid, user_uuid),
    CONSTRAINT            fk_model_last_login_model
        FOREIGN KEY           (model_uuid)
        REFERENCES            model(uuid),
    CONSTRAINT            fk_model_last_login_user
        FOREIGN KEY           (user_uuid)
        REFERENCES            user(uuid)
);

CREATE VIEW v_user_last_login AS
-- We cannot use MAX here because it returns a sqlite string value, not a
-- timestamp and this stops us scanning into time.Time.
SELECT time AS last_login, user_uuid
FROM model_last_login 
GROUP BY user_uuid
ORDER BY time DESC LIMIT 1;
`)
}

func upgradeInfoSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE upgrade_state_type (
    id   INT PRIMARY KEY,
    type TEXT
);

CREATE UNIQUE INDEX idx_upgrade_state_type_type
ON upgrade_state_type (type);

INSERT INTO upgrade_state_type VALUES
    (0, 'created'),
    (1, 'started'),
    (2, 'db-completed'),
    (3, 'steps-completed'),
    (4, 'error');

CREATE TABLE upgrade_info (
    uuid             TEXT PRIMARY KEY,
    previous_version TEXT NOT NULL,
    target_version   TEXT NOT NULL,
    state_type_id    INT NOT NULL,
    CONSTRAINT       fk_upgrade_info_upgrade_state_type
        FOREIGN KEY       (state_type_id)
        REFERENCES        upgrade_state_type(id)
);

-- A unique constraint over a constant index ensures only 1 entry matching the 
-- condition can exist. This states, that multiple upgrades can exist if they're
-- not active, but only one active upgrade can exist
CREATE UNIQUE INDEX idx_singleton_active_upgrade ON upgrade_info ((1)) WHERE state_type_id < 3;

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

func userSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE user (
    uuid            TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    display_name    TEXT,
    removed         BOOLEAN NOT NULL DEFAULT FALSE,
    created_by_uuid TEXT NOT NULL,
    created_at      TIMESTAMP NOT NULL,
    CONSTRAINT      fk_user_created_by_user
        FOREIGN KEY (created_by_uuid)
    REFERENCES      user(uuid)
);

CREATE UNIQUE INDEX idx_singleton_active_user ON user (name) WHERE removed IS FALSE;

CREATE TABLE user_authentication (
    user_uuid      TEXT PRIMARY KEY,
    disabled       BOOLEAN NOT NULL,
    CONSTRAINT     fk_user_authentication_user
        FOREIGN KEY (user_uuid)
    REFERENCES     user(uuid)
);

CREATE TABLE user_password (
    user_uuid       TEXT PRIMARY KEY,
    password_hash   TEXT NOT NULL,
    password_salt   TEXT NOT NULL,
    CONSTRAINT      fk_user_password_user
        FOREIGN KEY (user_uuid)
    REFERENCES      user_authentication(user_uuid)
);

CREATE TABLE user_activation_key (
    user_uuid       TEXT PRIMARY KEY,
    activation_key  TEXT NOT NULL,
    CONSTRAINT      fk_user_activation_key_user
        FOREIGN KEY (user_uuid)
    REFERENCES      user_authentication(user_uuid)
);

CREATE VIEW v_user_auth AS
SELECT u.uuid, 
       u.name, 
       u.display_name, 
       u.removed,
       u.created_by_uuid, 
       u.created_at,
       a.disabled
FROM   user u LEFT JOIN user_authentication a on u.uuid = a.user_uuid;
`)
}

func flagSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE flag (
    name TEXT PRIMARY KEY,
    value BOOLEAN DEFAULT 0,
    description TEXT NOT NULL
);
`)
}

func userPermissionSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE permission_access_type (
    id     INT PRIMARY KEY,
    type   TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_permission_access_type
ON permission_access_type (type);

-- Maps to the Access type in core/permission package.
INSERT INTO permission_access_type VALUES
    (0, 'read'),
    (1, 'write'),
    (2, 'consume'),
    (3, 'admin'),
    (4, 'login'),
    (5, 'add-model'),
    (6, 'superuser');

CREATE TABLE permission_object_type (
    id    INT PRIMARY KEY,
    type  TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_permission_object_type
ON permission_object_type (type);

-- Maps to the ObjectType type in core/permission package.
INSERT INTO permission_object_type VALUES
    (0, 'cloud'),
    (1, 'controller'),
    (2, 'model'),
    (3, 'offer');

CREATE TABLE permission_object_access (
    id              INT PRIMARY KEY,
    access_type_id  INT NOT NULL,
    object_type_id  INT NOT NULL,
    CONSTRAINT      fk_permission_access_type
        FOREIGN KEY (access_type_id)
        REFERENCES  permission_access_type(id),
    CONSTRAINT      fk_permission_object_type
        FOREIGN KEY (object_type_id)
        REFERENCES  permission_object_type(id)
);

CREATE UNIQUE INDEX idx_permission_object_access
ON permission_object_access (access_type_id, object_type_id);

INSERT INTO permission_object_access VALUES
    (0, 3, 0), -- admin, cloud
    (1, 5, 0), -- add-model, cloud
    (2, 4, 1), -- login, controller
    (3, 6, 1), -- superuser, controller
    (4, 0, 2), -- read, model
    (5, 1, 2), -- write, model
    (6, 3, 2), -- admin, model
    (7, 0, 3), -- read, offer
    (8, 2, 3), -- consume, offer
    (9, 3, 3); -- admin, offer

-- Column grant_to may extend to entities beyond users.
-- The name of the column is general, but for now we retain the FK constraint.
-- We will need to remove/replace it in the event of change
CREATE TABLE permission (
    uuid               TEXT PRIMARY KEY,
    access_type_id     INT NOT NULL,
    object_type_id     INT NOT NULL,
    grant_on  		   TEXT NOT NULL, -- name or uuid of the object
    grant_to           TEXT NOT NULL,
    CONSTRAINT         fk_permission_user_uuid
        FOREIGN KEY    (grant_to)
        REFERENCES     user(uuid),
    CONSTRAINT         fk_permission_object_access
        FOREIGN KEY    (access_type_id, object_type_id)
        REFERENCES     permission_object_access(access_type_id, object_type_id));

-- Allow only 1 combination of grant_on and grant_to
-- Otherwise we will get conflicting permissions.
CREATE UNIQUE INDEX idx_permission_type_to
ON permission (grant_on, grant_to);

-- All permissions
CREATE VIEW v_permission AS
SELECT p.uuid,
       p.grant_on,
       p.grant_to,
       at.type AS access_type,
       ot.type AS object_type
FROM   permission p
       JOIN permission_access_type at ON at.id = p.access_type_id
       JOIN permission_object_type ot ON ot.id = p.object_type_id;

-- All model permissions, verifying the model does exist.
CREATE VIEW v_permission_model AS
SELECT p.uuid,
       p.grant_on,
       p.grant_to,
       p.access_type,
       p.object_type
FROM   v_permission p
       INNER JOIN model ON model.uuid = p.grant_on
WHERE  p.object_type = 'model';

-- All controller cloud, verifying the cloud does exist.
CREATE VIEW v_permission_cloud AS
SELECT p.uuid,
       p.grant_on,
       p.grant_to,
       p.access_type,
       p.object_type
FROM   v_permission p
       INNER JOIN cloud ON cloud.name = p.grant_on
WHERE  p.object_type = 'cloud';

-- All controller permissions
CREATE VIEW v_permission_controller AS
SELECT p.uuid,
       p.grant_on,
       p.grant_to,
       p.access_type,
       p.object_type
FROM   v_permission p
WHERE  grant_on = 'controller' AND p.object_type = 'controller';

`)
}
