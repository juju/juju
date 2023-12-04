// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"github.com/juju/juju/core/database/schema"
)

const (
	tableModelConfig tableNamespaceID = iota + 1
	tableModelObjectStoreMetadata
)

// ModelDDL is used to create model databases.
func ModelDDL() *schema.Schema {
	patches := []func() schema.Patch{
		lifeSchema,
		changeLogSchema,
		changeLogModelNamespace,
		modelConfig,
		changeLogTriggersForTable("model_config", "key", tableModelConfig),
		objectStoreMetadataSchema,
		changeLogTriggersForTable("object_store_metadata_path", "path", tableModelObjectStoreMetadata),
		applicationSchema,
		nodeSchema,
		unitSchema,
		spaceSchema,
		subnetSchema,
		blockDeviceSchema,
		storageSchema,
	}

	schema := schema.New()
	for _, fn := range patches {
		schema.Add(fn())
	}
	return schema
}

func lifeSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE life (
    id    INT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO life VALUES
    (0, 'alive'), 
    (1, 'dying'),
    (2, 'dead');
`)
}

func changeLogModelNamespace() schema.Patch {
	// Note: These should match exactly the values of the tableNamespaceID
	// constants above.
	return schema.MakePatch(`
INSERT INTO change_log_namespace VALUES
    (1, 'model_config', 'model config changes based on config key'),
    (2, 'object_store_metadata_path', 'object store metadata path changes based on the path');
`)
}

func modelConfig() schema.Patch {
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
    name            TEXT NOT NULL,
    is_public       BOOLEAN
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
`)
}

func subnetSchema() schema.Patch {
	return schema.MakePatch(`
--  subnet                                    subnet_type
-- +-----------------------+                 +-------------------------+
-- |*uuid              text|                 |*uuid                text|
-- |cidr               text|1               1|name                 text|
-- |vlan_tag            int+-----------------+is_usable         boolean|
-- |is_public       boolean|                 |is_space_settable boolean|
-- |space_uuid         text|                 +------------+------------+
-- |subnet_type_uuid   text|                              |1
-- +---------+-------------+                              |
--           |1                                           |
--           |                                            |
--           |                                            |
--           |                                            |
--           |n                                           |n
--  subnet_association                        subject_subnet_type_uuid
-- +---------------------------+             +--------------------------------+
-- |*subject_subnet_uuid   text|             |*subject_subnet_type_uuid   text|
-- |associated_subnet_uuid text|             |associated_subnet_type_uuid text|
-- |association_type_uuid  text|             |association_type_uuid       text|
-- +---------+-----------------+             +------------+-------------------+
--           |1                                           |1
--           |                                            |
--           |                                            |
--           |                                            |
--           |1                                           |
--  subnet_association_type                               |
-- +-----------------------+                              |
-- |*uuid              text+------------------------------+
-- |name               text|1
-- +-----------------------+
CREATE TABLE subnet (
    uuid                         TEXT PRIMARY KEY,
    cidr                         TEXT NOT NULL,
    vlan_tag                     INT,
    is_public                    BOOLEAN,
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
    (1, 'fan_overlay', false, false),    
    (2, 'fan_overlay_segment', true, true);

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
    provider_network_id TEXT
);

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
    name            TEXT
);

CREATE TABLE availability_zone_subnet (
    uuid                   TEXT PRIMARY KEY,
    availability_zone_uuid TEXT NOT NULL,
    subnet_uuid            TEXT NOT NULL,
    CONSTRAINT             fk_availability_zone_availability_zone_uuid
        FOREIGN KEY            (availability_zone_uuid)
        REFERENCES             availability_zone(uuid),
    CONSTRAINT             fk_availability_zone_subnet_uuid
        FOREIGN KEY            (subnet_uuid)
        REFERENCES             subnet(uuid)
);

CREATE INDEX idx_availability_zone_subnet_availability_zone_uuid
ON availability_zone_subnet (uuid);

CREATE INDEX idx_availability_zone_subnet_subnet_uuid
ON availability_zone_subnet (subnet_uuid);
`)
}

func applicationSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE application (
    uuid TEXT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_application_name
ON application (name);
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
    CONSTRAINT      fk_machine_net_node
        FOREIGN KEY (net_node_uuid)
        REFERENCES  net_node(uuid)
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
    CONSTRAINT       fk_unit_application
        FOREIGN KEY  (application_uuid)
        REFERENCES   application(uuid),
    CONSTRAINT       fk_unit_net_node
        FOREIGN KEY  (net_node_uuid)
        REFERENCES   net_node(uuid)
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
    name               TEXT NOT NULL,
    label              TEXT,
    device_uuid        TEXT,
    hardware_id        TEXT,
    wwn                TEXT,
    bus_address        TEXT,
    serial_id          TEXT,
    filesystem_type_id INT,
    size               INT,
    mount_point        TEXT,
    in_use             BOOLEAN,
    CONSTRAINT         fk_filesystem_type
        FOREIGN KEY    (filesystem_type_id)
        REFERENCES     filesystem_type(id)
);

CREATE UNIQUE INDEX idx_block_device_name
ON block_device (name);

CREATE TABLE filesystem_type (
    id   INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_filesystem_type_name
ON filesystem_type (name);

INSERT INTO filesystem_type VALUES
    (0, 'ext4'),
    (1, 'xfs'),
    (2, 'btrfs'),
    (3, 'zfs');

CREATE TABLE block_device_link_device (
    block_device_uuid TEXT NOT NULL,
    name              TEXT NOT NULL,
    CONSTRAINT        fk_block_device_link_device
        FOREIGN KEY   (block_device_uuid)
        REFERENCES    block_device(uuid)
);

CREATE UNIQUE INDEX idx_block_device_link_device
ON block_device_link_device (block_device_uuid, name);

CREATE INDEX idx_block_device_link_device_device
ON block_device_link_device (block_device_uuid);

CREATE TABLE block_device_machine (
    block_device_uuid TEXT PRIMARY KEY,
    machine_uuid    TEXT NOT NULL,
    CONSTRAINT      fk_block_device_machine_device
        FOREIGN KEY (block_device_uuid)
        REFERENCES  block_device(uuid),
    CONSTRAINT      fk_block_device_machine
        FOREIGN KEY (machine_uuid)
        REFERENCES  machine(uuid)
);

CREATE INDEX idx_block_device_machine
ON block_device_machine (machine_uuid);
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

CREATE TABLE storage_pool_attribute (
    storage_pool_uuid TEXT NOT NULL,
    key               TEXT,
    value             TEXT,
    CONSTRAINT       fk_storage_pool_attribute_pool
        FOREIGN KEY  (storage_pool_uuid)
        REFERENCES   storage_pool(uuid)
);

CREATE UNIQUE INDEX idx_storage_pool_attribute
ON storage_pool_attribute (storage_pool_uuid, key);

CREATE TABLE storage_kind (
    id   INT PRIMARY KEY,
    kind TEXT
);

CREATE UNIQUE INDEX idx_storage_kind
ON storage_kind (kind);

INSERT INTO storage_kind VALUES
    (0, 'block'), 
    (1, 'filesystem');

CREATE TABLE storage_instance (
    uuid            TEXT PRIMARY KEY,
    storage_kind_id INT NOT NULL,
    name            TEXT NOT NULL,
    life_id         INT NOT NULL,
    CONSTRAINT       fk_storage_instance_kind
        FOREIGN KEY  (storage_kind_id)
        REFERENCES   storage_kind(id),
    CONSTRAINT       fk_storage_instance_life
        FOREIGN KEY  (life_id)
        REFERENCES   life(id)
);

CREATE TABLE storage_instance_pool (
    storage_instance_uuid TEXT PRIMARY KEY,
    storage_pool_uuid     TEXT NOT NULL,
    CONSTRAINT       fk_storage_instance_pool_instance
        FOREIGN KEY  (storage_instance_uuid)
        REFERENCES   storage_instance(uuid),
    CONSTRAINT       fk_storage_instance_pool_pool
        FOREIGN KEY  (storage_pool_uuid)
        REFERENCES   storage_pool(uuid)
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
    CONSTRAINT       fk_storage_owner_storage
        FOREIGN KEY  (storage_instance_uuid)
        REFERENCES   storage_instance(uuid),
    CONSTRAINT       fk_storage_owner_unit
        FOREIGN KEY  (unit_uuid)
        REFERENCES   unit(uuid)
);

CREATE TABLE storage_constraint_type (
    id   INT PRIMARY KEY,
    name TEXT NOT NULL 
);

CREATE UNIQUE INDEX idx_storage_constraint_type
ON storage_constraint_type (name);

INSERT INTO storage_constraint_type VALUES
    (0, 'pool'), 
    (1, 'size'), -- MiB.
    (2, 'count'); 

CREATE TABLE storage_instance_constraint (
    uuid                  TEXT PRIMARY KEY,
    storage_instance_uuid TEXT NOT NULL,
    constraint_type_id    INT NOT NULL,
    value                 TEXT NOT NULL,
    CONSTRAINT       fk_storage_instance_constraint_instance
        FOREIGN KEY  (storage_instance_uuid)
        REFERENCES   storage_instance(uuid),
    CONSTRAINT       fk_storage_instance_constraint_type
        FOREIGN KEY  (constraint_type_id)
        REFERENCES   storage_constraint_type(id)
);

CREATE UNIQUE INDEX idx_storage_instance_constraint
ON storage_instance_constraint (storage_instance_uuid, constraint_type_id);
`)
}
