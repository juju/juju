CREATE TABLE charm (
    uuid TEXT PRIMARY KEY,
    url TEXT NOT NULL
) STRICT;

CREATE TABLE charm_storage (
    charm_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    storage_kind_id INT NOT NULL,
    shared INT,
    read_only INT,
    count_min INT NOT NULL,
    count_max INT NOT NULL,
    minimum_size_mib INT,
    location TEXT,
    CONSTRAINT fk_storage_instance_kind
    FOREIGN KEY (storage_kind_id)
    REFERENCES storage_kind (id),
    CONSTRAINT fk_charm_storage_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    PRIMARY KEY (charm_uuid, name)
) STRICT;

CREATE INDEX idx_charm_storage_charm
ON charm_storage (charm_uuid);
