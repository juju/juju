CREATE TABLE constraint_tag (
    constraint_uuid TEXT NOT NULL,
    tag TEXT NOT NULL,
    CONSTRAINT fk_constraint_tag_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid),
    PRIMARY KEY (constraint_uuid, tag)
);

-- v_model_constraint_tag is a view of all the constraint tags set for the
-- current model. It is expected that this view can be empty.
CREATE VIEW v_model_constraint_tag AS
SELECT
    ct.constraint_uuid,
    ct.tag
FROM constraint_tag AS ct
JOIN "constraint" AS c ON ct.constraint_uuid = c.uuid
JOIN model_constraint AS mc ON c.uuid = mc.constraint_uuid;

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

CREATE INDEX idx_constraint_space_space
ON constraint_space (space);

CREATE TABLE constraint_zone (
    constraint_uuid TEXT NOT NULL,
    zone TEXT NOT NULL,
    CONSTRAINT fk_constraint_zone_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid),
    PRIMARY KEY (constraint_uuid, zone)
);

-- v_model_constraint_zone is a view of all the constraint zones set for the
-- current model. It is expected that this view can be empty.
CREATE VIEW v_model_constraint_zone AS
SELECT
    cz.constraint_uuid,
    cz.zone
FROM constraint_zone AS cz
JOIN "constraint" AS c ON cz.constraint_uuid = c.uuid
JOIN model_constraint AS mc ON c.uuid = mc.constraint_uuid;
