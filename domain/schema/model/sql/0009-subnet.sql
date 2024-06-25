CREATE TABLE subnet (
    uuid TEXT NOT NULL PRIMARY KEY,
    cidr TEXT NOT NULL,
    vlan_tag INT,
    space_uuid TEXT,
    CONSTRAINT fk_subnets_spaces
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid)
);

CREATE TABLE provider_subnet (
    provider_id TEXT NOT NULL PRIMARY KEY,
    subnet_uuid TEXT NOT NULL,
    CONSTRAINT fk_provider_subnet_subnet_uuid
    FOREIGN KEY (subnet_uuid)
    REFERENCES subnet (uuid)
);

CREATE UNIQUE INDEX idx_provider_subnet_subnet_uuid
ON provider_subnet (subnet_uuid);

CREATE TABLE provider_network (
    uuid TEXT NOT NULL PRIMARY KEY,
    provider_network_id TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_provider_network_id
ON provider_network (provider_network_id);

CREATE TABLE provider_network_subnet (
    subnet_uuid TEXT NOT NULL PRIMARY KEY,
    provider_network_uuid TEXT NOT NULL,
    CONSTRAINT fk_provider_network_subnet_provider_network_uuid
    FOREIGN KEY (provider_network_uuid)
    REFERENCES provider_network (uuid),
    CONSTRAINT fk_provider_network_subnet_uuid
    FOREIGN KEY (subnet_uuid)
    REFERENCES subnet (uuid)
);

CREATE TABLE availability_zone (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_availability_zone_name
ON availability_zone (name);

CREATE TABLE availability_zone_subnet (
    availability_zone_uuid TEXT NOT NULL,
    subnet_uuid TEXT NOT NULL,
    PRIMARY KEY (availability_zone_uuid, subnet_uuid),
    CONSTRAINT fk_availability_zone_availability_zone_uuid
    FOREIGN KEY (availability_zone_uuid)
    REFERENCES availability_zone (uuid),
    CONSTRAINT fk_availability_zone_subnet_uuid
    FOREIGN KEY (subnet_uuid)
    REFERENCES subnet (uuid)
);

CREATE VIEW v_subnet AS
SELECT
    subnet.uuid AS subnet_uuid,
    subnet.cidr AS subnet_cidr,
    subnet.vlan_tag AS subnet_vlan_tag,
    subnet.space_uuid AS subnet_space_uuid,
    space.name AS subnet_space_name,
    provider_subnet.provider_id AS subnet_provider_id,
    provider_network.provider_network_id AS subnet_provider_network_id,
    availability_zone.name AS subnet_az,
    provider_space.provider_id AS subnet_provider_space_uuid
FROM subnet
LEFT JOIN space
    ON subnet.space_uuid = space.uuid
INNER JOIN provider_subnet
    ON subnet.uuid = provider_subnet.subnet_uuid
INNER JOIN provider_network_subnet
    ON subnet.uuid = provider_network_subnet.subnet_uuid
INNER JOIN provider_network
    ON provider_network_subnet.provider_network_uuid = provider_network.uuid
LEFT JOIN availability_zone_subnet
    ON subnet.uuid = availability_zone_subnet.subnet_uuid
LEFT JOIN availability_zone
    ON availability_zone_subnet.availability_zone_uuid = availability_zone.uuid
LEFT JOIN provider_space
    ON subnet.space_uuid = provider_space.space_uuid;
