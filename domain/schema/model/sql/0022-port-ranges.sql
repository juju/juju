CREATE TABLE protocol (
    id INT PRIMARY KEY,
    protocol TEXT NOT NULL
);

INSERT INTO protocol VALUES
(0, 'icmp'),
(1, 'tcp'),
(2, 'udp');

CREATE TABLE port_range (
    uuid TEXT NOT NULL PRIMARY KEY,
    protocol_id INT NOT NULL,
    from_port INT,
    to_port INT,
    relation_uuid TEXT, -- NULL-able, where null represents a wildcard endpoint
    unit_uuid TEXT NOT NULL,
    CONSTRAINT fk_port_range_protocol
    FOREIGN KEY (protocol_id)
    REFERENCES protocol (id),
    CONSTRAINT fk_port_range_relation
    FOREIGN KEY (relation_uuid)
    REFERENCES charm_relation (uuid),
    CONSTRAINT fk_port_range_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

-- We disallow overlapping port ranges, however this cannot reasonably
-- be enforced in the schema. Including the from_port in the uniqueness
-- constraint is as far as we go here. Non-overlapping ranges must be
-- enforced in the service/state layer.
CREATE UNIQUE INDEX idx_port_range_endpoint ON port_range (protocol_id, from_port, relation_uuid, unit_uuid);

CREATE VIEW v_port_range
AS
SELECT
    pr.uuid,
    pr.from_port,
    pr.to_port,
    pr.unit_uuid,
    u.name AS unit_name,
    protocol.protocol,
    cr.name AS endpoint
FROM port_range AS pr
LEFT JOIN protocol ON pr.protocol_id = protocol.id
LEFT JOIN charm_relation AS cr ON pr.relation_uuid = cr.uuid
LEFT JOIN unit AS u ON pr.unit_uuid = u.uuid;

CREATE VIEW v_endpoint
AS
SELECT
    cr.uuid,
    cr.name AS endpoint,
    u.uuid AS unit_uuid
FROM unit AS u
LEFT JOIN application AS a ON u.application_uuid = a.uuid
LEFT JOIN charm_relation AS cr ON a.charm_uuid = cr.charm_uuid;
