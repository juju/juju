CREATE TABLE protocol (
    id INT PRIMARY KEY,
    protocol TEXT NOT NULL
);

INSERT INTO protocol VALUES
(0, 'icmp'),
(1, 'tcp'),
(2, 'udp');

CREATE TABLE port_range (
    unit_endpoint_uuid TEXT NOT NULL,
    protocol_id INT NOT NULL,
    from_port INT,
    to_port INT,
    -- We disallow overlapping port ranges, however this cannot
    -- reasonably be enforced in the schema. Including the from_port
    -- in the primary key is as far as we go here. Non-overlapping
    -- ranges must be enforced in the service/state layer.
    PRIMARY KEY (unit_endpoint_uuid, protocol_id, from_port),
    CONSTRAINT fk_port_range_protocol
    FOREIGN KEY (protocol_id)
    REFERENCES protocol (id),
    CONSTRAINT fk_port_range_unit_endpoint
    FOREIGN KEY (unit_endpoint_uuid)
    REFERENCES unit_endpoint (uuid)
);

CREATE TABLE unit_endpoint (
    uuid TEXT PRIMARY KEY,
    endpoint TEXT NOT NULL,
    unit_uuid TEXT NOT NULL,
    CONSTRAINT fk_endpoint_unit
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

CREATE UNIQUE INDEX idx_unit_endpoint_endpoint_unit_uuid ON unit_endpoint (endpoint, unit_uuid);
