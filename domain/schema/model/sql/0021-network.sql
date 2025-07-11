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
-- Note that this corresponds with corenetwork.NonVirtualPortType
(0, ''),
(1, 'openvswitch');

CREATE TABLE link_layer_device (
    uuid TEXT NOT NULL PRIMARY KEY,
    net_node_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    mtu INT,
    -- NULL for a placeholder link layer device.
    mac_address TEXT,
    device_type_id INT NOT NULL,
    virtual_port_type_id INT NOT NULL,
    -- True if the device should be activated on boot.
    is_auto_start BOOLEAN NOT NULL DEFAULT true,
    -- True when the device is in the up state.
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    -- True if traffic is routed out of this device by default.
    is_default_gateway BOOLEAN NOT NULL DEFAULT false,
    -- IP address of the default gateway.
    gateway_address TEXT,
    -- 0 for normal networks; 1-4094 for VLANs.
    vlan_tag INT NOT NULL DEFAULT 0,
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

CREATE UNIQUE INDEX idx_link_layer_device_net_node_uuid_name
ON link_layer_device (net_node_uuid, name);

CREATE TABLE link_layer_device_parent (
    device_uuid TEXT NOT NULL PRIMARY KEY,
    parent_uuid TEXT NOT NULL,
    CONSTRAINT fk_link_layer_device_parent_device
    FOREIGN KEY (device_uuid)
    REFERENCES link_layer_device (uuid),
    CONSTRAINT fk_link_layer_device_parent_parent
    FOREIGN KEY (parent_uuid)
    REFERENCES link_layer_device (uuid)
);

CREATE TABLE provider_link_layer_device (
    provider_id TEXT NOT NULL PRIMARY KEY,
    device_uuid TEXT NOT NULL,
    CONSTRAINT fk_provider_device_uuid
    FOREIGN KEY (device_uuid)
    REFERENCES link_layer_device (uuid)
);

CREATE TABLE link_layer_device_dns_domain (
    device_uuid TEXT NOT NULL,
    search_domain TEXT NOT NULL,
    CONSTRAINT fk_dns_search_domain_dev
    FOREIGN KEY (device_uuid)
    REFERENCES link_layer_device (uuid),
    PRIMARY KEY (device_uuid, search_domain)
);

CREATE TABLE link_layer_device_dns_address (
    device_uuid TEXT NOT NULL,
    dns_address TEXT NOT NULL,
    CONSTRAINT fk_dns_server_device
    FOREIGN KEY (device_uuid)
    REFERENCES link_layer_device (uuid),
    PRIMARY KEY (device_uuid, dns_address)
);

-- Note that this table is defined for completeness in
-- reflecting what we capture as network configuration.
-- At the time of writing, we do not store any routes,
-- but this can be easily changed at a later date.
CREATE TABLE link_layer_device_route (
    device_uuid TEXT NOT NULL,
    destination_cidr TEXT NOT NULL,
    gateway_ip TEXT NOT NULL,
    metric INT NOT NULL,
    CONSTRAINT fk_dns_server_device
    FOREIGN KEY (device_uuid)
    REFERENCES link_layer_device (uuid),
    PRIMARY KEY (device_uuid, destination_cidr)
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
(0, 'machine'),
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
    net_node_uuid TEXT NOT NULL,
    device_uuid TEXT NOT NULL,
    -- The IP address *including the subnet mask*.
    address_value TEXT NOT NULL,
    -- NOTE (manadart 2025-03--25): The fact that this is nullable is a wart
    -- from our Kubernetes provider. There is nothing to say we couldn't do
    -- subnet discovery on K8s by listing nodes, then accumulating
    -- NodeSpec.PodCIDRs for each. We could then match the incoming pod IPs to
    -- those and assign this field.
    subnet_uuid TEXT,
    type_id INT NOT NULL,
    config_type_id INT NOT NULL,
    origin_id INT NOT NULL,
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
    CONSTRAINT fk_ip_address_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
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

CREATE INDEX idx_ip_address_device_uuid
ON ip_address (device_uuid);

CREATE INDEX idx_ip_address_subnet_uuid
ON ip_address (subnet_uuid);

CREATE INDEX idx_ip_address_net_node_uuid
ON ip_address (net_node_uuid);

CREATE TABLE provider_ip_address (
    provider_id TEXT NOT NULL PRIMARY KEY,
    address_uuid TEXT NOT NULL,
    CONSTRAINT fk_provider_ip_address
    FOREIGN KEY (address_uuid)
    REFERENCES ip_address (uuid)
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

-- v_ip_address_with_names returns a ip_address with the
-- type ids converted to their names.
CREATE VIEW v_ip_address_with_names AS
SELECT
    ipa.uuid,
    ipa.address_value,
    ipa.subnet_uuid,
    ipa.device_uuid,
    ipa.net_node_uuid,
    ipa.is_secondary,
    ipa.is_shadow,
    iact.name AS config_type_name,
    ias.name AS scope_name,
    iao.name AS origin_name,
    iat.name AS type_name
FROM ip_address AS ipa
JOIN ip_address_config_type AS iact ON ipa.config_type_id = iact.id
JOIN ip_address_scope AS ias ON ipa.scope_id = ias.id
JOIN ip_address_origin AS iao ON ipa.origin_id = iao.id
JOIN ip_address_type AS iat ON ipa.type_id = iat.id;

CREATE VIEW v_machine_interface AS
SELECT
    m.uuid AS machine_uuid,
    m.name AS machine_name,
    d.net_node_uuid,
    d.uuid AS device_uuid,
    d.name AS device_name,
    d.mtu,
    d.mac_address,
    d.device_type_id,
    d.virtual_port_type_id,
    d.is_auto_start,
    d.is_enabled,
    d.is_default_gateway,
    d.gateway_address,
    d.vlan_tag,
    pd.provider_id AS device_provider_id,
    dp.parent_uuid AS parent_device_uuid,
    dd.name AS parent_device_name,
    dnsa.dns_address,
    dnsd.search_domain,
    a.uuid AS address_uuid,
    pa.provider_id AS provider_address_id,
    a.address_value,
    a.subnet_uuid,
    s.cidr,
    ps.provider_id AS provider_subnet_id,
    a.type_id AS address_type_id,
    a.config_type_id,
    a.origin_id,
    a.scope_id,
    a.is_secondary,
    a.is_shadow
FROM machine AS m
JOIN link_layer_device AS d ON m.net_node_uuid = d.net_node_uuid
LEFT JOIN provider_link_layer_device AS pd ON d.uuid = pd.device_uuid
LEFT JOIN link_layer_device_parent AS dp ON d.uuid = dp.device_uuid
LEFT JOIN link_layer_device AS dd ON dp.parent_uuid = dd.uuid
LEFT JOIN link_layer_device_dns_address AS dnsa ON d.uuid = dnsa.device_uuid
LEFT JOIN link_layer_device_dns_domain AS dnsd ON d.uuid = dnsd.device_uuid
LEFT JOIN ip_address AS a ON d.uuid = a.device_uuid
LEFT JOIN provider_ip_address AS pa ON a.uuid = pa.address_uuid
LEFT JOIN subnet AS s ON a.subnet_uuid = s.uuid
LEFT JOIN provider_subnet AS ps ON a.subnet_uuid = ps.subnet_uuid;


-- This view allows to retrieves any address belonging to a unit.
-- It is useful when we have a unit UUID and we want all addresses relative to this unit,
-- without caring if we are in a k8s provider or a machine provider.
-- This add addresses belonging to the k8s service of the unit application
-- alongside those belonging directly to the unit
CREATE VIEW v_all_unit_address AS
SELECT
    n.uuid AS unit_uuid,
    ipa.address_value,
    ipa.config_type_name,
    ipa.type_name,
    ipa.origin_name,
    ipa.scope_name,
    ipa.device_uuid,
    sn.space_uuid,
    sn.cidr
FROM (
    SELECT
        s.net_node_uuid,
        u.uuid
    FROM unit AS u
    JOIN application AS a ON u.application_uuid = a.uuid
    JOIN k8s_service AS s ON a.uuid = s.application_uuid
    UNION
    SELECT
        net_node_uuid,
        uuid
    FROM unit
) AS n
JOIN v_ip_address_with_names AS ipa ON n.net_node_uuid = ipa.net_node_uuid
LEFT JOIN subnet AS sn ON ipa.subnet_uuid = sn.uuid;
