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
