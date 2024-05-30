CREATE TABLE "constraint" (
    uuid                TEXT PRIMARY KEY,
    arch                TEXT,
    cpu_cores           INT,
    cpu_power           INT,
    mem                 INT,
    root_disk           INT,
    root_disk_source    TEXT,
    instance_role       TEXT,
    instance_type       TEXT,
    container_type_id   INT,
    virt_type           TEXT,
    allocate_public_ip  BOOLEAN,
    image_id            TEXT,
    CONSTRAINT          fk_constraint_container_type
        FOREIGN KEY     (container_type_id)
        REFERENCES      container_type(id)
);

CREATE TABLE constraint_tag (
    id                  INT PRIMARY KEY,
    constraint_uuid     TEXT NOT NULL,
    tag                 TEXT NOT NULL,
    CONSTRAINT          fk_constraint_tag_constraint
        FOREIGN KEY     (constraint_uuid)
        REFERENCES      "constraint"(uuid)
);

CREATE TABLE constraint_space (
    id                  INT PRIMARY KEY,
    constraint_uuid     TEXT NOT NULL,
    space               TEXT NOT NULL,
    CONSTRAINT          fk_constraint_space_constraint
        FOREIGN KEY     (constraint_uuid)
        REFERENCES      "constraint"(uuid)
    CONSTRAINT          fk_constraint_space_space
        FOREIGN KEY     (space)
        REFERENCES      space(name)
);

CREATE TABLE constraint_zone (
    id                  INT PRIMARY KEY,
    constraint_uuid     TEXT NOT NULL,
    zone                TEXT NOT NULL,
    CONSTRAINT          fk_constraint_zone_constraint
        FOREIGN KEY     (constraint_uuid)
        REFERENCES      "constraint"(uuid)
);