CREATE TABLE net_node (
    uuid TEXT NOT NULL PRIMARY KEY
);

CREATE TABLE link_layer_device_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_link_layer_device_type_name
ON link_layer_device_type (name);

INSERT INTO link_layer_device_type VALUES
(0, 'unknown'),
(1, 'loopback'),
(2, 'ethernet'),
(3, '802.1q'),
(4, 'bond'),
(5, 'bridge'),
(6, 'vxlan');

CREATE TABLE virtual_port_type (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_virtual_port_type_name
ON virtual_port_type (name);

INSERT INTO virtual_port_type VALUES
(0, 'nonvirtualport'),
(1, 'openvswitch');

CREATE TABLE link_layer_device (
    uuid TEXT NOT NULL PRIMARY KEY,
    net_node_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    mtu INT,
    -- will be NULL for a placeholder link layer device.
    mac_address TEXT,
    -- provider_id comes from the provider, no FK.
    provider_id TEXT,
    -- one of loopback, ethernet, bridge etc.
    device_type_id INT NOT NULL,
    -- one of nonvirtualport, openvswitch
    virtual_port_type_id INT NOT NULL,
    -- true if the device should be activated on boot.
    is_auto_start BOOLEAN NOT NULL DEFAULT false,
    -- true when the device is up (enabled).
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    -- the link layer device which is the parent to this device.
    parent_device_uuid TEXT,
    CONSTRAINT fk_link_layer_device_parent_device
    FOREIGN KEY (parent_device_uuid)
    REFERENCES link_layer_device (uuid),
    CONSTRAINT fk_link_layer_device_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_link_layer_device_device_type
    FOREIGN KEY (device_type_id)
    REFERENCES link_layer_device_type (id),
    CONSTRAINT fk_link_layer_device_virtual_port_type
    FOREIGN KEY (virtual_port_type_id)
    REFERENCES virtual_port_type (id)
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
(0, 'ipv4'),
(1, 'ipv6');

-- ip_address_origin represents the authoritative source of
-- an ip address.
CREATE TABLE ip_address_origin (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_ip_address_origin_name
ON ip_address_origin (name);

INSERT INTO ip_address_origin VALUES
(0, 'host'),
(1, 'provider');

-- ip_address_scope denotes the context an ip address may apply to.
-- If an address can be reached from the wider internet,
-- it is considered public. A private ip address is either
-- specific to the cloud or cloud subnet a node belongs to,
-- or to the node itself for containers.
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
(2, 'dhcpv6'),
(3, 'slaac'),
(4, 'static'),
(5, 'manual'),
(6, 'loopback');

CREATE TABLE ip_address (
    uuid TEXT NOT NULL PRIMARY KEY,
    -- the link layer device this address belongs to.
    device_uuid TEXT NOT NULL,
    -- The value of the configured IP address.
    -- e.g. 192.168.1.2 or 2001:db8:0000:0000:0000:0000:0000:00001.
    address_value TEXT NOT NULL,
    subnet_uuid TEXT NOT NULL,
    -- one of ipv4, ipv6 etc.
    type_id INT NOT NULL,
    -- one of dhcp, static, manual, loopback etc.
    config_type_id INT NOT NULL,
    -- one of host, provider
    origin_id INT NOT NULL,
    -- one of public, local-cloud, local-machine, link-local etc.
    scope_id INT NOT NULL,
    -- indicates that this address is not the primary
    -- address associated with the NIC.
    is_secondary BOOLEAN DEFAULT false,
    -- indicates whether this address is a virtual/floating/shadow
    -- address assigned by a provider rather than being
    -- associated directly with a device on-machine.
    is_shadow BOOLEAN DEFAULT false,

    CONSTRAINT fk_ip_address_link_layer_device
    FOREIGN KEY (device_uuid)
    REFERENCES link_layer_device (uuid),
    CONSTRAINT fk_ip_address_subnet
    FOREIGN KEY (subnet_uuid)
    REFERENCES subnet (uuid),
    CONSTRAINT fk_ip_address_origin
    FOREIGN KEY (origin_id)
    REFERENCES ip_address_origin (id),
    CONSTRAINT fk_ip_address_type
    FOREIGN KEY (type_id)
    REFERENCES ip_address_type (id),
    CONSTRAINT fk_ip_address_config_type
    FOREIGN KEY (config_type_id)
    REFERENCES ip_address_config_type (id),
    CONSTRAINT fk_ip_address_scope
    FOREIGN KEY (scope_id)
    REFERENCES ip_address_scope (id)
);

CREATE TABLE ip_address_dns_server_address (
    address_uuid TEXT NOT NULL,
    dns_server_address_uuid TEXT NOT NULL,
    CONSTRAINT fk_ip_address_dns_server_address_dns_server_address
    FOREIGN KEY (dns_server_address_uuid)
    REFERENCES ip_address (uuid),
    CONSTRAINT fk_ip_address_dns_server_address_ip_address
    FOREIGN KEY (address_uuid)
    REFERENCES ip_address (uuid),
    PRIMARY KEY (dns_server_address_uuid, address_uuid)
);

CREATE TABLE ip_address_dns_search_domain (
    address_uuid TEXT NOT NULL,
    search_domain TEXT NOT NULL,
    CONSTRAINT fk_ip_address_dns_search_domain_ip_address
    FOREIGN KEY (address_uuid)
    REFERENCES ip_address (uuid),
    PRIMARY KEY (address_uuid, search_domain)
);

-- network_address_scope denotes the context a network address may apply to.
-- If an address can be reached from the wider internet,
-- it is considered public. A private address is either
-- specific to the cloud or cloud subnet a node belongs to,
-- or to the node itself for containers.
CREATE TABLE network_address_scope (
    id INT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_network_address_scope_name
ON network_address_scope (name);

INSERT INTO network_address_scope VALUES
(0, 'local-host'),
(1, 'local-cloud'),
(2, 'public');

CREATE TABLE fqdn_address (
    uuid TEXT NOT NULL PRIMARY KEY,
    address TEXT NOT NULL,
    -- one of local-cloud, public.
    scope_id INT NOT NULL,

    CONSTRAINT chk_fqdn_address_scope
    CHECK (scope_id != 0), -- scope can't be local-host
    CONSTRAINT fk_fqdn_address_scope
    FOREIGN KEY (scope_id)
    REFERENCES network_address_scope (id)
);

CREATE UNIQUE INDEX idx_fqdn_address_address
ON fqdn_address (address);

CREATE TABLE net_node_fqdn_address (
    net_node_uuid TEXT NOT NULL,
    address_uuid TEXT NOT NULL,
    CONSTRAINT fk_net_node_fqdn_address_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_net_node_fqdn_address_address
    FOREIGN KEY (address_uuid)
    REFERENCES fqdn_address (uuid),
    PRIMARY KEY (net_node_uuid, address_uuid)
);

CREATE TABLE hostname_address (
    uuid TEXT NOT NULL PRIMARY KEY,
    hostname TEXT NOT NULL,
    -- one of local-host, local-cloud, public.
    scope_id INT NOT NULL,

    CONSTRAINT fk_hostname_address_scope
    FOREIGN KEY (scope_id)
    REFERENCES network_address_scope (id)
);

CREATE UNIQUE INDEX idx_hostname_address_hostname
ON hostname_address (hostname);

CREATE TABLE net_node_hostname_address (
    net_node_uuid TEXT NOT NULL,
    address_uuid TEXT NOT NULL,
    CONSTRAINT fk_net_node_hostname_address_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_net_node_hostname_address_address
    FOREIGN KEY (address_uuid)
    REFERENCES hostname_address (uuid),
    PRIMARY KEY (net_node_uuid, address_uuid)
);

-- v_address exposes ip and fqdn addresses as a single table.
-- Used for compatibility with the current core network model.
CREATE VIEW v_address AS
SELECT
    ipa.address_value,
    ipa.type_id,
    ipa.config_type_id,
    ipa.origin_id,
    ipa.scope_id
FROM ip_address AS ipa
UNION
SELECT
    fa.hostname AS address_value,
    -- FQDN address type is always "hostname".
    0 AS type_id,
    -- FQDN address config type is always "manual".
    3 AS config_type_id,
    -- FQDN address doesn't have an origin.
    null AS origin_id,
    fa.scope_id
FROM fqdn_address AS fa;

CREATE TABLE ip_address_provider (
    -- a provider-specific ID of the IP address.
    provider_id TEXT NOT NULL PRIMARY KEY,
    address_uuid TEXT NOT NULL,
    CONSTRAINT fk_provider_ip_address_ip_address
    FOREIGN KEY (address_uuid)
    REFERENCES ip_address (uuid)
);

CREATE TABLE ip_address_gateway (
    address_uuid TEXT NOT NULL,
    -- The IP address of the gateway this IP address's
    -- device uses.
    gateway_address TEXT NOT NULL,
    -- true if that device/subnet is the default gateway
    -- of the node.
    is_default_gateway BOOLEAN DEFAULT false,
    CONSTRAINT fk_ip_address_subnet_ip_address
    FOREIGN KEY (address_uuid)
    REFERENCES ip_address (uuid),
    PRIMARY KEY (address_uuid, gateway_address)
);
