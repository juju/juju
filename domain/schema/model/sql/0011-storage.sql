CREATE TABLE storage_pool (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    -- Types are provider sourced, so we do not use a lookup with ID.
    -- This constitutes "repeating data" and would tend to indicate
    -- bad relational design. However we choose that here over the burden of:
    --   - Knowing every possible type up front to populate a look-up or;
    --   - Sourcing the lookup from the provider and keeping it updated.
    type TEXT NOT NULL,
    -- The origin sets to "user" by default for user created pools.
    -- The "built-in" and "provider-default" origins are used
    -- for pools that are created by the system when a model is created.
    origin_id INT NOT NULL DEFAULT 1,
    CONSTRAINT chk_storage_pool_name_not_empty
    CHECK (name <> ''),
    CONSTRAINT chk_storage_pool_type_not_empty
    CHECK (type <> ''),
    CONSTRAINT fk_storage_pool_origin
    FOREIGN KEY (origin_id)
    REFERENCES storage_pool_origin (id)
);

-- It is important that the name is unique and speed up access by name.
CREATE UNIQUE INDEX idx_storage_pool_name
ON storage_pool (name);

-- This index is used to speed up access by type, type and name.
-- Warning: if the "type" is not the first column in the composite query condition,
-- then the index will not be used.
CREATE INDEX idx_storage_pool_type_name
ON storage_pool (type, name);

CREATE TABLE storage_pool_attribute (
    storage_pool_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_storage_pool_attribute_pool
    FOREIGN KEY (storage_pool_uuid)
    REFERENCES storage_pool (uuid),
    PRIMARY KEY (storage_pool_uuid, "key")
);

CREATE TABLE storage_pool_origin (
    id INT NOT NULL PRIMARY KEY,
    origin TEXT NOT NULL UNIQUE,
    CONSTRAINT chk_storage_pool_origin_not_empty
    CHECK (origin <> '')
);

INSERT INTO storage_pool_origin (id, origin) VALUES
(1, 'user'),
(2, 'built-in'),
(3, 'provider-default');

-- This table stores storage directive values for each named storage item
-- defined by the application's current charm. If the charm is updated, then
-- so too will be the rows in this table to reflect the current charm's
-- storage definitions.
CREATE TABLE application_storage_directive (
    application_uuid TEXT NOT NULL,
    charm_uuid TEXT NOT NULL,
    storage_name TEXT NOT NULL,
    storage_pool_uuid TEXT NOT NULL,
    size_mib INT NOT NULL,
    count INT NOT NULL,
    CONSTRAINT fk_application_storage_directive_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_storage_directive_storage_pool
    FOREIGN KEY (storage_pool_uuid)
    REFERENCES storage_pool (uuid),
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
    storage_pool_uuid TEXT NOT NULL,
    size_mib INT NOT NULL,
    count INT NOT NULL,
    CONSTRAINT fk_unit_storage_directive_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_unit_storage_directive_storage_pool
    FOREIGN KEY (storage_pool_uuid)
    REFERENCES storage_pool (uuid),
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
    charm_uuid TEXT NOT NULL,
    storage_name TEXT NOT NULL,
    -- storage_id is created from the storage name and a unique id number.
    storage_id TEXT NOT NULL,
    life_id INT NOT NULL,
    storage_pool_uuid TEXT NOT NULL,
    requested_size_mib INT NOT NULL,
    CONSTRAINT fk_storage_instance_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_storage_instance_storage_pool
    FOREIGN KEY (storage_pool_uuid)
    REFERENCES storage_pool (uuid),
    CONSTRAINT fk_storage_instance_charm_storage
    FOREIGN KEY (charm_uuid, storage_name)
    REFERENCES charm_storage (charm_uuid, name)
);

CREATE UNIQUE INDEX idx_storage_instance_id
ON storage_instance (storage_id);

-- storage_unit_owner is used to indicate when
-- a unit is the owner of a storage instance.
-- This is different to a storage attachment.
CREATE TABLE storage_unit_owner (
    storage_instance_uuid TEXT NOT NULL PRIMARY KEY,
    unit_uuid TEXT NOT NULL,
    CONSTRAINT fk_storage_owner_storage_instance
    FOREIGN KEY (storage_instance_uuid)
    REFERENCES storage_instance (uuid),
    CONSTRAINT fk_storage_owner_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

CREATE TABLE storage_attachment (
    uuid TEXT NOT NULL PRIMARY KEY,
    storage_instance_uuid TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    life_id INT NOT NULL,
    CONSTRAINT fk_storage_attachment_storage_instance
    FOREIGN KEY (storage_instance_uuid)
    REFERENCES storage_instance (uuid),
    CONSTRAINT fk_storage_attachment_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_storage_attachment_life
    FOREIGN KEY (life_id)
    REFERENCES life (id)
);

CREATE UNIQUE INDEX idx_storage_attachment_unit_uuid_storage_instance_uuid
ON storage_attachment (unit_uuid, storage_instance_uuid);

-- Note that this is not unique; it speeds access by unit.
CREATE INDEX idx_storage_attachment_unit
ON storage_attachment (unit_uuid);

CREATE TABLE storage_volume_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_storage_volume_status_value
ON storage_volume_status_value (status);

INSERT INTO storage_volume_status_value VALUES
(0, 'pending'),
(1, 'error'),
(2, 'attaching'),
(3, 'attached'),
(4, 'detaching'),
(5, 'detached'),
(6, 'destroying');

CREATE TABLE storage_volume_status (
    volume_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_storage_volume_status_storage_volume
    FOREIGN KEY (volume_uuid)
    REFERENCES storage_volume (uuid),
    CONSTRAINT fk_storage_volume_status_status
    FOREIGN KEY (status_id)
    REFERENCES storage_volume_status_value (id)
);

CREATE TABLE storage_provision_scope (
    id INT PRIMARY KEY,
    scope TEXT NOT NULL UNIQUE,
    CONSTRAINT chk_storage_provision_scope_scope_not_empty
    CHECK (scope <> '')
);

INSERT INTO storage_provision_scope (id, scope) VALUES
(0, 'model'),
(1, 'machine');

CREATE TABLE storage_volume (
    uuid TEXT NOT NULL PRIMARY KEY,
    volume_id TEXT NOT NULL,
    life_id INT NOT NULL,
    provider_id TEXT,
    size_mib INT,
    hardware_id TEXT,
    wwn TEXT,
    persistent BOOLEAN,
    -- TODO: we may change provision_scope_id to NOT NULL in the future.
    -- We leave it nullable for now to avoid too much code churn.
    provision_scope_id INT,
    CONSTRAINT fk_storage_instance_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_storage_volume_provision_scope_id
    FOREIGN KEY (provision_scope_id)
    REFERENCES storage_provision_scope (id)
);

CREATE UNIQUE INDEX idx_storage_volume_id
ON storage_volume (volume_id);

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
    -- TODO: we may change provision_scope_id to NOT NULL in the future.
    -- We leave it nullable for now to avoid too much code churn.
    provision_scope_id INT,
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
    CONSTRAINT fk_storage_volume_attachment_provision_scope_id
    FOREIGN KEY (provision_scope_id)
    REFERENCES storage_provision_scope (id)
);

CREATE INDEX idx_storage_volume_attachment_net_node_uuid
ON storage_volume_attachment (net_node_uuid);

CREATE TABLE storage_filesystem_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_storage_filesystem_status_value
ON storage_filesystem_status_value (status);

INSERT INTO storage_filesystem_status_value VALUES
(0, 'pending'),
(1, 'error'),
(2, 'attaching'),
(3, 'attached'),
(4, 'detaching'),
(5, 'detached'),
(6, 'destroying');

CREATE TABLE storage_filesystem_status (
    filesystem_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_storage_filesystem_status_storage_filesystem
    FOREIGN KEY (filesystem_uuid)
    REFERENCES storage_filesystem (uuid),
    CONSTRAINT fk_storage_filesystem_status_status
    FOREIGN KEY (status_id)
    REFERENCES storage_filesystem_status_value (id)
);

CREATE TABLE storage_filesystem (
    uuid TEXT NOT NULL PRIMARY KEY,
    filesystem_id TEXT NOT NULL,
    life_id INT NOT NULL,
    provider_id TEXT,
    size_mib INT,
    -- TODO: we may change provision_scope_id to NOT NULL in the future.
    -- We leave it nullable for now to avoid too much code churn.
    provision_scope_id INT,
    CONSTRAINT fk_storage_filesystem_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_storage_filesystem_provision_scope_id
    FOREIGN KEY (provision_scope_id)
    REFERENCES storage_provision_scope (id)
);

CREATE UNIQUE INDEX idx_storage_filesystem_id
ON storage_filesystem (filesystem_id);

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
    -- TODO: we may change provision_scope_id to NOT NULL in the future.
    -- We leave it nullable for now to avoid too much code churn.
    provision_scope_id INT,
    CONSTRAINT fk_storage_filesystem_attachment_fs
    FOREIGN KEY (storage_filesystem_uuid)
    REFERENCES storage_filesystem (uuid),
    CONSTRAINT fk_storage_filesystem_attachment_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_storage_filesystem_attachment_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_storage_filesystem_attachment_provision_scope_id
    FOREIGN KEY (provision_scope_id)
    REFERENCES storage_provision_scope (id)
);

CREATE INDEX idx_storage_filesystem_attachment_net_node_uuid
ON storage_filesystem_attachment (net_node_uuid);

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
    -- TODO: we may change provision_scope_id to NOT NULL in the future.
    -- We leave it nullable for now to avoid too much code churn.
    provision_scope_id INT,
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
    CONSTRAINT fk_storage_volume_attachment_plan_provision_scope_id
    FOREIGN KEY (provision_scope_id)
    REFERENCES storage_provision_scope (id)
);

CREATE TABLE storage_volume_attachment_plan_attr (
    uuid TEXT NOT NULL PRIMARY KEY,
    attachment_plan_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_storage_vol_attach_plan_attr_plan
    FOREIGN KEY (attachment_plan_uuid)
    REFERENCES storage_volume_attachment_plan (uuid)
);

CREATE UNIQUE INDEX idx_storage_vol_attachment_plan_attr
ON storage_volume_attachment_plan_attr (attachment_plan_uuid, "key");
