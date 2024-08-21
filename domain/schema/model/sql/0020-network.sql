CREATE TABLE net_node (
    uuid TEXT PRIMARY KEY
);

CREATE TABLE cloud_service (
    net_node_uuid TEXT NOT NULL PRIMARY KEY,
    application_uuid TEXT NOT NULL,
    CONSTRAINT fk_cloud_service_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_cloud_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

CREATE UNIQUE INDEX idx_cloud_service_application
ON cloud_service (application_uuid);

CREATE TABLE cloud_container (
    net_node_uuid TEXT NOT NULL PRIMARY KEY,
    provider_id TEXT NOT NULL,
    CONSTRAINT fk_cloud_container_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid)
);

CREATE TABLE cloud_container_ip_address (
    net_node_uuid TEXT NOT NULL,
    ip_address_uuid TEXT NOT NULL,
    CONSTRAINT fk_cloud_container_ip_address_net_node_uuid
    FOREIGN KEY (net_node_uuid)
    REFERENCES cloud_container (net_node_uuid),
    CONSTRAINT fk_cloud_container_ip_address_ip_address_uuid
    FOREIGN KEY (ip_address_uuid)
    REFERENCES ip_address (uuid),
    PRIMARY KEY (net_node_uuid, ip_address_uuid)
);

CREATE TABLE cloud_container_port (
    net_node_uuid TEXT NOT NULL,
    port TEXT NOT NULL,
    CONSTRAINT fk_cloud_container_port_net_node_uuid
    FOREIGN KEY (net_node_uuid)
    REFERENCES cloud_container (net_node_uuid),
    PRIMARY KEY (net_node_uuid, port)
);

-- ip_address_type represents the possible ways of specifying
-- an address, either a hostname resolvable by dns lookup,
-- or IPv4 or IPv6 address.
CREATE TABLE ip_address_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_ip_address_type_name
ON ip_address_type (name);

INSERT INTO ip_address_type VALUES
(0, 'hostname'),
(1, 'ipv4'),
(2, 'ipv6');

-- ip_address_origin represents the authoritative source of
-- an ip address.
CREATE TABLE ip_address_origin (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_ip_address_origin_name
ON ip_address_origin (name);

INSERT INTO ip_address_origin VALUES
(0, 'unknown'),
(1, 'machine'),
(2, 'provider');

-- ip_address_scope denotes the context an address may apply to.
-- If a name or address can be reached from the wider internet,
-- it is considered public. A private network address is either
-- specific to the cloud or cloud subnet a machine belongs to,
-- or to the machine itself for containers.
CREATE TABLE ip_address_scope (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_ip_address_scope_name
ON ip_address_scope (name);

INSERT INTO ip_address_scope VALUES
(0, 'unknown'),
(1, 'public'),
(2, 'local-cloud'),
(3, 'local-machine'),
(4, 'link-local');

-- ip_address_config_type defines valid network
-- link configuration types.
CREATE TABLE ip_address_config_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_ip_address_config_type_name
ON ip_address_config_type (name);

INSERT INTO ip_address_config_type VALUES
(0, 'unknown'),
(1, 'dhcp'),
(2, 'static'),
(3, 'manual'),
(4, 'loopback');

CREATE TABLE ip_address (
    uuid TEXT NOT NULL PRIMARY KEY,
    -- The value of the configured IP address.
    -- e.g. 192.168.1.2 or 2001:db8::1.
    address_value TEXT NOT NULL,
    type_id INT NOT NULL,
    origin_id INT NOT NULL,
    config_type_id INT NOT NULL,
    scope_id INT NOT NULL,

    CONSTRAINT fk_ip_address_type
    FOREIGN KEY (type_id)
    REFERENCES ip_address_type (id),
    CONSTRAINT fk_ip_address_origin
    FOREIGN KEY (origin_id)
    REFERENCES ip_address_origin (id),
    CONSTRAINT fk_ip_address_scope
    FOREIGN KEY (scope_id)
    REFERENCES ip_address_scope (id),
    CONSTRAINT fk_ip_address_config_type
    FOREIGN KEY (config_type_id)
    REFERENCES ip_address_config_type (id)
);

CREATE TABLE ip_address_provider (
    -- a provider-specific ID of the IP address.
    provider_id TEXT NOT NULL PRIMARY KEY,
    ip_address_uuid TEXT NOT NULL,
    CONSTRAINT fk_provider_ip_address_ip_address_uuid
    FOREIGN KEY (ip_address_uuid)
    REFERENCES ip_address (uuid)
);

CREATE TABLE ip_address_space (
    space_uuid TEXT NOT NULL,
    ip_address_uuid TEXT NOT NULL,
    CONSTRAINT fk_ip_address_space_space_uuid
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid),
    CONSTRAINT fk_ip_address_space_ip_address_uuid
    FOREIGN KEY (ip_address_uuid)
    REFERENCES ip_address (uuid),
    PRIMARY KEY (space_uuid, ip_address_uuid)
);

CREATE TABLE ip_address_subnet (
    subnet_uuid TEXT NOT NULL,
    ip_address_uuid TEXT NOT NULL,
    CONSTRAINT fk_ip_address_subnet_subnet_uuid
    FOREIGN KEY (subnet_uuid)
    REFERENCES subnet (uuid),
    CONSTRAINT fk_ip_address_subnet_ip_address_uuid
    FOREIGN KEY (ip_address_uuid)
    REFERENCES ip_address (uuid),
    PRIMARY KEY (subnet_uuid, ip_address_uuid)
);
