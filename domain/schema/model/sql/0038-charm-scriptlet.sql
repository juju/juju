CREATE TABLE charm_scriptlet (
    charm_uuid TEXT NOT NULL,
    path TEXT NOT NULL,
    content BLOB NOT NULL,
    PRIMARY KEY (charm_uuid, path),
    CONSTRAINT fk_charm_scriptlet_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid)
);
