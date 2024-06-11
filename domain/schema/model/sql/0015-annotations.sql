CREATE TABLE annotation_model (
    "key" TEXT PRIMARY KEY,
    value TEXT NOT NULL
) STRICT;

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
) STRICT;

CREATE TABLE annotation_charm (
    uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (uuid, "key")
    -- Following needs to be uncommented when we do have the
    -- annotatables as real domain entities.
    -- CONSTRAINT          fk_annotation_charm
    --     FOREIGN KEY     (uuid)
    --     REFERENCES      charm(uuid)
) STRICT;

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
) STRICT;

CREATE TABLE annotation_unit (
    uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (uuid, "key")
    -- Following needs to be uncommented when we do have the
    -- annotatables as real domain entities.
    -- CONSTRAINT          fk_annotation_unit
    --     FOREIGN KEY     (uuid)
    --     REFERENCES      unit(uuid)
) STRICT;

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
) STRICT;

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
) STRICT;

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
) STRICT;
