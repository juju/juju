CREATE TABLE machine_cloud_instance (
    machine_uuid TEXT NOT NULL PRIMARY KEY,
    instance_id TEXT,
    display_name TEXT,
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
    CONSTRAINT fk_machine_constraint_instance
    FOREIGN KEY (machine_uuid)
    REFERENCES machine_cloud_instance (machine_uuid),
    CONSTRAINT fk_machine_constraint_status
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
