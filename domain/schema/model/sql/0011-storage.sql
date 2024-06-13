CREATE TABLE storage_pool (
    uuid TEXT NOT NULL PRIMARY KEY,
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
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_storage_pool_attribute_pool
    FOREIGN KEY (storage_pool_uuid)
    REFERENCES storage_pool (uuid),
    PRIMARY KEY (storage_pool_uuid, "key")
);

CREATE TABLE storage_kind (
    id INT PRIMARY KEY,
    kind TEXT NOT NULL,
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
    charm_uuid TEXT NOT NULL,
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
    size INT NOT NULL,
    count INT NOT NULL,
    CONSTRAINT fk_application_storage_directive_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_storage_directive_charm_storage
    FOREIGN KEY (charm_uuid, storage_name)
    REFERENCES charm_storage (charm_uuid, name),
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
    unit_uuid TEXT NOT NULL,
    charm_uuid TEXT NOT NULL,
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
    size INT NOT NULL,
    count INT NOT NULL,
    CONSTRAINT fk_unit_storage_directive_charm_storage
    FOREIGN KEY (charm_uuid, storage_name)
    REFERENCES charm_storage (charm_uuid, name),
    PRIMARY KEY (unit_uuid, charm_uuid, storage_name)
);

-- Note that this is not unique; it speeds access by unit.
CREATE INDEX idx_unit_storage_directive
ON unit_storage_directive (unit_uuid);

CREATE TABLE storage_instance (
    uuid TEXT NOT NULL PRIMARY KEY,
    storage_kind_id INT NOT NULL,
    name TEXT NOT NULL,
    life_id INT NOT NULL,
    -- Note: one might wonder why storage_pool below is not a
    -- FK to a row defined in the storage pool table. This value
    -- can also be one of the pool types. As with the comment on the
    -- type column in the storage pool table, it's problematic to use a lookup
    -- with an ID. Storage pools, once created, cannot be renamed so
    -- this will not be able to become "orphaned".
    storage_pool TEXT NOT NULL,
    CONSTRAINT fk_storage_instance_kind
    FOREIGN KEY (storage_kind_id)
    REFERENCES storage_kind (id),
    CONSTRAINT fk_storage_instance_life
    FOREIGN KEY (life_id)
    REFERENCES life (id)
);

-- storage_unit_owner is used to indicate when
-- a unit is the owner of a storage instance.
-- This is different to a storage attachment.
CREATE TABLE storage_unit_owner (
    storage_instance_uuid TEXT NOT NULL PRIMARY KEY,
    unit_uuid TEXT NOT NULL,
    CONSTRAINT fk_storage_owner_storage
    FOREIGN KEY (storage_instance_uuid)
    REFERENCES storage_instance (uuid),
    CONSTRAINT fk_storage_owner_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

CREATE TABLE storage_attachment (
    storage_instance_uuid TEXT NOT NULL PRIMARY KEY,
    unit_uuid TEXT NOT NULL,
    life_id INT NOT NULL,
    CONSTRAINT fk_storage_owner_storage
    FOREIGN KEY (storage_instance_uuid)
    REFERENCES storage_instance (uuid),
    CONSTRAINT fk_storage_owner_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_storage_attachment_life
    FOREIGN KEY (life_id)
    REFERENCES life (id)
);

-- Note that this is not unique; it speeds access by unit.
CREATE INDEX idx_storage_attachment_unit
ON storage_attachment (unit_uuid);

CREATE TABLE storage_provisioning_status (
    id INT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT
);

CREATE UNIQUE INDEX idx_storage_provisioning_status
ON storage_provisioning_status (name);

INSERT INTO storage_provisioning_status VALUES
(0, 'pending', 'Creation or attachment is awaiting completion'),
(1, 'provisioned', 'Requested creation or attachment has been completed'),
(2, 'error', 'An error was encountered during creation or attachment');

CREATE TABLE storage_volume (
    uuid TEXT NOT NULL PRIMARY KEY,
    life_id INT NOT NULL,
    name TEXT NOT NULL,
    provider_id TEXT,
    storage_pool_uuid TEXT,
    size_mib INT,
    hardware_id TEXT,
    wwn TEXT,
    persistent BOOLEAN,
    provisioning_status_id INT NOT NULL,
    CONSTRAINT fk_storage_instance_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_storage_volume_pool
    FOREIGN KEY (storage_pool_uuid)
    REFERENCES storage_pool (uuid),
    CONSTRAINT fk_storage_vol_provisioning_status
    FOREIGN KEY (provisioning_status_id)
    REFERENCES storage_provisioning_status (id)
);

-- An instance can have at most one volume.
-- A volume can have at most one instance.
CREATE TABLE storage_instance_volume (
    storage_instance_uuid TEXT NOT NULL PRIMARY KEY,
    storage_volume_uuid TEXT NOT NULL,
    CONSTRAINT fk_storage_instance_volume_instance
    FOREIGN KEY (storage_instance_uuid)
    REFERENCES storage_instance (uuid),
    CONSTRAINT fk_storage_instance_volume_volume
    FOREIGN KEY (storage_volume_uuid)
    REFERENCES storage_volume (uuid)
);

CREATE UNIQUE INDEX idx_storage_instance_volume
ON storage_instance_volume (storage_volume_uuid);

CREATE TABLE storage_volume_attachment (
    uuid TEXT NOT NULL PRIMARY KEY,
    storage_volume_uuid TEXT NOT NULL,
    net_node_uuid TEXT NOT NULL,
    life_id INT NOT NULL,
    block_device_uuid TEXT,
    read_only BOOLEAN,
    provisioning_status_id INT NOT NULL,
    CONSTRAINT fk_storage_volume_attachment_vol
    FOREIGN KEY (storage_volume_uuid)
    REFERENCES storage_volume (uuid),
    CONSTRAINT fk_storage_volume_attachment_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_storage_volume_attachment_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_storage_volume_attachment_block
    FOREIGN KEY (block_device_uuid)
    REFERENCES block_device (uuid),
    CONSTRAINT fk_storage_vol_att_provisioning_status
    FOREIGN KEY (provisioning_status_id)
    REFERENCES storage_provisioning_status (id)
);

CREATE TABLE storage_filesystem (
    uuid TEXT NOT NULL PRIMARY KEY,
    life_id INT NOT NULL,
    provider_id TEXT,
    storage_pool_uuid TEXT,
    size_mib INT,
    provisioning_status_id INT NOT NULL,
    CONSTRAINT fk_storage_instance_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_storage_filesystem_pool
    FOREIGN KEY (storage_pool_uuid)
    REFERENCES storage_pool (uuid),
    CONSTRAINT fk_storage_fs_provisioning_status
    FOREIGN KEY (provisioning_status_id)
    REFERENCES storage_provisioning_status (id)
);

-- An instance can have at most one filesystem.
-- A filesystem can have at most one instance.
CREATE TABLE storage_instance_filesystem (
    storage_instance_uuid TEXT NOT NULL PRIMARY KEY,
    storage_filesystem_uuid TEXT NOT NULL,
    CONSTRAINT fk_storage_instance_filesystem_instance
    FOREIGN KEY (storage_instance_uuid)
    REFERENCES storage_instance (uuid),
    CONSTRAINT fk_storage_instance_filesystem_fs
    FOREIGN KEY (storage_filesystem_uuid)
    REFERENCES storage_filesystem (uuid)
);

CREATE UNIQUE INDEX idx_storage_instance_filesystem
ON storage_instance_filesystem (storage_filesystem_uuid);

CREATE TABLE storage_filesystem_attachment (
    uuid TEXT NOT NULL PRIMARY KEY,
    storage_filesystem_uuid TEXT NOT NULL,
    net_node_uuid TEXT NOT NULL,
    life_id INT NOT NULL,
    mount_point TEXT,
    read_only BOOLEAN,
    provisioning_status_id INT NOT NULL,
    CONSTRAINT fk_storage_filesystem_attachment_fs
    FOREIGN KEY (storage_filesystem_uuid)
    REFERENCES storage_filesystem (uuid),
    CONSTRAINT fk_storage_filesystem_attachment_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_storage_filesystem_attachment_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_storage_fs_provisioning_status
    FOREIGN KEY (provisioning_status_id)
    REFERENCES storage_provisioning_status (id)
);

CREATE TABLE storage_volume_device_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT
);

CREATE UNIQUE INDEX idx_storage_volume_dev_type
ON storage_volume_device_type (name);

INSERT INTO storage_volume_device_type VALUES
(0, 'local', 'Default device type for on-machine volume attachments'),
(1, 'iscsi', 'iSCSI protocol for linking storage');

CREATE TABLE storage_volume_attachment_plan (
    uuid TEXT NOT NULL PRIMARY KEY,
    storage_volume_uuid TEXT NOT NULL,
    net_node_uuid TEXT NOT NULL,
    life_id INT NOT NULL,
    device_type_id INT,
    block_device_uuid TEXT,
    provisioning_status_id INT NOT NULL,
    CONSTRAINT fk_storage_volume_attachment_plan_vol
    FOREIGN KEY (storage_volume_uuid)
    REFERENCES storage_volume (uuid),
    CONSTRAINT fk_storage_volume_attachment_plan_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_storage_volume_attachment_plan_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_storage_volume_attachment_plan_device
    FOREIGN KEY (device_type_id)
    REFERENCES storage_volume_device_type (id),
    CONSTRAINT fk_storage_volume_attachment_plan_block
    FOREIGN KEY (block_device_uuid)
    REFERENCES block_device (uuid),
    CONSTRAINT fk_storage_fs_provisioning_status
    FOREIGN KEY (provisioning_status_id)
    REFERENCES storage_provisioning_status (id)
);

CREATE TABLE storage_volume_attachment_plan_attr (
    uuid TEXT NOT NULL PRIMARY KEY,
    attachment_plan_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_storage_vol_attach_plan_attr_plan
    FOREIGN KEY (attachment_plan_uuid)
    REFERENCES storage_volume_attachment_plan (attachment_plan_uuid)
);

CREATE UNIQUE INDEX idx_storage_vol_attachment_plan_attr
ON storage_volume_attachment_plan_attr (attachment_plan_uuid, "key");
