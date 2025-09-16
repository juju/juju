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
