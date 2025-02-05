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
