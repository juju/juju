CREATE TABLE protocol (
    id INT PRIMARY KEY,
    protocol TEXT NOT NULL
);

INSERT INTO protocol VALUES
(0, 'icmp'),
(1, 'tcp'),
(2, 'udp');

CREATE TABLE port_range (
    uuid TEXT PRIMARY KEY,
    unit_endpoint_uuid TEXT NOT NULL,
    protocol_id INT NOT NULL,
    from_port INT,
    to_port INT,
    CONSTRAINT fk_port_range_protocol
    FOREIGN KEY (protocol_id)
    REFERENCES protocol (id),
    CONSTRAINT fk_port_range_unit_endpoint
    FOREIGN KEY (unit_endpoint_uuid)
    REFERENCES unit_endpoint (uuid)
);

-- We disallow overlapping port ranges, however this cannot reasonably
-- be enforced in the schema. Including the from_port in the uniqueness
-- constraint is as far as we go here. Non-overlapping ranges must be
-- enforced in the service/state layer.
CREATE UNIQUE INDEX idx_port_range_endpoint_port_range ON port_range (unit_endpoint_uuid, protocol_id, from_port);

CREATE TABLE unit_endpoint (
    uuid TEXT PRIMARY KEY,
    endpoint TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    CONSTRAINT fk_endpoint_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

CREATE UNIQUE INDEX idx_unit_endpoint_endpoint_unit_uuid ON unit_endpoint (endpoint, unit_uuid);

-- v_port_range is used to fetch well constructed information about a port range.
-- This will include information about the port range's endpoint and unit, as well
-- as the uuids for the net node and application each endpoint is located on.
CREATE VIEW v_port_range
AS
SELECT
    pr.from_port,
    pr.to_port,
    protocol.protocol,
    ue.endpoint,
    u.name AS unit_name,
    u.net_node_uuid,
    u.application_uuid
FROM port_range AS pr
LEFT JOIN protocol ON pr.protocol_id = protocol.id
LEFT JOIN unit_endpoint AS ue ON pr.unit_endpoint_uuid = ue.uuid
LEFT JOIN unit AS u ON ue.unit_uuid = u.uuid;
