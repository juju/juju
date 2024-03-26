// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"fmt"

	"github.com/juju/juju/core/database/schema"
)

const (
	tableModelConfig tableNamespaceID = iota
	tableModelObjectStoreMetadata
	tableBlockDeviceMachine
	tableStorageAttachment
	tableFileSystem
	tableFileSystemAttachment
	tableVolume
	tableVolumeAttachment
	tableVolumeAttachmentPlan
	tableSecretAutoPrune
	tableSecretRotation
	tableSecretRevisionObsolete
	tableSecretRevisionExpire
)

// ModelDDL is used to create model databases.
func ModelDDL() *schema.Schema {
	patches := []func() schema.Patch{
		lifeSchema,
		changeLogSchema,
		changeLogModelNamespaceSchema,
		modelReadSchema,
		modelConfigSchema,
		objectStoreMetadataSchema,
		applicationSchema,
		charmSchema,
		nodeSchema,
		unitSchema,
		spaceSchema,
		subnetSchema,
		blockDeviceSchema,
		storageSchema,
		secretSchema,
		annotationModelSchema,
	}

	patches = append(patches,
		changeLogTriggersForTable("block_device", "machine_uuid", tableBlockDeviceMachine),
		changeLogTriggersForTable("model_config", "key", tableModelConfig),
		changeLogTriggersForTable("object_store_metadata_path", "path", tableModelObjectStoreMetadata),
		changeLogTriggersForTable("storage_attachment", "storage_instance_uuid", tableStorageAttachment),
		changeLogTriggersForTable("storage_filesystem", "uuid", tableFileSystem),
		changeLogTriggersForTable("storage_filesystem_attachment", "uuid", tableFileSystemAttachment),
		changeLogTriggersForTable("storage_volume", "uuid", tableVolume),
		changeLogTriggersForTable("storage_volume_attachment", "uuid", tableVolumeAttachment),
		changeLogTriggersForTable("storage_volume_attachment_plan", "uuid", tableVolumeAttachmentPlan),
		changeLogTriggersForTableOnColumn(
			"secret_metadata", "secret_id", "auto_prune", tableSecretAutoPrune),
		changeLogTriggersForTableOnColumn(
			"secret_rotation", "secret_id", "next_rotation_time", tableSecretRotation),
		changeLogTriggersForTableOnColumn(
			"secret_revision", "uuid", "obsolete", tableSecretRevisionObsolete),
		changeLogTriggersForTableOnColumn(
			"secret_revision_expire", "revision_uuid", "expire_time", tableSecretRevisionExpire),

		triggersForImmutableTable("model", "", "model table is immutable"),

		// Secret permissions do not allow subject or scope to be updated.
		triggersForImmutableTableUpdates("secret_permission",
			"OLD.subject_type_id <> NEW.subject_type_id OR OLD.scope_uuid <> NEW.scope_uuid OR OLD.scope_type_id <> NEW.scope_type_id",
			"secret permission subjects and scopes are immutable"),
	)

	patches = append(patches,
		annotationSchemaForTable("application"),
		annotationSchemaForTable("charm"),
		annotationSchemaForTable("machine"),
		annotationSchemaForTable("unit"),
		annotationSchemaForTable("storage_instance"),
		annotationSchemaForTable("storage_volume"),
		annotationSchemaForTable("storage_filesystem"),
	)

	modelSchema := schema.New()
	for _, fn := range patches {
		modelSchema.Add(fn())
	}
	return modelSchema
}

func annotationModelSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE annotation_model (
    key                 TEXT PRIMARY KEY,
    value               TEXT NOT NULL
);`[1:])
}

func annotationSchemaForTable(table string) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
CREATE TABLE annotation_%[1]s (
    uuid                TEXT NOT NULL,
    key                 TEXT NOT NULL,
    value               TEXT NOT NULL,
    PRIMARY KEY         (uuid, key)
    -- Following needs to be uncommented when we do have the
    -- annotatables as real domain entities.
    -- CONSTRAINT          fk_annotation_%[1]s
    --     FOREIGN KEY     (uuid)
    --     REFERENCES      %[1]s(uuid)
);`[1:], table))
	}
}

func changeLogModelNamespaceSchema() schema.Patch {
	// Note: These should match exactly the values of the tableNamespaceID
	// constants above.
	return schema.MakePatch(`
INSERT INTO change_log_namespace VALUES
    (0, 'model_config', 'Model configuration changes based on key'),
    (1, 'object_store_metadata_path', 'Object store metadata path changes based on the path'),
    (2, 'block_device', 'Block device changes based on the machine UUID'),
    (3, 'storage_attachment', 'Storage attachment changes based on the storage instance UUID'),
    (4, 'storage_filesystem', 'File system changes based on UUID'),
    (5, 'storage_filesystem_attachment', 'File system attachment changes based on UUID'),
    (6, 'storage_volume', 'Storage volume changes based on UUID'),
    (7, 'storage_volume_attachment', 'Volume attachment changes based on UUID'),
    (8, 'storage_volume_attachment_plan', 'Volume attachment plan changes based on UUID'),
    (9, 'secret', 'Secret auto prune changes based on UUID'),
    (10, 'secret_rotation', 'Secret rotation changes based on UUID'),
    (11, 'secret_revision', 'Secret revision obsolete changes based on UUID'),
    (12, 'secret_revision_expire', 'Secret revision next expire time changes based on UUID'),
    (13, 'secret_unit_consumer', 'Secret unit consumer current revision changes based on UUID'),
    (14, 'secret_remote_unit_consumer', 'Secret remote unit consumer current revision changes based on UUID');
`)
}

func modelReadSchema() schema.Patch {
	return schema.MakePatch(`
-- The model table represents a readonly denormalised model data. The intended
-- use is to provide a read-only view of the model data for the purpose of
-- accessing common model data without the need to span multiple databases.
CREATE TABLE model (
    uuid             TEXT PRIMARY KEY,
    controller_uuid  TEXT NOT NULL,
    name             TEXT NOT NULL,
    type             TEXT NOT NULL,
    cloud            TEXT NOT NULL,
    cloud_region     TEXT,
    credential_owner TEXT,
    credential_name  TEXT
);

-- A unique constraint over a constant index ensures only 1 entry matching the
-- condition can exist.
CREATE UNIQUE INDEX idx_singleton_model ON model ((1));
`)
}

func modelConfigSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE model_config (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`)
}

func spaceSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE space (
    uuid            TEXT PRIMARY KEY,
    name            TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_spaces_uuid_name
ON space (name);

CREATE TABLE provider_space (
    provider_id     TEXT PRIMARY KEY,
    space_uuid      TEXT NOT NULL,
    CONSTRAINT      fk_provider_space_space_uuid
        FOREIGN KEY     (space_uuid)
        REFERENCES      space(uuid)
);

CREATE UNIQUE INDEX idx_provider_space_space_uuid
ON provider_space (space_uuid);

INSERT INTO space VALUES
    (0, 'alpha');
`)
}

func subnetSchema() schema.Patch {
	return schema.MakePatch(`
--  subnet                                    subnet_type
-- +-----------------------+                 +-------------------------+
-- |*uuid              text|                 |*id                  text|
-- |cidr               text|1               1|name                 text|
-- |vlan_tag            int+-----------------+is_usable         boolean|
-- |space_uuid         text|                 |is_space_settable boolean|
-- |subnet_type_uuid   text|                 +------------+------------+
-- +---------+-------------+                              |1
--           |1                                           |
--           |                                            |
--           |                                            |
--           |                                            |
--           |n                                           |n
--  subnet_association                        subnet_type_association_type
-- +---------------------------+             +------------------------------+
-- |*subject_subnet_uuid   text|             |*subject_subnet_type_id   text|
-- |associated_subnet_uuid text|             |associated_subnet_type_id text|
-- |association_type_id    text|             |association_type_id       text|
-- +---------+-----------------+             +------------+-----------------+
--           |1                                           |1
--           |                                            |
--           |                                            |
--           |                                            |
--           |1                                           |
--  subnet_association_type                               |
-- +-----------------------+                              |
-- |*id                text+------------------------------+
-- |name               text|1
-- +-----------------------+
CREATE TABLE subnet (
    uuid                         TEXT PRIMARY KEY,
    cidr                         TEXT NOT NULL,
    vlan_tag                     INT,
    space_uuid                   TEXT,
    subnet_type_id               INT,
    CONSTRAINT                   fk_subnets_spaces
        FOREIGN KEY                  (space_uuid)
        REFERENCES                   space(uuid),
    CONSTRAINT                   fk_subnet_types
        FOREIGN KEY                  (subnet_type_id)
        REFERENCES                   subnet_type(id)
);

CREATE TABLE subnet_type (
    id                           INT PRIMARY KEY,
    name                         TEXT NOT NULL,
    is_usable                    BOOLEAN,
    is_space_settable            BOOLEAN
);

INSERT INTO subnet_type VALUES
    (0, 'base', true, true),    -- The base (or standard) subnet type. If another subnet is an overlay of a base subnet in fan bridging, then the base subnet is the underlay in fan terminology.
    (1, 'fan_overlay_segment', true, false);

CREATE TABLE subnet_association_type (
    id                           INT PRIMARY KEY,
    name                         TEXT NOT NULL
);

INSERT INTO subnet_association_type VALUES
    (0, 'overlay_of');    -- The subnet is an overlay of other (an underlay) subnet.

CREATE TABLE subnet_type_association_type (
    subject_subnet_type_id         INT PRIMARY KEY,
    associated_subnet_type_id      INT NOT NULL,
    association_type_id            INT NOT NULL,
    CONSTRAINT                     fk_subject_subnet_type_id
        FOREIGN KEY                    (subject_subnet_type_id)
        REFERENCES                     subnet_type(id),
    CONSTRAINT                     fk_associated_subnet_type_id
        FOREIGN KEY                    (associated_subnet_type_id)
        REFERENCES                     subnet_association_type(id),
    CONSTRAINT                     fk_association_type_id
        FOREIGN KEY                    (association_type_id)
        REFERENCES                     subnet_association_type(id)
);

INSERT INTO subnet_type_association_type VALUES
    (1, 0, 0);    -- This reference "allowable" association means that a 'fan_overlay' subnet can only be an overlay of a 'base' subnet.

CREATE TABLE subnet_association (
    subject_subnet_uuid            TEXT PRIMARY KEY,
    associated_subnet_uuid         TEXT NOT NULL,
    association_type_id            INT NOT NULL,
    CONSTRAINT                     fk_subject_subnet_uuid
        FOREIGN KEY                    (subject_subnet_uuid)
        REFERENCES                     subnet(uuid),
    CONSTRAINT                     fk_associated_subnet_uuid
        FOREIGN KEY                    (associated_subnet_uuid)
        REFERENCES                     subnet(uuid),
    CONSTRAINT                     fk_association_type_id
        FOREIGN KEY                    (association_type_id)
        REFERENCES                     subnet_association_type(id)
);

CREATE UNIQUE INDEX idx_subnet_association
ON subnet_association (subject_subnet_uuid, associated_subnet_uuid);

CREATE TABLE provider_subnet (
    provider_id     TEXT PRIMARY KEY,
    subnet_uuid     TEXT NOT NULL,
    CONSTRAINT      fk_provider_subnet_subnet_uuid
        FOREIGN KEY     (subnet_uuid)
        REFERENCES      subnet(uuid)
);

CREATE UNIQUE INDEX idx_provider_subnet_subnet_uuid
ON provider_subnet (subnet_uuid);

CREATE TABLE provider_network (
    uuid                TEXT PRIMARY KEY,
    provider_network_id TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_provider_network_id
ON provider_network (provider_network_id);

CREATE TABLE provider_network_subnet (
    provider_network_uuid TEXT PRIMARY KEY,
    subnet_uuid           TEXT NOT NULL,
    CONSTRAINT            fk_provider_network_subnet_provider_network_uuid
        FOREIGN KEY           (provider_network_uuid)
        REFERENCES            provider_network(uuid),
    CONSTRAINT            fk_provider_network_subnet_uuid
        FOREIGN KEY           (subnet_uuid)
        REFERENCES            subnet(uuid)
);

CREATE UNIQUE INDEX idx_provider_network_subnet_uuid
ON provider_network_subnet (subnet_uuid);

CREATE TABLE availability_zone (
    uuid            TEXT PRIMARY KEY,
    name            TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_availability_zone_name
ON availability_zone (name);

CREATE TABLE availability_zone_subnet (
    availability_zone_uuid TEXT NOT NULL,
    subnet_uuid            TEXT NOT NULL,
    PRIMARY KEY            (availability_zone_uuid, subnet_uuid),
    CONSTRAINT             fk_availability_zone_availability_zone_uuid
        FOREIGN KEY            (availability_zone_uuid)
        REFERENCES             availability_zone(uuid),
    CONSTRAINT             fk_availability_zone_subnet_uuid
        FOREIGN KEY            (subnet_uuid)
        REFERENCES             subnet(uuid)
);`)
}

func applicationSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE application (
    uuid    TEXT PRIMARY KEY,
    name    TEXT NOT NULL,
    life_id INT NOT NULL,
    CONSTRAINT      fk_application_life
        FOREIGN KEY (life_id)
        REFERENCES  life(id)
);

CREATE UNIQUE INDEX idx_application_name
ON application (name);
`)
}

func charmSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE charm (
    uuid    TEXT PRIMARY KEY,
    url     TEXT NOT NULL
);

CREATE TABLE charm_storage (
    charm_uuid       TEXT NOT NULL,
    name             TEXT NOT NULL,
    description      TEXT,
    storage_kind_id  INT NOT NULL,
    shared           BOOLEAN,
    read_only        BOOLEAN,
    count_min        INT NOT NULL,
    count_max        INT NOT NULL,
    minimum_size_mib INT,
    location         TEXT,
    CONSTRAINT       fk_storage_instance_kind
        FOREIGN KEY  (storage_kind_id)
        REFERENCES   storage_kind(id),
    CONSTRAINT       fk_charm_storage_charm
        FOREIGN KEY  (charm_uuid)
        REFERENCES   charm(uuid),
    PRIMARY KEY (charm_uuid, name)
);

CREATE INDEX idx_charm_storage_charm
ON charm_storage (charm_uuid);
`)
}

func nodeSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE net_node (
    uuid TEXT PRIMARY KEY
);

CREATE TABLE machine (
    uuid            TEXT PRIMARY KEY,
    machine_id      TEXT NOT NULL,
    net_node_uuid   TEXT NOT NULL,
    life_id         INT NOT NULL,
    CONSTRAINT      fk_machine_net_node
        FOREIGN KEY (net_node_uuid)
        REFERENCES  net_node(uuid),
    CONSTRAINT      fk_machine_life
        FOREIGN KEY (life_id)
        REFERENCES  life(id)
);

CREATE UNIQUE INDEX idx_machine_id
ON machine (machine_id);

CREATE UNIQUE INDEX idx_machine_net_node
ON machine (net_node_uuid);

CREATE TABLE cloud_service (
    uuid             TEXT PRIMARY KEY,
    net_node_uuid    TEXT NOT NULL,
    application_uuid TEXT NOT NULL,
    CONSTRAINT       fk_cloud_service_net_node
        FOREIGN KEY  (net_node_uuid)
        REFERENCES   net_node(uuid),
    CONSTRAINT       fk_cloud_application
        FOREIGN KEY  (application_uuid)
        REFERENCES   application(uuid)
);

CREATE UNIQUE INDEX idx_cloud_service_net_node
ON cloud_service (net_node_uuid);

CREATE UNIQUE INDEX idx_cloud_service_application
ON cloud_service (application_uuid);

CREATE TABLE cloud_container (
    uuid            TEXT PRIMARY KEY,
    net_node_uuid   TEXT NOT NULL,
    CONSTRAINT      fk_cloud_container_net_node
        FOREIGN KEY (net_node_uuid)
        REFERENCES  net_node(uuid)
);

CREATE UNIQUE INDEX idx_cloud_container_net_node
ON cloud_container (net_node_uuid);
`)
}

func unitSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE unit (
    uuid             TEXT PRIMARY KEY,
    unit_id          TEXT NOT NULL,
    application_uuid TEXT NOT NULL,
    net_node_uuid    TEXT NOT NULL,
    life_id          INT NOT NULL,
    CONSTRAINT       fk_unit_application
        FOREIGN KEY  (application_uuid)
        REFERENCES   application(uuid),
    CONSTRAINT       fk_unit_net_node
        FOREIGN KEY  (net_node_uuid)
        REFERENCES   net_node(uuid),
    CONSTRAINT       fk_unit_life
        FOREIGN KEY  (life_id)
        REFERENCES   life(id)
);

CREATE UNIQUE INDEX idx_unit_id
ON unit (unit_id);

CREATE INDEX idx_unit_application
ON unit (application_uuid);

CREATE UNIQUE INDEX idx_unit_net_node
ON unit (net_node_uuid);
`)
}

func blockDeviceSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE block_device (
    uuid               TEXT PRIMARY KEY,
    machine_uuid       TEXT NOT NULL,
    name               TEXT NOT NULL,
    label              TEXT,
    device_uuid        TEXT,
    hardware_id        TEXT,
    wwn                TEXT,
    bus_address        TEXT,
    serial_id          TEXT,
    filesystem_type_id INT,
    size_mib           INT,
    mount_point        TEXT,
    in_use             BOOLEAN,
    CONSTRAINT         fk_filesystem_type
        FOREIGN KEY    (filesystem_type_id)
        REFERENCES     filesystem_type(id),
    CONSTRAINT         fk_block_device_machine
        FOREIGN KEY    (machine_uuid)
        REFERENCES     machine(uuid)
);

CREATE UNIQUE INDEX idx_block_device_name
ON block_device (machine_uuid, name);

CREATE TABLE filesystem_type (
    id   INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_filesystem_type_name
ON filesystem_type (name);

INSERT INTO filesystem_type VALUES
    (0, 'unspecified'),
    (1, 'vfat'),
    (2, 'ext4'),
    (3, 'xfs'),
    (4, 'btrfs'),
    (5, 'zfs'),
    (6, 'jfs'),
    (7, 'squashfs'),
    (8, 'bcachefs');

CREATE TABLE block_device_link_device (
    block_device_uuid TEXT NOT NULL,
    name              TEXT NOT NULL,
    CONSTRAINT        fk_block_device_link_device
        FOREIGN KEY   (block_device_uuid)
        REFERENCES    block_device(uuid),
    PRIMARY KEY (block_device_uuid, name)
);

CREATE UNIQUE INDEX idx_block_device_link_device
ON block_device_link_device (block_device_uuid, name);

CREATE INDEX idx_block_device_link_device_device
ON block_device_link_device (block_device_uuid);
`)
}

func storageSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE storage_pool (
    uuid TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    -- Types are provider sourced, so we do not use a lookup with ID.
    -- This constitutes "repeating data" and would tend to indicate 
    -- bad relational design. However we choose that here over the
    -- burden of:
    --   - Knowing every possible type up front to populate a look-up or;
    --   - Sourcing the lookup from the provider and keeping it updated. 
    type TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_storage_pool_name
ON storage_pool (name);

CREATE TABLE storage_pool_attribute (
    storage_pool_uuid TEXT NOT NULL,
    key               TEXT NOT NULL,
    value             TEXT NOT NULL,
    CONSTRAINT       fk_storage_pool_attribute_pool
        FOREIGN KEY  (storage_pool_uuid)
        REFERENCES   storage_pool(uuid),
    PRIMARY KEY (storage_pool_uuid, key)
);

CREATE TABLE storage_kind (
    id          INT PRIMARY KEY,
    kind        TEXT NOT NULL,
    description TEXT                     
);

CREATE UNIQUE INDEX idx_storage_kind
ON storage_kind (kind);

INSERT INTO storage_kind VALUES
    (0, 'block', 'Allows for the creation of raw storage volumes'), 
    (1, 'filesystem', 'Provides a hierarchical file storage system');

-- This table stores storage directive values for each named storage item
-- defined by the application's current charm. If the charm is updated, then
-- so too will be the rows in this table to reflect the current charm's
-- storage definitions.
CREATE TABLE application_storage_directive (
    application_uuid TEXT NOT NULL,
    charm_uuid       TEXT NOT NULL,
    storage_name     TEXT NOT NULL,
    -- These attributes are filled in by sourcing data from:
    -- user supplied, model config, charm config, opinionated fallbacks.
    -- By the time the row is written, all values are known.
    -- Directive value attributes (pool, size, count) hitherto have
    -- been fixed (since first implemented). We don't envisage
    -- any change to how these are modelled.
    --
    -- Note: one might wonder why storage_pool below is not a
    -- FK to a row defined in the storage pool table. This value
    -- can also be one of the pool types. As with the comment on the
    -- type column in the storage pool table, it's problematic to use a lookup
    -- with an ID. Storage pools, once created, cannot be renamed so
    -- this will not be able to become "orphaned".
    storage_pool TEXT NOT NULL,
    size         INT  NOT NULL,
    count        INT  NOT NULL,
    CONSTRAINT       fk_application_storage_directive_application
        FOREIGN KEY  (application_uuid)
        REFERENCES   application(uuid),
    CONSTRAINT       fk_application_storage_directive_charm_storage
        FOREIGN KEY  (charm_uuid, storage_name)
        REFERENCES   charm_storage(charm_uuid, name),
    PRIMARY KEY (application_uuid, charm_uuid, storage_name)
);

-- Note that this is not unique; it speeds access by application.
CREATE INDEX idx_application_storage_directive
ON application_storage_directive (application_uuid);

-- This table stores storage directive values for each named storage item
-- defined by the unit's current charm. If the charm is updated, then
-- so too will be the rows in this table to reflect the current charm's
-- storage definitions.
-- Note: usually we just get the storage directives off the application
-- but need to allow for a unit's charm to temporarily diverge from that
-- of its application.
CREATE TABLE unit_storage_directive (
    unit_uuid    TEXT NOT NULL,
    charm_uuid   TEXT NOT NULL,
    storage_name TEXT NOT NULL,
    -- These attributes are filled in by sourcing data from:
    -- user supplied, model config, charm config, opinionated fallbacks.
    -- By the time the row is written, all values are known.
    -- Directive value attributes (pool, size, count) hitherto have
    -- been fixed (since first implemented). We don't envisage
    -- any change to how these are modelled.
    --
    -- Note: one might wonder why storage_pool below is not a
    -- FK to a row defined in the storage pool table. This value
    -- can also be one of the pool types. As with the comment on the
    -- type column in the storage pool table, it's problematic to use a lookup
    -- with an ID. Storage pools, once created, cannot be renamed so
    -- this will not be able to become "orphaned".
    storage_pool TEXT NOT NULL,
    size         INT  NOT NULL,
    count        INT  NOT NULL,
    CONSTRAINT       fk_unit_storage_directive_charm_storage
        FOREIGN KEY  (charm_uuid, storage_name)
        REFERENCES   charm_storage(charm_uuid, name),
    PRIMARY KEY (unit_uuid, charm_uuid, storage_name)
);

-- Note that this is not unique; it speeds access by unit.
CREATE INDEX idx_unit_storage_directive
ON unit_storage_directive (unit_uuid);

CREATE TABLE storage_instance (
    uuid            TEXT PRIMARY KEY,
    storage_kind_id INT NOT NULL,
    name            TEXT NOT NULL,
    life_id         INT NOT NULL,
    -- Note: one might wonder why storage_pool below is not a
    -- FK to a row defined in the storage pool table. This value
    -- can also be one of the pool types. As with the comment on the
    -- type column in the storage pool table, it's problematic to use a lookup
    -- with an ID. Storage pools, once created, cannot be renamed so
    -- this will not be able to become "orphaned".
    storage_pool TEXT NOT NULL,
    CONSTRAINT       fk_storage_instance_kind
        FOREIGN KEY  (storage_kind_id)
        REFERENCES   storage_kind(id),
    CONSTRAINT       fk_storage_instance_life
        FOREIGN KEY  (life_id)
        REFERENCES   life(id)
);

-- storage_unit_owner is used to indicate when
-- a unit is the owner of a storage instance.
-- This is different to a storage attachment.
CREATE TABLE storage_unit_owner (
    storage_instance_uuid TEXT PRIMARY KEY,
    unit_uuid             TEXT NOT NULL,
    CONSTRAINT       fk_storage_owner_storage
        FOREIGN KEY  (storage_instance_uuid)
        REFERENCES   storage_instance(uuid),
    CONSTRAINT       fk_storage_owner_unit
        FOREIGN KEY  (unit_uuid)
        REFERENCES   unit(uuid)
);

CREATE TABLE storage_attachment (
    storage_instance_uuid TEXT PRIMARY KEY,
    unit_uuid             TEXT NOT NULL,
    life_id               INT NOT NULL,  
    CONSTRAINT       fk_storage_owner_storage
        FOREIGN KEY  (storage_instance_uuid)
        REFERENCES   storage_instance(uuid),
    CONSTRAINT       fk_storage_owner_unit
        FOREIGN KEY  (unit_uuid)
        REFERENCES   unit(uuid),
    CONSTRAINT      fk_storage_attachment_life
        FOREIGN KEY (life_id)
        REFERENCES  life(id)
);

-- Note that this is not unique; it speeds access by unit.
CREATE INDEX idx_storage_attachment_unit
ON storage_attachment (unit_uuid);

CREATE TABLE storage_provisioning_status (
    id          INT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT
);

CREATE UNIQUE INDEX idx_storage_provisioning_status
ON storage_provisioning_status (name);

INSERT INTO storage_provisioning_status VALUES
    (0, 'pending', 'Creation or attachment is awaiting completion'), 
    (1, 'provisioned', 'Requested creation or attachment has been completed'),
    (2, 'error', 'An error was encountered during creation or attachment'); 

CREATE TABLE storage_volume (
    uuid                   TEXT PRIMARY KEY,
    life_id                INT NOT NULL,
    name                   TEXT NOT NULL,
    provider_id            TEXT,
    storage_pool_uuid      TEXT,
    size_mib               INT,
    hardware_id            TEXT,
    wwn                    TEXT,
    persistent             BOOLEAN,
    provisioning_status_id INT NOT NULL,
    CONSTRAINT      fk_storage_instance_life
        FOREIGN KEY (life_id)
        REFERENCES  life(id),
    CONSTRAINT      fk_storage_volume_pool
        FOREIGN KEY (storage_pool_uuid)
        REFERENCES  storage_pool(uuid),
    CONSTRAINT      fk_storage_vol_provisioning_status
        FOREIGN KEY (provisioning_status_id)
        REFERENCES  storage_provisioning_status(id)
);

-- An instance can have at most one volume.
-- A volume can have at most one instance.
CREATE TABLE storage_instance_volume (
    storage_instance_uuid TEXT PRIMARY KEY,
    storage_volume_uuid   TEXT NOT NULL,
    CONSTRAINT       fk_storage_instance_volume_instance
        FOREIGN KEY  (storage_instance_uuid)
        REFERENCES   storage_instance(uuid),
    CONSTRAINT       fk_storage_instance_volume_volume
        FOREIGN KEY  (storage_volume_uuid)
        REFERENCES   storage_volume(uuid)
);

CREATE UNIQUE INDEX idx_storage_instance_volume
ON storage_instance_volume (storage_volume_uuid);

CREATE TABLE storage_volume_attachment (
    uuid                   TEXT PRIMARY KEY,
    storage_volume_uuid    TEXT NOT NULL,
    net_node_uuid          TEXT NOT NULL,
    life_id                INT NOT NULL,
    block_device_uuid      TEXT,
    read_only              BOOLEAN,
    provisioning_status_id INT NOT NULL,
    CONSTRAINT       fk_storage_volume_attachment_vol
        FOREIGN KEY  (storage_volume_uuid)
        REFERENCES   storage_volume(uuid),
    CONSTRAINT       fk_storage_volume_attachment_node
        FOREIGN KEY  (net_node_uuid)
        REFERENCES   net_node(uuid),
    CONSTRAINT       fk_storage_volume_attachment_life
        FOREIGN KEY  (life_id)
        REFERENCES   life(id),
    CONSTRAINT       fk_storage_volume_attachment_block
        FOREIGN KEY  (block_device_uuid)
        REFERENCES   block_device(uuid),
    CONSTRAINT       fk_storage_vol_att_provisioning_status
        FOREIGN KEY  (provisioning_status_id)
        REFERENCES   storage_provisioning_status(id)
);

CREATE TABLE storage_filesystem (
    uuid                   TEXT PRIMARY KEY,
    life_id                INT NOT NULL,
    provider_id            TEXT,
    storage_pool_uuid      TEXT,
    size_mib               INT,
    provisioning_status_id INT NOT NULL,
    CONSTRAINT       fk_storage_instance_life
        FOREIGN KEY  (life_id)
        REFERENCES   life(id),
    CONSTRAINT       fk_storage_filesystem_pool
        FOREIGN KEY  (storage_pool_uuid)
        REFERENCES   storage_pool(uuid),
    CONSTRAINT       fk_storage_fs_provisioning_status
        FOREIGN KEY  (provisioning_status_id)
        REFERENCES   storage_provisioning_status(id)
);

-- An instance can have at most one filesystem.
-- A filesystem can have at most one instance.
CREATE TABLE storage_instance_filesystem (
    storage_instance_uuid   TEXT PRIMARY KEY,
    storage_filesystem_uuid TEXT NOT NULL,
    CONSTRAINT       fk_storage_instance_filesystem_instance
        FOREIGN KEY  (storage_instance_uuid)
        REFERENCES   storage_instance(uuid),
    CONSTRAINT       fk_storage_instance_filesystem_fs
        FOREIGN KEY  (storage_filesystem_uuid)
        REFERENCES   storage_filesystem(uuid)
);

CREATE UNIQUE INDEX idx_storage_instance_filesystem
ON storage_instance_filesystem (storage_filesystem_uuid);

CREATE TABLE storage_filesystem_attachment (
    uuid                    TEXT PRIMARY KEY,
    storage_filesystem_uuid TEXT NOT NULL,
    net_node_uuid           TEXT NOT NULL,
    life_id                 INT NOT NULL,
    mount_point             TEXT,
    read_only               BOOLEAN, 
    provisioning_status_id  INT NOT NULL,
    CONSTRAINT       fk_storage_filesystem_attachment_fs
        FOREIGN KEY  (storage_filesystem_uuid)
        REFERENCES   storage_filesystem(uuid),
    CONSTRAINT       fk_storage_filesystem_attachment_node
        FOREIGN KEY  (net_node_uuid)
        REFERENCES   net_node(uuid),
    CONSTRAINT       fk_storage_filesystem_attachment_life
        FOREIGN KEY  (life_id)
        REFERENCES   life(id),
    CONSTRAINT       fk_storage_fs_provisioning_status
        FOREIGN KEY  (provisioning_status_id)
        REFERENCES   storage_provisioning_status(id)
);

CREATE TABLE storage_volume_device_type (
    id          INT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT
);

CREATE UNIQUE INDEX idx_storage_volume_dev_type
ON storage_volume_device_type (name);

INSERT INTO storage_volume_device_type VALUES
    (0, 'local', 'Default device type for on-machine volume attachments'), 
    (1, 'iscsi', 'iSCSI protocol for linking storage');

CREATE TABLE storage_volume_attachment_plan (
    uuid                   TEXT PRIMARY KEY,
    storage_volume_uuid    TEXT NOT NULL,
    net_node_uuid          TEXT NOT NULL,
    life_id                INT NOT NULL,
    device_type_id         INT,
    block_device_uuid      TEXT,
    provisioning_status_id INT NOT NULL,
    CONSTRAINT       fk_storage_volume_attachment_plan_vol
        FOREIGN KEY  (storage_volume_uuid)
        REFERENCES   storage_volume(uuid),
    CONSTRAINT       fk_storage_volume_attachment_plan_node
        FOREIGN KEY  (net_node_uuid)
        REFERENCES   net_node(uuid),
    CONSTRAINT       fk_storage_volume_attachment_plan_life
        FOREIGN KEY  (life_id)
        REFERENCES   life(id),
    CONSTRAINT       fk_storage_volume_attachment_plan_device
        FOREIGN KEY  (device_type_id)
        REFERENCES   storage_volume_device_type(id),
    CONSTRAINT       fk_storage_volume_attachment_plan_block
        FOREIGN KEY  (block_device_uuid)
        REFERENCES   block_device(uuid),
    CONSTRAINT       fk_storage_fs_provisioning_status
        FOREIGN KEY  (provisioning_status_id)
        REFERENCES   storage_provisioning_status(id)
);

CREATE TABLE storage_volume_attachment_plan_attr (
    uuid                 TEXT PRIMARY KEY, 
    attachment_plan_uuid TEXT NOT NULL,
    key                  TEXT NOT NULL,
    value                TEXT NOT NULL,
    CONSTRAINT       fk_storage_vol_attach_plan_attr_plan
        FOREIGN KEY  (attachment_plan_uuid)
        REFERENCES   storage_volume_attachment_plan(attachment_plan_uuid)
);

CREATE UNIQUE INDEX idx_storage_vol_attachment_plan_attr
ON storage_volume_attachment_plan_attr (attachment_plan_uuid, key);
`)
}
