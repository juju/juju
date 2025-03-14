CREATE TABLE space (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_spaces_uuid_name ON space (name);

CREATE TABLE provider_space (
    provider_id TEXT NOT NULL PRIMARY KEY,
    space_uuid TEXT NOT NULL,
    CONSTRAINT fk_provider_space_space_uuid FOREIGN KEY (space_uuid) REFERENCES space (uuid)
);

CREATE UNIQUE INDEX idx_provider_space_space_uuid ON provider_space (space_uuid);

INSERT INTO space VALUES
('019593ec-84f2-7772-bad2-7a770aed04bc', 'alpha');

CREATE VIEW v_space_subnet AS
SELECT
    space.uuid,
    space.name,
    provider_space.provider_id,
    subnet.uuid AS subnet_uuid,
    subnet.cidr AS subnet_cidr,
    subnet.vlan_tag AS subnet_vlan_tag,
    subnet.space_uuid AS subnet_space_uuid,
    space.name AS subnet_space_name,
    provider_subnet.provider_id AS subnet_provider_id,
    provider_network.provider_network_id AS subnet_provider_network_id,
    availability_zone.name AS subnet_az,
    provider_space.provider_id AS subnet_provider_space_uuid
FROM
    space
LEFT JOIN provider_space ON space.uuid = provider_space.space_uuid
LEFT JOIN subnet ON space.uuid = subnet.space_uuid
LEFT JOIN provider_subnet ON subnet.uuid = provider_subnet.subnet_uuid
LEFT JOIN provider_network_subnet ON subnet.uuid = provider_network_subnet.subnet_uuid
LEFT JOIN provider_network ON provider_network_subnet.provider_network_uuid = provider_network.uuid
LEFT JOIN availability_zone_subnet ON subnet.uuid = availability_zone_subnet.subnet_uuid
LEFT JOIN availability_zone ON availability_zone_subnet.availability_zone_uuid = availability_zone.uuid;
