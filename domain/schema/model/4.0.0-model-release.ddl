-- Code genrated by ddlgen. DO NOT EDIT.
-- Source: github.com/juju/juju/generate/ddlgen

CREATE TABLE life (
    id INT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO life VALUES
(0, 'alive'),
(1, 'dying'),
(2, 'dead');

CREATE TABLE change_log_edit_type (
    id INT PRIMARY KEY,
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
    id INT PRIMARY KEY,
    namespace TEXT,
    description TEXT
);

CREATE UNIQUE INDEX idx_change_log_namespace_namespace
ON change_log_namespace (namespace);

CREATE TABLE change_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    edit_type_id INT NOT NULL,
    namespace_id INT NOT NULL,
    changed TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    CONSTRAINT fk_change_log_edit_type
    FOREIGN KEY (edit_type_id)
    REFERENCES change_log_edit_type (id),
    CONSTRAINT fk_change_log_namespace
    FOREIGN KEY (namespace_id)
    REFERENCES change_log_namespace (id)
);

-- The change log witness table is used to track which nodes have seen
-- which change log entries. This is used to determine when a change log entry
-- can be deleted.
-- We'll delete all change log entries that are older than the lower_bound
-- change log entry that has been seen by all controllers.
CREATE TABLE change_log_witness (
    controller_id TEXT NOT NULL PRIMARY KEY,
    lower_bound INT NOT NULL DEFAULT (-1),
    upper_bound INT NOT NULL DEFAULT (-1),
    updated_at DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc'))
);

-- changelog namespaces are now generated with the triggers.

CREATE TABLE password_hash_algorithm (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_password_hash_algorithm
ON password_hash_algorithm (name);

INSERT INTO password_hash_algorithm VALUES
(0, 'sha512');

-- The model table represents a readonly denormalised model data. The intended
-- use is to provide a read-only view of the model data for the purpose of
-- accessing common model data without the need to span multiple databases.
--
-- The model table primarily is used to drive the provider tracker. The model
-- table should *not* be changed in a patch/build release. The only time to make
-- changes to this table is during a major/minor release. 
CREATE TABLE model (
    uuid TEXT NOT NULL PRIMARY KEY,
    controller_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    qualifier TEXT NOT NULL,
    type TEXT NOT NULL,
    cloud TEXT NOT NULL,
    cloud_type TEXT NOT NULL,
    cloud_region TEXT,
    credential_owner TEXT,
    credential_name TEXT,
    is_controller_model BOOLEAN DEFAULT FALSE
);

-- A unique constraint over a constant index ensures only 1 entry matching the
-- condition can exist.
CREATE UNIQUE INDEX idx_singleton_model ON model ((1));

CREATE VIEW v_model_metrics AS
SELECT
    (SELECT COUNT(DISTINCT uuid) FROM application) AS application_count,
    (SELECT COUNT(DISTINCT uuid) FROM machine) AS machine_count,
    (SELECT COUNT(DISTINCT uuid) FROM unit) AS unit_count;

-- The model_config table is a new table that is used to store configuration 
-- data for the model.
--
-- The provider tracker relies on the model_config table. Do not modify the
-- model_config table in a patch/build release. Only make changes to this table
-- during a major/minor release.
CREATE TABLE model_config (
    "key" TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL
);

-- The model_constraint table is a new table that is used to store the
-- constraints that are associated with a model.
CREATE TABLE model_constraint (
    model_uuid TEXT NOT NULL PRIMARY KEY,
    constraint_uuid TEXT NOT NULL,
    CONSTRAINT fk_model_constraint_model
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid),
    CONSTRAINT fk_model_constraint_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid)
);

-- v_model_constraint is a view to represent the current model constraints. If
-- no constraints have been set then expect this view to be empty. There will
-- also only ever be a maximum of 1 record in this view.
CREATE VIEW v_model_constraint AS
SELECT
    c.uuid,
    c.arch,
    c.cpu_cores,
    c.cpu_power,
    c.mem,
    c.root_disk,
    c.root_disk_source,
    c.instance_role,
    c.instance_type,
    c.container_type,
    c.virt_type,
    c.allocate_public_ip,
    c.image_id
FROM model_constraint AS mc
JOIN v_constraint AS c ON mc.constraint_uuid = c.uuid;

-- v_model_constraint_tag is a view of all the constraint tags set for the
-- current model. It is expected that this view can be empty.
CREATE VIEW v_model_constraint_tag AS
SELECT
    ct.constraint_uuid,
    ct.tag
FROM constraint_tag AS ct
JOIN "constraint" AS c ON ct.constraint_uuid = c.uuid
JOIN model_constraint AS mc ON c.uuid = mc.constraint_uuid;

-- v_model_constraint_space is a view of all the constraint spaces set for the
-- current model. It is expected that this view can be empty.
CREATE VIEW v_model_constraint_space AS
SELECT
    cs.constraint_uuid,
    cs.space,
    cs."exclude"
FROM constraint_space AS cs
JOIN "constraint" AS c ON cs.constraint_uuid = c.uuid
JOIN model_constraint AS mc ON c.uuid = mc.constraint_uuid;

-- v_model_constraint_zone is a view of all the constraint zones set for the
-- current model. It is expected that this view can be empty.
CREATE VIEW v_model_constraint_zone AS
SELECT
    cz.constraint_uuid,
    cz.zone
FROM constraint_zone AS cz
JOIN "constraint" AS c ON cz.constraint_uuid = c.uuid
JOIN model_constraint AS mc ON c.uuid = mc.constraint_uuid;

-- This table is best effort to track the life of a model. The real location
-- of the model life is in the controller database. This is just a facsimile
-- of the model life.
CREATE TABLE model_life (
    model_uuid TEXT NOT NULL PRIMARY KEY,
    life_id TEXT NOT NULL,
    CONSTRAINT fk_model_constraint_model
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid),
    CONSTRAINT fk_model_life
    FOREIGN KEY (life_id)
    REFERENCES life (id)
);

CREATE TABLE object_store_metadata (
    uuid TEXT NOT NULL PRIMARY KEY,
    sha_256 TEXT NOT NULL,
    sha_384 TEXT NOT NULL,
    size INT NOT NULL
);

-- Add a unique index for each hash and a composite unique index for both hashes
-- to ensure that the same hash is not stored multiple times.
CREATE UNIQUE INDEX idx_object_store_metadata_sha_256 ON object_store_metadata (sha_256);
CREATE UNIQUE INDEX idx_object_store_metadata_sha_384 ON object_store_metadata (sha_384);

CREATE TABLE object_store_metadata_path (
    path TEXT NOT NULL PRIMARY KEY,
    metadata_uuid TEXT NOT NULL,
    CONSTRAINT fk_object_store_metadata_metadata_uuid
    FOREIGN KEY (metadata_uuid)
    REFERENCES object_store_metadata (uuid)
);

CREATE VIEW v_object_store_metadata AS
SELECT
    osm.uuid,
    osm.sha_256,
    osm.sha_384,
    osm.size,
    osmp.path
FROM object_store_metadata AS osm
LEFT JOIN object_store_metadata_path AS osmp
    ON osm.uuid = osmp.metadata_uuid;

CREATE TABLE os (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_os_name
ON os (name);

INSERT INTO os VALUES
(0, 'ubuntu');

CREATE TABLE architecture (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_architecture_name
ON architecture (name);

INSERT INTO architecture VALUES
(0, 'amd64'),
(1, 'arm64'),
(2, 'ppc64el'),
(3, 's390x'),
(4, 'riscv64');

CREATE TABLE space (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_spaces_uuid_name ON space (name);

CREATE TABLE provider_space (
    provider_id TEXT NOT NULL PRIMARY KEY,
    space_uuid TEXT NOT NULL,
    CONSTRAINT fk_provider_space_space_uuid FOREIGN KEY (space_uuid) REFERENCES space (uuid)
);

CREATE UNIQUE INDEX idx_provider_space_space_uuid ON provider_space (space_uuid);

INSERT INTO space VALUES
('656b4a82-e28c-53d6-a014-f0dd53417eb6', 'alpha');

CREATE VIEW v_space_subnet AS
SELECT
    space.uuid,
    space.name,
    provider_space.provider_id,
    subnet.uuid AS subnet_uuid,
    subnet.cidr AS subnet_cidr,
    subnet.vlan_tag AS subnet_vlan_tag,
    subnet.space_uuid AS subnet_space_uuid,
    space.name AS subnet_space_name,
    provider_subnet.provider_id AS subnet_provider_id,
    provider_network.provider_network_id AS subnet_provider_network_id,
    availability_zone.name AS subnet_az,
    provider_space.provider_id AS subnet_provider_space_uuid
FROM
    space
LEFT JOIN provider_space ON space.uuid = provider_space.space_uuid
LEFT JOIN subnet ON space.uuid = subnet.space_uuid
LEFT JOIN provider_subnet ON subnet.uuid = provider_subnet.subnet_uuid
LEFT JOIN provider_network_subnet ON subnet.uuid = provider_network_subnet.subnet_uuid
LEFT JOIN provider_network ON provider_network_subnet.provider_network_uuid = provider_network.uuid
LEFT JOIN availability_zone_subnet ON subnet.uuid = availability_zone_subnet.subnet_uuid
LEFT JOIN availability_zone ON availability_zone_subnet.availability_zone_uuid = availability_zone.uuid;

CREATE TABLE subnet (
    uuid TEXT NOT NULL PRIMARY KEY,
    cidr TEXT NOT NULL,
    vlan_tag INT,
    space_uuid TEXT,
    CONSTRAINT fk_subnets_spaces
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid)
);

CREATE TABLE provider_subnet (
    provider_id TEXT NOT NULL PRIMARY KEY,
    subnet_uuid TEXT NOT NULL,
    CONSTRAINT chk_provider_id_empty CHECK (provider_id != ''),
    CONSTRAINT fk_provider_subnet_subnet_uuid
    FOREIGN KEY (subnet_uuid)
    REFERENCES subnet (uuid)
);

CREATE UNIQUE INDEX idx_provider_subnet_subnet_uuid
ON provider_subnet (subnet_uuid);

CREATE TABLE provider_network (
    uuid TEXT NOT NULL PRIMARY KEY,
    provider_network_id TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_provider_network_id
ON provider_network (provider_network_id);

CREATE TABLE provider_network_subnet (
    subnet_uuid TEXT NOT NULL PRIMARY KEY,
    provider_network_uuid TEXT NOT NULL,
    CONSTRAINT fk_provider_network_subnet_provider_network_uuid
    FOREIGN KEY (provider_network_uuid)
    REFERENCES provider_network (uuid),
    CONSTRAINT fk_provider_network_subnet_uuid
    FOREIGN KEY (subnet_uuid)
    REFERENCES subnet (uuid)
);

CREATE TABLE availability_zone (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_availability_zone_name
ON availability_zone (name);

CREATE TABLE availability_zone_subnet (
    availability_zone_uuid TEXT NOT NULL,
    subnet_uuid TEXT NOT NULL,
    PRIMARY KEY (availability_zone_uuid, subnet_uuid),
    CONSTRAINT fk_availability_zone_availability_zone_uuid
    FOREIGN KEY (availability_zone_uuid)
    REFERENCES availability_zone (uuid),
    CONSTRAINT fk_availability_zone_subnet_uuid
    FOREIGN KEY (subnet_uuid)
    REFERENCES subnet (uuid)
);

CREATE TABLE block_device (
    uuid TEXT NOT NULL PRIMARY KEY,
    machine_uuid TEXT NOT NULL,
    name TEXT,
    hardware_id TEXT,
    wwn TEXT,
    serial_id TEXT,
    bus_address TEXT,
    size_mib INT,
    mount_point TEXT,
    in_use BOOLEAN,
    filesystem_label TEXT,
    host_filesystem_uuid TEXT,
    filesystem_type TEXT,
    CONSTRAINT fk_block_device_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

-- name can be NULL. In Sqlite all NULLs are distinct.
CREATE UNIQUE INDEX idx_block_device_name
ON block_device (machine_uuid, name);

CREATE TABLE block_device_link_device (
    block_device_uuid TEXT NOT NULL,
    machine_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    CONSTRAINT fk_block_device_link_device
    FOREIGN KEY (block_device_uuid)
    REFERENCES block_device (uuid),
    PRIMARY KEY (block_device_uuid, name),
    CONSTRAINT fk_block_device_link_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

CREATE UNIQUE INDEX idx_block_device_link_device
ON block_device_link_device (block_device_uuid, name);

CREATE UNIQUE INDEX idx_block_device_link_device_name_machine
ON block_device_link_device (name, machine_uuid);

CREATE INDEX idx_block_device_link_device_device
ON block_device_link_device (block_device_uuid);

CREATE TABLE storage_pool_origin (
    id INT NOT NULL PRIMARY KEY,
    origin TEXT NOT NULL UNIQUE,
    CONSTRAINT chk_storage_pool_origin_not_empty
    CHECK (origin <> '')
);

INSERT INTO storage_pool_origin (id, origin) VALUES
(0, 'user'),
(1, 'provider-default');

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
    origin_id INT NOT NULL DEFAULT 0,
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

-- storage_kind defines what type Juju considers a storage instance in the model
-- to be of. While we have the concept of charm storage kind it is not
-- necessarily the same as the storage instance. This is even more true when we
-- are trying to understand the composition of a storage_instance and not the
-- purpose it may be fulfilling.
CREATE TABLE storage_kind (
    id INT PRIMARY KEY,
    kind TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_storage_kind_kind
ON storage_kind (kind);

INSERT INTO storage_kind VALUES
(0, 'block'),
(1, 'filesystem');

-- model_storage_pool instructs the model what is considered the default
-- storage pool to use for a given storage kind.
CREATE TABLE model_storage_pool (
    storage_kind_id INT NOT NULL,
    storage_pool_uuid TEXT NOT NULL,
    PRIMARY KEY (storage_kind_id, storage_pool_uuid),
    CONSTRAINT fk_model_storage_pool_storage_kind
    FOREIGN KEY (storage_kind_id)
    REFERENCES storage_kind (id),
    CONSTRAINT fk_model_storage_pool_storage_pool_uuid
    FOREIGN KEY (storage_pool_uuid)
    REFERENCES storage_pool (uuid)
);

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
    -- charm_name is the charm name that this storage instance serves. This
    -- storage instance MUST never be used with any other charm that doesn't
    -- match on charm and storage names.
    -- When NULL then the storage instance has never been used. The first
    -- attachment of this storage instance MUST make this association.
    charm_name TEXT,
    storage_name TEXT NOT NULL,
    storage_kind_id INT NOT NULL,
    -- storage_id is created from the storage name and a unique id number.
    storage_id TEXT NOT NULL,
    life_id INT NOT NULL,
    storage_pool_uuid TEXT NOT NULL,
    requested_size_mib INT NOT NULL,
    CONSTRAINT fk_storage_instance_storage_kind
    FOREIGN KEY (storage_kind_id)
    REFERENCES storage_kind (id),
    CONSTRAINT fk_storage_instance_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_storage_instance_storage_pool
    FOREIGN KEY (storage_pool_uuid)
    REFERENCES storage_pool (uuid)
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

-- Order of columns is important for this index. The storage_instance_uuid MUST
-- come first so that an index exists for this column. A seperate index
-- idx_storage_attachment_unit already exists for the unit_uuid.
CREATE UNIQUE INDEX idx_storage_attachment_unit_uuid_storage_instance_uuid
ON storage_attachment (storage_instance_uuid, unit_uuid);

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
(6, 'destroying'),
(7, 'tombstone');

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

-- storage_volume describes a volume held by a storage_instance.
--
-- obliterate_on_cleanup can only be set when life_id is not-alive(>0), to
-- ensure it is only set with intent to cause volume death.
CREATE TABLE storage_volume (
    uuid TEXT NOT NULL PRIMARY KEY,
    volume_id TEXT NOT NULL,
    life_id INT NOT NULL,
    provision_scope_id INT NOT NULL,
    provider_id TEXT,
    size_mib INT,
    hardware_id TEXT,
    wwn TEXT,
    persistent BOOLEAN,
    obliterate_on_cleanup BOOLEAN,
    CONSTRAINT chk_storage_volume_obliterate_on_cleanup_set_when_not_alive
    CHECK (obliterate_on_cleanup IS NULL OR life_id <> 0),
    CONSTRAINT fk_storage_instance_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_storage_volume_provision_scope_id
    FOREIGN KEY (provision_scope_id)
    REFERENCES storage_provision_scope (id)
);

CREATE UNIQUE INDEX idx_storage_volume_id
ON storage_volume (volume_id);

-- index is required on provider_id because this is how we associate what we
-- find in the environ and re-attach it to a unit.
CREATE INDEX idx_storage_volume_provider_id
ON storage_volume (provider_id);

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
    provision_scope_id INT NOT NULL,
    provider_id TEXT,
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
    REFERENCES block_device (uuid),
    CONSTRAINT fk_storage_volume_attachment_provision_scope_id
    FOREIGN KEY (provision_scope_id)
    REFERENCES storage_provision_scope (id)
);

-- Until the storage provisioner can handle multi-attachment of volumes,
-- this will prevent that.
CREATE UNIQUE INDEX idx_storage_volume_attachment_volume_uuid
ON storage_volume_attachment (storage_volume_uuid);

CREATE INDEX idx_storage_volume_attachment_net_node_uuid
ON storage_volume_attachment (net_node_uuid);

CREATE INDEX idx_storage_volume_attachment_block_device_uuid
ON storage_volume_attachment (block_device_uuid);

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
(6, 'destroying'),
(7, 'tombstone');

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

-- storage_filesystem describes a filsystem held by a storage_instance.
--
-- obliterate_on_cleanup can only be set when life_id is not-alive(>0), to
-- ensure it is only set with intent to cause fileystem death.
CREATE TABLE storage_filesystem (
    uuid TEXT NOT NULL PRIMARY KEY,
    filesystem_id TEXT NOT NULL,
    life_id INT NOT NULL,
    provision_scope_id INT NOT NULL,
    provider_id TEXT,
    size_mib INT,
    obliterate_on_cleanup BOOLEAN,
    CONSTRAINT chk_storage_filesystem_obliterate_on_cleanup_set_when_not_alive
    CHECK (obliterate_on_cleanup IS NULL OR life_id <> 0),
    CONSTRAINT fk_storage_filesystem_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_storage_filesystem_provision_scope_id
    FOREIGN KEY (provision_scope_id)
    REFERENCES storage_provision_scope (id)
);

CREATE UNIQUE INDEX idx_storage_filesystem_id
ON storage_filesystem (filesystem_id);

-- index is required on provider_id because this is how we associate what we
-- find in the environ and re-attach it to a unit.
CREATE INDEX idx_storage_filesystem_provider_id
ON storage_filesystem (provider_id);

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
    provision_scope_id INT NOT NULL,
    provider_id TEXT,
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
    provision_scope_id INT NOT NULL,
    device_type_id INT,
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

-- There should only one volume attachment plan per net node and volume tuple.
CREATE UNIQUE INDEX idx_storage_volume_attachment_plan_net_node_uuid_volume_uuid
ON storage_volume_attachment_plan (storage_volume_uuid, net_node_uuid);

CREATE TABLE storage_volume_attachment_plan_attr (
    attachment_plan_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (attachment_plan_uuid, "key"),
    CONSTRAINT fk_storage_vol_attach_plan_attr_plan
    FOREIGN KEY (attachment_plan_uuid)
    REFERENCES storage_volume_attachment_plan (uuid)
);

CREATE UNIQUE INDEX idx_storage_vol_attachment_plan_attr
ON storage_volume_attachment_plan_attr (attachment_plan_uuid, "key");

-- Model database tables for secrets.

CREATE TABLE secret_rotate_policy (
    id INT PRIMARY KEY,
    policy TEXT NOT NULL,
    CONSTRAINT chk_empty_policy
    CHECK (policy != '')
);

CREATE UNIQUE INDEX idx_secret_rotate_policy_policy
ON secret_rotate_policy (policy);

INSERT INTO secret_rotate_policy VALUES
(0, 'never'),
(1, 'hourly'),
(2, 'daily'),
(3, 'weekly'),
(4, 'monthly'),
(5, 'quarterly'),
(6, 'yearly');

CREATE TABLE secret (
    id TEXT NOT NULL PRIMARY KEY
);

-- secret_reference stores details about
-- secrets hosted by another model and
-- is used on the consumer side of cross
-- model secrets.
CREATE TABLE secret_reference (
    secret_id TEXT NOT NULL PRIMARY KEY,
    latest_revision INT NOT NULL,
    owner_application_uuid TEXT NOT NULL,
    CONSTRAINT fk_secret_id
    FOREIGN KEY (secret_id)
    REFERENCES secret (id),
    CONSTRAINT fk_secret_reference_application_uuid
    FOREIGN KEY (owner_application_uuid)
    REFERENCES application (uuid)
);

CREATE TABLE secret_metadata (
    secret_id TEXT NOT NULL PRIMARY KEY,
    version INT NOT NULL,
    description TEXT,
    rotate_policy_id INT NOT NULL,
    auto_prune BOOLEAN NOT NULL DEFAULT (FALSE),
    latest_revision_checksum TEXT,
    create_time DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    update_time DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    CONSTRAINT fk_secret_id
    FOREIGN KEY (secret_id)
    REFERENCES secret (id),
    CONSTRAINT fk_secret_rotate_policy
    FOREIGN KEY (rotate_policy_id)
    REFERENCES secret_rotate_policy (id)
);

CREATE TABLE secret_rotation (
    secret_id TEXT NOT NULL PRIMARY KEY,
    next_rotation_time DATETIME NOT NULL,
    CONSTRAINT fk_secret_rotation_secret_metadata_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id)
);

-- 1:1
CREATE TABLE secret_value_ref (
    revision_uuid TEXT NOT NULL PRIMARY KEY,
    -- backend_uuid is the UUID of the backend in the controller database.
    backend_uuid TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    CONSTRAINT fk_secret_value_ref_secret_revision_uuid
    FOREIGN KEY (revision_uuid)
    REFERENCES secret_revision (uuid)
);

-- Deleted revisions for which content is stored externally.
-- These rows are deleted after the external content has been deleted.
CREATE TABLE secret_deleted_value_ref (
    revision_uuid TEXT NOT NULL PRIMARY KEY,
    backend_uuid TEXT NOT NULL,
    revision_id TEXT NOT NULL
);

-- 1:many
CREATE TABLE secret_content (
    revision_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    content TEXT NOT NULL,
    CONSTRAINT chk_empty_name
    CHECK (name != ''),
    CONSTRAINT chk_empty_content
    CHECK (content != ''),
    CONSTRAINT pk_secret_content_revision_uuid_name
    PRIMARY KEY (revision_uuid, name),
    CONSTRAINT fk_secret_content_secret_revision_uuid
    FOREIGN KEY (revision_uuid)
    REFERENCES secret_revision (uuid)
);

CREATE INDEX idx_secret_content_revision_uuid
ON secret_content (revision_uuid);

CREATE TABLE secret_revision (
    uuid TEXT NOT NULL PRIMARY KEY,
    secret_id TEXT NOT NULL,
    revision INT NOT NULL,
    create_time DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    CONSTRAINT fk_secret_revision_secret_metadata_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id)
);

CREATE UNIQUE INDEX idx_secret_revision_secret_id_revision
ON secret_revision (secret_id, revision);

CREATE TABLE secret_revision_obsolete (
    revision_uuid TEXT NOT NULL PRIMARY KEY,
    obsolete BOOLEAN NOT NULL DEFAULT (FALSE),
    -- pending_delete is true if the revision is to be deleted.
    -- It will not be drained to a new active backend.
    pending_delete BOOLEAN NOT NULL DEFAULT (FALSE),
    CONSTRAINT fk_secret_revision_obsolete_revision_uuid
    FOREIGN KEY (revision_uuid)
    REFERENCES secret_revision (uuid)
);

CREATE TABLE secret_revision_expire (
    revision_uuid TEXT NOT NULL PRIMARY KEY,
    expire_time DATETIME NOT NULL,
    CONSTRAINT fk_secret_revision_expire_revision_uuid
    FOREIGN KEY (revision_uuid)
    REFERENCES secret_revision (uuid)
);

CREATE TABLE secret_application_owner (
    secret_id TEXT NOT NULL,
    application_uuid TEXT NOT NULL,
    label TEXT,
    CONSTRAINT fk_secret_application_owner_secret_metadata_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id),
    CONSTRAINT fk_secret_application_owner_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    PRIMARY KEY (secret_id, application_uuid)
);

CREATE INDEX idx_secret_application_owner_secret_id
ON secret_application_owner (secret_id);
-- We need to ensure the label is unique per the application.
CREATE UNIQUE INDEX idx_secret_application_owner_label
ON secret_application_owner (label, application_uuid) WHERE label != '';

CREATE TABLE secret_unit_owner (
    secret_id TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    label TEXT,
    CONSTRAINT fk_secret_unit_owner_secret_metadata_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id),
    CONSTRAINT fk_secret_unit_owner_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    PRIMARY KEY (secret_id, unit_uuid)
);

CREATE INDEX idx_secret_unit_owner_secret_id
ON secret_unit_owner (secret_id);
-- We need to ensure the label is unique per unit.
CREATE UNIQUE INDEX idx_secret_unit_owner_label
ON secret_unit_owner (label, unit_uuid) WHERE label != '';

CREATE TABLE secret_model_owner (
    secret_id TEXT NOT NULL PRIMARY KEY,
    label TEXT,
    CONSTRAINT fk_secret_model_owner_secret_metadata_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id)
);

CREATE UNIQUE INDEX idx_secret_model_owner_label
ON secret_model_owner (label) WHERE label != '';

CREATE TABLE secret_unit_consumer (
    secret_id TEXT NOT NULL,
    -- source model uuid may be this model or a different model
    -- possibly on another controller
    source_model_uuid TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    label TEXT,
    current_revision INT NOT NULL,
    CONSTRAINT fk_secret_unit_consumer_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_secret_unit_consumer_secret_id
    FOREIGN KEY (secret_id)
    REFERENCES secret (id)
);

CREATE UNIQUE INDEX idx_secret_unit_consumer_secret_id_unit_uuid
ON secret_unit_consumer (secret_id, unit_uuid);

CREATE UNIQUE INDEX idx_secret_unit_consumer_label
ON secret_unit_consumer (label, unit_uuid) WHERE label != '';

-- This table records the tracked revisions from
-- units in the consuming model for cross model secrets.
CREATE TABLE secret_remote_unit_consumer (
    secret_id TEXT NOT NULL,
    -- unit_name is the anonymised name of the unit
    -- from the consuming model.
    unit_name TEXT NOT NULL,
    current_revision INT NOT NULL,
    CONSTRAINT fk_secret_remote_unit_consumer_secret_metadata_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id)
);

CREATE UNIQUE INDEX idx_secret_remote_unit_consumer_secret_id_unit_name
ON secret_remote_unit_consumer (secret_id, unit_name);

CREATE TABLE secret_role (
    id INT PRIMARY KEY,
    role TEXT
);

CREATE UNIQUE INDEX idx_secret_role_role ON secret_role (role);

INSERT INTO secret_role VALUES
(0, 'none'),
(1, 'view'),
(2, 'manage');

CREATE TABLE secret_grant_subject_type (
    id INT PRIMARY KEY,
    type TEXT
);

INSERT INTO secret_grant_subject_type VALUES
(0, 'unit'),
(1, 'application'),
(2, 'model');

CREATE TABLE secret_grant_scope_type (
    id INT PRIMARY KEY,
    type TEXT
);

INSERT INTO secret_grant_scope_type VALUES
(0, 'unit'),
(1, 'application'),
(2, 'model'),
(3, 'relation');

CREATE TABLE secret_permission (
    secret_id TEXT NOT NULL,
    role_id INT NOT NULL,
    -- subject_uuid is the entity which
    -- has been granted access to a secret.
    -- It will be an application, unit, or model uuid.
    subject_uuid TEXT NOT NULL,
    subject_type_id INT NOT NULL,
    -- scope_uuid is the entity which
    -- defines the scope of the grant.
    -- It will be an application, unit, relation, or model uuid.
    scope_uuid TEXT NOT NULL,
    scope_type_id TEXT NOT NULL,
    CONSTRAINT pk_secret_permission_secret_id_subject_uuid
    PRIMARY KEY (secret_id, subject_uuid),
    CONSTRAINT chk_empty_scope_uuid
    CHECK (scope_uuid != ''),
    CONSTRAINT chk_empty_subject_uuid
    CHECK (subject_uuid != ''),
    CONSTRAINT fk_secret_permission_secret_id
    FOREIGN KEY (secret_id)
    REFERENCES secret_metadata (secret_id),
    CONSTRAINT fk_secret_permission_secret_role_id
    FOREIGN KEY (role_id)
    REFERENCES secret_role (id),
    CONSTRAINT fk_secret_permission_secret_grant_subject_type_id
    FOREIGN KEY (subject_type_id)
    REFERENCES secret_grant_subject_type (id),
    CONSTRAINT fk_secret_permission_secret_grant_scope_type_id
    FOREIGN KEY (scope_type_id)
    REFERENCES secret_grant_scope_type (id)
);

CREATE INDEX idx_secret_permission_secret_id
ON secret_permission (secret_id);

CREATE INDEX idx_secret_permission_subject_uuid_subject_type_id
ON secret_permission (subject_uuid, subject_type_id);

-- v_secret_permission is used to query secrets which can
-- be accessed by a subject of application, unit, or model.
CREATE VIEW v_secret_permission AS
SELECT
    sp.secret_id,
    sp.role_id,
    sp.subject_type_id,
    sp.subject_uuid,
    sp.scope_type_id,
    sp.scope_uuid,
    -- subject_id is the natural id of the subject entity (uuid for model)
    (CASE
        WHEN sp.subject_type_id = 0 THEN suu.name
        WHEN sp.subject_type_id = 1 THEN sua.name
        WHEN sp.subject_type_id = 2 THEN m.uuid
    END) AS subject_id,
    -- scope_id is the natural id of the scope entity (uuid for model)
    -- relations are not processed here - their scope_id is the relation key
    -- which needs to be composed in code. 
    (CASE
        WHEN sp.scope_type_id = 0 THEN scu.name
        WHEN sp.scope_type_id = 1 THEN sca.name
        WHEN sp.scope_type_id = 2 THEN m.uuid
    END) AS scope_id
FROM secret_permission AS sp
LEFT JOIN unit AS suu ON sp.subject_uuid = suu.uuid
LEFT JOIN application AS sua ON sp.subject_uuid = sua.uuid
LEFT JOIN unit AS scu ON sp.scope_uuid = scu.uuid
LEFT JOIN application AS sca ON sp.scope_uuid = sca.uuid
JOIN model AS m;

CREATE TABLE annotation_model (
    "key" TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE annotation_application (
    uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (uuid, "key")
    -- Following needs to be uncommented when we do have the
    -- annotatables as real domain entities.
    -- CONSTRAINT          fk_annotation_application
    --     FOREIGN KEY     (uuid)
    --     REFERENCES      application(uuid)
);

CREATE TABLE annotation_charm (
    uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (uuid, "key"),
    CONSTRAINT fk_annotation_charm
    FOREIGN KEY (uuid)
    REFERENCES charm (uuid)
);

CREATE TABLE annotation_machine (
    uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (uuid, "key")
    -- Following needs to be uncommented when we do have the
    -- annotatables as real domain entities.
    -- CONSTRAINT          fk_annotation_machine
    --     FOREIGN KEY     (uuid)
    --     REFERENCES      machine(uuid)
);

CREATE TABLE annotation_unit (
    uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (uuid, "key"),
    CONSTRAINT fk_annotation_unit
    FOREIGN KEY (uuid)
    REFERENCES unit (uuid)
);

CREATE TABLE annotation_storage_instance (
    uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (uuid, "key")
    -- Following needs to be uncommented when we do have the
    -- annotatables as real domain entities.
    -- CONSTRAINT          fk_annotation_storage_instance
    --     FOREIGN KEY     (uuid)
    --     REFERENCES      storage_instance(uuid)
);

CREATE TABLE annotation_storage_volume (
    uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (uuid, "key")
    -- Following needs to be uncommented when we do have the
    -- annotatables as real domain entities.
    -- CONSTRAINT          fk_annotation_storage_volume
    --     FOREIGN KEY     (uuid)
    --     REFERENCES      storage_volume(uuid)
);

CREATE TABLE annotation_storage_filesystem (
    uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (uuid, "key")
    -- Following needs to be uncommented when we do have the
    -- annotatables as real domain entities.
    -- CONSTRAINT          fk_annotation_storage_filesystem
    --     FOREIGN KEY     (uuid)
    --     REFERENCES      storage_filesystem(uuid)
);

-- The allocate_public_ip column shold have been a (nullable) boolean, but
-- there is currently a bug somewhere between dqlite and go-dqlite that returns
-- a false boolean instead a null. Since the driver will correctly map a INT
-- to a boolean, then we can safely use it here as a workaround.
CREATE TABLE "constraint" (
    uuid TEXT NOT NULL PRIMARY KEY,
    arch TEXT,
    cpu_cores INT,
    cpu_power INT,
    mem INT,
    root_disk INT,
    root_disk_source TEXT,
    instance_role TEXT,
    instance_type TEXT,
    container_type_id INT,
    virt_type TEXT,
    -- allocate_public_ip is a bool value. We only use int to get around DQlite
    -- limitations with NULL bools.
    allocate_public_ip INT,
    image_id TEXT,
    CONSTRAINT fk_constraint_container_type
    FOREIGN KEY (container_type_id)
    REFERENCES container_type (id)
) STRICT;

-- v_constraint represents a view of the constraints in the model with foreign
-- keys resolved for the viewer.
CREATE VIEW v_constraint AS
SELECT
    c.uuid,
    c.arch,
    c.cpu_cores,
    c.cpu_power,
    c.mem,
    c.root_disk,
    c.root_disk_source,
    c.instance_role,
    c.instance_type,
    ct.value AS container_type,
    c.virt_type,
    c.allocate_public_ip,
    c.image_id
FROM "constraint" AS c
LEFT JOIN container_type AS ct ON c.container_type_id = ct.id;

CREATE TABLE constraint_tag (
    constraint_uuid TEXT NOT NULL,
    tag TEXT NOT NULL,
    CONSTRAINT fk_constraint_tag_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid),
    PRIMARY KEY (constraint_uuid, tag)
);

CREATE TABLE constraint_space (
    constraint_uuid TEXT NOT NULL,
    space TEXT NOT NULL,
    "exclude" BOOLEAN NOT NULL,
    CONSTRAINT fk_constraint_space_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid),
    CONSTRAINT fk_constraint_space_space
    FOREIGN KEY (space)
    REFERENCES space (name),
    PRIMARY KEY (constraint_uuid, space)
);

CREATE TABLE constraint_zone (
    constraint_uuid TEXT NOT NULL,
    zone TEXT NOT NULL,
    CONSTRAINT fk_constraint_zone_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid),
    PRIMARY KEY (constraint_uuid, zone)
);

CREATE TABLE charm_run_as_kind (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_run_as_kind_name
ON charm_run_as_kind (name);

INSERT INTO charm_run_as_kind VALUES
(0, 'default'),
(1, 'root'),
(2, 'sudoer'),
(3, 'non-root');

CREATE TABLE charm_source (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_source_name
ON charm_source (name);

INSERT INTO charm_source VALUES
(0, 'local'),
(1, 'charmhub'),
(2, 'cmr');

-- The charm table exists as the nexus to all charm data. 
--
-- The fact that the charm is in the database indicates that it's a placeholder.
-- Updating the available flag to true indicates that the charm is now available
-- for deployment.
CREATE TABLE charm (
    uuid TEXT NOT NULL PRIMARY KEY,
    -- Archive path is the path to the charm archive on disk. This is used to
    -- determine the source of the charm.
    archive_path TEXT,
    object_store_uuid TEXT,

    available BOOLEAN DEFAULT FALSE,

    version TEXT,
    lxd_profile TEXT,

    -- The following fields are purely here to reconstruct the charm URL.
    -- Once we have the ability to only talk about charms in terms of a UUID,
    -- these fields can be removed.
    -- These values are not intended to be used for any other purpose, they
    -- should not be used as a way to "derive" the charm origin. That concept
    -- is for applications.
    -- Note: revision is used to create lxd profile names, if removed, the
    -- unique profile naming must be resolved.

    source_id INT NOT NULL DEFAULT 1,
    revision INT NOT NULL DEFAULT -1,

    -- architecture_id may be null for local charms, but must be NULL for
    -- CMR charms as they are architecture agnostic.
    architecture_id INT,

    -- reference_name is the name of the charm that was originally supplied.
    -- The charm name can be different from the actual charm name in the
    -- metadata. If it's downloaded from charmhub the reference_name will be
    -- the name of the charm in the charmhub store. This is the transient
    -- name of the charm.
    --
    -- This can happen if the charm was uploaded to charmhub with a different
    -- name than the charm name in the metadata.yaml file.
    reference_name TEXT NOT NULL,

    -- create_time is purely used for ordering a charm by time, as we can't
    -- use the revision number to determine the order of the charm.
    create_time DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),

    CONSTRAINT fk_charm_source_source
    FOREIGN KEY (source_id)
    REFERENCES charm_source (id),
    CONSTRAINT fk_charm_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id),
    CONSTRAINT fk_charm_object_store_metadata
    FOREIGN KEY (object_store_uuid)
    REFERENCES object_store_metadata (uuid),

    -- Ensure we have an architecture if the source is local or charmhub.
    CONSTRAINT chk_charm_architecture
    CHECK (((source_id = 0 OR source_id = 1) AND architecture_id >= 0) OR (source_id = 2 AND architecture_id IS NULL)),

    -- Ensure we don't have an empty reference
    CONSTRAINT chk_charm_reference_name
    CHECK (reference_name <> '')
);

-- This ensures that the reference name and revision are unique. This is to
-- ensure that we don't have two charms with the same reference name and
-- revision. If this happens, we can just link the application to the existing
-- charm.
CREATE UNIQUE INDEX idx_charm_reference_name_revision
ON charm (source_id, reference_name, revision);

CREATE TABLE charm_provenance (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_provenance_name
ON charm_provenance (name);

-- The provenance of the charm. This is used to determine where the charm
-- came from, which can then determine if the download information is still
-- relevant.
INSERT INTO charm_provenance VALUES
(0, 'download'),
(1, 'migration'),
(2, 'upload'),
(3, 'bootstrap');

CREATE TABLE charm_download_info (
    charm_uuid TEXT NOT NULL PRIMARY KEY,

    -- The provenance_id is the origin from which the download information
    -- was obtained. Ideally, we would have used origin, but that's already
    -- taken and I don't want to confuse the two.
    provenance_id INT NOT NULL,

    -- charmhub_identifier is the identifier that charmhub uses to identify the
    -- charm. This is used to refresh the charm from charmhub. The
    -- reference_name can change but the charmhub_identifier will not.
    charmhub_identifier TEXT,

    download_url TEXT NOT NULL,
    download_size INT NOT NULL,

    CONSTRAINT fk_charm_download_info_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid)
);

CREATE VIEW v_application_charm_download_info AS
SELECT
    a.uuid AS application_uuid,
    c.uuid AS charm_uuid,
    c.reference_name AS name,
    c.available,
    cs.id AS source_id,
    cp.name AS provenance,
    cdi.charmhub_identifier,
    cdi.download_url,
    cdi.download_size,
    ch.hash
FROM application AS a
LEFT JOIN charm AS c ON a.charm_uuid = c.uuid
LEFT JOIN charm_download_info AS cdi ON c.uuid = cdi.charm_uuid
LEFT JOIN charm_provenance AS cp ON cdi.provenance_id = cp.id
LEFT JOIN charm_source AS cs ON c.source_id = cs.id
LEFT JOIN charm_hash AS ch ON c.uuid = ch.charm_uuid;

CREATE TABLE charm_metadata (
    charm_uuid TEXT NOT NULL PRIMARY KEY,
    -- name represents the original name of the charm. This is what is stored
    -- in the charm metadata.yaml file.
    name TEXT NOT NULL,
    description TEXT,
    summary TEXT,
    subordinate BOOLEAN NOT NULL DEFAULT FALSE,
    min_juju_version TEXT,
    run_as_id INT DEFAULT 0,
    -- Assumes is a blob of YAML that will be parsed by the charm to compute
    -- the result of the SAT expression.
    -- As the expression tree is generic, you can't use RI or index into the
    -- blob without constraining the expression to a specific set of rules.
    assumes TEXT,
    CONSTRAINT fk_charm_run_as_kind_charm
    FOREIGN KEY (run_as_id)
    REFERENCES charm_run_as_kind (id),
    CONSTRAINT fk_charm_metadata_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid)
);


CREATE INDEX idx_charm_metadata_subordinate
ON charm_metadata (subordinate);

CREATE VIEW v_charm_metadata AS
SELECT
    c.uuid,
    cm.name,
    cm.description,
    cm.summary,
    cm.subordinate,
    cm.min_juju_version,
    crak.name AS run_as,
    cm.assumes,
    c.available
FROM charm AS c
LEFT JOIN charm_metadata AS cm ON c.uuid = cm.charm_uuid
LEFT JOIN charm_run_as_kind AS crak ON cm.run_as_id = crak.id;

CREATE VIEW v_charm_annotation_index AS
SELECT
    c.uuid,
    c.revision,
    cm.name
FROM charm AS c
LEFT JOIN charm_metadata AS cm ON c.uuid = cm.charm_uuid;

CREATE TABLE hash_kind (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_hash_kind_name
ON hash_kind (name);

-- We only support sha256 hashes for now.
INSERT INTO hash_kind VALUES
(0, 'sha256');

CREATE TABLE charm_hash (
    charm_uuid TEXT NOT NULL,
    hash_kind_id INT NOT NULL DEFAULT 0,
    hash TEXT NOT NULL,
    CONSTRAINT fk_charm_hash_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_hash_kind
    FOREIGN KEY (hash_kind_id)
    REFERENCES hash_kind (id)
);

CREATE TABLE charm_relation_role (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_relation_role_name
ON charm_relation_role (name);

INSERT INTO charm_relation_role VALUES
(0, 'provider'),
(1, 'requirer'),
(2, 'peer');

CREATE TABLE charm_relation_scope (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_relation_scope_name
ON charm_relation_scope (name);

INSERT INTO charm_relation_scope VALUES
(0, 'global'),
(1, 'container');

CREATE TABLE charm_relation (
    uuid TEXT NOT NULL PRIMARY KEY,
    charm_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    role_id INT NOT NULL,
    scope_id INT NOT NULL,
    interface TEXT,
    optional BOOLEAN,
    capacity INT,
    CONSTRAINT fk_charm_relation_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_relation_role
    FOREIGN KEY (role_id)
    REFERENCES charm_relation_role (id),
    CONSTRAINT fk_charm_relation_scope
    FOREIGN KEY (scope_id)
    REFERENCES charm_relation_scope (id)
);

CREATE UNIQUE INDEX idx_charm_relation_charm_key
ON charm_relation (charm_uuid, name);

CREATE VIEW v_charm_relation AS
SELECT
    cr.uuid,
    cr.charm_uuid,
    cr.name,
    crr.name AS role,
    cr.interface,
    cr.optional,
    cr.capacity,
    crs.name AS scope
FROM charm_relation AS cr
JOIN charm_relation_role AS crr ON cr.role_id = crr.id
JOIN charm_relation_scope AS crs ON cr.scope_id = crs.id;

CREATE INDEX idx_charm_relation_charm
ON charm_relation (charm_uuid);

CREATE TABLE charm_extra_binding (
    uuid TEXT NOT NULL PRIMARY KEY,
    charm_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    CONSTRAINT fk_charm_extra_binding_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid)
);

CREATE INDEX idx_charm_extra_binding_charm
ON charm_extra_binding (charm_uuid, name);

-- charm_category is a limited set of categories that a charm can be tagged
-- for the charmhub store. This is free form and driven by the charmhub store.
-- We're not enforcing any constraints on the values as that can be changed
-- by 3rd party stores.
CREATE TABLE charm_category (
    charm_uuid TEXT NOT NULL,
    array_index INT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_charm_category_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, array_index, value)
);

CREATE INDEX idx_charm_category_charm
ON charm_category (charm_uuid);

-- charm_tag is a free form tag that can be applied to a charm.
CREATE TABLE charm_tag (
    charm_uuid TEXT NOT NULL,
    array_index INT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_charm_tag_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, array_index, value)
);

CREATE INDEX idx_charm_tag_charm
ON charm_tag (charm_uuid);

CREATE TABLE charm_storage_kind (
    id INT PRIMARY KEY,
    kind TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_storage_kind
ON charm_storage_kind (kind);

INSERT INTO charm_storage_kind VALUES
(0, 'block'),
(1, 'filesystem');

CREATE TABLE charm_storage (
    charm_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    storage_kind_id INT NOT NULL,
    shared BOOLEAN NOT NULL,
    read_only BOOLEAN,
    count_min INT NOT NULL,
    count_max INT NOT NULL,
    minimum_size_mib INT,
    location TEXT,
    CONSTRAINT fk_charm_storage_charm_storage_kind
    FOREIGN KEY (storage_kind_id)
    REFERENCES charm_storage_kind (id),
    CONSTRAINT fk_charm_storage_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, name)
);

CREATE VIEW v_charm_storage AS
SELECT
    cs.charm_uuid,
    cs.name,
    cs.description,
    csk.kind,
    cs.shared,
    cs.read_only,
    cs.count_min,
    cs.count_max,
    cs.minimum_size_mib,
    cs.location,
    csp.array_index AS property_index,
    csp.value AS property
FROM charm_storage AS cs
LEFT JOIN charm_storage_kind AS csk ON cs.storage_kind_id = csk.id
LEFT JOIN charm_storage_property AS csp ON cs.charm_uuid = csp.charm_uuid AND cs.name = csp.charm_storage_name;

CREATE INDEX idx_charm_storage_charm
ON charm_storage (charm_uuid);

CREATE TABLE charm_storage_property (
    charm_uuid TEXT NOT NULL,
    charm_storage_name TEXT NOT NULL,
    array_index INT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_charm_storage_property_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_storage_property_charm_storage
    FOREIGN KEY (charm_uuid, charm_storage_name)
    REFERENCES charm_storage (charm_uuid, name),
    PRIMARY KEY (charm_uuid, charm_storage_name, array_index, value)
);

CREATE INDEX idx_charm_storage_property_charm
ON charm_storage_property (charm_uuid);

CREATE TABLE charm_device (
    charm_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    name TEXT,
    description TEXT,
    device_type TEXT,
    count_min INT NOT NULL,
    count_max INT NOT NULL,
    CONSTRAINT fk_charm_device_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, "key")
);

CREATE INDEX idx_charm_device_charm
ON charm_device (charm_uuid);

CREATE TABLE charm_resource_kind (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_resource_kind_name
ON charm_resource_kind (name);

INSERT INTO charm_resource_kind VALUES
(0, 'file'),
(1, 'oci-image');

CREATE TABLE charm_resource (
    charm_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    kind_id INT NOT NULL,
    path TEXT,
    description TEXT,
    CONSTRAINT fk_charm_resource_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_resource_charm_resource_kind
    FOREIGN KEY (kind_id)
    REFERENCES charm_resource_kind (id),
    PRIMARY KEY (charm_uuid, name)
);

CREATE VIEW v_charm_resource AS
SELECT
    cr.charm_uuid,
    cr.name,
    crk.name AS kind,
    cr.path,
    cr.description
FROM charm_resource AS cr
LEFT JOIN charm_resource_kind AS crk ON cr.kind_id = crk.id;

CREATE INDEX idx_charm_resource_charm
ON charm_resource (charm_uuid);

CREATE TABLE charm_term (
    charm_uuid TEXT NOT NULL,
    array_index INT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_charm_term_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, array_index, value)
);

CREATE INDEX idx_charm_term_charm
ON charm_term (charm_uuid);

CREATE TABLE charm_container (
    charm_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    resource TEXT,
    -- Enforce the optional uid and gid to -1 if not set, otherwise the it might
    -- become 0, which happens to be root.
    uid INT DEFAULT -1,
    gid INT DEFAULT -1,
    CONSTRAINT fk_charm_container_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, "key")
);

CREATE VIEW v_charm_container AS
SELECT
    cc.charm_uuid,
    cc."key",
    cc.resource,
    cc.uid,
    cc.gid,
    ccm.array_index,
    ccm.storage,
    ccm.location
FROM charm_container AS cc
LEFT JOIN charm_container_mount AS ccm ON cc.charm_uuid = ccm.charm_uuid AND cc."key" = ccm.charm_container_key;

CREATE INDEX idx_charm_container_charm
ON charm_container (charm_uuid);

CREATE TABLE charm_container_mount (
    array_index INT NOT NULL,
    charm_uuid TEXT NOT NULL,
    charm_container_key TEXT,
    storage TEXT,
    location TEXT,
    CONSTRAINT fk_charm_container_mount_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_container_mount_charm_container
    FOREIGN KEY (charm_uuid, charm_container_key)
    REFERENCES charm_container (charm_uuid, "key"),
    PRIMARY KEY (charm_uuid, charm_container_key, array_index)
);

CREATE INDEX idx_charm_container_mount_charm
ON charm_container_mount (charm_uuid);

CREATE TABLE charm_action (
    charm_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    description TEXT,
    parallel BOOLEAN,
    execution_group TEXT,
    params TEXT,
    CONSTRAINT fk_charm_actions_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, "key")
);

CREATE TABLE charm_manifest_base (
    charm_uuid TEXT NOT NULL,
    array_index INT NOT NULL,
    nested_array_index INT NOT NULL,
    os_id INT DEFAULT 0,
    track TEXT,
    risk TEXT NOT NULL,
    branch TEXT,
    architecture_id INT DEFAULT 0,
    CONSTRAINT fk_charm_manifest_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_manifest_base_os
    FOREIGN KEY (os_id)
    REFERENCES os (id),
    CONSTRAINT fk_charm_manifest_base_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id),
    PRIMARY KEY (charm_uuid, array_index, nested_array_index, os_id, track, risk, branch, architecture_id)
);

CREATE VIEW v_charm_manifest AS
SELECT
    cmb.charm_uuid,
    cmb.array_index,
    cmb.nested_array_index,
    cmb.track,
    cmb.risk,
    cmb.branch,
    os.name AS os,
    architecture.name AS architecture
FROM charm_manifest_base AS cmb
LEFT JOIN os ON cmb.os_id = os.id
LEFT JOIN architecture ON cmb.architecture_id = architecture.id;

CREATE TABLE charm_config_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_charm_config_type_name
ON charm_config_type (name);

INSERT INTO charm_config_type VALUES
(0, 'string'),
(1, 'int'),
(2, 'float'),
(3, 'boolean'),
(4, 'secret');

CREATE TABLE charm_config (
    charm_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    type_id TEXT,
    default_value TEXT,
    description TEXT,
    CONSTRAINT fk_charm_config_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_config_charm_config_type
    FOREIGN KEY (type_id)
    REFERENCES charm_config_type (id),
    PRIMARY KEY (charm_uuid, "key")
);

CREATE VIEW v_charm_config AS
SELECT
    cc.charm_uuid,
    cc."key",
    cct.name AS type,
    cc.default_value,
    cc.description
FROM charm_config AS cc
LEFT JOIN charm_config_type AS cct ON cc.type_id = cct.id;

-- Status values for unit and application workloads.
CREATE TABLE workload_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO workload_status_value VALUES
(0, 'unset'),
(1, 'unknown'),
(2, 'maintenance'),
(3, 'waiting'),
(4, 'blocked'),
(5, 'active'),
(6, 'terminated'),
(7, 'error');

CREATE TABLE machine_cloud_instance (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    life_id INT NOT NULL,
    -- Instance ID is optional, because it won't be set until the instance
    -- is actually created in the cloud provider. Otherwise, the record is used
    -- to track the status of the instance creation process.
    instance_id TEXT,
    display_name TEXT,
    -- The data that is reported here is the cloud specific instance 
    -- information.
    arch TEXT,
    availability_zone_uuid TEXT,
    cpu_cores INT,
    cpu_power INT,
    mem INT,
    root_disk INT,
    root_disk_source TEXT,
    virt_type TEXT,
    CONSTRAINT fk_machine_machine_uuid
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_instance_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_availability_zone_availability_zone_uuid
    FOREIGN KEY (availability_zone_uuid)
    REFERENCES availability_zone (uuid)
);

CREATE UNIQUE INDEX idx_machine_cloud_instance_instance_id
ON machine_cloud_instance (instance_id)
WHERE instance_id IS NOT NULL AND instance_id != '';

CREATE UNIQUE INDEX idx_machine_cloud_instance_display_name
ON machine_cloud_instance (display_name)
WHERE display_name IS NOT NULL AND display_name != '';

CREATE VIEW v_hardware_characteristics AS
SELECT
    m.machine_uuid,
    m.instance_id,
    m.arch,
    m.mem,
    m.root_disk,
    m.root_disk_source,
    m.cpu_cores,
    m.cpu_power,
    m.virt_type,
    az.name AS availability_zone_name,
    az.uuid AS availability_zone_uuid
FROM machine_cloud_instance AS m
LEFT JOIN availability_zone AS az ON m.availability_zone_uuid = az.uuid;

CREATE TABLE instance_tag (
    machine_uuid TEXT NOT NULL,
    tag TEXT NOT NULL,
    PRIMARY KEY (machine_uuid, tag),
    CONSTRAINT fk_machine_machine_uuid
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

CREATE TABLE machine_cloud_instance_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO machine_cloud_instance_status_value VALUES
(0, 'unknown'),
(1, 'pending'),
(2, 'allocating'),
(3, 'running'),
(4, 'provisioning error');

CREATE TABLE machine_cloud_instance_status (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    data TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_machine_instance_status_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine_cloud_instance (machine_uuid),
    CONSTRAINT fk_machine_instance_status_id
    FOREIGN KEY (status_id)
    REFERENCES machine_cloud_instance_status_value (id)
);

CREATE VIEW v_machine_cloud_instance_status AS
SELECT
    ms.machine_uuid,
    ms.message,
    ms.data,
    ms.updated_at,
    msv.status
FROM machine_cloud_instance_status AS ms
JOIN machine_cloud_instance_status_value AS msv ON ms.status_id = msv.id;

CREATE TABLE machine (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    net_node_uuid TEXT NOT NULL,
    life_id INT NOT NULL,
    nonce TEXT,
    password_hash_algorithm_id TEXT,
    password_hash TEXT,
    force_destroyed BOOLEAN DEFAULT FALSE,
    agent_started_at DATETIME,
    hostname TEXT,
    keep_instance BOOLEAN,
    CONSTRAINT fk_machine_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_machine_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_machine_password_hash_algorithm
    FOREIGN KEY (password_hash_algorithm_id)
    REFERENCES password_hash_algorithm (id)
);

CREATE UNIQUE INDEX idx_machine_name
ON machine (name);

CREATE UNIQUE INDEX idx_machine_net_node
ON machine (net_node_uuid);

CREATE TABLE machine_manual (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    CONSTRAINT fk_machine_manual_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

CREATE TABLE machine_platform (
    machine_uuid TEXT NOT NULL,
    os_id TEXT NOT NULL,
    channel TEXT,
    architecture_id INT NOT NULL,
    CONSTRAINT fk_machine_platform_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_platform_os
    FOREIGN KEY (os_id)
    REFERENCES os (id),
    CONSTRAINT fk_machine_platform_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

-- machine_placement_scope is a table which represents the valid scopes
-- that can exist for a machine placement. The provider scope is the only
-- placement that is deferred until the instance is started by the provider.
-- Other scopes can be added i.e. scriptlets.
CREATE TABLE machine_placement_scope (
    id INT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO machine_placement_scope VALUES
(0, 'provider');

CREATE TABLE machine_placement (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    scope_id INT NOT NULL,
    directive TEXT NOT NULL,
    CONSTRAINT fk_machine_placement_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_placement_scope
    FOREIGN KEY (scope_id)
    REFERENCES machine_placement_scope (id)
);

-- machine_parent table is a table which represents parents-children
-- relationships of machines. Each machine can have a single parent or be a
-- parent to multiple children.
CREATE TABLE machine_parent (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    parent_uuid TEXT NOT NULL,
    CONSTRAINT fk_machine_parent_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_parent_parent
    FOREIGN KEY (parent_uuid)
    REFERENCES machine (uuid)
);

-- machine_agent_version tracks the reported agent version running for each
-- machine.
CREATE TABLE machine_agent_version (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    version TEXT NOT NULL,
    -- We don't want to link architecture here with that of the architecture
    -- that is on the machine. While correlation can be applied one deals with
    -- what should be the case and this field deals with what is running.
    architecture_id INT NOT NULL,
    CONSTRAINT fk_machine_agent_version_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_agent_version_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

-- v_machine_agent_version provides a convenience view on the
-- machine_agent_version reporting the architecture name as well as the id.
-- This currently exists as a view because SQLAir doesn't support AS redefines
-- on select columns. SQLAir issue #179 was created to track this.
CREATE VIEW v_machine_agent_version AS
SELECT
    m.name,
    mav.machine_uuid,
    mav.architecture_id,
    mav.version,
    a.name AS architecture_name
FROM machine_agent_version AS mav
JOIN machine AS m ON mav.machine_uuid = m.uuid
JOIN architecture AS a ON mav.architecture_id = a.id;

-- v_machine_target_agent_version provides a convenience view for establishing
-- what the current target agent version  for a machine. A machine will only
-- have a record in this view if a target agent version has been set for the
-- model and the machine has had its running machine agent version set.
CREATE VIEW v_machine_target_agent_version AS
SELECT
    m.name,
    mav.machine_uuid,
    mav.architecture_id,
    a.name AS architecture_name,
    mav.version,
    av.target_version
FROM machine_agent_version AS mav
JOIN machine AS m ON mav.machine_uuid = m.uuid
JOIN architecture AS a ON mav.architecture_id = a.id
JOIN agent_version AS av;

CREATE TABLE machine_constraint (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    constraint_uuid TEXT NOT NULL,
    CONSTRAINT fk_machine_constraint_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_constraint_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid)
);

CREATE TABLE machine_volume (
    machine_uuid TEXT NOT NULL,
    volume_uuid TEXT NOT NULL,
    CONSTRAINT fk_machine_volume_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_volume_volume
    FOREIGN KEY (volume_uuid)
    REFERENCES storage_volume (uuid),
    PRIMARY KEY (machine_uuid, volume_uuid)
);

CREATE TABLE machine_filesystem (
    machine_uuid TEXT NOT NULL,
    filesystem_uuid TEXT NOT NULL,
    CONSTRAINT fk_machine_filesystem_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_filesystem_filesystem
    FOREIGN KEY (filesystem_uuid)
    REFERENCES storage_filesystem (uuid),
    PRIMARY KEY (machine_uuid, filesystem_uuid)
);

CREATE TABLE machine_requires_reboot (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    created_at DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    CONSTRAINT fk_machine_requires_reboot_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

CREATE TABLE machine_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO machine_status_value VALUES
(0, 'error'),
(1, 'started'),
(2, 'pending'),
(3, 'stopped'),
(4, 'down');

CREATE TABLE machine_status (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    data TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_machine_constraint_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_constraint_status
    FOREIGN KEY (status_id)
    REFERENCES machine_status_value (id)
);

CREATE VIEW v_machine_status AS
SELECT
    ms.machine_uuid,
    ms.message,
    ms.data,
    ms.updated_at,
    msv.status
FROM machine_status AS ms
JOIN machine_status_value AS msv ON ms.status_id = msv.id;

-- machine_lxd_profile table keeps track of the lxd profiles (previously
-- charm-profiles) for a machine.
CREATE TABLE machine_lxd_profile (
    machine_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    array_index INT NOT NULL,
    PRIMARY KEY (machine_uuid, name),
    CONSTRAINT fk_lxd_profile_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

-- container_type represents the valid container types that can exist for an
-- instance.
CREATE TABLE container_type (
    id INT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO container_type VALUES
(0, 'none'),
(1, 'lxd');

CREATE TABLE machine_container_type (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    container_type_id INT NOT NULL,
    CONSTRAINT fk_machine_container_type_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_machine_container_type_container_type
    FOREIGN KEY (container_type_id)
    REFERENCES container_type (id)
);

CREATE TABLE machine_agent_presence (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    last_seen DATETIME,
    CONSTRAINT fk_machine_agent_presence_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

CREATE VIEW v_machine_is_controller AS
SELECT m.uuid AS machine_uuid
FROM machine AS m
JOIN net_node AS n ON m.net_node_uuid = n.uuid
JOIN unit AS u ON n.uuid = u.net_node_uuid
JOIN application AS a ON u.application_uuid = a.uuid
JOIN application_controller AS ac ON a.uuid = ac.application_uuid;

CREATE VIEW v_machine_constraint AS
SELECT
    mc.machine_uuid,
    c.arch,
    c.cpu_cores,
    c.cpu_power,
    c.mem,
    c.root_disk,
    c.root_disk_source,
    c.instance_role,
    c.instance_type,
    ctype.value AS container_type,
    c.virt_type,
    c.allocate_public_ip,
    c.image_id,
    ctag.tag,
    cspace.space AS space_name,
    cspace."exclude" AS space_exclude,
    czone.zone
FROM machine_constraint AS mc
JOIN "constraint" AS c ON mc.constraint_uuid = c.uuid
LEFT JOIN container_type AS ctype ON c.container_type_id = ctype.id
LEFT JOIN constraint_tag AS ctag ON c.uuid = ctag.constraint_uuid
LEFT JOIN constraint_space AS cspace ON c.uuid = cspace.constraint_uuid
LEFT JOIN constraint_zone AS czone ON c.uuid = czone.constraint_uuid;

CREATE VIEW v_machine_platform AS
SELECT
    mp.machine_uuid,
    os.name AS os_name,
    mp.channel,
    a.name AS architecture
FROM machine_platform AS mp
JOIN os ON mp.os_id = os.id
JOIN architecture AS a ON mp.architecture_id = a.id;

CREATE TABLE machine_ssh_host_key (
    uuid TEXT NOT NULL PRIMARY KEY,
    machine_uuid TEXT NOT NULL,
    ssh_key TEXT NOT NULL,
    CONSTRAINT fk_machine_ssh_host_key_machine
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

CREATE TABLE application (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    life_id INT NOT NULL,
    charm_uuid TEXT NOT NULL,
    charm_modified_version INT NOT NULL DEFAULT 0,
    charm_upgrade_on_error BOOLEAN DEFAULT FALSE,
    -- space_uuid is the default binding for this application.
    space_uuid TEXT NOT NULL,
    CONSTRAINT fk_application_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_application_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_space_uuid
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid)
);

CREATE UNIQUE INDEX idx_application_name
ON application (name);

-- This table is only used to track whether a application is a controller or
-- not. It should be sparse and only contain a single row for the controller
-- application.
CREATE TABLE application_controller (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    CONSTRAINT fk_application_controller_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

-- A unique constraint over a constant index ensures only 1 entry matching the 
-- condition can exist.
CREATE UNIQUE INDEX idx_singleton_application_controller ON application_controller ((1));

CREATE TABLE application_workload_version (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    version TEXT NOT NULL,
    CONSTRAINT fk_application_workload_version_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

CREATE TABLE k8s_service (
    uuid TEXT NOT NULL PRIMARY KEY,
    application_uuid TEXT NOT NULL,
    net_node_uuid TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    CONSTRAINT fk_k8s_service_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_k8s_service_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid)
);

CREATE UNIQUE INDEX idx_k8s_service_provider
ON k8s_service (provider_id);

CREATE INDEX idx_k8s_service_application
ON k8s_service (application_uuid);

CREATE UNIQUE INDEX idx_k8s_service_net_node
ON k8s_service (net_node_uuid);

-- Application scale is currently only targeting k8s applications.
CREATE TABLE application_scale (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    scale INT,
    scale_target INT,
    scaling BOOLEAN DEFAULT FALSE,
    CONSTRAINT fk_application_endpoint_scale_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

CREATE TABLE application_exposed_endpoint_space (
    application_uuid TEXT NOT NULL,
    -- NULL application_endpoint_uuid represents the wildcard endpoint.
    application_endpoint_uuid TEXT,
    space_uuid TEXT NOT NULL,
    CONSTRAINT fk_application_exposed_endpoint_space_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_exposed_endpoint_space_application_endpoint
    FOREIGN KEY (application_endpoint_uuid)
    REFERENCES application_endpoint (uuid),
    CONSTRAINT fk_application_exposed_endpoint_space_space
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid),
    PRIMARY KEY (application_uuid, application_endpoint_uuid, space_uuid)
);

-- There is no FK against the CIDR, because it's currently free-form.
CREATE TABLE application_exposed_endpoint_cidr (
    application_uuid TEXT NOT NULL,
    -- NULL application_endpoint_uuid represents the wildcard endpoint.
    application_endpoint_uuid TEXT,
    cidr TEXT NOT NULL,
    CONSTRAINT fk_application_exposed_endpoint_cidr_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_exposed_endpoint_cidr_application_endpoint
    FOREIGN KEY (application_endpoint_uuid)
    REFERENCES application_endpoint (uuid),
    PRIMARY KEY (application_uuid, application_endpoint_uuid, cidr)
);

CREATE VIEW v_application_exposed_endpoint (
    application_uuid,
    application_endpoint_uuid,
    space_uuid,
    cidr
) AS
SELECT
    aes.application_uuid,
    aes.application_endpoint_uuid,
    aes.space_uuid,
    NULL AS n
FROM application_exposed_endpoint_space AS aes
UNION
SELECT
    aec.application_uuid,
    aec.application_endpoint_uuid,
    NULL AS n,
    aec.cidr
FROM application_exposed_endpoint_cidr AS aec;

CREATE TABLE application_config_hash (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    sha256 TEXT NOT NULL,
    CONSTRAINT fk_application_config_hash_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

CREATE TABLE application_config (
    application_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    -- TODO(jack-w-shaw): Drop this field, instead look it up from the charm config
    type_id INT NOT NULL,
    value TEXT,
    CONSTRAINT fk_application_config_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_config_charm_config_type
    FOREIGN KEY (type_id)
    REFERENCES charm_config_type (id),
    PRIMARY KEY (application_uuid, "key")
);

CREATE VIEW v_application_config AS
SELECT
    a.uuid,
    ac."key",
    ac.value,
    cct.name AS type
FROM application AS a
LEFT JOIN application_config AS ac ON a.uuid = ac.application_uuid
JOIN charm_config_type AS cct ON ac.type_id = cct.id;

CREATE TABLE application_constraint (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    constraint_uuid TEXT,
    CONSTRAINT fk_application_constraint_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_constraint_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid)
);

CREATE TABLE application_setting (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    trust BOOLEAN DEFAULT FALSE,
    CONSTRAINT fk_application_setting_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

CREATE TABLE application_platform (
    application_uuid TEXT NOT NULL,
    os_id TEXT NOT NULL,
    channel TEXT,
    architecture_id INT NOT NULL,
    CONSTRAINT fk_application_platform_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_platform_os
    FOREIGN KEY (os_id)
    REFERENCES os (id),
    CONSTRAINT fk_application_platform_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

CREATE TABLE application_channel (
    application_uuid TEXT NOT NULL,
    track TEXT,
    risk TEXT NOT NULL,
    branch TEXT,
    CONSTRAINT fk_application_origin_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    PRIMARY KEY (application_uuid)
);

CREATE TABLE application_status (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    data TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_application_status_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_workload_status_value_status
    FOREIGN KEY (status_id)
    REFERENCES workload_status_value (id)
);

CREATE TABLE application_agent (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    password_hash_algorithm_id TEXT,
    password_hash TEXT,
    CONSTRAINT fk_application_agent_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_agent_password_hash_algorithm
    FOREIGN KEY (password_hash_algorithm_id)
    REFERENCES password_hash_algorithm (id)
);

CREATE TABLE device_constraint (
    uuid TEXT NOT NULL PRIMARY KEY,
    application_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    count INT,
    CONSTRAINT fk_device_constraint_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

CREATE UNIQUE INDEX idx_device_constraint_application_name
ON device_constraint (application_uuid, name);

CREATE TABLE device_constraint_attribute (
    device_constraint_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_device_constraint_attribute_device_constraint
    FOREIGN KEY (device_constraint_uuid)
    REFERENCES device_constraint (uuid),
    PRIMARY KEY (device_constraint_uuid, "key")
);

CREATE VIEW v_application_constraint AS
SELECT
    ac.application_uuid,
    c.arch,
    c.cpu_cores,
    c.cpu_power,
    c.mem,
    c.root_disk,
    c.root_disk_source,
    c.instance_role,
    c.instance_type,
    ctype.value AS container_type,
    c.virt_type,
    c.allocate_public_ip,
    c.image_id,
    ctag.tag,
    cspace.space AS space_name,
    cspace."exclude" AS space_exclude,
    czone.zone
FROM application_constraint AS ac
JOIN "constraint" AS c ON ac.constraint_uuid = c.uuid
LEFT JOIN container_type AS ctype ON c.container_type_id = ctype.id
LEFT JOIN constraint_tag AS ctag ON c.uuid = ctag.constraint_uuid
LEFT JOIN constraint_space AS cspace ON c.uuid = cspace.constraint_uuid
LEFT JOIN constraint_zone AS czone ON c.uuid = czone.constraint_uuid;

CREATE VIEW v_application_platform_channel AS
SELECT
    ap.application_uuid,
    os.name AS platform_os,
    os.id AS platform_os_id,
    ap.channel AS platform_channel,
    a.name AS platform_architecture,
    a.id AS platform_architecture_id,
    ac.track AS channel_track,
    ac.risk AS channel_risk,
    ac.branch AS channel_branch
FROM application_platform AS ap
JOIN os ON ap.os_id = os.id
JOIN architecture AS a ON ap.architecture_id = a.id
LEFT JOIN application_channel AS ac ON ap.application_uuid = ac.application_uuid;

CREATE VIEW v_application_origin AS
SELECT
    a.uuid,
    c.reference_name,
    c.source_id,
    c.revision,
    cdi.charmhub_identifier,
    ch.hash
FROM application AS a
JOIN charm AS c ON a.charm_uuid = c.uuid
LEFT JOIN charm_download_info AS cdi ON c.uuid = cdi.charm_uuid
JOIN charm_hash AS ch ON c.uuid = ch.charm_uuid;

CREATE VIEW v_application_export AS
SELECT
    a.uuid,
    a.name,
    a.life_id,
    a.charm_uuid,
    a.charm_modified_version,
    a.charm_upgrade_on_error,
    cm.subordinate,
    c.reference_name,
    c.source_id,
    c.revision,
    c.architecture_id,
    k8s.provider_id AS k8s_provider_id
FROM application AS a
JOIN charm AS c ON a.charm_uuid = c.uuid
JOIN charm_metadata AS cm ON c.uuid = cm.charm_uuid
LEFT JOIN k8s_service AS k8s ON a.uuid = k8s.application_uuid;

CREATE VIEW v_application_endpoint_uuid AS
SELECT
    a.uuid,
    c.name,
    a.application_uuid
FROM application_endpoint AS a
JOIN charm_relation AS c ON a.charm_relation_uuid = c.uuid;

-- v_application_subordinate provides an application, whether its charm is a
-- subordinate, and a relation_uuid if it exists. It's possible the application
-- is in zero or multiple relations.
CREATE VIEW v_application_subordinate AS
SELECT
    a.uuid AS application_uuid,
    cm.subordinate,
    re.relation_uuid
FROM application AS a
JOIN charm AS c ON a.charm_uuid = c.uuid
JOIN charm_metadata AS cm ON c.uuid = cm.charm_uuid
JOIN charm_relation AS cr ON c.uuid = cr.charm_uuid
JOIN application_endpoint AS ae ON cr.uuid = ae.charm_relation_uuid
JOIN relation_endpoint AS re ON ae.uuid = re.endpoint_uuid;

CREATE TABLE unit (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    life_id INT NOT NULL,
    application_uuid TEXT NOT NULL,
    net_node_uuid TEXT NOT NULL,
    charm_uuid TEXT NOT NULL,
    password_hash_algorithm_id TEXT,
    password_hash TEXT,
    CONSTRAINT fk_unit_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_unit_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_unit_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_unit_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_unit_password_hash_algorithm
    FOREIGN KEY (password_hash_algorithm_id)
    REFERENCES password_hash_algorithm (id)
);

CREATE UNIQUE INDEX idx_unit_name
ON unit (name);

-- unit passwords are unique across all units. This is to prevent
-- a unit from being able to impersonate another unit. NULL passwords are
-- allowed for multiple units, as NULLs are considered distinct.
CREATE UNIQUE INDEX idx_unit_password_hash
ON unit (password_hash);

CREATE INDEX idx_unit_application
ON unit (application_uuid);

CREATE INDEX idx_unit_net_node
ON unit (net_node_uuid);

-- unit_principal table is a table which is used to store the
-- principal units for subordinate units.
CREATE TABLE unit_principal (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    principal_uuid TEXT NOT NULL,
    CONSTRAINT fk_unit_principal_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_unit_principal_principal
    FOREIGN KEY (principal_uuid)
    REFERENCES unit (uuid)
);

CREATE TABLE unit_workload_version (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    version TEXT NOT NULL,
    CONSTRAINT fk_unit_workload_version_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

-- unit_agent_version tracks the reported agent version running for each
-- unit.
CREATE TABLE unit_agent_version (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    version TEXT NOT NULL,
    architecture_id INT NOT NULL,
    CONSTRAINT fk_unit_agent_version_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_unit_agent_version_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

-- v_unit_target_agent_version provides a convenience view for establishing what
-- the current target agent version of a unit should be. A unit will only have
-- a record in this view if a target agent version has been set for the model
-- and the unit has had its running unit agent version set.
CREATE VIEW v_unit_target_agent_version AS
SELECT
    u.name,
    uav.unit_uuid,
    uav.architecture_id,
    uav.version,
    av.target_version,
    a.name AS architecture_name
FROM unit_agent_version AS uav
JOIN unit AS u ON uav.unit_uuid = u.uuid
JOIN architecture AS a ON uav.architecture_id = a.id
JOIN agent_version AS av;

CREATE TABLE unit_state (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    uniter_state TEXT,
    storage_state TEXT,
    secret_state TEXT,
    CONSTRAINT fk_unit_state_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

-- Local charm state stored upon hook commit with uniter state.
CREATE TABLE unit_state_charm (
    unit_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (unit_uuid, "key"),
    CONSTRAINT fk_unit_state_charm_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

-- Local relation state stored upon hook commit with uniter state.
CREATE TABLE unit_state_relation (
    unit_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (unit_uuid, "key"),
    CONSTRAINT fk_unit_state_relation_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

-- cloud containers belong to a k8s unit.
CREATE TABLE k8s_pod (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    -- provider_id comes from the provider, no FK.
    -- it represents the k8s pod name.
    provider_id TEXT NOT NULL,
    CONSTRAINT fk_k8s_pod_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

CREATE UNIQUE INDEX idx_k8s_pod_provider
ON k8s_pod (provider_id);

CREATE TABLE k8s_pod_port (
    unit_uuid TEXT NOT NULL,
    port TEXT NOT NULL,
    CONSTRAINT fk_k8s_pod_port_k8s_pod
    FOREIGN KEY (unit_uuid)
    REFERENCES k8s_pod (unit_uuid),
    PRIMARY KEY (unit_uuid, port)
);

-- Status values for unit agents.
CREATE TABLE unit_agent_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO unit_agent_status_value VALUES
(0, 'allocating'),
(1, 'executing'),
(2, 'idle'),
(3, 'error'),
(4, 'failed'),
(5, 'lost'),
(6, 'rebooting');

-- Status values for cloud containers.
CREATE TABLE k8s_pod_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO k8s_pod_status_value VALUES
(0, 'unset'),
(1, 'waiting'),
(2, 'blocked'),
(3, 'running'),
(4, 'error');

CREATE TABLE unit_agent_status (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    data TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_unit_agent_status_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_unit_agent_status_status
    FOREIGN KEY (status_id)
    REFERENCES unit_agent_status_value (id)
);

CREATE TABLE unit_workload_status (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    data TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_unit_workload_status_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_workload_status_value_status
    FOREIGN KEY (status_id)
    REFERENCES workload_status_value (id)
);

CREATE TABLE k8s_pod_status (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    data TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_k8s_pod_status_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_k8s_pod_status_status
    FOREIGN KEY (status_id)
    REFERENCES k8s_pod_status_value (id)
);

CREATE TABLE unit_agent_presence (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    last_seen DATETIME,
    CONSTRAINT fk_unit_agent_presence_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

CREATE VIEW v_unit_agent_presence AS
SELECT
    unit.uuid,
    unit_agent_presence.last_seen,
    unit.name
FROM unit
JOIN unit_agent_presence ON unit.uuid = unit_agent_presence.unit_uuid;

CREATE VIEW v_unit_agent_status AS
SELECT
    u.uuid AS unit_uuid,
    u.name AS unit_name,
    u.application_uuid,
    uas.status_id,
    uas.message,
    uas.data,
    uas.updated_at,
    EXISTS(
        SELECT 1 FROM unit_agent_presence AS uap
        WHERE u.uuid = uap.unit_uuid
    ) AS present
FROM unit AS u
JOIN unit_agent_status AS uas ON u.uuid = uas.unit_uuid;

CREATE VIEW v_unit_workload_status AS
SELECT
    u.uuid AS unit_uuid,
    u.name AS unit_name,
    u.application_uuid,
    uws.status_id,
    uws.message,
    uws.data,
    uws.updated_at,
    EXISTS(
        SELECT 1 FROM unit_agent_presence AS uap
        WHERE u.uuid = uap.unit_uuid
    ) AS present
FROM unit AS u
JOIN unit_workload_status AS uws ON u.uuid = uws.unit_uuid;

CREATE VIEW v_unit_k8s_pod_status AS
SELECT
    u.uuid AS unit_uuid,
    u.name AS unit_name,
    u.application_uuid,
    kps.status_id,
    kps.message,
    kps.data,
    kps.updated_at
FROM unit AS u
JOIN k8s_pod_status AS kps ON u.uuid = kps.unit_uuid;

CREATE VIEW v_unit_workload_agent_status AS
SELECT
    u.uuid AS unit_uuid,
    u.name AS unit_name,
    u.application_uuid,
    uws.status_id AS workload_status_id,
    uws.message AS workload_message,
    uws.data AS workload_data,
    uws.updated_at AS workload_updated_at,
    uas.status_id AS agent_status_id,
    uas.message AS agent_message,
    uas.data AS agent_data,
    uas.updated_at AS agent_updated_at,
    EXISTS(
        SELECT 1 FROM unit_agent_presence AS uap
        WHERE u.uuid = uap.unit_uuid
    ) AS present
FROM unit AS u
LEFT JOIN unit_workload_status AS uws ON u.uuid = uws.unit_uuid
LEFT JOIN unit_agent_status AS uas ON u.uuid = uas.unit_uuid;

CREATE VIEW v_full_unit_status AS
SELECT
    u.uuid AS unit_uuid,
    u.name AS unit_name,
    u.application_uuid,
    uws.status_id AS workload_status_id,
    uws.message AS workload_message,
    uws.data AS workload_data,
    uws.updated_at AS workload_updated_at,
    uas.status_id AS agent_status_id,
    uas.message AS agent_message,
    uas.data AS agent_data,
    uas.updated_at AS agent_updated_at,
    kps.status_id AS k8s_pod_status_id,
    kps.message AS k8s_pod_message,
    kps.data AS k8s_pod_data,
    kps.updated_at AS k8s_pod_updated_at,
    EXISTS(
        SELECT 1 FROM unit_agent_presence AS uap
        WHERE u.uuid = uap.unit_uuid
    ) AS present
FROM unit AS u
LEFT JOIN unit_workload_status AS uws ON u.uuid = uws.unit_uuid
LEFT JOIN unit_agent_status AS uas ON u.uuid = uas.unit_uuid
LEFT JOIN k8s_pod_status AS kps ON u.uuid = kps.unit_uuid;

CREATE VIEW v_unit_password_hash AS
SELECT
    a.uuid AS application_uuid,
    a.name AS application_name,
    u.uuid AS unit_uuid,
    u.name AS unit_name,
    u.password_hash
FROM application AS a
LEFT JOIN unit AS u ON a.uuid = u.application_uuid;

CREATE TABLE unit_resolved (
    unit_uuid TEXT NOT NULL PRIMARY KEY,
    mode_id INT NOT NULL,
    CONSTRAINT fk_unit_resolved_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_unit_resolved_mode
    FOREIGN KEY (mode_id)
    REFERENCES resolve_mode (id)
);

CREATE TABLE resolve_mode (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

INSERT INTO resolve_mode VALUES
(0, 'retry-hooks'),
(1, 'no-hooks');

CREATE VIEW v_unit_attribute AS
SELECT
    u.uuid,
    u.name,
    u.life_id,
    ur.mode_id AS resolve_mode_id,
    k.provider_id
FROM unit AS u
LEFT JOIN unit_resolved AS ur ON u.uuid = ur.unit_uuid
LEFT JOIN k8s_pod AS k ON u.uuid = k.unit_uuid;

CREATE VIEW v_unit_export AS
SELECT
    u.uuid,
    u.name,
    u.password_hash,
    u.application_uuid,
    m.name AS machine_name,
    upname.name AS principal_name
FROM unit AS u
LEFT JOIN machine AS m ON u.net_node_uuid = m.net_node_uuid
LEFT JOIN unit_principal AS up ON u.uuid = up.unit_uuid
LEFT JOIN unit AS upname ON up.principal_uuid = upname.uuid;

CREATE TABLE net_node (
    uuid TEXT NOT NULL PRIMARY KEY
);

CREATE TABLE link_layer_device_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_link_layer_device_type_name
ON link_layer_device_type (name);

INSERT INTO link_layer_device_type VALUES
(0, 'unknown'),
(1, 'loopback'),
(2, 'ethernet'),
(3, '802.1q'),
(4, 'bond'),
(5, 'bridge'),
(6, 'vxlan');

CREATE TABLE virtual_port_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_virtual_port_type_name
ON virtual_port_type (name);

INSERT INTO virtual_port_type VALUES
-- Note that this corresponds with corenetwork.NonVirtualPortType
(0, ''),
(1, 'openvswitch');

CREATE TABLE link_layer_device (
    uuid TEXT NOT NULL PRIMARY KEY,
    net_node_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    mtu INT,
    -- NULL for a placeholder link layer device.
    mac_address TEXT,
    device_type_id INT NOT NULL,
    virtual_port_type_id INT NOT NULL,
    -- True if the device should be activated on boot.
    is_auto_start BOOLEAN NOT NULL DEFAULT true,
    -- True when the device is in the up state.
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    -- True if traffic is routed out of this device by default.
    is_default_gateway BOOLEAN NOT NULL DEFAULT false,
    -- IP address of the default gateway.
    gateway_address TEXT,
    -- 0 for normal networks; 1-4094 for VLANs.
    vlan_tag INT NOT NULL DEFAULT 0,
    CONSTRAINT fk_link_layer_device_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_link_layer_device_device_type
    FOREIGN KEY (device_type_id)
    REFERENCES link_layer_device_type (id),
    CONSTRAINT fk_link_layer_device_virtual_port_type
    FOREIGN KEY (virtual_port_type_id)
    REFERENCES virtual_port_type (id)
);

CREATE UNIQUE INDEX idx_link_layer_device_net_node_uuid_name
ON link_layer_device (net_node_uuid, name);

CREATE TABLE link_layer_device_parent (
    device_uuid TEXT NOT NULL PRIMARY KEY,
    parent_uuid TEXT NOT NULL,
    CONSTRAINT fk_link_layer_device_parent_device
    FOREIGN KEY (device_uuid)
    REFERENCES link_layer_device (uuid),
    CONSTRAINT fk_link_layer_device_parent_parent
    FOREIGN KEY (parent_uuid)
    REFERENCES link_layer_device (uuid)
);

CREATE TABLE provider_link_layer_device (
    provider_id TEXT NOT NULL PRIMARY KEY,
    device_uuid TEXT NOT NULL,
    CONSTRAINT chk_provider_id_empty CHECK (provider_id != ''),
    CONSTRAINT fk_provider_device_uuid
    FOREIGN KEY (device_uuid)
    REFERENCES link_layer_device (uuid)
);

CREATE TABLE link_layer_device_dns_domain (
    device_uuid TEXT NOT NULL,
    search_domain TEXT NOT NULL,
    CONSTRAINT fk_dns_search_domain_dev
    FOREIGN KEY (device_uuid)
    REFERENCES link_layer_device (uuid),
    PRIMARY KEY (device_uuid, search_domain)
);

CREATE TABLE link_layer_device_dns_address (
    device_uuid TEXT NOT NULL,
    dns_address TEXT NOT NULL,
    CONSTRAINT fk_dns_server_device
    FOREIGN KEY (device_uuid)
    REFERENCES link_layer_device (uuid),
    PRIMARY KEY (device_uuid, dns_address)
);

-- Note that this table is defined for completeness in
-- reflecting what we capture as network configuration.
-- At the time of writing, we do not store any routes,
-- but this can be easily changed at a later date.
CREATE TABLE link_layer_device_route (
    device_uuid TEXT NOT NULL,
    destination_cidr TEXT NOT NULL,
    gateway_ip TEXT NOT NULL,
    metric INT NOT NULL,
    CONSTRAINT fk_dns_server_device
    FOREIGN KEY (device_uuid)
    REFERENCES link_layer_device (uuid),
    PRIMARY KEY (device_uuid, destination_cidr)
);

-- ip_address_type represents the possible ways of specifying
-- an address, either a hostname resolvable by dns lookup,
-- or IPv4 or IPv6 address.
CREATE TABLE ip_address_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_ip_address_type_name
ON ip_address_type (name);

INSERT INTO ip_address_type VALUES
(0, 'ipv4'),
(1, 'ipv6');

-- ip_address_origin represents the authoritative source of
-- an ip address.
CREATE TABLE ip_address_origin (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_ip_address_origin_name
ON ip_address_origin (name);

INSERT INTO ip_address_origin VALUES
(0, 'machine'),
(1, 'provider');

-- ip_address_scope denotes the context an ip address may apply to.
-- If an address can be reached from the wider internet,
-- it is considered public. A private ip address is either
-- specific to the cloud or cloud subnet a node belongs to,
-- or to the node itself for containers.
CREATE TABLE ip_address_scope (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_ip_address_scope_name
ON ip_address_scope (name);

INSERT INTO ip_address_scope VALUES
(0, 'unknown'),
(1, 'public'),
(2, 'local-cloud'),
(3, 'local-machine'),
(4, 'link-local');

-- ip_address_config_type defines valid network
-- link configuration types.
CREATE TABLE ip_address_config_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_ip_address_config_type_name
ON ip_address_config_type (name);

INSERT INTO ip_address_config_type VALUES
(0, 'unknown'),
(1, 'dhcp'),
(2, 'dhcpv6'),
(3, 'slaac'),
(4, 'static'),
(5, 'manual'),
(6, 'loopback');

CREATE TABLE ip_address (
    uuid TEXT NOT NULL PRIMARY KEY,
    -- the link layer device this address belongs to.
    net_node_uuid TEXT NOT NULL,
    device_uuid TEXT NOT NULL,
    -- The IP address *including the subnet mask*.
    address_value TEXT NOT NULL,
    -- NOTE (manadart 2025-03--25): The fact that this is nullable is a wart
    -- from our Kubernetes provider. There is nothing to say we couldn't do
    -- subnet discovery on K8s by listing nodes, then accumulating
    -- NodeSpec.PodCIDRs for each. We could then match the incoming pod IPs to
    -- those and assign this field.
    subnet_uuid TEXT,
    type_id INT NOT NULL,
    config_type_id INT NOT NULL,
    origin_id INT NOT NULL,
    scope_id INT NOT NULL,
    -- indicates that this address is not the primary
    -- address associated with the NIC.
    is_secondary BOOLEAN DEFAULT false,
    -- indicates whether this address is a virtual/floating/shadow
    -- address assigned by a provider rather than being
    -- associated directly with a device on-machine.
    is_shadow BOOLEAN DEFAULT false,

    CONSTRAINT fk_ip_address_link_layer_device
    FOREIGN KEY (device_uuid)
    REFERENCES link_layer_device (uuid),
    CONSTRAINT fk_ip_address_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_ip_address_subnet
    FOREIGN KEY (subnet_uuid)
    REFERENCES subnet (uuid),
    CONSTRAINT fk_ip_address_origin
    FOREIGN KEY (origin_id)
    REFERENCES ip_address_origin (id),
    CONSTRAINT fk_ip_address_type
    FOREIGN KEY (type_id)
    REFERENCES ip_address_type (id),
    CONSTRAINT fk_ip_address_config_type
    FOREIGN KEY (config_type_id)
    REFERENCES ip_address_config_type (id),
    CONSTRAINT fk_ip_address_scope
    FOREIGN KEY (scope_id)
    REFERENCES ip_address_scope (id)
);

CREATE INDEX idx_ip_address_device_uuid
ON ip_address (device_uuid);

CREATE INDEX idx_ip_address_subnet_uuid
ON ip_address (subnet_uuid);

CREATE INDEX idx_ip_address_net_node_uuid
ON ip_address (net_node_uuid);

CREATE TABLE provider_ip_address (
    provider_id TEXT NOT NULL PRIMARY KEY,
    address_uuid TEXT NOT NULL,
    CONSTRAINT chk_provider_id_empty CHECK (provider_id != ''),
    CONSTRAINT fk_provider_ip_address
    FOREIGN KEY (address_uuid)
    REFERENCES ip_address (uuid)
);

-- network_address_scope denotes the context a network address may apply to.
-- If an address can be reached from the wider internet,
-- it is considered public. A private address is either
-- specific to the cloud or cloud subnet a node belongs to,
-- or to the node itself for containers.
CREATE TABLE network_address_scope (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_network_address_scope_name
ON network_address_scope (name);

INSERT INTO network_address_scope VALUES
(0, 'local-host'),
(1, 'local-cloud'),
(2, 'public');

CREATE TABLE fqdn_address (
    uuid TEXT NOT NULL PRIMARY KEY,
    address TEXT NOT NULL,
    -- one of local-cloud, public.
    scope_id INT NOT NULL,

    CONSTRAINT chk_fqdn_address_scope
    CHECK (scope_id != 0), -- scope can't be local-host
    CONSTRAINT fk_fqdn_address_scope
    FOREIGN KEY (scope_id)
    REFERENCES network_address_scope (id)
);

CREATE UNIQUE INDEX idx_fqdn_address_address
ON fqdn_address (address);

CREATE TABLE net_node_fqdn_address (
    net_node_uuid TEXT NOT NULL,
    address_uuid TEXT NOT NULL,
    CONSTRAINT fk_net_node_fqdn_address_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_net_node_fqdn_address_address
    FOREIGN KEY (address_uuid)
    REFERENCES fqdn_address (uuid),
    PRIMARY KEY (net_node_uuid, address_uuid)
);

CREATE TABLE hostname_address (
    uuid TEXT NOT NULL PRIMARY KEY,
    hostname TEXT NOT NULL,
    -- one of local-host, local-cloud, public.
    scope_id INT NOT NULL,

    CONSTRAINT fk_hostname_address_scope
    FOREIGN KEY (scope_id)
    REFERENCES network_address_scope (id)
);

CREATE UNIQUE INDEX idx_hostname_address_hostname
ON hostname_address (hostname);

CREATE TABLE net_node_hostname_address (
    net_node_uuid TEXT NOT NULL,
    address_uuid TEXT NOT NULL,
    CONSTRAINT fk_net_node_hostname_address_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_net_node_hostname_address_address
    FOREIGN KEY (address_uuid)
    REFERENCES hostname_address (uuid),
    PRIMARY KEY (net_node_uuid, address_uuid)
);

-- v_address exposes ip and fqdn addresses as a single table.
-- Used for compatibility with the current core network model.
CREATE VIEW v_address AS
SELECT
    ipa.address_value,
    ipa.type_id,
    ipa.config_type_id,
    ipa.origin_id,
    ipa.scope_id
FROM ip_address AS ipa
UNION
SELECT
    fa.hostname AS address_value,
    -- FQDN address type is always "hostname".
    0 AS type_id,
    -- FQDN address config type is always "manual".
    3 AS config_type_id,
    -- FQDN address doesn't have an origin.
    null AS origin_id,
    fa.scope_id
FROM fqdn_address AS fa;

-- v_ip_address_with_names returns a ip_address with the
-- type ids converted to their names.
CREATE VIEW v_ip_address_with_names AS
SELECT
    ipa.uuid,
    ipa.address_value,
    ipa.subnet_uuid,
    ipa.device_uuid,
    ipa.net_node_uuid,
    ipa.is_secondary,
    ipa.is_shadow,
    iact.name AS config_type_name,
    ias.name AS scope_name,
    iao.name AS origin_name,
    iat.name AS type_name
FROM ip_address AS ipa
JOIN ip_address_config_type AS iact ON ipa.config_type_id = iact.id
JOIN ip_address_scope AS ias ON ipa.scope_id = ias.id
JOIN ip_address_origin AS iao ON ipa.origin_id = iao.id
JOIN ip_address_type AS iat ON ipa.type_id = iat.id;

CREATE VIEW v_machine_interface AS
SELECT
    m.uuid AS machine_uuid,
    m.name AS machine_name,
    d.net_node_uuid,
    d.uuid AS device_uuid,
    d.name AS device_name,
    d.mtu,
    d.mac_address,
    d.device_type_id,
    d.virtual_port_type_id,
    d.is_auto_start,
    d.is_enabled,
    d.is_default_gateway,
    d.gateway_address,
    d.vlan_tag,
    pd.provider_id AS device_provider_id,
    dp.parent_uuid AS parent_device_uuid,
    dd.name AS parent_device_name,
    dnsa.dns_address,
    dnsd.search_domain,
    a.uuid AS address_uuid,
    pa.provider_id AS provider_address_id,
    a.address_value,
    a.subnet_uuid,
    s.cidr,
    ps.provider_id AS provider_subnet_id,
    a.type_id AS address_type_id,
    a.config_type_id,
    a.origin_id,
    a.scope_id,
    a.is_secondary,
    a.is_shadow
FROM machine AS m
JOIN link_layer_device AS d ON m.net_node_uuid = d.net_node_uuid
LEFT JOIN provider_link_layer_device AS pd ON d.uuid = pd.device_uuid
LEFT JOIN link_layer_device_parent AS dp ON d.uuid = dp.device_uuid
LEFT JOIN link_layer_device AS dd ON dp.parent_uuid = dd.uuid
LEFT JOIN link_layer_device_dns_address AS dnsa ON d.uuid = dnsa.device_uuid
LEFT JOIN link_layer_device_dns_domain AS dnsd ON d.uuid = dnsd.device_uuid
LEFT JOIN ip_address AS a ON d.uuid = a.device_uuid
LEFT JOIN provider_ip_address AS pa ON a.uuid = pa.address_uuid
LEFT JOIN subnet AS s ON a.subnet_uuid = s.uuid
LEFT JOIN provider_subnet AS ps ON a.subnet_uuid = ps.subnet_uuid;


-- This view allows to retrieves any address belonging to a unit.
-- It is useful when we have a unit UUID and we want all addresses relative to this unit,
-- without caring if we are in a k8s provider or a machine provider.
-- This add addresses belonging to the k8s service of the unit application
-- alongside those belonging directly to the unit
CREATE VIEW v_all_unit_address AS
SELECT
    n.uuid AS unit_uuid,
    ipa.address_value,
    ipa.config_type_name,
    ipa.type_name,
    ipa.origin_name,
    ipa.scope_name,
    ipa.device_uuid,
    sn.space_uuid,
    sn.cidr
FROM (
    SELECT
        s.net_node_uuid,
        u.uuid
    FROM unit AS u
    JOIN application AS a ON u.application_uuid = a.uuid
    JOIN k8s_service AS s ON a.uuid = s.application_uuid
    UNION
    SELECT
        net_node_uuid,
        uuid
    FROM unit
) AS n
JOIN v_ip_address_with_names AS ipa ON n.net_node_uuid = ipa.net_node_uuid
LEFT JOIN subnet AS sn ON ipa.subnet_uuid = sn.uuid;

CREATE TABLE protocol (
    id INT PRIMARY KEY,
    protocol TEXT NOT NULL
);

INSERT INTO protocol VALUES
(0, 'icmp'),
(1, 'tcp'),
(2, 'udp');

CREATE TABLE port_range (
    uuid TEXT NOT NULL PRIMARY KEY,
    protocol_id INT NOT NULL,
    from_port INT,
    to_port INT,
    relation_uuid TEXT, -- NULL-able, where null represents a wildcard endpoint
    unit_uuid TEXT NOT NULL,
    CONSTRAINT fk_port_range_protocol
    FOREIGN KEY (protocol_id)
    REFERENCES protocol (id),
    CONSTRAINT fk_port_range_relation
    FOREIGN KEY (relation_uuid)
    REFERENCES charm_relation (uuid),
    CONSTRAINT fk_port_range_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

-- We disallow overlapping port ranges, however this cannot reasonably
-- be enforced in the schema. Including the from_port in the uniqueness
-- constraint is as far as we go here. Non-overlapping ranges must be
-- enforced in the service/state layer.
CREATE UNIQUE INDEX idx_port_range_endpoint ON port_range (protocol_id, from_port, relation_uuid, unit_uuid);

CREATE VIEW v_port_range
AS
SELECT
    pr.uuid,
    pr.from_port,
    pr.to_port,
    pr.unit_uuid,
    u.name AS unit_name,
    protocol.protocol,
    cr.name AS endpoint
FROM port_range AS pr
LEFT JOIN protocol ON pr.protocol_id = protocol.id
LEFT JOIN charm_relation AS cr ON pr.relation_uuid = cr.uuid
LEFT JOIN unit AS u ON pr.unit_uuid = u.uuid;

CREATE VIEW v_endpoint
AS
SELECT
    cr.uuid,
    cr.name AS endpoint,
    u.uuid AS unit_uuid
FROM unit AS u
LEFT JOIN application AS a ON u.application_uuid = a.uuid
LEFT JOIN charm_relation AS cr ON a.charm_uuid = cr.charm_uuid;

CREATE TABLE resource_origin_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_resource_origin_name
ON resource_origin_type (name);

INSERT INTO resource_origin_type VALUES
(0, 'upload'),
(1, 'store');

CREATE TABLE resource_state (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_resource_state
ON resource_state (name);

-- Resource state values:
-- Available is the application resource which will be used by any units
-- at this point in time.
-- Potential indicates there is a different revision of the resource available
-- in a repository. Used to let users know a resource can be upgraded.
INSERT INTO resource_state VALUES
(0, 'available'),
(1, 'potential');

CREATE TABLE resource (
    uuid TEXT NOT NULL PRIMARY KEY,
    -- This indicates the resource name for the specific
    -- revision for which this resource is downloaded.
    charm_uuid TEXT NOT NULL,
    charm_resource_name TEXT NOT NULL,
    revision INT,
    origin_type_id INT NOT NULL,
    state_id INT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    -- last_polled is when the repository was last polled for new resource
    -- revisions. Only set if resource_state is 1 ("potential").
    last_polled TIMESTAMP,
    CONSTRAINT fk_charm_resource
    FOREIGN KEY (charm_uuid, charm_resource_name)
    REFERENCES charm_resource (charm_uuid, name),
    CONSTRAINT fk_resource_origin_type_id
    FOREIGN KEY (origin_type_id)
    REFERENCES resource_origin_type (id),
    CONSTRAINT fk_resource_state_id
    FOREIGN KEY (state_id)
    REFERENCES resource_state (id)
);

-- Links applications to the resources that they are *using*.
-- This resource may in turn be linked through to a *different* charm than the
-- application is using, because the charm_resource_name field indicates the
-- charm revision that it was acquired for at the time.
CREATE TABLE application_resource (
    resource_uuid TEXT NOT NULL PRIMARY KEY,
    application_uuid TEXT NOT NULL,
    CONSTRAINT fk_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid)
);

-- Links a resource to an application which does not exist yet.
CREATE TABLE pending_application_resource (
    resource_uuid TEXT NOT NULL PRIMARY KEY,
    application_name TEXT NOT NULL,
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid)
);

CREATE TABLE resource_retrieved_by_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_resource_retrieved_by_type
ON resource_retrieved_by_type (name);

INSERT INTO resource_retrieved_by_type VALUES
(0, 'user'),
(1, 'unit'),
(2, 'application');

CREATE TABLE resource_retrieved_by (
    resource_uuid TEXT NOT NULL PRIMARY KEY,
    retrieved_by_type_id INT NOT NULL,
    -- Name is the entity who retrieved the resource blob:
    --   The name of the user who uploaded the resource.
    --   Unit or application name of which triggered the download
    --     from a repository.
    name TEXT NOT NULL,
    CONSTRAINT fk_resource
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid),
    CONSTRAINT fk_resource_retrieved_by_type
    FOREIGN KEY (retrieved_by_type_id)
    REFERENCES resource_retrieved_by_type (id)
);

-- This is a resource used by to a unit.
CREATE TABLE unit_resource (
    resource_uuid TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    added_at TIMESTAMP NOT NULL,
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid),
    CONSTRAINT fk_resource_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    PRIMARY KEY (resource_uuid, unit_uuid)
);

-- This is the actual store for container image resources. The metadata
-- necessary to retrieve the OCI Image from a registry.
CREATE TABLE resource_container_image_metadata_store (
    storage_key TEXT NOT NULL PRIMARY KEY,
    registry_path TEXT NOT NULL,
    username TEXT,
    password TEXT
);

-- Link table between a file resource and where its stored.
CREATE TABLE resource_file_store (
    resource_uuid TEXT NOT NULL PRIMARY KEY,
    store_uuid TEXT NOT NULL,
    size INTEGER, -- in bytes
    sha384 TEXT,
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid),
    CONSTRAINT fk_store_uuid
    FOREIGN KEY (store_uuid)
    REFERENCES object_store_metadata (uuid)
);

-- Link table between a container image resource and where its stored.
CREATE TABLE resource_image_store (
    resource_uuid TEXT NOT NULL PRIMARY KEY,
    store_storage_key TEXT NOT NULL,
    size INTEGER, -- in bytes
    sha384 TEXT,
    CONSTRAINT fk_resource_uuid
    FOREIGN KEY (resource_uuid)
    REFERENCES resource (uuid),
    CONSTRAINT fk_store_uuid
    FOREIGN KEY (store_storage_key)
    REFERENCES resource_container_image_metadata_store (storage_key)
);

-- View of all resources with plain text enum types, and solved fields from charm table
CREATE VIEW v_resource AS
SELECT
    r.uuid,
    r.charm_resource_name AS name,
    r.created_at,
    r.revision,
    rot.name AS origin_type,
    rs.name AS state,
    rrb.name AS retrieved_by,
    rrbt.name AS retrieved_by_type,
    cr.path,
    cr.description,
    crk.name AS kind_name,
    -- Select the size and sha384 from whichever store contains the resource
    -- blob.
    COALESCE(rfs.size, ris.size) AS size,
    COALESCE(rfs.sha384, ris.sha384) AS sha384
FROM resource AS r
JOIN charm_resource AS cr ON r.charm_uuid = cr.charm_uuid AND r.charm_resource_name = cr.name
JOIN charm_resource_kind AS crk ON cr.kind_id = crk.id
JOIN resource_origin_type AS rot ON r.origin_type_id = rot.id
JOIN resource_state AS rs ON r.state_id = rs.id
LEFT JOIN resource_retrieved_by AS rrb ON r.uuid = rrb.resource_uuid
LEFT JOIN resource_retrieved_by_type AS rrbt ON rrb.retrieved_by_type_id = rrbt.id
LEFT JOIN resource_file_store AS rfs ON r.uuid = rfs.resource_uuid
LEFT JOIN resource_image_store AS ris ON r.uuid = ris.resource_uuid;

-- View of all resources linked to application
CREATE VIEW v_application_resource AS
SELECT
    r.uuid,
    r.name,
    r.created_at,
    r.revision,
    r.origin_type,
    r.state,
    r.retrieved_by,
    r.retrieved_by_type,
    r.path,
    r.description,
    r.kind_name,
    r.size,
    r.sha384,
    ar.application_uuid,
    a.name AS application_name
FROM v_resource AS r
LEFT JOIN application_resource AS ar ON r.uuid = ar.resource_uuid
LEFT JOIN application AS a ON ar.application_uuid = a.uuid;

-- View of all resources linked to units
CREATE VIEW v_unit_resource AS
SELECT
    r.uuid,
    r.name,
    r.created_at,
    r.revision,
    r.origin_type,
    r.state,
    r.retrieved_by,
    r.retrieved_by_type,
    r.path,
    r.description,
    r.kind_name,
    r.size,
    r.sha384,
    ur.unit_uuid,
    u.name AS unit_name,
    a.uuid AS application_uuid,
    a.name AS application_name
FROM v_resource AS r
JOIN unit_resource AS ur ON r.uuid = ur.resource_uuid
JOIN unit AS u ON ur.unit_uuid = u.uuid
JOIN application AS a ON u.application_uuid = a.uuid;

-- The application_endpoint ties an application's relation definition to an
-- endpoint binding via a space. A null space_uuid represents the endpoint
-- is bound to the application's default space. Each relation has 2 endpoints,
-- unless it is a peer relation. The space and charm relation combine represent
-- the endpoint binding of this application endpoint.
CREATE TABLE application_endpoint (
    uuid TEXT NOT NULL PRIMARY KEY,
    application_uuid TEXT NOT NULL,
    space_uuid TEXT,
    charm_relation_uuid TEXT NOT NULL,
    CONSTRAINT fk_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_space_uuid
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid),
    CONSTRAINT fk_charm_relation_uuid
    FOREIGN KEY (charm_relation_uuid)
    REFERENCES charm_relation (uuid)
);

CREATE INDEX idx_application_endpoint_app
ON application_endpoint (application_uuid);

CREATE UNIQUE INDEX idx_application_endpoint_app_relation
ON application_endpoint (application_uuid, charm_relation_uuid);

-- The application_endpoint ties an application's relation definition to an
-- endpoint binding via a space. Only endpoint bindings which differ from the
-- application default binding will be listed.
CREATE TABLE application_extra_endpoint (
    application_uuid TEXT NOT NULL,
    space_uuid TEXT,
    charm_extra_binding_uuid TEXT NOT NULL,
    CONSTRAINT fk_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_space_uuid
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid),
    CONSTRAINT fk_charm_extra_binding_uuid
    FOREIGN KEY (charm_extra_binding_uuid)
    REFERENCES charm_extra_binding (uuid),
    PRIMARY KEY (application_uuid, charm_extra_binding_uuid)
);

CREATE INDEX idx_application_extra_endpoint_app
ON application_extra_endpoint (application_uuid);

CREATE UNIQUE INDEX idx_application_extra_endpoint_app_relation
ON application_extra_endpoint (application_uuid, charm_extra_binding_uuid);

-- The relation_endpoint table links a relation to a single
-- application endpoint. If the relation is of type peer,
-- there will be one row in the table. If the relation has
-- a provider and requirer endpoint, there will be two rows
-- in the table.
CREATE TABLE relation_endpoint (
    uuid TEXT NOT NULL PRIMARY KEY,
    relation_uuid TEXT NOT NULL,
    endpoint_uuid TEXT NOT NULL,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    CONSTRAINT fk_endpoint_uuid
    FOREIGN KEY (endpoint_uuid)
    REFERENCES application_endpoint (uuid)
);

CREATE UNIQUE INDEX idx_relation_endpoint
ON relation_endpoint (relation_uuid, endpoint_uuid);

-- The relation table represents a relation between two
-- applications, or a peer relation.
CREATE TABLE relation (
    uuid TEXT NOT NULL PRIMARY KEY,
    life_id INT NOT NULL,
    relation_id INT NOT NULL,
    suspended BOOLEAN DEFAULT FALSE,
    suspended_reason TEXT,
    -- NOTE: the scope of a relation is not just the same as the scope of either
    -- of it's endpoints. It's a property we need to consider as intrinsic to
    -- the relation itself. This is because a relation is considered
    -- container-scoped if either of it's endpoints are container-scoped.
    scope_id INT NOT NULL,
    CONSTRAINT fk_relation_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_relation_scope
    FOREIGN KEY (scope_id)
    REFERENCES charm_relation_scope (id)
);

CREATE UNIQUE INDEX idx_relation_id
ON relation (relation_id);

-- The relation_unit table links a relation to a specific unit.
CREATE TABLE relation_unit (
    uuid TEXT NOT NULL PRIMARY KEY,
    relation_endpoint_uuid TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    CONSTRAINT fk_relation_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid),
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_endpoint_uuid)
    REFERENCES relation_endpoint (uuid)
);

CREATE UNIQUE INDEX idx_relation_unit
ON relation_unit (relation_endpoint_uuid, unit_uuid);

-- The relation_unit_setting holds key value pair settings
-- for a relation at the unit level. Keys must be unique
-- per unit.
CREATE TABLE relation_unit_setting (
    relation_unit_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT,
    CONSTRAINT chk_key_empty CHECK ("key" != ''),
    CONSTRAINT fk_relation_unit_uuid
    FOREIGN KEY (relation_unit_uuid)
    REFERENCES relation_unit (uuid),
    PRIMARY KEY (relation_unit_uuid, "key")
);

CREATE INDEX idx_relation_unit_setting_unit
ON relation_unit_setting (relation_unit_uuid);

-- relation_unit_settings_hash holds a hash of all settings for a relation unit.
-- It allows watchers to easily determine when the relation units settings have
-- changed.
CREATE TABLE relation_unit_settings_hash (
    relation_unit_uuid TEXT NOT NULL PRIMARY KEY,
    sha256 TEXT NOT NULL,
    CONSTRAINT fk_relation_unit_setting_hash_relation_unit
    FOREIGN KEY (relation_unit_uuid)
    REFERENCES relation_unit (uuid)
);

-- relation_unit_setting_archive is used to fullfil a contract we have, whereby
-- the settings for a relation unit are accessible for the lifetime of a
-- relation, regardless of whether the unit has departed the relation, or even
-- exists any longer.
-- Upon leaving scope, we copy the unit's relation settings into this table.
-- Accessing relation settings via the relation-get hook tool will cause Juju to
-- check this table if the requested unit is not in scope.
-- We need no triggers for this table, because we copy the settings before doing
-- the relation_unit_settings deletion, and once copied they are static until
-- the relation itself is deleted.
CREATE TABLE relation_unit_setting_archive (
    relation_uuid TEXT NOT NULL,
    unit_name TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    PRIMARY KEY (relation_uuid, unit_name, "key")
);

-- The relation_application_setting holds key value pair settings
-- for a relation at the application level. Keys must be unique
-- per application.
CREATE TABLE relation_application_setting (
    relation_endpoint_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT,
    CONSTRAINT chk_key_empty CHECK ("key" != ''),
    CONSTRAINT fk_relation_endpoint_uuid
    FOREIGN KEY (relation_endpoint_uuid)
    REFERENCES relation_endpoint (uuid),
    PRIMARY KEY (relation_endpoint_uuid, "key")
);

CREATE INDEX idx_relation_ep_setting_ep
ON relation_application_setting (relation_endpoint_uuid);

-- relation_application_settings_hash holds a hash of all application settings
-- for a relation endpoint. It allows watchers to easily determine when the
-- relations application settings have changed.
CREATE TABLE relation_application_settings_hash (
    relation_endpoint_uuid TEXT NOT NULL PRIMARY KEY,
    sha256 TEXT NOT NULL,
    CONSTRAINT fk_relation_application_setting_hash_relation_endpoint
    FOREIGN KEY (relation_endpoint_uuid)
    REFERENCES relation_endpoint (uuid)
);

CREATE TABLE relation_status_type (
    id TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_relation_status_type_name
ON relation_status_type (name);

INSERT INTO relation_status_type VALUES
(0, 'joining'),
(1, 'joined'),
(2, 'broken'),
(3, 'suspending'),
(4, 'suspended'),
(5, 'error');

-- The relation_status maps a relation to its status
-- as defined in the relation_status_type table.
CREATE TABLE relation_status (
    relation_uuid TEXT NOT NULL PRIMARY KEY,
    relation_status_type_id TEXT NOT NULL,
    message TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    CONSTRAINT fk_relation_status_type_id
    FOREIGN KEY (relation_status_type_id)
    REFERENCES relation_status_type (id)
);

CREATE VIEW v_application_endpoint AS
SELECT
    ae.uuid AS application_endpoint_uuid,
    cr.name AS endpoint_name,
    ae.application_uuid,
    a.name AS application_name,
    cr.interface,
    cr.optional,
    cr.capacity,
    crr.name AS role,
    crs.name AS scope
FROM application_endpoint AS ae
JOIN application AS a ON ae.application_uuid = a.uuid
JOIN charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
JOIN charm_relation_role AS crr ON cr.role_id = crr.id
JOIN charm_relation_scope AS crs ON cr.scope_id = crs.id;

CREATE VIEW v_relation_endpoint AS
SELECT
    re.uuid AS relation_endpoint_uuid,
    re.endpoint_uuid AS application_endpoint_uuid,
    re.relation_uuid,
    ae.application_uuid,
    a.name AS application_name,
    cr.name AS endpoint_name,
    cr.interface,
    cr.optional,
    cr.capacity,
    crr.name AS role,
    crs.name AS scope
FROM relation_endpoint AS re
JOIN relation AS r ON re.relation_uuid = r.uuid
JOIN application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN application AS a ON ae.application_uuid = a.uuid
JOIN charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
JOIN charm_relation_role AS crr ON cr.role_id = crr.id
JOIN charm_relation_scope AS crs ON r.scope_id = crs.id;

CREATE VIEW v_relation_endpoint_identifier AS
SELECT
    re.relation_uuid,
    a.name AS application_name,
    cr.name AS endpoint_name
FROM relation_endpoint AS re
JOIN application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
JOIN charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
JOIN application AS a ON ae.application_uuid = a.uuid;

CREATE VIEW v_relation_status AS
SELECT
    rs.relation_uuid,
    rst.name AS status,
    rs.message,
    rs.updated_at
FROM relation_status AS rs
JOIN relation_status_type AS rst ON rs.relation_status_type_id = rst.id;

CREATE TABLE block_command_type (
    id INT PRIMARY KEY,
    name_type TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_block_command_type_name
ON block_command_type (name_type);

INSERT INTO block_command_type VALUES
(0, 'destroy'),
(1, 'remove'),
(2, 'change');

CREATE TABLE block_command (
    uuid TEXT NOT NULL PRIMARY KEY,
    block_command_type_id INT NOT NULL,
    message TEXT,
    CONSTRAINT fk_block_command_type
    FOREIGN KEY (block_command_type_id)
    REFERENCES block_command_type (id)
);

CREATE UNIQUE INDEX idx_block_command_type
ON block_command (block_command_type_id);

CREATE VIEW v_revision_updater_application AS
SELECT
    a.uuid,
    a.name,
    c.reference_name,
    c.revision,
    c.architecture_id AS charm_architecture_id,
    ac.track AS channel_track,
    ac.risk AS channel_risk,
    ac.branch AS channel_branch,
    ap.os_id AS platform_os_id,
    ap.channel AS platform_channel,
    ap.architecture_id AS platform_architecture_id,
    cdi.charmhub_identifier
FROM application AS a
LEFT JOIN charm AS c ON a.charm_uuid = c.uuid
LEFT JOIN application_channel AS ac ON a.uuid = ac.application_uuid
LEFT JOIN application_platform AS ap ON a.uuid = ap.application_uuid
LEFT JOIN charm_download_info AS cdi ON c.uuid = cdi.charm_uuid
WHERE a.life_id = 0 AND c.source_id = 1;

CREATE VIEW v_revision_updater_application_unit AS
SELECT
    a.uuid,
    COUNT(u.uuid) AS num_units
FROM application AS a
LEFT JOIN unit AS u ON a.uuid = u.application_uuid
GROUP BY u.uuid;

-- agent_stream defines the recognised streams available in the model for
-- fetching agent binaries.
CREATE TABLE agent_stream (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_agent_stream_name
ON agent_stream (name);

INSERT INTO agent_stream VALUES
(0, 'released'),
(1, 'proposed'),
(2, 'testing'),
(3, 'devel');

CREATE TABLE agent_version (
    stream_id INT NOT NULL,
    target_version TEXT NOT NULL,
    latest_version TEXT NOT NULL,
    FOREIGN KEY (stream_id)
    REFERENCES agent_stream (id)
);

-- A unique constraint over a constant index
-- ensures only 1 row can exist.
CREATE UNIQUE INDEX idx_singleton_agent_version ON agent_version ((1));

CREATE TABLE removal_type (
    id INT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_removal_type_name
ON removal_type (name);

INSERT INTO removal_type VALUES
(0, 'relation'),
(1, 'unit'),
(2, 'application'),
(3, 'machine'),
(4, 'model'),
(5, 'storage instance'),
(6, 'storage attachment'),
(7, 'storage volume'),
(8, 'storage filesystem'),
(9, 'storage volume attachment'),
(10, 'storage volume attachment plan'),
(11, 'storage filesystem attachment'),
(12, 'remote application offerer'),
(13, 'relation with remote offerer'),
(14, 'relation with remote consumer');

CREATE TABLE removal (
    uuid TEXT NOT NULL PRIMARY KEY,
    removal_type_id INT NOT NULL,
    entity_uuid TEXT NOT NULL,
    force INT NOT NULL DEFAULT 0,
    -- Indicates when the job should be actioned by the worker,
    -- allowing us to schedule removals in the future.
    scheduled_for DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    -- JSON for free-form job argumentation.
    arg TEXT,
    CONSTRAINT fk_removal_type
    FOREIGN KEY (removal_type_id)
    REFERENCES removal_type (id)
);

CREATE TABLE sequence (
    namespace TEXT NOT NULL PRIMARY KEY,
    value INT NOT NULL
);

-- The agent_binary_store table stores information about agent binaries stored in the model's object store,
-- including their version, SHA, architecture, and object store information.
CREATE TABLE agent_binary_store (
    version TEXT NOT NULL,
    architecture_id INT NOT NULL,
    object_store_uuid TEXT NOT NULL,
    PRIMARY KEY (version, architecture_id),
    CONSTRAINT fk_agent_binary_metadata_object_store_metadata
    FOREIGN KEY (object_store_uuid)
    REFERENCES object_store_metadata (uuid),
    CONSTRAINT fk_agent_binary_metadata_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

CREATE VIEW v_agent_binary_store AS
SELECT
    abs.version,
    abs.object_store_uuid,
    abs.architecture_id,
    a.name AS architecture_name,
    osm.size,
    osm.sha_256,
    osm.sha_384,
    osmp.path
FROM agent_binary_store AS abs
JOIN architecture AS a ON abs.architecture_id = a.id
JOIN object_store_metadata AS osm ON abs.object_store_uuid = osm.uuid
JOIN object_store_metadata_path AS osmp ON osm.uuid = osmp.metadata_uuid;

-- model_agent represents information about the agent that runs on behalf of a
-- model.
CREATE TABLE model_agent (
    model_uuid TEXT NOT NULL,
    password_hash_algorithm_id INT,
    password_hash TEXT,
    CONSTRAINT fk_model_uuid
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid),
    CONSTRAINT fk_model_agent_password_hash_algorithm
    FOREIGN KEY (password_hash_algorithm_id)
    REFERENCES password_hash_algorithm (id)
);

CREATE TABLE offer (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

-- The offer_endpoint table is a join table to indicate which application
-- endpoints are included in the offer.
--
-- Note: trg_ensure_single_app_per_offer ensures that for every offer,
-- each endpoint_uuid is for the same application.
CREATE TABLE offer_endpoint (
    offer_uuid TEXT NOT NULL,
    endpoint_uuid TEXT NOT NULL,
    PRIMARY KEY (offer_uuid, endpoint_uuid),
    CONSTRAINT fk_endpoint_uuid
    FOREIGN KEY (endpoint_uuid)
    REFERENCES application_endpoint (uuid),
    CONSTRAINT fk_offer_uuid
    FOREIGN KEY (offer_uuid)
    REFERENCES offer (uuid)
);

CREATE VIEW v_offer_detail AS
WITH conn AS (
    SELECT offer_uuid FROM offer_connection
),

active_conn AS (
    SELECT oc.offer_uuid FROM offer_connection AS oc
    JOIN relation_status AS rs ON oc.remote_relation_uuid = rs.relation_uuid
    WHERE rs.relation_status_type_id = 1
)

SELECT
    o.uuid AS offer_uuid,
    o.name AS offer_name,
    a.name AS application_name,
    cm.description AS application_description,
    cm.name AS charm_name,
    c.revision AS charm_revision,
    cs.name AS charm_source,
    c.architecture_id AS charm_architecture,
    cr.name AS endpoint_name,
    crr.name AS endpoint_role,
    cr.interface AS endpoint_interface,
    (SELECT COUNT(*) FROM conn AS c WHERE o.uuid = c.offer_uuid) AS total_connections,
    (SELECT COUNT(*) FROM active_conn AS ac WHERE o.uuid = ac.offer_uuid) AS total_active_connections
FROM offer AS o
JOIN offer_endpoint AS oe ON o.uuid = oe.offer_uuid
JOIN application_endpoint AS ae ON oe.endpoint_uuid = ae.uuid
JOIN application AS a ON ae.application_uuid = a.uuid
JOIN charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
JOIN charm_relation_role AS crr ON cr.role_id = crr.id
JOIN charm AS c ON a.charm_uuid = c.uuid
JOIN charm_source AS cs ON c.source_id = cs.id
JOIN charm_metadata AS cm ON c.uuid = cm.charm_uuid;

-- An operation is an overview of an action or commands run on a remote
-- target by the user. It will be linked to N number of tasks, depending
-- on the number of entities it is run on.
-- An operation can be an action (meaning there will be an entry in 
-- operation_action) or not. If there is no entry, then the operation is an 
-- exec.
CREATE TABLE operation (
    uuid TEXT NOT NULL PRIMARY KEY,
    -- operation_id is a sequence number, and the sequence is shared with 
    -- the operation_task.task_id sequence.
    operation_id TEXT NOT NULL,
    summary TEXT,
    enqueued_at TIMESTAMP NOT NULL,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    parallel BOOLEAN DEFAULT false,
    execution_group TEXT
);

CREATE UNIQUE INDEX idx_operation_id
ON operation (operation_id);

-- operation_action is a join table to link an operation to its charm_action.
CREATE TABLE operation_action (
    operation_uuid TEXT NOT NULL PRIMARY KEY,
    charm_uuid TEXT NOT NULL,
    charm_action_key TEXT NOT NULL,
    CONSTRAINT fk_operation_uuid
    FOREIGN KEY (operation_uuid)
    REFERENCES operation (uuid),
    CONSTRAINT fk_charm_action
    FOREIGN KEY (charm_uuid, charm_action_key)
    REFERENCES charm_action (charm_uuid, "key")
);

CREATE INDEX idx_operation_action_charm_action_key_operation_uuid
ON operation_action (charm_action_key, operation_uuid);

-- A operation_task is the individual representation of an operation on a specific
-- receiver. Either a machine or unit.
CREATE TABLE operation_task (
    uuid TEXT NOT NULL PRIMARY KEY,
    operation_uuid TEXT NOT NULL,
    -- task_id is a sequence number, and the sequence is shared with 
    -- the operation.operation_id sequence.
    task_id TEXT NOT NULL,
    enqueued_at DATETIME NOT NULL,
    started_at DATETIME,
    completed_at DATETIME,
    CONSTRAINT fk_operation
    FOREIGN KEY (operation_uuid)
    REFERENCES operation (uuid)
);

CREATE UNIQUE INDEX idx_task_id
ON operation_task (task_id);

-- operation_unit_task is a join table to link a task with its unit receiver.
CREATE TABLE operation_unit_task (
    task_uuid TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    PRIMARY KEY (task_uuid, unit_uuid),
    CONSTRAINT fk_task_uuid
    FOREIGN KEY (task_uuid)
    REFERENCES operation_task (uuid),
    CONSTRAINT fk_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

-- operation_machine_task is a join table to link a task with its machine receiver.
CREATE TABLE operation_machine_task (
    task_uuid TEXT NOT NULL,
    machine_uuid TEXT NOT NULL,
    PRIMARY KEY (task_uuid, machine_uuid),
    CONSTRAINT fk_task_uuid
    FOREIGN KEY (task_uuid)
    REFERENCES operation_task (uuid),
    CONSTRAINT fk_machine_uuid
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
);

-- operation_task_output is a join table to link a task with where
-- its output is stored.
CREATE TABLE operation_task_output (
    task_uuid TEXT NOT NULL PRIMARY KEY,
    store_path TEXT NOT NULL,
    CONSTRAINT fk_task_uuid
    FOREIGN KEY (task_uuid)
    REFERENCES operation_task (uuid),
    CONSTRAINT fk_store_path
    FOREIGN KEY (store_path)
    REFERENCES object_store_metadata_path (path)
);

-- operation_task_status is the status of the task.
CREATE TABLE operation_task_status (
    task_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_task_uuid
    FOREIGN KEY (task_uuid)
    REFERENCES operation_task (uuid),
    CONSTRAINT fk_task_status
    FOREIGN KEY (status_id)
    REFERENCES operation_task_status_value (id)
);

-- operation_task_status_value holds the possible status values for a task.
CREATE TABLE operation_task_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

INSERT INTO operation_task_status_value VALUES
(0, 'error'),
(1, 'running'),
(2, 'pending'),
(3, 'failed'),
(4, 'cancelled'),
(5, 'completed'),
(6, 'aborting'),
(7, 'aborted');

-- operation_task_log holds log messages of the task.
CREATE TABLE operation_task_log (
    task_uuid TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT fk_task_uuid
    FOREIGN KEY (task_uuid)
    REFERENCES operation_task (uuid)
);

CREATE INDEX idx_operation_task_log_id
ON operation_task_log (task_uuid, created_at);

-- operation_parameter holds the parameters passed to an operation.
-- In the case of an action, these are the user-passed parameters, where the 
-- keys should match the charm_action's parameters.
-- In the case of an exec, these will contain the "command" and "timeout" 
-- parameters.
CREATE TABLE operation_parameter (
    operation_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (operation_uuid, "key"),
    CONSTRAINT fk_operation_uuid
    FOREIGN KEY (operation_uuid)
    REFERENCES operation (uuid)
);

-- application_remote_offerer represents a remote offerer application
-- inside of the consumer model.
CREATE TABLE application_remote_offerer (
    uuid TEXT NOT NULL PRIMARY KEY,
    life_id INT NOT NULL,
    -- application_uuid is the synthetic application in the consumer model.
    -- Locating charm is done through the application.
    application_uuid TEXT NOT NULL,
    -- offer_uuid is the offer uuid that ties both the offerer and the consumer
    -- together.
    offer_uuid TEXT NOT NULL,
    -- offer_url is the URL of the offer that the remote application is
    -- consuming.
    offer_url TEXT NOT NULL,
    -- offerer_controller_uuid is the offering controller where the
    -- offerer application is located. There is no FK constraint on it,
    -- because that information is located in the controller DB.
    offerer_controller_uuid TEXT,
    -- offerer_model_uuid is the model in the offering controller where
    -- the offerer application is located. There is no FK constraint on it,
    -- because we don't have the model locally.
    offerer_model_uuid TEXT NOT NULL,
    -- macaroon represents the credentials to access the offering model.
    macaroon TEXT NOT NULL,
    CONSTRAINT fk_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_life_id
    FOREIGN KEY (life_id)
    REFERENCES life (id)
);

-- Ensure that an offer can only be consumed once in a model.
CREATE UNIQUE INDEX idx_application_remote_offerer_offer_uuid
ON application_remote_offerer (offer_uuid);

-- Ensure that an application can only be used once as a remote offerer.
CREATE UNIQUE INDEX idx_application_remote_offerer_application_uuid
ON application_remote_offerer (application_uuid);

-- application_remote_offerer_status represents the status of the remote
-- offerer application inside of the consumer model.
CREATE TABLE application_remote_offerer_status (
    application_remote_offerer_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    data TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_application_remote_offerer_status
    FOREIGN KEY (application_remote_offerer_uuid)
    REFERENCES application_remote_offerer (uuid),
    CONSTRAINT fk_workload_status_value_status
    FOREIGN KEY (status_id)
    REFERENCES workload_status_value (id)
);

-- application_remote_offerer_relation_macaroon represents the macaroon
-- used to authenticate against the offering model for a given relation.
CREATE TABLE application_remote_offerer_relation_macaroon (
    relation_uuid TEXT NOT NULL PRIMARY KEY,
    macaroon TEXT NOT NULL,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid)
);

-- offer connection links the application remote consumer to the offer.
CREATE TABLE offer_connection (
    uuid TEXT NOT NULL PRIMARY KEY,
    -- offer_uuid is the offer that the remote application is using.
    offer_uuid TEXT NOT NULL,
    -- remote_relation_uuid is the relation for which the offer connection
    -- is made. It uses the relation, as we can identify both the
    -- relation id and the relation key from it.
    remote_relation_uuid TEXT NOT NULL,
    -- username is the user in the consumer model that created the offer
    -- connection. This is not a user, but an offer user for which offers are
    -- granted permissions on.
    username TEXT NOT NULL,
    CONSTRAINT fk_offer_uuid
    FOREIGN KEY (offer_uuid)
    REFERENCES offer (uuid),
    CONSTRAINT fk_remote_relation_uuid
    FOREIGN KEY (remote_relation_uuid)
    REFERENCES relation (uuid)
);

-- application_remote_consumer represents a remote consumer application
-- inside of the offering model.
CREATE TABLE application_remote_consumer (
    -- offer_connection_uuid is the offer connection that links the remote
    -- consumer to the offer. This is the same UUID as the synthetic application
    -- that represents the remote application in the consumer model.
    offer_connection_uuid TEXT NOT NULL PRIMARY KEY,
    -- offerer_application_uuid is application UUID of the offer in the offering
    -- model.
    offerer_application_uuid TEXT NOT NULL,
    -- consumed_application_uuid is the (remote token) application UUID in 
    -- the consumer model.
    consumer_application_uuid TEXT NOT NULL,
    -- consumer_model_uuid is the model in the consuming controller where
    -- the consumer application is located. There is no FK constraint on it,
    -- because we don't have the model locally.
    consumer_model_uuid TEXT NOT NULL,
    life_id INT NOT NULL,
    CONSTRAINT fk_life_id
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_offerer_application_uuid
    FOREIGN KEY (offerer_application_uuid)
    REFERENCES application (uuid),
    -- This is correct, the offer_connection_uuid is both the connection_uuid
    -- and the application uuid representing the synth application. The
    CONSTRAINT fk_offer_connection_uuid
    FOREIGN KEY (offer_connection_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_offer_connection_uuid
    FOREIGN KEY (offer_connection_uuid)
    REFERENCES offer_connection (uuid)
);

-- relation_network_ingress holds information about ingress CIDRs for a 
-- relation.
CREATE TABLE relation_network_ingress (
    relation_uuid TEXT NOT NULL,
    cidr TEXT NOT NULL,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    PRIMARY KEY (relation_uuid, cidr)
);

-- relation_network_egress holds information about egress CIDRs for a relation.
CREATE TABLE relation_network_egress (
    relation_uuid TEXT NOT NULL,
    cidr TEXT NOT NULL,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    PRIMARY KEY (relation_uuid, cidr)
);


-- insert namespace for BlockDevice
INSERT INTO change_log_namespace VALUES (10002, 'block_device', 'BlockDevice changes based on machine_uuid');

-- insert trigger for BlockDevice
CREATE TRIGGER trg_log_block_device_insert
AFTER INSERT ON block_device FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10002, NEW.machine_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for BlockDevice
CREATE TRIGGER trg_log_block_device_update
AFTER UPDATE ON block_device FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.machine_uuid != OLD.machine_uuid OR
	(NEW.name != OLD.name OR (NEW.name IS NOT NULL AND OLD.name IS NULL) OR (NEW.name IS NULL AND OLD.name IS NOT NULL)) OR
	(NEW.hardware_id != OLD.hardware_id OR (NEW.hardware_id IS NOT NULL AND OLD.hardware_id IS NULL) OR (NEW.hardware_id IS NULL AND OLD.hardware_id IS NOT NULL)) OR
	(NEW.wwn != OLD.wwn OR (NEW.wwn IS NOT NULL AND OLD.wwn IS NULL) OR (NEW.wwn IS NULL AND OLD.wwn IS NOT NULL)) OR
	(NEW.serial_id != OLD.serial_id OR (NEW.serial_id IS NOT NULL AND OLD.serial_id IS NULL) OR (NEW.serial_id IS NULL AND OLD.serial_id IS NOT NULL)) OR
	(NEW.bus_address != OLD.bus_address OR (NEW.bus_address IS NOT NULL AND OLD.bus_address IS NULL) OR (NEW.bus_address IS NULL AND OLD.bus_address IS NOT NULL)) OR
	(NEW.size_mib != OLD.size_mib OR (NEW.size_mib IS NOT NULL AND OLD.size_mib IS NULL) OR (NEW.size_mib IS NULL AND OLD.size_mib IS NOT NULL)) OR
	(NEW.mount_point != OLD.mount_point OR (NEW.mount_point IS NOT NULL AND OLD.mount_point IS NULL) OR (NEW.mount_point IS NULL AND OLD.mount_point IS NOT NULL)) OR
	(NEW.in_use != OLD.in_use OR (NEW.in_use IS NOT NULL AND OLD.in_use IS NULL) OR (NEW.in_use IS NULL AND OLD.in_use IS NOT NULL)) OR
	(NEW.filesystem_label != OLD.filesystem_label OR (NEW.filesystem_label IS NOT NULL AND OLD.filesystem_label IS NULL) OR (NEW.filesystem_label IS NULL AND OLD.filesystem_label IS NOT NULL)) OR
	(NEW.host_filesystem_uuid != OLD.host_filesystem_uuid OR (NEW.host_filesystem_uuid IS NOT NULL AND OLD.host_filesystem_uuid IS NULL) OR (NEW.host_filesystem_uuid IS NULL AND OLD.host_filesystem_uuid IS NOT NULL)) OR
	(NEW.filesystem_type != OLD.filesystem_type OR (NEW.filesystem_type IS NOT NULL AND OLD.filesystem_type IS NULL) OR (NEW.filesystem_type IS NULL AND OLD.filesystem_type IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10002, OLD.machine_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for BlockDevice
CREATE TRIGGER trg_log_block_device_delete
AFTER DELETE ON block_device FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10002, OLD.machine_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for ModelConfig
INSERT INTO change_log_namespace VALUES (10000, 'model_config', 'ModelConfig changes based on key');

-- insert trigger for ModelConfig
CREATE TRIGGER trg_log_model_config_insert
AFTER INSERT ON model_config FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10000, NEW.key, DATETIME('now', 'utc'));
END;

-- update trigger for ModelConfig
CREATE TRIGGER trg_log_model_config_update
AFTER UPDATE ON model_config FOR EACH ROW
WHEN 
	NEW.key != OLD.key OR
	NEW.value != OLD.value 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10000, OLD.key, DATETIME('now', 'utc'));
END;
-- delete trigger for ModelConfig
CREATE TRIGGER trg_log_model_config_delete
AFTER DELETE ON model_config FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10000, OLD.key, DATETIME('now', 'utc'));
END;

-- insert namespace for ObjectStoreMetadataPath
INSERT INTO change_log_namespace VALUES (10001, 'object_store_metadata_path', 'ObjectStoreMetadataPath changes based on path');

-- insert trigger for ObjectStoreMetadataPath
CREATE TRIGGER trg_log_object_store_metadata_path_insert
AFTER INSERT ON object_store_metadata_path FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10001, NEW.path, DATETIME('now', 'utc'));
END;

-- update trigger for ObjectStoreMetadataPath
CREATE TRIGGER trg_log_object_store_metadata_path_update
AFTER UPDATE ON object_store_metadata_path FOR EACH ROW
WHEN 
	NEW.path != OLD.path OR
	NEW.metadata_uuid != OLD.metadata_uuid 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10001, OLD.path, DATETIME('now', 'utc'));
END;
-- delete trigger for ObjectStoreMetadataPath
CREATE TRIGGER trg_log_object_store_metadata_path_delete
AFTER DELETE ON object_store_metadata_path FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10001, OLD.path, DATETIME('now', 'utc'));
END;

-- insert namespace for SecretMetadata
INSERT INTO change_log_namespace VALUES (10003, 'secret_metadata', 'SecretMetadata changes based on secret_id');

-- insert trigger for SecretMetadata
CREATE TRIGGER trg_log_secret_metadata_insert
AFTER INSERT ON secret_metadata FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10003, NEW.secret_id, DATETIME('now', 'utc'));
END;

-- update trigger for SecretMetadata
CREATE TRIGGER trg_log_secret_metadata_update
AFTER UPDATE ON secret_metadata FOR EACH ROW
WHEN 
	NEW.secret_id != OLD.secret_id OR
	NEW.version != OLD.version OR
	(NEW.description != OLD.description OR (NEW.description IS NOT NULL AND OLD.description IS NULL) OR (NEW.description IS NULL AND OLD.description IS NOT NULL)) OR
	NEW.rotate_policy_id != OLD.rotate_policy_id OR
	NEW.auto_prune != OLD.auto_prune OR
	(NEW.latest_revision_checksum != OLD.latest_revision_checksum OR (NEW.latest_revision_checksum IS NOT NULL AND OLD.latest_revision_checksum IS NULL) OR (NEW.latest_revision_checksum IS NULL AND OLD.latest_revision_checksum IS NOT NULL)) OR
	NEW.create_time != OLD.create_time OR
	NEW.update_time != OLD.update_time 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10003, OLD.secret_id, DATETIME('now', 'utc'));
END;
-- delete trigger for SecretMetadata
CREATE TRIGGER trg_log_secret_metadata_delete
AFTER DELETE ON secret_metadata FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10003, OLD.secret_id, DATETIME('now', 'utc'));
END;

-- insert namespace for SecretRotation
INSERT INTO change_log_namespace VALUES (10004, 'secret_rotation', 'SecretRotation changes based on secret_id');

-- insert trigger for SecretRotation
CREATE TRIGGER trg_log_secret_rotation_insert
AFTER INSERT ON secret_rotation FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10004, NEW.secret_id, DATETIME('now', 'utc'));
END;

-- update trigger for SecretRotation
CREATE TRIGGER trg_log_secret_rotation_update
AFTER UPDATE ON secret_rotation FOR EACH ROW
WHEN 
	NEW.secret_id != OLD.secret_id OR
	NEW.next_rotation_time != OLD.next_rotation_time 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10004, OLD.secret_id, DATETIME('now', 'utc'));
END;
-- delete trigger for SecretRotation
CREATE TRIGGER trg_log_secret_rotation_delete
AFTER DELETE ON secret_rotation FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10004, OLD.secret_id, DATETIME('now', 'utc'));
END;

-- insert namespace for SecretRevisionObsolete
INSERT INTO change_log_namespace VALUES (10005, 'secret_revision_obsolete', 'SecretRevisionObsolete changes based on revision_uuid');

-- insert trigger for SecretRevisionObsolete
CREATE TRIGGER trg_log_secret_revision_obsolete_insert
AFTER INSERT ON secret_revision_obsolete FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10005, NEW.revision_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for SecretRevisionObsolete
CREATE TRIGGER trg_log_secret_revision_obsolete_update
AFTER UPDATE ON secret_revision_obsolete FOR EACH ROW
WHEN 
	NEW.revision_uuid != OLD.revision_uuid OR
	NEW.obsolete != OLD.obsolete OR
	NEW.pending_delete != OLD.pending_delete 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10005, OLD.revision_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for SecretRevisionObsolete
CREATE TRIGGER trg_log_secret_revision_obsolete_delete
AFTER DELETE ON secret_revision_obsolete FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10005, OLD.revision_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for SecretRevisionExpire
INSERT INTO change_log_namespace VALUES (10006, 'secret_revision_expire', 'SecretRevisionExpire changes based on revision_uuid');

-- insert trigger for SecretRevisionExpire
CREATE TRIGGER trg_log_secret_revision_expire_insert
AFTER INSERT ON secret_revision_expire FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10006, NEW.revision_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for SecretRevisionExpire
CREATE TRIGGER trg_log_secret_revision_expire_update
AFTER UPDATE ON secret_revision_expire FOR EACH ROW
WHEN 
	NEW.revision_uuid != OLD.revision_uuid OR
	NEW.expire_time != OLD.expire_time 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10006, OLD.revision_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for SecretRevisionExpire
CREATE TRIGGER trg_log_secret_revision_expire_delete
AFTER DELETE ON secret_revision_expire FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10006, OLD.revision_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for SecretRevision
INSERT INTO change_log_namespace VALUES (10007, 'secret_revision', 'SecretRevision changes based on uuid');

-- insert trigger for SecretRevision
CREATE TRIGGER trg_log_secret_revision_insert
AFTER INSERT ON secret_revision FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10007, NEW.uuid, DATETIME('now', 'utc'));
END;

-- update trigger for SecretRevision
CREATE TRIGGER trg_log_secret_revision_update
AFTER UPDATE ON secret_revision FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.secret_id != OLD.secret_id OR
	NEW.revision != OLD.revision OR
	NEW.create_time != OLD.create_time 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10007, OLD.uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for SecretRevision
CREATE TRIGGER trg_log_secret_revision_delete
AFTER DELETE ON secret_revision FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10007, OLD.uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for SecretReference
INSERT INTO change_log_namespace VALUES (10008, 'secret_reference', 'SecretReference changes based on secret_id');

-- insert trigger for SecretReference
CREATE TRIGGER trg_log_secret_reference_insert
AFTER INSERT ON secret_reference FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10008, NEW.secret_id, DATETIME('now', 'utc'));
END;

-- update trigger for SecretReference
CREATE TRIGGER trg_log_secret_reference_update
AFTER UPDATE ON secret_reference FOR EACH ROW
WHEN 
	NEW.secret_id != OLD.secret_id OR
	NEW.latest_revision != OLD.latest_revision OR
	NEW.owner_application_uuid != OLD.owner_application_uuid 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10008, OLD.secret_id, DATETIME('now', 'utc'));
END;
-- delete trigger for SecretReference
CREATE TRIGGER trg_log_secret_reference_delete
AFTER DELETE ON secret_reference FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10008, OLD.secret_id, DATETIME('now', 'utc'));
END;

-- insert namespace for Subnet
INSERT INTO change_log_namespace VALUES (10009, 'subnet', 'Subnet changes based on uuid');

-- insert trigger for Subnet
CREATE TRIGGER trg_log_subnet_insert
AFTER INSERT ON subnet FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10009, NEW.uuid, DATETIME('now', 'utc'));
END;

-- update trigger for Subnet
CREATE TRIGGER trg_log_subnet_update
AFTER UPDATE ON subnet FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.cidr != OLD.cidr OR
	(NEW.vlan_tag != OLD.vlan_tag OR (NEW.vlan_tag IS NOT NULL AND OLD.vlan_tag IS NULL) OR (NEW.vlan_tag IS NULL AND OLD.vlan_tag IS NOT NULL)) OR
	(NEW.space_uuid != OLD.space_uuid OR (NEW.space_uuid IS NOT NULL AND OLD.space_uuid IS NULL) OR (NEW.space_uuid IS NULL AND OLD.space_uuid IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10009, OLD.uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for Subnet
CREATE TRIGGER trg_log_subnet_delete
AFTER DELETE ON subnet FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10009, OLD.uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for Machine
INSERT INTO change_log_namespace VALUES (10010, 'machine', 'Machine changes based on uuid');

-- insert trigger for Machine
CREATE TRIGGER trg_log_machine_insert
AFTER INSERT ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10010, NEW.uuid, DATETIME('now', 'utc'));
END;

-- update trigger for Machine
CREATE TRIGGER trg_log_machine_update
AFTER UPDATE ON machine FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.name != OLD.name OR
	NEW.net_node_uuid != OLD.net_node_uuid OR
	NEW.life_id != OLD.life_id OR
	(NEW.nonce != OLD.nonce OR (NEW.nonce IS NOT NULL AND OLD.nonce IS NULL) OR (NEW.nonce IS NULL AND OLD.nonce IS NOT NULL)) OR
	(NEW.password_hash_algorithm_id != OLD.password_hash_algorithm_id OR (NEW.password_hash_algorithm_id IS NOT NULL AND OLD.password_hash_algorithm_id IS NULL) OR (NEW.password_hash_algorithm_id IS NULL AND OLD.password_hash_algorithm_id IS NOT NULL)) OR
	(NEW.password_hash != OLD.password_hash OR (NEW.password_hash IS NOT NULL AND OLD.password_hash IS NULL) OR (NEW.password_hash IS NULL AND OLD.password_hash IS NOT NULL)) OR
	(NEW.force_destroyed != OLD.force_destroyed OR (NEW.force_destroyed IS NOT NULL AND OLD.force_destroyed IS NULL) OR (NEW.force_destroyed IS NULL AND OLD.force_destroyed IS NOT NULL)) OR
	(NEW.agent_started_at != OLD.agent_started_at OR (NEW.agent_started_at IS NOT NULL AND OLD.agent_started_at IS NULL) OR (NEW.agent_started_at IS NULL AND OLD.agent_started_at IS NOT NULL)) OR
	(NEW.hostname != OLD.hostname OR (NEW.hostname IS NOT NULL AND OLD.hostname IS NULL) OR (NEW.hostname IS NULL AND OLD.hostname IS NOT NULL)) OR
	(NEW.keep_instance != OLD.keep_instance OR (NEW.keep_instance IS NOT NULL AND OLD.keep_instance IS NULL) OR (NEW.keep_instance IS NULL AND OLD.keep_instance IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10010, OLD.uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for Machine
CREATE TRIGGER trg_log_machine_delete
AFTER DELETE ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10010, OLD.uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for MachineLxdProfile
INSERT INTO change_log_namespace VALUES (10011, 'machine_lxd_profile', 'MachineLxdProfile changes based on machine_uuid');

-- insert trigger for MachineLxdProfile
CREATE TRIGGER trg_log_machine_lxd_profile_insert
AFTER INSERT ON machine_lxd_profile FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10011, NEW.machine_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for MachineLxdProfile
CREATE TRIGGER trg_log_machine_lxd_profile_update
AFTER UPDATE ON machine_lxd_profile FOR EACH ROW
WHEN 
	NEW.machine_uuid != OLD.machine_uuid OR
	NEW.name != OLD.name OR
	NEW.array_index != OLD.array_index 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10011, OLD.machine_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for MachineLxdProfile
CREATE TRIGGER trg_log_machine_lxd_profile_delete
AFTER DELETE ON machine_lxd_profile FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10011, OLD.machine_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for MachineCloudInstance
INSERT INTO change_log_namespace VALUES (10012, 'machine_cloud_instance', 'MachineCloudInstance changes based on machine_uuid');

-- insert trigger for MachineCloudInstance
CREATE TRIGGER trg_log_machine_cloud_instance_insert
AFTER INSERT ON machine_cloud_instance FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10012, NEW.machine_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for MachineCloudInstance
CREATE TRIGGER trg_log_machine_cloud_instance_update
AFTER UPDATE ON machine_cloud_instance FOR EACH ROW
WHEN 
	NEW.machine_uuid != OLD.machine_uuid OR
	NEW.life_id != OLD.life_id OR
	(NEW.instance_id != OLD.instance_id OR (NEW.instance_id IS NOT NULL AND OLD.instance_id IS NULL) OR (NEW.instance_id IS NULL AND OLD.instance_id IS NOT NULL)) OR
	(NEW.display_name != OLD.display_name OR (NEW.display_name IS NOT NULL AND OLD.display_name IS NULL) OR (NEW.display_name IS NULL AND OLD.display_name IS NOT NULL)) OR
	(NEW.arch != OLD.arch OR (NEW.arch IS NOT NULL AND OLD.arch IS NULL) OR (NEW.arch IS NULL AND OLD.arch IS NOT NULL)) OR
	(NEW.availability_zone_uuid != OLD.availability_zone_uuid OR (NEW.availability_zone_uuid IS NOT NULL AND OLD.availability_zone_uuid IS NULL) OR (NEW.availability_zone_uuid IS NULL AND OLD.availability_zone_uuid IS NOT NULL)) OR
	(NEW.cpu_cores != OLD.cpu_cores OR (NEW.cpu_cores IS NOT NULL AND OLD.cpu_cores IS NULL) OR (NEW.cpu_cores IS NULL AND OLD.cpu_cores IS NOT NULL)) OR
	(NEW.cpu_power != OLD.cpu_power OR (NEW.cpu_power IS NOT NULL AND OLD.cpu_power IS NULL) OR (NEW.cpu_power IS NULL AND OLD.cpu_power IS NOT NULL)) OR
	(NEW.mem != OLD.mem OR (NEW.mem IS NOT NULL AND OLD.mem IS NULL) OR (NEW.mem IS NULL AND OLD.mem IS NOT NULL)) OR
	(NEW.root_disk != OLD.root_disk OR (NEW.root_disk IS NOT NULL AND OLD.root_disk IS NULL) OR (NEW.root_disk IS NULL AND OLD.root_disk IS NOT NULL)) OR
	(NEW.root_disk_source != OLD.root_disk_source OR (NEW.root_disk_source IS NOT NULL AND OLD.root_disk_source IS NULL) OR (NEW.root_disk_source IS NULL AND OLD.root_disk_source IS NOT NULL)) OR
	(NEW.virt_type != OLD.virt_type OR (NEW.virt_type IS NOT NULL AND OLD.virt_type IS NULL) OR (NEW.virt_type IS NULL AND OLD.virt_type IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10012, OLD.machine_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for MachineCloudInstance
CREATE TRIGGER trg_log_machine_cloud_instance_delete
AFTER DELETE ON machine_cloud_instance FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10012, OLD.machine_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for MachineRequiresReboot
INSERT INTO change_log_namespace VALUES (10013, 'machine_requires_reboot', 'MachineRequiresReboot changes based on machine_uuid');

-- insert trigger for MachineRequiresReboot
CREATE TRIGGER trg_log_machine_requires_reboot_insert
AFTER INSERT ON machine_requires_reboot FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10013, NEW.machine_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for MachineRequiresReboot
CREATE TRIGGER trg_log_machine_requires_reboot_update
AFTER UPDATE ON machine_requires_reboot FOR EACH ROW
WHEN 
	NEW.machine_uuid != OLD.machine_uuid OR
	NEW.created_at != OLD.created_at 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10013, OLD.machine_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for MachineRequiresReboot
CREATE TRIGGER trg_log_machine_requires_reboot_delete
AFTER DELETE ON machine_requires_reboot FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10013, OLD.machine_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for Charm
INSERT INTO change_log_namespace VALUES (10014, 'charm', 'Charm changes based on uuid');

-- insert trigger for Charm
CREATE TRIGGER trg_log_charm_insert
AFTER INSERT ON charm FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10014, NEW.uuid, DATETIME('now', 'utc'));
END;

-- update trigger for Charm
CREATE TRIGGER trg_log_charm_update
AFTER UPDATE ON charm FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	(NEW.archive_path != OLD.archive_path OR (NEW.archive_path IS NOT NULL AND OLD.archive_path IS NULL) OR (NEW.archive_path IS NULL AND OLD.archive_path IS NOT NULL)) OR
	(NEW.object_store_uuid != OLD.object_store_uuid OR (NEW.object_store_uuid IS NOT NULL AND OLD.object_store_uuid IS NULL) OR (NEW.object_store_uuid IS NULL AND OLD.object_store_uuid IS NOT NULL)) OR
	(NEW.available != OLD.available OR (NEW.available IS NOT NULL AND OLD.available IS NULL) OR (NEW.available IS NULL AND OLD.available IS NOT NULL)) OR
	(NEW.version != OLD.version OR (NEW.version IS NOT NULL AND OLD.version IS NULL) OR (NEW.version IS NULL AND OLD.version IS NOT NULL)) OR
	(NEW.lxd_profile != OLD.lxd_profile OR (NEW.lxd_profile IS NOT NULL AND OLD.lxd_profile IS NULL) OR (NEW.lxd_profile IS NULL AND OLD.lxd_profile IS NOT NULL)) OR
	NEW.source_id != OLD.source_id OR
	NEW.revision != OLD.revision OR
	(NEW.architecture_id != OLD.architecture_id OR (NEW.architecture_id IS NOT NULL AND OLD.architecture_id IS NULL) OR (NEW.architecture_id IS NULL AND OLD.architecture_id IS NOT NULL)) OR
	NEW.reference_name != OLD.reference_name OR
	NEW.create_time != OLD.create_time 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10014, OLD.uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for Charm
CREATE TRIGGER trg_log_charm_delete
AFTER DELETE ON charm FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10014, OLD.uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for Unit
INSERT INTO change_log_namespace VALUES (10015, 'unit', 'Unit changes based on uuid');

-- insert trigger for Unit
CREATE TRIGGER trg_log_unit_insert
AFTER INSERT ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10015, NEW.uuid, DATETIME('now', 'utc'));
END;

-- update trigger for Unit
CREATE TRIGGER trg_log_unit_update
AFTER UPDATE ON unit FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.name != OLD.name OR
	NEW.life_id != OLD.life_id OR
	NEW.application_uuid != OLD.application_uuid OR
	NEW.net_node_uuid != OLD.net_node_uuid OR
	NEW.charm_uuid != OLD.charm_uuid OR
	(NEW.password_hash_algorithm_id != OLD.password_hash_algorithm_id OR (NEW.password_hash_algorithm_id IS NOT NULL AND OLD.password_hash_algorithm_id IS NULL) OR (NEW.password_hash_algorithm_id IS NULL AND OLD.password_hash_algorithm_id IS NOT NULL)) OR
	(NEW.password_hash != OLD.password_hash OR (NEW.password_hash IS NOT NULL AND OLD.password_hash IS NULL) OR (NEW.password_hash IS NULL AND OLD.password_hash IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10015, OLD.uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for Unit
CREATE TRIGGER trg_log_unit_delete
AFTER DELETE ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10015, OLD.uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for UnitPrincipal
INSERT INTO change_log_namespace VALUES (10016, 'unit_principal', 'UnitPrincipal changes based on principal_uuid');

-- insert trigger for UnitPrincipal
CREATE TRIGGER trg_log_unit_principal_insert
AFTER INSERT ON unit_principal FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10016, NEW.principal_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for UnitPrincipal
CREATE TRIGGER trg_log_unit_principal_update
AFTER UPDATE ON unit_principal FOR EACH ROW
WHEN 
	NEW.unit_uuid != OLD.unit_uuid OR
	NEW.principal_uuid != OLD.principal_uuid 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10016, OLD.principal_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for UnitPrincipal
CREATE TRIGGER trg_log_unit_principal_delete
AFTER DELETE ON unit_principal FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10016, OLD.principal_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for UnitResolved
INSERT INTO change_log_namespace VALUES (10017, 'unit_resolved', 'UnitResolved changes based on unit_uuid');

-- insert trigger for UnitResolved
CREATE TRIGGER trg_log_unit_resolved_insert
AFTER INSERT ON unit_resolved FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10017, NEW.unit_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for UnitResolved
CREATE TRIGGER trg_log_unit_resolved_update
AFTER UPDATE ON unit_resolved FOR EACH ROW
WHEN 
	NEW.unit_uuid != OLD.unit_uuid OR
	NEW.mode_id != OLD.mode_id 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10017, OLD.unit_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for UnitResolved
CREATE TRIGGER trg_log_unit_resolved_delete
AFTER DELETE ON unit_resolved FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10017, OLD.unit_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for ApplicationScale
INSERT INTO change_log_namespace VALUES (10018, 'application_scale', 'ApplicationScale changes based on application_uuid');

-- insert trigger for ApplicationScale
CREATE TRIGGER trg_log_application_scale_insert
AFTER INSERT ON application_scale FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10018, NEW.application_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for ApplicationScale
CREATE TRIGGER trg_log_application_scale_update
AFTER UPDATE ON application_scale FOR EACH ROW
WHEN 
	NEW.application_uuid != OLD.application_uuid OR
	(NEW.scale != OLD.scale OR (NEW.scale IS NOT NULL AND OLD.scale IS NULL) OR (NEW.scale IS NULL AND OLD.scale IS NOT NULL)) OR
	(NEW.scale_target != OLD.scale_target OR (NEW.scale_target IS NOT NULL AND OLD.scale_target IS NULL) OR (NEW.scale_target IS NULL AND OLD.scale_target IS NOT NULL)) OR
	(NEW.scaling != OLD.scaling OR (NEW.scaling IS NOT NULL AND OLD.scaling IS NULL) OR (NEW.scaling IS NULL AND OLD.scaling IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10018, OLD.application_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for ApplicationScale
CREATE TRIGGER trg_log_application_scale_delete
AFTER DELETE ON application_scale FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10018, OLD.application_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for PortRange
INSERT INTO change_log_namespace VALUES (10019, 'port_range', 'PortRange changes based on unit_uuid');

-- insert trigger for PortRange
CREATE TRIGGER trg_log_port_range_insert
AFTER INSERT ON port_range FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10019, NEW.unit_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for PortRange
CREATE TRIGGER trg_log_port_range_update
AFTER UPDATE ON port_range FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.protocol_id != OLD.protocol_id OR
	(NEW.from_port != OLD.from_port OR (NEW.from_port IS NOT NULL AND OLD.from_port IS NULL) OR (NEW.from_port IS NULL AND OLD.from_port IS NOT NULL)) OR
	(NEW.to_port != OLD.to_port OR (NEW.to_port IS NOT NULL AND OLD.to_port IS NULL) OR (NEW.to_port IS NULL AND OLD.to_port IS NOT NULL)) OR
	(NEW.relation_uuid != OLD.relation_uuid OR (NEW.relation_uuid IS NOT NULL AND OLD.relation_uuid IS NULL) OR (NEW.relation_uuid IS NULL AND OLD.relation_uuid IS NOT NULL)) OR
	NEW.unit_uuid != OLD.unit_uuid 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10019, OLD.unit_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for PortRange
CREATE TRIGGER trg_log_port_range_delete
AFTER DELETE ON port_range FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10019, OLD.unit_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for ApplicationExposedEndpointSpace
INSERT INTO change_log_namespace VALUES (10020, 'application_exposed_endpoint_space', 'ApplicationExposedEndpointSpace changes based on application_uuid');

-- insert trigger for ApplicationExposedEndpointSpace
CREATE TRIGGER trg_log_application_exposed_endpoint_space_insert
AFTER INSERT ON application_exposed_endpoint_space FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10020, NEW.application_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for ApplicationExposedEndpointSpace
CREATE TRIGGER trg_log_application_exposed_endpoint_space_update
AFTER UPDATE ON application_exposed_endpoint_space FOR EACH ROW
WHEN 
	NEW.application_uuid != OLD.application_uuid OR
	(NEW.application_endpoint_uuid != OLD.application_endpoint_uuid OR (NEW.application_endpoint_uuid IS NOT NULL AND OLD.application_endpoint_uuid IS NULL) OR (NEW.application_endpoint_uuid IS NULL AND OLD.application_endpoint_uuid IS NOT NULL)) OR
	NEW.space_uuid != OLD.space_uuid 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10020, OLD.application_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for ApplicationExposedEndpointSpace
CREATE TRIGGER trg_log_application_exposed_endpoint_space_delete
AFTER DELETE ON application_exposed_endpoint_space FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10020, OLD.application_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for ApplicationExposedEndpointCidr
INSERT INTO change_log_namespace VALUES (10021, 'application_exposed_endpoint_cidr', 'ApplicationExposedEndpointCidr changes based on application_uuid');

-- insert trigger for ApplicationExposedEndpointCidr
CREATE TRIGGER trg_log_application_exposed_endpoint_cidr_insert
AFTER INSERT ON application_exposed_endpoint_cidr FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10021, NEW.application_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for ApplicationExposedEndpointCidr
CREATE TRIGGER trg_log_application_exposed_endpoint_cidr_update
AFTER UPDATE ON application_exposed_endpoint_cidr FOR EACH ROW
WHEN 
	NEW.application_uuid != OLD.application_uuid OR
	(NEW.application_endpoint_uuid != OLD.application_endpoint_uuid OR (NEW.application_endpoint_uuid IS NOT NULL AND OLD.application_endpoint_uuid IS NULL) OR (NEW.application_endpoint_uuid IS NULL AND OLD.application_endpoint_uuid IS NOT NULL)) OR
	NEW.cidr != OLD.cidr 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10021, OLD.application_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for ApplicationExposedEndpointCidr
CREATE TRIGGER trg_log_application_exposed_endpoint_cidr_delete
AFTER DELETE ON application_exposed_endpoint_cidr FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10021, OLD.application_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for SecretDeletedValueRef
INSERT INTO change_log_namespace VALUES (10022, 'secret_deleted_value_ref', 'SecretDeletedValueRef changes based on revision_uuid');

-- insert trigger for SecretDeletedValueRef
CREATE TRIGGER trg_log_secret_deleted_value_ref_insert
AFTER INSERT ON secret_deleted_value_ref FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10022, NEW.revision_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for SecretDeletedValueRef
CREATE TRIGGER trg_log_secret_deleted_value_ref_update
AFTER UPDATE ON secret_deleted_value_ref FOR EACH ROW
WHEN 
	NEW.revision_uuid != OLD.revision_uuid OR
	NEW.backend_uuid != OLD.backend_uuid OR
	NEW.revision_id != OLD.revision_id 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10022, OLD.revision_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for SecretDeletedValueRef
CREATE TRIGGER trg_log_secret_deleted_value_ref_delete
AFTER DELETE ON secret_deleted_value_ref FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10022, OLD.revision_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for Application
INSERT INTO change_log_namespace VALUES (10023, 'application', 'Application changes based on uuid');

-- insert trigger for Application
CREATE TRIGGER trg_log_application_insert
AFTER INSERT ON application FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10023, NEW.uuid, DATETIME('now', 'utc'));
END;

-- update trigger for Application
CREATE TRIGGER trg_log_application_update
AFTER UPDATE ON application FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.name != OLD.name OR
	NEW.life_id != OLD.life_id OR
	NEW.charm_uuid != OLD.charm_uuid OR
	NEW.charm_modified_version != OLD.charm_modified_version OR
	(NEW.charm_upgrade_on_error != OLD.charm_upgrade_on_error OR (NEW.charm_upgrade_on_error IS NOT NULL AND OLD.charm_upgrade_on_error IS NULL) OR (NEW.charm_upgrade_on_error IS NULL AND OLD.charm_upgrade_on_error IS NOT NULL)) OR
	NEW.space_uuid != OLD.space_uuid 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10023, OLD.uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for Application
CREATE TRIGGER trg_log_application_delete
AFTER DELETE ON application FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10023, OLD.uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for Removal
INSERT INTO change_log_namespace VALUES (10024, 'removal', 'Removal changes based on uuid');

-- insert trigger for Removal
CREATE TRIGGER trg_log_removal_insert
AFTER INSERT ON removal FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10024, NEW.uuid, DATETIME('now', 'utc'));
END;

-- update trigger for Removal
CREATE TRIGGER trg_log_removal_update
AFTER UPDATE ON removal FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.removal_type_id != OLD.removal_type_id OR
	NEW.entity_uuid != OLD.entity_uuid OR
	NEW.force != OLD.force OR
	NEW.scheduled_for != OLD.scheduled_for OR
	(NEW.arg != OLD.arg OR (NEW.arg IS NOT NULL AND OLD.arg IS NULL) OR (NEW.arg IS NULL AND OLD.arg IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10024, OLD.uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for Removal
CREATE TRIGGER trg_log_removal_delete
AFTER DELETE ON removal FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10024, OLD.uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for ApplicationConfigHash
INSERT INTO change_log_namespace VALUES (10025, 'application_config_hash', 'ApplicationConfigHash changes based on application_uuid');

-- insert trigger for ApplicationConfigHash
CREATE TRIGGER trg_log_application_config_hash_insert
AFTER INSERT ON application_config_hash FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10025, NEW.application_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for ApplicationConfigHash
CREATE TRIGGER trg_log_application_config_hash_update
AFTER UPDATE ON application_config_hash FOR EACH ROW
WHEN 
	NEW.application_uuid != OLD.application_uuid OR
	NEW.sha256 != OLD.sha256 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10025, OLD.application_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for ApplicationConfigHash
CREATE TRIGGER trg_log_application_config_hash_delete
AFTER DELETE ON application_config_hash FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10025, OLD.application_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for ApplicationSetting
INSERT INTO change_log_namespace VALUES (10026, 'application_setting', 'ApplicationSetting changes based on application_uuid');

-- insert trigger for ApplicationSetting
CREATE TRIGGER trg_log_application_setting_insert
AFTER INSERT ON application_setting FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10026, NEW.application_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for ApplicationSetting
CREATE TRIGGER trg_log_application_setting_update
AFTER UPDATE ON application_setting FOR EACH ROW
WHEN 
	NEW.application_uuid != OLD.application_uuid OR
	(NEW.trust != OLD.trust OR (NEW.trust IS NOT NULL AND OLD.trust IS NULL) OR (NEW.trust IS NULL AND OLD.trust IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10026, OLD.application_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for ApplicationSetting
CREATE TRIGGER trg_log_application_setting_delete
AFTER DELETE ON application_setting FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10026, OLD.application_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for RelationApplicationSettingsHash
INSERT INTO change_log_namespace VALUES (10028, 'relation_application_settings_hash', 'RelationApplicationSettingsHash changes based on relation_endpoint_uuid');

-- insert trigger for RelationApplicationSettingsHash
CREATE TRIGGER trg_log_relation_application_settings_hash_insert
AFTER INSERT ON relation_application_settings_hash FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10028, NEW.relation_endpoint_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for RelationApplicationSettingsHash
CREATE TRIGGER trg_log_relation_application_settings_hash_update
AFTER UPDATE ON relation_application_settings_hash FOR EACH ROW
WHEN 
	NEW.relation_endpoint_uuid != OLD.relation_endpoint_uuid OR
	NEW.sha256 != OLD.sha256 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10028, OLD.relation_endpoint_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for RelationApplicationSettingsHash
CREATE TRIGGER trg_log_relation_application_settings_hash_delete
AFTER DELETE ON relation_application_settings_hash FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10028, OLD.relation_endpoint_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for RelationUnitSettingsHash
INSERT INTO change_log_namespace VALUES (10029, 'relation_unit_settings_hash', 'RelationUnitSettingsHash changes based on relation_unit_uuid');

-- insert trigger for RelationUnitSettingsHash
CREATE TRIGGER trg_log_relation_unit_settings_hash_insert
AFTER INSERT ON relation_unit_settings_hash FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10029, NEW.relation_unit_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for RelationUnitSettingsHash
CREATE TRIGGER trg_log_relation_unit_settings_hash_update
AFTER UPDATE ON relation_unit_settings_hash FOR EACH ROW
WHEN 
	NEW.relation_unit_uuid != OLD.relation_unit_uuid OR
	NEW.sha256 != OLD.sha256 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10029, OLD.relation_unit_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for RelationUnitSettingsHash
CREATE TRIGGER trg_log_relation_unit_settings_hash_delete
AFTER DELETE ON relation_unit_settings_hash FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10029, OLD.relation_unit_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for Relation
INSERT INTO change_log_namespace VALUES (10030, 'relation', 'Relation changes based on uuid');

-- insert trigger for Relation
CREATE TRIGGER trg_log_relation_insert
AFTER INSERT ON relation FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10030, NEW.uuid, DATETIME('now', 'utc'));
END;

-- update trigger for Relation
CREATE TRIGGER trg_log_relation_update
AFTER UPDATE ON relation FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.life_id != OLD.life_id OR
	NEW.relation_id != OLD.relation_id OR
	(NEW.suspended != OLD.suspended OR (NEW.suspended IS NOT NULL AND OLD.suspended IS NULL) OR (NEW.suspended IS NULL AND OLD.suspended IS NOT NULL)) OR
	(NEW.suspended_reason != OLD.suspended_reason OR (NEW.suspended_reason IS NOT NULL AND OLD.suspended_reason IS NULL) OR (NEW.suspended_reason IS NULL AND OLD.suspended_reason IS NOT NULL)) OR
	NEW.scope_id != OLD.scope_id 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10030, OLD.uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for Relation
CREATE TRIGGER trg_log_relation_delete
AFTER DELETE ON relation FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10030, OLD.uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for RelationUnit
INSERT INTO change_log_namespace VALUES (10031, 'relation_unit', 'RelationUnit changes based on unit_uuid');

-- insert trigger for RelationUnit
CREATE TRIGGER trg_log_relation_unit_insert
AFTER INSERT ON relation_unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10031, NEW.unit_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for RelationUnit
CREATE TRIGGER trg_log_relation_unit_update
AFTER UPDATE ON relation_unit FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.relation_endpoint_uuid != OLD.relation_endpoint_uuid OR
	NEW.unit_uuid != OLD.unit_uuid 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10031, OLD.unit_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for RelationUnit
CREATE TRIGGER trg_log_relation_unit_delete
AFTER DELETE ON relation_unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10031, OLD.unit_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for IpAddress
INSERT INTO change_log_namespace VALUES (10032, 'ip_address', 'IpAddress changes based on net_node_uuid');

-- insert trigger for IpAddress
CREATE TRIGGER trg_log_ip_address_insert
AFTER INSERT ON ip_address FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10032, NEW.net_node_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for IpAddress
CREATE TRIGGER trg_log_ip_address_update
AFTER UPDATE ON ip_address FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.net_node_uuid != OLD.net_node_uuid OR
	NEW.device_uuid != OLD.device_uuid OR
	NEW.address_value != OLD.address_value OR
	(NEW.subnet_uuid != OLD.subnet_uuid OR (NEW.subnet_uuid IS NOT NULL AND OLD.subnet_uuid IS NULL) OR (NEW.subnet_uuid IS NULL AND OLD.subnet_uuid IS NOT NULL)) OR
	NEW.type_id != OLD.type_id OR
	NEW.config_type_id != OLD.config_type_id OR
	NEW.origin_id != OLD.origin_id OR
	NEW.scope_id != OLD.scope_id OR
	(NEW.is_secondary != OLD.is_secondary OR (NEW.is_secondary IS NOT NULL AND OLD.is_secondary IS NULL) OR (NEW.is_secondary IS NULL AND OLD.is_secondary IS NOT NULL)) OR
	(NEW.is_shadow != OLD.is_shadow OR (NEW.is_shadow IS NOT NULL AND OLD.is_shadow IS NULL) OR (NEW.is_shadow IS NULL AND OLD.is_shadow IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10032, OLD.net_node_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for IpAddress
CREATE TRIGGER trg_log_ip_address_delete
AFTER DELETE ON ip_address FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10032, OLD.net_node_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for ApplicationEndpoint
INSERT INTO change_log_namespace VALUES (10033, 'application_endpoint', 'ApplicationEndpoint changes based on application_uuid');

-- insert trigger for ApplicationEndpoint
CREATE TRIGGER trg_log_application_endpoint_insert
AFTER INSERT ON application_endpoint FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10033, NEW.application_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for ApplicationEndpoint
CREATE TRIGGER trg_log_application_endpoint_update
AFTER UPDATE ON application_endpoint FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.application_uuid != OLD.application_uuid OR
	(NEW.space_uuid != OLD.space_uuid OR (NEW.space_uuid IS NOT NULL AND OLD.space_uuid IS NULL) OR (NEW.space_uuid IS NULL AND OLD.space_uuid IS NOT NULL)) OR
	NEW.charm_relation_uuid != OLD.charm_relation_uuid 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10033, OLD.application_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for ApplicationEndpoint
CREATE TRIGGER trg_log_application_endpoint_delete
AFTER DELETE ON application_endpoint FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10033, OLD.application_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for OperationTaskLog
INSERT INTO change_log_namespace VALUES (10034, 'operation_task_log', 'OperationTaskLog changes based on task_uuid');

-- insert trigger for OperationTaskLog
CREATE TRIGGER trg_log_operation_task_log_insert
AFTER INSERT ON operation_task_log FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10034, NEW.task_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for OperationTaskLog
CREATE TRIGGER trg_log_operation_task_log_update
AFTER UPDATE ON operation_task_log FOR EACH ROW
WHEN 
	NEW.task_uuid != OLD.task_uuid OR
	NEW.content != OLD.content OR
	NEW.created_at != OLD.created_at 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10034, OLD.task_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for OperationTaskLog
CREATE TRIGGER trg_log_operation_task_log_delete
AFTER DELETE ON operation_task_log FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10034, OLD.task_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for ApplicationRemoteOfferer
INSERT INTO change_log_namespace VALUES (10035, 'application_remote_offerer', 'ApplicationRemoteOfferer changes based on uuid');

-- insert trigger for ApplicationRemoteOfferer
CREATE TRIGGER trg_log_application_remote_offerer_insert
AFTER INSERT ON application_remote_offerer FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10035, NEW.uuid, DATETIME('now', 'utc'));
END;

-- update trigger for ApplicationRemoteOfferer
CREATE TRIGGER trg_log_application_remote_offerer_update
AFTER UPDATE ON application_remote_offerer FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.life_id != OLD.life_id OR
	NEW.application_uuid != OLD.application_uuid OR
	NEW.offer_uuid != OLD.offer_uuid OR
	NEW.offer_url != OLD.offer_url OR
	(NEW.offerer_controller_uuid != OLD.offerer_controller_uuid OR (NEW.offerer_controller_uuid IS NOT NULL AND OLD.offerer_controller_uuid IS NULL) OR (NEW.offerer_controller_uuid IS NULL AND OLD.offerer_controller_uuid IS NOT NULL)) OR
	NEW.offerer_model_uuid != OLD.offerer_model_uuid OR
	NEW.macaroon != OLD.macaroon 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10035, OLD.uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for ApplicationRemoteOfferer
CREATE TRIGGER trg_log_application_remote_offerer_delete
AFTER DELETE ON application_remote_offerer FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10035, OLD.uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for ApplicationRemoteConsumer
INSERT INTO change_log_namespace VALUES (10036, 'application_remote_consumer', 'ApplicationRemoteConsumer changes based on offer_connection_uuid');

-- insert trigger for ApplicationRemoteConsumer
CREATE TRIGGER trg_log_application_remote_consumer_insert
AFTER INSERT ON application_remote_consumer FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10036, NEW.offer_connection_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for ApplicationRemoteConsumer
CREATE TRIGGER trg_log_application_remote_consumer_update
AFTER UPDATE ON application_remote_consumer FOR EACH ROW
WHEN 
	NEW.offer_connection_uuid != OLD.offer_connection_uuid OR
	NEW.offerer_application_uuid != OLD.offerer_application_uuid OR
	NEW.consumer_application_uuid != OLD.consumer_application_uuid OR
	NEW.consumer_model_uuid != OLD.consumer_model_uuid OR
	NEW.life_id != OLD.life_id 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10036, OLD.offer_connection_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for ApplicationRemoteConsumer
CREATE TRIGGER trg_log_application_remote_consumer_delete
AFTER DELETE ON application_remote_consumer FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10036, OLD.offer_connection_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for Offer
INSERT INTO change_log_namespace VALUES (10037, 'offer', 'Offer changes based on uuid');

-- insert trigger for Offer
CREATE TRIGGER trg_log_offer_insert
AFTER INSERT ON offer FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10037, NEW.uuid, DATETIME('now', 'utc'));
END;

-- update trigger for Offer
CREATE TRIGGER trg_log_offer_update
AFTER UPDATE ON offer FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.name != OLD.name 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10037, OLD.uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for Offer
CREATE TRIGGER trg_log_offer_delete
AFTER DELETE ON offer FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10037, OLD.uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for ApplicationStatus
INSERT INTO change_log_namespace VALUES (10038, 'application_status', 'ApplicationStatus changes based on application_uuid');

-- insert trigger for ApplicationStatus
CREATE TRIGGER trg_log_application_status_insert
AFTER INSERT ON application_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10038, NEW.application_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for ApplicationStatus
CREATE TRIGGER trg_log_application_status_update
AFTER UPDATE ON application_status FOR EACH ROW
WHEN 
	NEW.application_uuid != OLD.application_uuid OR
	NEW.status_id != OLD.status_id OR
	(NEW.message != OLD.message OR (NEW.message IS NOT NULL AND OLD.message IS NULL) OR (NEW.message IS NULL AND OLD.message IS NOT NULL)) OR
	(NEW.data != OLD.data OR (NEW.data IS NOT NULL AND OLD.data IS NULL) OR (NEW.data IS NULL AND OLD.data IS NOT NULL)) OR
	(NEW.updated_at != OLD.updated_at OR (NEW.updated_at IS NOT NULL AND OLD.updated_at IS NULL) OR (NEW.updated_at IS NULL AND OLD.updated_at IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10038, OLD.application_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for ApplicationStatus
CREATE TRIGGER trg_log_application_status_delete
AFTER DELETE ON application_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10038, OLD.application_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for RelationNetworkIngress
INSERT INTO change_log_namespace VALUES (10039, 'relation_network_ingress', 'RelationNetworkIngress changes based on relation_uuid');

-- insert trigger for RelationNetworkIngress
CREATE TRIGGER trg_log_relation_network_ingress_insert
AFTER INSERT ON relation_network_ingress FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10039, NEW.relation_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for RelationNetworkIngress
CREATE TRIGGER trg_log_relation_network_ingress_update
AFTER UPDATE ON relation_network_ingress FOR EACH ROW
WHEN 
	NEW.relation_uuid != OLD.relation_uuid OR
	NEW.cidr != OLD.cidr 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10039, OLD.relation_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for RelationNetworkIngress
CREATE TRIGGER trg_log_relation_network_ingress_delete
AFTER DELETE ON relation_network_ingress FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10039, OLD.relation_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for RelationNetworkEgress
INSERT INTO change_log_namespace VALUES (10040, 'relation_network_egress', 'RelationNetworkEgress changes based on relation_uuid');

-- insert trigger for RelationNetworkEgress
CREATE TRIGGER trg_log_relation_network_egress_insert
AFTER INSERT ON relation_network_egress FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10040, NEW.relation_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for RelationNetworkEgress
CREATE TRIGGER trg_log_relation_network_egress_update
AFTER UPDATE ON relation_network_egress FOR EACH ROW
WHEN 
	NEW.relation_uuid != OLD.relation_uuid OR
	NEW.cidr != OLD.cidr 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10040, OLD.relation_uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for RelationNetworkEgress
CREATE TRIGGER trg_log_relation_network_egress_delete
AFTER DELETE ON relation_network_egress FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10040, OLD.relation_uuid, DATETIME('now', 'utc'));
END;
CREATE TRIGGER trg_model_immutable_update
    BEFORE UPDATE ON model
    FOR EACH ROW

    BEGIN
        SELECT RAISE(FAIL, 'model table is immutable, only insertions are allowed');
    END;

CREATE TRIGGER trg_model_immutable_delete
    BEFORE DELETE ON model
    FOR EACH ROW

    BEGIN
        SELECT RAISE(FAIL, 'model table is immutable, only insertions are allowed');
    END;
CREATE TRIGGER trg_charm_action_immutable_update
    BEFORE UPDATE ON charm_action
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'charm_action table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_charm_config_immutable_update
    BEFORE UPDATE ON charm_config
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'charm_config table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_charm_container_mount_immutable_update
    BEFORE UPDATE ON charm_container_mount
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'charm_container_mount table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_charm_container_immutable_update
    BEFORE UPDATE ON charm_container
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'charm_container table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_charm_device_immutable_update
    BEFORE UPDATE ON charm_device
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'charm_device table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_charm_extra_binding_immutable_update
    BEFORE UPDATE ON charm_extra_binding
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'charm_extra_binding table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_charm_hash_immutable_update
    BEFORE UPDATE ON charm_hash
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'charm_hash table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_charm_manifest_base_immutable_update
    BEFORE UPDATE ON charm_manifest_base
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'charm_manifest base table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_charm_metadata_immutable_update
    BEFORE UPDATE ON charm_metadata
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'charm_metadata table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_charm_relation_immutable_update
    BEFORE UPDATE ON charm_relation
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'charm_relation table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_charm_resource_immutable_update
    BEFORE UPDATE ON charm_resource
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'charm_resource table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_charm_storage_immutable_update
    BEFORE UPDATE ON charm_storage
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'charm_storage table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_charm_term_immutable_update
    BEFORE UPDATE ON charm_term
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'charm_term table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_application_controller_immutable_update
    BEFORE UPDATE ON application_controller
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'application_controller table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_offer_endpoint_immutable_update
    BEFORE UPDATE ON offer_endpoint
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'offer_endpoint table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_relation_network_ingress_immutable_update
    BEFORE UPDATE ON relation_network_ingress
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'relation_network_ingress table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_relation_network_egress_immutable_update
    BEFORE UPDATE ON relation_network_egress
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'relation_network_egress table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_secret_permission_guard_update
    BEFORE UPDATE ON secret_permission
    FOR EACH ROW
        WHEN OLD.subject_type_id <> NEW.subject_type_id OR OLD.scope_uuid <> NEW.scope_uuid OR OLD.scope_type_id <> NEW.scope_type_id
    BEGIN
        SELECT RAISE(FAIL, 'secret permission subjects and scopes must be identical');
    END;
CREATE TRIGGER trg_sequence_guard_update
    BEFORE UPDATE ON sequence
    FOR EACH ROW
        WHEN OLD.namespace = NEW.namespace AND NEW.value <= OLD.value
    BEGIN
        SELECT RAISE(FAIL, 'sequence number must monotonically increase');
    END;
CREATE TRIGGER trg_storage_pool_guard_update
    BEFORE UPDATE ON storage_pool
    FOR EACH ROW
        WHEN OLD.origin_id <> NEW.origin_id
    BEGIN
        SELECT RAISE(FAIL, 'storage pool origin cannot be changed');
    END;
CREATE TRIGGER trg_application_guard_life
    BEFORE UPDATE ON application
    FOR EACH ROW
    WHEN NEW.life_id < OLD.life_id
    BEGIN
        SELECT RAISE(FAIL, 'Cannot transition life for application backwards');
    END;
CREATE TRIGGER trg_unit_guard_life
    BEFORE UPDATE ON unit
    FOR EACH ROW
    WHEN NEW.life_id < OLD.life_id
    BEGIN
        SELECT RAISE(FAIL, 'Cannot transition life for unit backwards');
    END;
CREATE TRIGGER trg_machine_guard_life
    BEFORE UPDATE ON machine
    FOR EACH ROW
    WHEN NEW.life_id < OLD.life_id
    BEGIN
        SELECT RAISE(FAIL, 'Cannot transition life for machine backwards');
    END;
CREATE TRIGGER trg_machine_cloud_instance_guard_life
    BEFORE UPDATE ON machine_cloud_instance
    FOR EACH ROW
    WHEN NEW.life_id < OLD.life_id
    BEGIN
        SELECT RAISE(FAIL, 'Cannot transition life for machine_cloud_instance backwards');
    END;
CREATE TRIGGER trg_storage_instance_guard_life
    BEFORE UPDATE ON storage_instance
    FOR EACH ROW
    WHEN NEW.life_id < OLD.life_id
    BEGIN
        SELECT RAISE(FAIL, 'Cannot transition life for storage_instance backwards');
    END;
CREATE TRIGGER trg_storage_attachment_guard_life
    BEFORE UPDATE ON storage_attachment
    FOR EACH ROW
    WHEN NEW.life_id < OLD.life_id
    BEGIN
        SELECT RAISE(FAIL, 'Cannot transition life for storage_attachment backwards');
    END;
CREATE TRIGGER trg_storage_volume_guard_life
    BEFORE UPDATE ON storage_volume
    FOR EACH ROW
    WHEN NEW.life_id < OLD.life_id
    BEGIN
        SELECT RAISE(FAIL, 'Cannot transition life for storage_volume backwards');
    END;
CREATE TRIGGER trg_storage_volume_attachment_guard_life
    BEFORE UPDATE ON storage_volume_attachment
    FOR EACH ROW
    WHEN NEW.life_id < OLD.life_id
    BEGIN
        SELECT RAISE(FAIL, 'Cannot transition life for storage_volume_attachment backwards');
    END;
CREATE TRIGGER trg_storage_filesystem_guard_life
    BEFORE UPDATE ON storage_filesystem
    FOR EACH ROW
    WHEN NEW.life_id < OLD.life_id
    BEGIN
        SELECT RAISE(FAIL, 'Cannot transition life for storage_filesystem backwards');
    END;
CREATE TRIGGER trg_storage_filesystem_attachment_guard_life
    BEFORE UPDATE ON storage_filesystem_attachment
    FOR EACH ROW
    WHEN NEW.life_id < OLD.life_id
    BEGIN
        SELECT RAISE(FAIL, 'Cannot transition life for storage_filesystem_attachment backwards');
    END;
CREATE TRIGGER trg_storage_volume_attachment_plan_guard_life
    BEFORE UPDATE ON storage_volume_attachment_plan
    FOR EACH ROW
    WHEN NEW.life_id < OLD.life_id
    BEGIN
        SELECT RAISE(FAIL, 'Cannot transition life for storage_volume_attachment_plan backwards');
    END;
INSERT INTO change_log_namespace VALUES (0, 'custom_unit_name_lifecycle', 'Changes to the lifecycle of unit (name) entities');

CREATE TRIGGER trg_log_custom_unit_name_lifecycle_insert
AFTER INSERT ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 0, NEW.name, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_unit_name_lifecycle_update
AFTER UPDATE ON unit FOR EACH ROW
WHEN
	NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 0, OLD.name, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_unit_name_lifecycle_delete
AFTER DELETE ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 0, OLD.name, DATETIME('now', 'utc'));
END;
INSERT INTO change_log_namespace VALUES (1, 'custom_machine_name_lifecycle', 'Changes to the lifecycle of machine (name) entities');

CREATE TRIGGER trg_log_custom_machine_name_lifecycle_insert
AFTER INSERT ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 1, NEW.name, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_machine_name_lifecycle_update
AFTER UPDATE ON machine FOR EACH ROW
WHEN
	NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 1, OLD.name, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_machine_name_lifecycle_delete
AFTER DELETE ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 1, OLD.name, DATETIME('now', 'utc'));
END;
INSERT INTO change_log_namespace VALUES (3, 'custom_machine_uuid_lifecycle_with_dependants', 'Changes to the lifecycle of machines, machine units and storage entities for the machine');

-- machine life triggers
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_insert
AFTER INSERT ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES(1, 3, NEW.uuid, DATETIME('now'));
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_update
AFTER UPDATE ON machine FOR EACH ROW
WHEN
    NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES(2, 3, OLD.uuid, DATETIME('now'));
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_delete
AFTER DELETE ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES(4, 3, OLD.uuid, DATETIME('now'));
END;

-- machine parent (child) life triggers
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_parent_insert
AFTER INSERT ON machine_parent FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES(1, 3, NEW.parent_uuid, DATETIME('now'));
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_parent_update
AFTER UPDATE ON machine FOR EACH ROW
WHEN
    NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, 3, mp.parent_uuid, DATETIME('now')
    FROM machine_parent AS mp
    WHERE mp.machine_uuid = OLD.uuid;
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_parent_delete
AFTER DELETE ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, 3, mp.parent_uuid, DATETIME('now')
    FROM machine_parent AS mp
    WHERE mp.machine_uuid = OLD.uuid;
END;

-- unit on machine life triggers
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_unit_insert
AFTER INSERT ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, 3, m.uuid, DATETIME('now', 'utc')
    FROM machine AS m
    WHERE m.net_node_uuid = NEW.net_node_uuid;
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_unit_update
AFTER UPDATE ON unit FOR EACH ROW
WHEN
    NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, 3, m.uuid, DATETIME('now', 'utc')
    FROM machine AS m
    WHERE m.net_node_uuid = OLD.net_node_uuid;
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_unit_delete
AFTER DELETE ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, 3, m.uuid, DATETIME('now', 'utc')
    FROM machine m
    WHERE m.net_node_uuid = OLD.net_node_uuid;
END;

-- machine_filesystem delete trigger
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_filesystem_delete
AFTER DELETE ON machine_filesystem FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES(4, 3, OLD.machine_uuid, DATETIME('now'));
END;

-- machine_volume delete trigger
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_volume_delete
AFTER DELETE ON machine_volume FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES(4, 3, OLD.machine_uuid, DATETIME('now'));
END;

-- storage_filesystem_attachment on machine net node delete trigger
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_storage_filesystem_attachment_delete
AFTER DELETE ON storage_filesystem_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, 3, m.uuid, DATETIME('now')
    FROM machine AS m
    WHERE m.net_node_uuid = OLD.net_node_uuid;
END;

-- storage_volume_attachment on machine net node delete trigger
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_storage_volume_attachment_delete
AFTER DELETE ON storage_volume_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, 3, m.uuid, DATETIME('now')
    FROM machine AS m
    WHERE m.net_node_uuid = OLD.net_node_uuid;
END;

-- storage_volume_attachment_plan on machine net node delete trigger
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_storage_volume_attachment_plan_delete
AFTER DELETE ON storage_volume_attachment_plan FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, 3, m.uuid, DATETIME('now')
    FROM machine AS m
    WHERE m.net_node_uuid = OLD.net_node_uuid;
END;
INSERT INTO change_log_namespace VALUES (17, 'custom_application_uuid_lifecycle', 'Changes to the lifecycle of application (uuid) entities');

CREATE TRIGGER trg_log_custom_application_uuid_lifecycle_insert
AFTER INSERT ON application FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 17, NEW.uuid, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_application_uuid_lifecycle_update
AFTER UPDATE ON application FOR EACH ROW
WHEN
	NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 17, OLD.uuid, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_application_uuid_lifecycle_delete
AFTER DELETE ON application FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 17, OLD.uuid, DATETIME('now', 'utc'));
END;
INSERT INTO change_log_namespace VALUES (16, 'custom_machine_uuid_lifecycle', 'Changes to the lifecycle of machine (uuid) entities');

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_insert
AFTER INSERT ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 16, NEW.uuid, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_update
AFTER UPDATE ON machine FOR EACH ROW
WHEN
	NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 16, OLD.uuid, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_delete
AFTER DELETE ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 16, OLD.uuid, DATETIME('now', 'utc'));
END;
INSERT INTO change_log_namespace VALUES (15, 'custom_unit_uuid_lifecycle', 'Changes to the lifecycle of unit (uuid) entities');

CREATE TRIGGER trg_log_custom_unit_uuid_lifecycle_insert
AFTER INSERT ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 15, NEW.uuid, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_unit_uuid_lifecycle_update
AFTER UPDATE ON unit FOR EACH ROW
WHEN
	NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 15, OLD.uuid, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_unit_uuid_lifecycle_delete
AFTER DELETE ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 15, OLD.uuid, DATETIME('now', 'utc'));
END;
INSERT INTO change_log_namespace VALUES (18, 'custom_relation_uuid_lifecycle', 'Changes to the lifecycle of relation (uuid) entities');

CREATE TRIGGER trg_log_custom_relation_uuid_lifecycle_insert
AFTER INSERT ON relation FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 18, NEW.uuid, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_relation_uuid_lifecycle_update
AFTER UPDATE ON relation FOR EACH ROW
WHEN
	NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 18, OLD.uuid, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_relation_uuid_lifecycle_delete
AFTER DELETE ON relation FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 18, OLD.uuid, DATETIME('now', 'utc'));
END;
INSERT INTO change_log_namespace VALUES (19, 'custom_model_life_model_uuid_lifecycle', 'Changes to the lifecycle of model_life (model_uuid) entities');

CREATE TRIGGER trg_log_custom_model_life_model_uuid_lifecycle_insert
AFTER INSERT ON model_life FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 19, NEW.model_uuid, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_model_life_model_uuid_lifecycle_update
AFTER UPDATE ON model_life FOR EACH ROW
WHEN
	NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 19, OLD.model_uuid, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_model_life_model_uuid_lifecycle_delete
AFTER DELETE ON model_life FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 19, OLD.model_uuid, DATETIME('now', 'utc'));
END;
INSERT INTO change_log_namespace VALUES (21, 'custom_storage_attachment_unit_uuid_lifecycle', 'Changes to the lifecycle of storage_attachment (unit_uuid) entities');

CREATE TRIGGER trg_log_custom_storage_attachment_unit_uuid_lifecycle_insert
AFTER INSERT ON storage_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 21, NEW.unit_uuid, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_storage_attachment_unit_uuid_lifecycle_update
AFTER UPDATE ON storage_attachment FOR EACH ROW
WHEN
	NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 21, OLD.unit_uuid, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_custom_storage_attachment_unit_uuid_lifecycle_delete
AFTER DELETE ON storage_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 21, OLD.unit_uuid, DATETIME('now', 'utc'));
END;
CREATE TRIGGER trg_operation_parameter_immutable_update
    BEFORE UPDATE ON operation_parameter
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'operation_parameter table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_operation_machine_task_immutable_update
    BEFORE UPDATE ON operation_machine_task
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'operation_machine_task table is unmodifiable, only insertions and deletions are allowed');
    END;

CREATE TRIGGER trg_operation_unit_task_immutable_update
    BEFORE UPDATE ON operation_unit_task
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, 'operation_unit_task table is unmodifiable, only insertions and deletions are allowed');
    END;


INSERT INTO change_log_namespace VALUES (2, 'custom_machine_lifecycle_start_time', 'Machine life or agent start time changes');

CREATE TRIGGER trg_log_machine_insert_life_start_time
AFTER INSERT ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 2, NEW.name, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_machine_update_life_start_time
AFTER UPDATE ON machine FOR EACH ROW
WHEN 
	NEW.life_id != OLD.life_id OR
	(NEW.agent_started_at != OLD.agent_started_at OR (NEW.agent_started_at IS NOT NULL AND OLD.agent_started_at IS NULL) OR (NEW.agent_started_at IS NULL AND OLD.agent_started_at IS NOT NULL))
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 2, OLD.name, DATETIME('now', 'utc'));
END;

CREATE TRIGGER trg_log_machine_delete_life_start_time
AFTER DELETE ON machine FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 2, OLD.name, DATETIME('now', 'utc'));
END;


INSERT INTO change_log_namespace VALUES (10027, 'agent_version', 'Agent version changes based on target version');

CREATE TRIGGER trg_log_agent_version_update
AFTER UPDATE ON agent_version FOR EACH ROW
WHEN
	NEW.target_version != OLD.target_version
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10027, NEW.target_version, DATETIME('now', 'utc'));
END;


CREATE TRIGGER trg_ensure_single_app_per_offer
BEFORE INSERT ON offer_endpoint
FOR EACH ROW
BEGIN
    -- Check if the new endpoint_uuid has a different application_uuid than
    -- existing ones for the same offer_uuid
    SELECT RAISE(ABORT, 'All endpoints for an offer must belong to the same application')
    WHERE EXISTS (
        SELECT 1
        FROM  offer_endpoint oe
        JOIN  application_endpoint ae_new ON ae_new.uuid = NEW.endpoint_uuid
        JOIN  application_endpoint ae_existing ON ae_existing.uuid = oe.endpoint_uuid
        WHERE oe.offer_uuid = NEW.offer_uuid
        AND   ae_new.application_uuid <> ae_existing.application_uuid
    );
END;	

CREATE TRIGGER trg_insert_machine_task_if_not_unit_task
BEFORE INSERT ON operation_machine_task
WHEN EXISTS (
    SELECT 1 FROM operation_unit_task WHERE task_uuid = NEW.task_uuid
)
BEGIN
    SELECT RAISE(ABORT, 'Task is already linked to a unit, cannot be added for a machine');
END;

CREATE TRIGGER trg_insert_unit_task_if_not_machine_task
BEFORE INSERT ON operation_unit_task
WHEN EXISTS (
    SELECT 1 FROM operation_machine_task WHERE task_uuid = NEW.task_uuid
)
BEGIN
    SELECT RAISE(ABORT, 'Task is already linked to a machine, cannot be added for a unit');
END;
	

-- insert namespace for storage entity change.
INSERT INTO change_log_namespace
VALUES (4,
        'storage_filesystem_life_machine_provisioning',
		'lifecycle changes for storage filesystem, that are machined provisioned');

-- insert trigger for storage entity attachment table. This assumes the storage
-- entity has a child table with an _attachment suffix.
CREATE TRIGGER trg_log_storage_filesystem_insert_life_machine_provisioning_on_attachment
AFTER INSERT ON storage_filesystem_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1,
           4,
           NEW.net_node_uuid,
           DATETIME('now', 'utc')
    FROM   storage_filesystem s
    WHERE  1 == (SELECT COUNT(*)
                 FROM   storage_filesystem_attachment
                 WHERE  storage_filesystem_uuid = NEW.storage_filesystem_uuid)
    AND    s.uuid = NEW.storage_filesystem_uuid
    AND    s.provision_scope_id = 1;
END;

-- update trigger for storage entity.
CREATE TRIGGER trg_log_storage_filesystem_update_life_machine_provisioning
AFTER UPDATE ON storage_filesystem
FOR EACH ROW
	WHEN NEW.provision_scope_id = 1
	AND  NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT DISTINCT 2,
           			4,
           			a.net_node_uuid,
           			DATETIME('now', 'utc')
    FROM  storage_filesystem_attachment a
    WHERE storage_filesystem_uuid = NEW.uuid;
END;

-- delete trigger for storage entity. Note the use of the OLD value in the
-- trigger.
CREATE TRIGGER trg_log_storage_filesystem_delete_life_machine_provisioning_last_attachment
AFTER DELETE ON storage_filesystem_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT DISTINCT 4,
           			4,
           			OLD.net_node_uuid,
           			DATETIME('now', 'utc')
    FROM   storage_filesystem s
    WHERE  0 == (SELECT COUNT(*)
                 FROM   storage_filesystem_attachment
                 WHERE  storage_filesystem_uuid = OLD.storage_filesystem_uuid)
    AND    s.uuid = OLD.storage_filesystem_uuid
    AND    s.provision_scope_id = 1;
END;


-- insert namespace for storage entity
INSERT INTO change_log_namespace
VALUES (5,
		'storage_filesystem_life_model_provisioning',
		'lifecycle changes for storage filesystem, that are model provisioned');

-- insert trigger for storage entity.
CREATE TRIGGER trg_log_storage_filesystem_insert_life_model_provisioning
AFTER INSERT ON storage_filesystem
FOR EACH ROW
	WHEN NEW.provision_scope_id = 0
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 5, NEW.filesystem_id, DATETIME('now', 'utc'));
END;

-- update trigger for storage entity.
CREATE TRIGGER trg_log_storage_filesystem_update_life_model_provisioning
AFTER UPDATE ON storage_filesystem
FOR EACH ROW
	WHEN NEW.provision_scope_id = 0
	AND NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 5, NEW.filesystem_id, DATETIME('now', 'utc'));
END;

-- delete trigger for storage entity. Note the use of the OLD value in the
-- trigger.
CREATE TRIGGER trg_log_storage_filesystem_delete_life_model_provisioning
AFTER DELETE ON storage_filesystem
FOR EACH ROW
	WHEN OLD.provision_scope_id = 0
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 5, OLD.filesystem_id, DATETIME('now', 'utc'));
END;


-- insert namespace for storage filesystem
INSERT INTO change_log_namespace
VALUES (6,
		'custom_filesystem_provider_id_model_provisioning',
		'changes for filesystem provider IDs that are model provisioned');

-- update trigger for storage filesystem.
CREATE TRIGGER trg_log_custom_filesystem_provider_id_model_provisioning
AFTER UPDATE ON storage_filesystem
FOR EACH ROW
    WHEN NEW.provision_scope_id = 0 AND
         (NEW.provider_id != OLD.provider_id OR (NEW.provider_id IS NOT NULL AND OLD.provider_id IS NULL) OR (NEW.provider_id IS NULL AND OLD.provider_id IS NOT NULL))
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 6, NEW.filesystem_id, DATETIME('now', 'utc'));
END;


-- insert namespace for storage attachment entity change.
INSERT INTO change_log_namespace
VALUES (7,
	'storage_filesystem_attachment_life_machine_provisioning',
	'lifecycle changes for storage filesystem_attachment, that are machined provisioned');

-- insert trigger for storage attachment entity.
CREATE TRIGGER trg_log_storage_filesystem_attachment_insert_life_machine_provisioning
AFTER INSERT ON storage_filesystem_attachment
FOR EACH ROW
	WHEN NEW.provision_scope_id = 1
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 7, NEW.net_node_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for storage attachment entity.
CREATE TRIGGER trg_log_storage_filesystem_attachment_update_life_machine_provisioning
AFTER UPDATE ON storage_filesystem_attachment
FOR EACH ROW
	WHEN NEW.provision_scope_id = 1
	AND NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 7, NEW.net_node_uuid, DATETIME('now', 'utc'));
END;

-- delete trigger for storage attachment entity. Note the use of the OLD value
-- in the trigger.
CREATE TRIGGER trg_log_storage_filesystem_attachment_delete_life_machine_provisioning
AFTER DELETE ON storage_filesystem_attachment
FOR EACH ROW
	WHEN OLD.provision_scope_id = 1
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 7, OLD.net_node_uuid, DATETIME('now', 'utc'));
END;


-- insert namespace for storage entity
INSERT INTO change_log_namespace
VALUES (8,
		'storage_filesystem_attachment_life_model_provisioning',
		'lifecycle changes for storage filesystem_attachment, that are model provisioned');

-- insert trigger for storage entity.
CREATE TRIGGER trg_log_storage_filesystem_attachment_insert_life_model_provisioning
AFTER INSERT ON storage_filesystem_attachment
FOR EACH ROW
	WHEN NEW.provision_scope_id = 0
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 8, NEW.uuid, DATETIME('now', 'utc'));
END;

-- update trigger for storage entity.
CREATE TRIGGER trg_log_storage_filesystem_attachment_update_life_model_provisioning
AFTER UPDATE ON storage_filesystem_attachment
FOR EACH ROW
	WHEN NEW.provision_scope_id = 0
	AND NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 8, NEW.uuid, DATETIME('now', 'utc'));
END;

-- delete trigger for storage entity. Note the use of the OLD value in the
-- trigger.
CREATE TRIGGER trg_log_storage_filesystem_attachment_delete_life_model_provisioning
AFTER DELETE ON storage_filesystem_attachment
FOR EACH ROW
	WHEN OLD.provision_scope_id = 0
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 8, OLD.uuid, DATETIME('now', 'utc'));
END;


-- insert namespace for storage filesystem attachment
INSERT INTO change_log_namespace
VALUES (9,
		'custom_filesystem_attachment_provider_id_model_provisioning',
		'changes for filesystem attachment provider IDs that are model provisioned');

-- update trigger for storage filesystem attachment.
CREATE TRIGGER trg_log_custom_filesystem_attachment_provider_id_model_provisioning
AFTER UPDATE ON storage_filesystem_attachment
FOR EACH ROW
    WHEN NEW.provision_scope_id = 0 AND
         (NEW.provider_id != OLD.provider_id OR (NEW.provider_id IS NOT NULL AND OLD.provider_id IS NULL) OR (NEW.provider_id IS NULL AND OLD.provider_id IS NOT NULL))
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 9, NEW.uuid, DATETIME('now', 'utc'));
END;


-- insert namespace for storage entity change.
INSERT INTO change_log_namespace
VALUES (10,
        'storage_volume_life_machine_provisioning',
		'lifecycle changes for storage volume, that are machined provisioned');

-- insert trigger for storage entity attachment table. This assumes the storage
-- entity has a child table with an _attachment suffix.
CREATE TRIGGER trg_log_storage_volume_insert_life_machine_provisioning_on_attachment
AFTER INSERT ON storage_volume_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1,
           10,
           NEW.net_node_uuid,
           DATETIME('now', 'utc')
    FROM   storage_volume s
    WHERE  1 == (SELECT COUNT(*)
                 FROM   storage_volume_attachment
                 WHERE  storage_volume_uuid = NEW.storage_volume_uuid)
    AND    s.uuid = NEW.storage_volume_uuid
    AND    s.provision_scope_id = 1;
END;

-- update trigger for storage entity.
CREATE TRIGGER trg_log_storage_volume_update_life_machine_provisioning
AFTER UPDATE ON storage_volume
FOR EACH ROW
	WHEN NEW.provision_scope_id = 1
	AND  NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT DISTINCT 2,
           			10,
           			a.net_node_uuid,
           			DATETIME('now', 'utc')
    FROM  storage_volume_attachment a
    WHERE storage_volume_uuid = NEW.uuid;
END;

-- delete trigger for storage entity. Note the use of the OLD value in the
-- trigger.
CREATE TRIGGER trg_log_storage_volume_delete_life_machine_provisioning_last_attachment
AFTER DELETE ON storage_volume_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT DISTINCT 4,
           			10,
           			OLD.net_node_uuid,
           			DATETIME('now', 'utc')
    FROM   storage_volume s
    WHERE  0 == (SELECT COUNT(*)
                 FROM   storage_volume_attachment
                 WHERE  storage_volume_uuid = OLD.storage_volume_uuid)
    AND    s.uuid = OLD.storage_volume_uuid
    AND    s.provision_scope_id = 1;
END;


-- insert namespace for storage entity
INSERT INTO change_log_namespace
VALUES (11,
		'storage_volume_life_model_provisioning',
		'lifecycle changes for storage volume, that are model provisioned');

-- insert trigger for storage entity.
CREATE TRIGGER trg_log_storage_volume_insert_life_model_provisioning
AFTER INSERT ON storage_volume
FOR EACH ROW
	WHEN NEW.provision_scope_id = 0
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 11, NEW.volume_id, DATETIME('now', 'utc'));
END;

-- update trigger for storage entity.
CREATE TRIGGER trg_log_storage_volume_update_life_model_provisioning
AFTER UPDATE ON storage_volume
FOR EACH ROW
	WHEN NEW.provision_scope_id = 0
	AND NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 11, NEW.volume_id, DATETIME('now', 'utc'));
END;

-- delete trigger for storage entity. Note the use of the OLD value in the
-- trigger.
CREATE TRIGGER trg_log_storage_volume_delete_life_model_provisioning
AFTER DELETE ON storage_volume
FOR EACH ROW
	WHEN OLD.provision_scope_id = 0
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 11, OLD.volume_id, DATETIME('now', 'utc'));
END;


-- insert namespace for storage attachment entity change.
INSERT INTO change_log_namespace
VALUES (12,
	'storage_volume_attachment_life_machine_provisioning',
	'lifecycle changes for storage volume_attachment, that are machined provisioned');

-- insert trigger for storage attachment entity.
CREATE TRIGGER trg_log_storage_volume_attachment_insert_life_machine_provisioning
AFTER INSERT ON storage_volume_attachment
FOR EACH ROW
	WHEN NEW.provision_scope_id = 1
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 12, NEW.net_node_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for storage attachment entity.
CREATE TRIGGER trg_log_storage_volume_attachment_update_life_machine_provisioning
AFTER UPDATE ON storage_volume_attachment
FOR EACH ROW
	WHEN NEW.provision_scope_id = 1
	AND NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 12, NEW.net_node_uuid, DATETIME('now', 'utc'));
END;

-- delete trigger for storage attachment entity. Note the use of the OLD value
-- in the trigger.
CREATE TRIGGER trg_log_storage_volume_attachment_delete_life_machine_provisioning
AFTER DELETE ON storage_volume_attachment
FOR EACH ROW
	WHEN OLD.provision_scope_id = 1
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 12, OLD.net_node_uuid, DATETIME('now', 'utc'));
END;


-- insert namespace for storage entity
INSERT INTO change_log_namespace
VALUES (13,
		'storage_volume_attachment_life_model_provisioning',
		'lifecycle changes for storage volume_attachment, that are model provisioned');

-- insert trigger for storage entity.
CREATE TRIGGER trg_log_storage_volume_attachment_insert_life_model_provisioning
AFTER INSERT ON storage_volume_attachment
FOR EACH ROW
	WHEN NEW.provision_scope_id = 0
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 13, NEW.uuid, DATETIME('now', 'utc'));
END;

-- update trigger for storage entity.
CREATE TRIGGER trg_log_storage_volume_attachment_update_life_model_provisioning
AFTER UPDATE ON storage_volume_attachment
FOR EACH ROW
	WHEN NEW.provision_scope_id = 0
	AND NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 13, NEW.uuid, DATETIME('now', 'utc'));
END;

-- delete trigger for storage entity. Note the use of the OLD value in the
-- trigger.
CREATE TRIGGER trg_log_storage_volume_attachment_delete_life_model_provisioning
AFTER DELETE ON storage_volume_attachment
FOR EACH ROW
	WHEN OLD.provision_scope_id = 0
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 13, OLD.uuid, DATETIME('now', 'utc'));
END;


-- insert namespace for storage attachment entity change.
INSERT INTO change_log_namespace
VALUES (14,
	'storage_volume_attachment_plan_life_machine_provisioning',
	'lifecycle changes for storage volume_attachment_plan, that are machined provisioned');

-- insert trigger for storage attachment entity.
CREATE TRIGGER trg_log_storage_volume_attachment_plan_insert_life_machine_provisioning
AFTER INSERT ON storage_volume_attachment_plan
FOR EACH ROW
	WHEN NEW.provision_scope_id = 1
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 14, NEW.net_node_uuid, DATETIME('now', 'utc'));
END;

-- update trigger for storage attachment entity.
CREATE TRIGGER trg_log_storage_volume_attachment_plan_update_life_machine_provisioning
AFTER UPDATE ON storage_volume_attachment_plan
FOR EACH ROW
	WHEN NEW.provision_scope_id = 1
	AND NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 14, NEW.net_node_uuid, DATETIME('now', 'utc'));
END;

-- delete trigger for storage attachment entity. Note the use of the OLD value
-- in the trigger.
CREATE TRIGGER trg_log_storage_volume_attachment_plan_delete_life_machine_provisioning
AFTER DELETE ON storage_volume_attachment_plan
FOR EACH ROW
	WHEN OLD.provision_scope_id = 1
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 14, OLD.net_node_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for storage attachment.
INSERT INTO change_log_namespace
VALUES (20,
		'custom_storage_attachment_entities_storage_attachment_uuid',
		'Changes for storage provisioning process');

-- storage_attachment for life update.
CREATE TRIGGER trg_log_custom_storage_attachment_lifecycle_update
AFTER UPDATE ON storage_attachment FOR EACH ROW
WHEN
	NEW.life_id != OLD.life_id
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 20, NEW.uuid, DATETIME('now', 'utc'));
END;

-- storage_attachment for delete.
CREATE TRIGGER trg_log_custom_storage_attachment_lifecycle_delete
AFTER DELETE ON storage_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 20, OLD.uuid, DATETIME('now', 'utc'));
END;

-- storage_instance_filesystem for insert.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_instance_filesystem_insert
AFTER INSERT ON storage_instance_filesystem FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, 20, sa.uuid, DATETIME('now', 'utc')
    FROM storage_attachment sa
    WHERE sa.storage_instance_uuid = NEW.storage_instance_uuid;
END;

-- storage_instance_volume for insert.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_instance_volume_insert
AFTER INSERT ON storage_instance_volume FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, 20, sa.uuid, DATETIME('now', 'utc')
    FROM storage_attachment sa
    WHERE sa.storage_instance_uuid = NEW.storage_instance_uuid;
END;

-- storage_volume_attachment for insert.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_volume_attachment_insert
AFTER INSERT ON storage_volume_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, 20, sa.uuid, DATETIME('now', 'utc')
    FROM storage_instance_volume siv
    JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
    WHERE siv.storage_volume_uuid = NEW.storage_volume_uuid;
END;

-- storage_volume_attachment for update.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_volume_attachment_update
AFTER UPDATE ON storage_volume_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, 20, sa.uuid, DATETIME('now', 'utc')
    FROM storage_instance_volume siv
    JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
    WHERE siv.storage_volume_uuid = NEW.storage_volume_uuid;
END;

-- storage_volume_attachment for delete.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_volume_attachment_delete
AFTER DELETE ON storage_volume_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, 20, sa.uuid, DATETIME('now', 'utc')
    FROM storage_instance_volume siv
    JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
    WHERE siv.storage_volume_uuid = OLD.storage_volume_uuid;
END;

-- storage_filesystem_attachment for insert.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_filesystem_attachment_insert
AFTER INSERT ON storage_filesystem_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, 20, sa.uuid, DATETIME('now', 'utc')
    FROM storage_instance_filesystem sif
    JOIN storage_attachment sa ON sa.storage_instance_uuid = sif.storage_instance_uuid
    WHERE sif.storage_filesystem_uuid = NEW.storage_filesystem_uuid;
END;

-- storage_filesystem_attachment for update.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_filesystem_attachment_update
AFTER UPDATE ON storage_filesystem_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, 20, sa.uuid, DATETIME('now', 'utc')
    FROM storage_instance_filesystem sif
    JOIN storage_attachment sa ON sa.storage_instance_uuid = sif.storage_instance_uuid
    WHERE sif.storage_filesystem_uuid = NEW.storage_filesystem_uuid;
END;

-- storage_filesystem_attachment for delete.
CREATE TRIGGER trg_log_custom_storage_attachment_storage_filesystem_attachment_delete
AFTER DELETE ON storage_filesystem_attachment FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, 20, sa.uuid, DATETIME('now', 'utc')
    FROM storage_instance_filesystem sif
    JOIN storage_attachment sa ON sa.storage_instance_uuid = sif.storage_instance_uuid
    WHERE sif.storage_filesystem_uuid = OLD.storage_filesystem_uuid;
END;

-- block_device for update.
CREATE TRIGGER trg_log_custom_storage_attachment_block_device_update
AFTER UPDATE ON block_device FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, 20, sa.uuid, DATETIME('now', 'utc')
    FROM storage_volume_attachment sva
    JOIN storage_instance_volume siv ON siv.storage_volume_uuid = sva.storage_volume_uuid
    JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
    WHERE sva.block_device_uuid = NEW.uuid;
END;

-- block_device_link_device for insert.
CREATE TRIGGER trg_log_custom_storage_attachment_block_device_link_device_insert
AFTER INSERT ON block_device_link_device FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, 20, sa.uuid, DATETIME('now', 'utc')
    FROM storage_volume_attachment sva
    JOIN storage_instance_volume siv ON siv.storage_volume_uuid = sva.storage_volume_uuid
    JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
    WHERE sva.block_device_uuid = NEW.block_device_uuid;
END;

-- block_device_link_device for update.
CREATE TRIGGER trg_log_custom_storage_attachment_block_device_link_device_update
AFTER UPDATE ON block_device_link_device FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, 20, sa.uuid, DATETIME('now', 'utc')
    FROM storage_volume_attachment sva
    JOIN storage_instance_volume siv ON siv.storage_volume_uuid = sva.storage_volume_uuid
    JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
    WHERE sva.block_device_uuid = NEW.block_device_uuid;
END;

-- block_device_link_device for delete.
CREATE TRIGGER trg_log_custom_storage_attachment_block_device_link_device_delete
AFTER DELETE ON block_device_link_device FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, 20, sa.uuid, DATETIME('now', 'utc')
    FROM storage_volume_attachment sva
    JOIN storage_instance_volume siv ON siv.storage_volume_uuid = sva.storage_volume_uuid
    JOIN storage_attachment sa ON sa.storage_instance_uuid = siv.storage_instance_uuid
    WHERE sva.block_device_uuid = OLD.block_device_uuid;
END;


INSERT INTO change_log_namespace
VALUES (22,
        'custom_operation_task_status_pending',
        'Operation task status changes to PENDING');

CREATE TRIGGER trg_log_custom_operation_task_status_pending_insert
AFTER INSERT ON operation_task_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, 22, ots.task_uuid, DATETIME('now', 'utc')
    FROM operation_task_status AS ots
    JOIN operation_task_status_value AS otsv ON ots.status_id = otsv.id
    WHERE ots.task_uuid = NEW.task_uuid 
    AND otsv.status = 'pending';
END;
        
CREATE TRIGGER trg_log_custom_operation_task_status_pending_update
AFTER UPDATE ON operation_task_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, 22, ots.task_uuid, DATETIME('now', 'utc')
    FROM operation_task_status AS ots
    JOIN operation_task_status_value AS otsv ON ots.status_id = otsv.id
    WHERE ots.task_uuid = NEW.task_uuid 
    AND otsv.status = 'pending';
END;


INSERT INTO change_log_namespace
VALUES (23,
        'custom_operation_task_status_pending_or_aborting',
        'Operation task status changes to PENDING or ABORTING');
        
CREATE TRIGGER trg_log_custom_operation_task_status_pending_or_aborting_insert
AFTER INSERT ON operation_task_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, 23, ots.task_uuid, DATETIME('now', 'utc')
    FROM operation_task_status AS ots
    JOIN operation_task_status_value AS otsv ON ots.status_id = otsv.id
    WHERE ots.task_uuid = NEW.task_uuid 
    AND (
        otsv.status = 'aborting'
        OR otsv.status = 'pending');
END;

CREATE TRIGGER trg_log_custom_operation_task_status_pending_or_aborting_update
AFTER UPDATE ON operation_task_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, 23, ots.task_uuid, DATETIME('now', 'utc')
    FROM operation_task_status AS ots
    JOIN operation_task_status_value AS otsv ON ots.status_id = otsv.id
    WHERE ots.task_uuid = NEW.task_uuid 
    AND (
        otsv.status = 'aborting'
        OR otsv.status = 'pending');
END;


-- insert namespace for RelationUnit
INSERT INTO change_log_namespace
VALUES (24,
        'custom_relation_unit_by_endpoint_uuid',
        'RelationUnit changes based on relation_endpoint_uuid');

-- insert trigger for RelationUnit
CREATE TRIGGER trg_log_custom_relation_unit_insert
AFTER INSERT ON relation_unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 24, NEW.relation_endpoint_uuid, DATETIME('now', 'utc'));
END;

-- delete trigger for RelationUnit
CREATE TRIGGER trg_log_custom_relation_unit_delete
AFTER DELETE ON relation_unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 24, OLD.relation_endpoint_uuid, DATETIME('now', 'utc'));
END;

-- insert namespace for record.
INSERT INTO change_log_namespace
VALUES (25,
        'custom_deleted_secret_revision_by_id',
        'Deleted secret revisions based on uri/revision_id');

-- delete trigger for secret revision.
CREATE TRIGGER trg_log_custom_secret_revision_delete
AFTER DELETE ON secret_revision FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 25, CONCAT(OLD.secret_id, '/', OLD.revision), DATETIME('now', 'utc'));
END;

-- insert namespace for unit_agent_status
INSERT INTO change_log_namespace
VALUES (26,
        'custom_unit_agent_status',
        'Unit agent status changes');

-- insert trigger for unit_agent_status
CREATE TRIGGER trg_log_custom_unit_agent_status_insert
AFTER INSERT ON unit_agent_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, 26, u.application_uuid, DATETIME('now', 'utc')
    FROM unit AS u
    WHERE u.uuid = NEW.unit_uuid;
END;

-- update trigger for unit_agent_status
CREATE TRIGGER trg_log_custom_unit_agent_status_update
AFTER UPDATE ON unit_agent_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, 26, u.application_uuid, DATETIME('now', 'utc')
    FROM unit AS u
    WHERE u.uuid = NEW.unit_uuid;
END;

-- delete trigger for unit_agent_status
CREATE TRIGGER trg_log_custom_unit_agent_status_delete
AFTER DELETE ON unit_agent_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, 26, u.application_uuid, DATETIME('now', 'utc')
    FROM unit AS u
    WHERE u.uuid = OLD.unit_uuid;
END;


-- insert namespace for unit_workload_status
INSERT INTO change_log_namespace
VALUES (27,
        'custom_unit_workload_status',
        'Unit workload status changes');

-- insert trigger for unit_workload_status
CREATE TRIGGER trg_log_custom_unit_workload_status_insert
AFTER INSERT ON unit_workload_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, 27, u.application_uuid, DATETIME('now', 'utc')
    FROM unit AS u
    WHERE u.uuid = NEW.unit_uuid;
END;

-- update trigger for unit_workload_status
CREATE TRIGGER trg_log_custom_unit_workload_status_update
AFTER UPDATE ON unit_workload_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, 27, u.application_uuid, DATETIME('now', 'utc')
    FROM unit AS u
    WHERE u.uuid = NEW.unit_uuid;
END;

-- delete trigger for unit_workload_status
CREATE TRIGGER trg_log_custom_unit_workload_status_delete
AFTER DELETE ON unit_workload_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, 27, u.application_uuid, DATETIME('now', 'utc')
    FROM unit AS u
    WHERE u.uuid = OLD.unit_uuid;
END;


-- insert namespace for k8s_pod_status
INSERT INTO change_log_namespace
VALUES (28,
        'custom_k8s_pod_status',
        'K8s pod status changes');

-- insert trigger for k8s_pod_status
CREATE TRIGGER trg_log_custom_k8s_pod_status_insert
AFTER INSERT ON k8s_pod_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 1, 28, u.application_uuid, DATETIME('now', 'utc')
    FROM unit AS u
    WHERE u.uuid = NEW.unit_uuid;
END;

-- update trigger for k8s_pod_status
CREATE TRIGGER trg_log_custom_k8s_pod_status_update
AFTER UPDATE ON k8s_pod_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 2, 28, u.application_uuid, DATETIME('now', 'utc')
    FROM unit AS u
    WHERE u.uuid = NEW.unit_uuid;
END;

-- delete trigger for k8s_pod_status
CREATE TRIGGER trg_log_custom_k8s_pod_status_delete
AFTER DELETE ON k8s_pod_status FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    SELECT 4, 28, u.application_uuid, DATETIME('now', 'utc')
    FROM unit AS u
    WHERE u.uuid = OLD.unit_uuid;
END;


-- insert namespace for Relation
INSERT INTO change_log_namespace
VALUES (29,
        'custom_relation_life_suspended',
        'Life or Suspended changes for a relation');

-- update trigger for Relation
CREATE TRIGGER trg_log_custom_relation_life_suspended_update
AFTER UPDATE ON relation FOR EACH ROW
WHEN
    NEW.life_id != OLD.life_id OR
    NEW.suspended != OLD.suspended
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 29, NEW.uuid, DATETIME('now'));
END;

-- delete trigger for Relation
CREATE TRIGGER trg_log_custom_relation_life_suspended_delete
AFTER DELETE ON relation FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 29, OLD.uuid, DATETIME('now'));
END;