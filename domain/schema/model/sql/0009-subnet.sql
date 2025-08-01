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
    CONSTRAINT chk_provider_id_empty CHECK (provider_id != ''),
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
