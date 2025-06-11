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
    -- One of storage_pool_uuid or storage_type must be set.
    -- Storage types are provider sourced, so we do not use a lookup with ID.
    -- This constitutes "repeating data" and would tend to indicate
    -- bad relational design. However we choose that here over the
    -- burden of:
    --   - Knowing every possible type up front to populate a look-up or;
    --   - Sourcing the lookup from the provider and keeping it updated.
    storage_pool_uuid TEXT,
    storage_type TEXT,
    size_mib INT NOT NULL,
    count INT NOT NULL,
    CONSTRAINT chk_application_storage_specified
    CHECK (storage_pool_uuid IS NOT NULL OR storage_type IS NOT NULL),
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

CREATE VIEW v_application_storage_directive AS
SELECT
    asd.application_uuid,
    asd.charm_uuid,
    asd.storage_name,
    asd.size_mib,
    asd.count,
    COALESCE(sp.name, asd.storage_type) AS storage_pool
FROM application_storage_directive AS asd
LEFT JOIN storage_pool AS sp ON asd.storage_pool_uuid = sp.uuid;

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
    -- One of storage_pool_uuid or storage_type must be set.
    -- Storage types are provider sourced, so we do not use a lookup with ID.
    -- This constitutes "repeating data" and would tend to indicate
    -- bad relational design. However we choose that here over the
    -- burden of:
    --   - Knowing every possible type up front to populate a look-up or;
    --   - Sourcing the lookup from the provider and keeping it updated.
    storage_pool_uuid TEXT,
    storage_type TEXT,
    size_mib INT NOT NULL,
    count INT NOT NULL,
    CONSTRAINT chk_unit_storage_specified
    CHECK (storage_pool_uuid IS NOT NULL OR storage_type IS NOT NULL),
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

CREATE VIEW v_unit_storage_directive AS
SELECT
    usd.unit_uuid,
    usd.charm_uuid,
    usd.storage_name,
    usd.size_mib,
    usd.count,
    COALESCE(sp.name, usd.storage_type) AS storage_pool
FROM unit_storage_directive AS usd
LEFT JOIN storage_pool AS sp ON usd.storage_pool_uuid = sp.uuid;

CREATE TABLE storage_scope (
    id INT PRIMARY KEY,
    scope TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_storage_scope
ON storage_scope (scope);

INSERT INTO storage_scope VALUES
(0, 'model'),
(1, 'host');

CREATE TABLE storage_instance (
    uuid TEXT NOT NULL PRIMARY KEY,
    charm_uuid TEXT NOT NULL,
    storage_name TEXT NOT NULL,
    -- storage_id is created from the storage name and a unique id number.
    storage_id TEXT NOT NULL,
    life_id INT NOT NULL,
    scope_id INT NOT NULL,
    -- One of storage_pool_uuid or storage_type must be set.
    -- Storage types are provider sourced, so we do not use a lookup with ID.
    -- This constitutes "repeating data" and would tend to indicate
    -- bad relational design. However we choose that here over the
    -- burden of:
    --   - Knowing every possible type up front to populate a look-up or;
    --   - Sourcing the lookup from the provider and keeping it updated.
    storage_pool_uuid TEXT,
    storage_type TEXT,
    requested_size_mib INT NOT NULL,
    CONSTRAINT chk_storage_instance_storage_specified
    CHECK (storage_pool_uuid IS NOT NULL OR storage_type IS NOT NULL),
    CONSTRAINT fk_storage_instance_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_storage_instance_scope
    FOREIGN KEY (scope_id)
    REFERENCES storage_scope (id),
    CONSTRAINT fk_storage_instance_storage_pool
    FOREIGN KEY (storage_pool_uuid)
    REFERENCES storage_pool (uuid),
    CONSTRAINT fk_storage_instance_charm_storage
    FOREIGN KEY (charm_uuid, storage_name)
    REFERENCES charm_storage (charm_uuid, name)
);

CREATE UNIQUE INDEX idx_storage_instance_id
ON storage_instance (storage_id);

CREATE VIEW v_storage_instance AS
SELECT
    si.uuid,
    si.charm_uuid,
    si.storage_name,
    si.storage_id,
    si.life_id,
    si.scope_id,
    si.requested_size_mib,
    COALESCE(sp.name, si.storage_type) AS storage_pool
FROM storage_instance AS si
LEFT JOIN storage_pool AS sp ON si.storage_pool_uuid = sp.uuid;

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

CREATE TABLE storage_volume (
    uuid TEXT NOT NULL PRIMARY KEY,
    volume_id TEXT NOT NULL,
    life_id INT NOT NULL,
    provider_id TEXT,
    size_mib INT,
    hardware_id TEXT,
    wwn TEXT,
    persistent BOOLEAN,
    CONSTRAINT fk_storage_instance_life
    FOREIGN KEY (life_id)
    REFERENCES life (id)
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
    REFERENCES block_device (uuid)
);

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
    CONSTRAINT fk_storage_instance_life
    FOREIGN KEY (life_id)
    REFERENCES life (id)
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
    CONSTRAINT fk_storage_filesystem_attachment_fs
    FOREIGN KEY (storage_filesystem_uuid)
    REFERENCES storage_filesystem (uuid),
    CONSTRAINT fk_storage_filesystem_attachment_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_storage_filesystem_attachment_life
    FOREIGN KEY (life_id)
    REFERENCES life (id)
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
    REFERENCES block_device (uuid)
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
