CREATE TABLE unit (
    uuid             TEXT PRIMARY KEY,
    unit_id          TEXT NOT NULL,
    application_uuid TEXT NOT NULL,
    net_node_uuid    TEXT NOT NULL,
    life_id          INT NOT NULL,
    CONSTRAINT       fk_unit_application
        FOREIGN KEY  (application_uuid)
        REFERENCES   application(uuid),
    CONSTRAINT       fk_unit_net_node
        FOREIGN KEY  (net_node_uuid)
        REFERENCES   net_node(uuid),
    CONSTRAINT       fk_unit_life
        FOREIGN KEY  (life_id)
        REFERENCES   life(id)
);

CREATE UNIQUE INDEX idx_unit_id
ON unit (unit_id);

CREATE INDEX idx_unit_application
ON unit (application_uuid);

CREATE INDEX idx_unit_net_node
ON unit (net_node_uuid);

CREATE TABLE unit_state (
    unit_uuid       TEXT PRIMARY KEY,
    uniter_state    TEXT,
    storage_state   TEXT,
    secret_state    TEXT,
    CONSTRAINT      fk_unit_state_unit
        FOREIGN KEY (unit_uuid)
        REFERENCES  unit(uuid)
);

-- Local charm state stored upon hook commit with uniter state.
CREATE TABLE unit_state_charm (
    unit_uuid       TEXT,
    key             TEXT,
    value           TEXT NOT NULL,
    PRIMARY KEY     (unit_uuid, key),
    CONSTRAINT      fk_unit_state_charm_unit
        FOREIGN KEY (unit_uuid)
        REFERENCES  unit(uuid)
);

-- Local relation state stored upon hook commit with uniter state.
CREATE TABLE unit_state_relation (
    unit_uuid       TEXT,
    key             TEXT,
    value           TEXT NOT NULL,
    PRIMARY KEY     (unit_uuid, key),
    CONSTRAINT      fk_unit_state_relation_unit
        FOREIGN KEY (unit_uuid)
        REFERENCES  unit(uuid)
);
