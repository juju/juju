CREATE TABLE instance_data (
    machine_uuid TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    arch TEXT,
    mem INT,
    root_disk INT,
    root_disk_source TEXT,
    cpu_cores INT,
    cpu_power INT,
    availability_zone_uuid TEXT,
    virt_type TEXT,
    CONSTRAINT fk_machine_machine_uuid
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid),
    CONSTRAINT fk_availability_zone_availability_zone_uuid
    FOREIGN KEY (availability_zone_uuid)
    REFERENCES availability_zone (uuid)
) STRICT;

CREATE TABLE instance_tag (
    machine_uuid TEXT NOT NULL,
    tag TEXT NOT NULL,
    PRIMARY KEY (machine_uuid, tag),
    CONSTRAINT fk_machine_machine_uuid
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
) STRICT;

CREATE TABLE machine_lxd_profile (
    machine_uuid TEXT NOT NULL,
    lxd_profile_uuid TEXT NOT NULL,
    -- TODO(nvinuesa): lxd_profile_uuid should be a foreign key to the 
    -- charm_lxd_profile uuid and therefore the CONSTRAINT should be added when 
    -- that table is implemented.
    PRIMARY KEY (machine_uuid, lxd_profile_uuid),
    CONSTRAINT fk_machine_machine_uuid
    FOREIGN KEY (machine_uuid)
    REFERENCES machine (uuid)
) STRICT;
