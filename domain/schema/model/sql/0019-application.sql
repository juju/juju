CREATE TABLE application (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    life_id INT NOT NULL,
    charm_uuid TEXT NOT NULL,
    charm_modified_version INT,
    charm_upgrade_on_error BOOLEAN DEFAULT FALSE,
    -- space_uuid is the default binding for this application.
    space_uuid TEXT NOT NULL,
    CONSTRAINT fk_application_life
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_application_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_space_uuid
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid)
);

CREATE UNIQUE INDEX idx_application_name
ON application (name);

CREATE TABLE k8s_service (
    uuid TEXT NOT NULL PRIMARY KEY,
    application_uuid TEXT NOT NULL,
    net_node_uuid TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    CONSTRAINT fk_k8s_service_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_k8s_service_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid)
);

CREATE UNIQUE INDEX idx_k8s_service_provider
ON k8s_service (provider_id);

CREATE INDEX idx_k8s_service_application
ON k8s_service (application_uuid);

CREATE UNIQUE INDEX idx_k8s_service_net_node
ON k8s_service (net_node_uuid);

-- Application scale is currently only targeting k8s applications.
CREATE TABLE application_scale (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    scale INT,
    scale_target INT,
    scaling BOOLEAN DEFAULT FALSE,
    CONSTRAINT fk_application_endpoint_scale_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

CREATE TABLE application_exposed_endpoint_space (
    application_uuid TEXT NOT NULL,
    -- NULL application_endpoint_uuid represents the wildcard endpoint.
    application_endpoint_uuid TEXT,
    space_uuid TEXT NOT NULL,
    CONSTRAINT fk_application_exposed_endpoint_space_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_exposed_endpoint_space_application_endpoint
    FOREIGN KEY (application_endpoint_uuid)
    REFERENCES application_endpoint (uuid),
    CONSTRAINT fk_application_exposed_endpoint_space_space
    FOREIGN KEY (space_uuid)
    REFERENCES space (uuid),
    PRIMARY KEY (application_uuid, application_endpoint_uuid, space_uuid)
);

-- There is no FK against the CIDR, because it's currently free-form.
CREATE TABLE application_exposed_endpoint_cidr (
    application_uuid TEXT NOT NULL,
    -- NULL application_endpoint_uuid represents the wildcard endpoint.
    application_endpoint_uuid TEXT,
    cidr TEXT NOT NULL,
    CONSTRAINT fk_application_exposed_endpoint_cidr_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_exposed_endpoint_cidr_application_endpoint
    FOREIGN KEY (application_endpoint_uuid)
    REFERENCES application_endpoint (uuid),
    PRIMARY KEY (application_uuid, application_endpoint_uuid, cidr)
);

CREATE VIEW v_application_exposed_endpoint (
    application_uuid,
    application_endpoint_uuid,
    space_uuid,
    cidr
) AS
SELECT
    aes.application_uuid,
    aes.application_endpoint_uuid,
    aes.space_uuid,
    NULL AS n
FROM application_exposed_endpoint_space AS aes
UNION
SELECT
    aec.application_uuid,
    aec.application_endpoint_uuid,
    NULL AS n,
    aec.cidr
FROM application_exposed_endpoint_cidr AS aec;

CREATE TABLE application_config_hash (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    sha256 TEXT NOT NULL,
    CONSTRAINT fk_application_config_hash_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

CREATE TABLE application_config (
    application_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    -- TODO(jack-w-shaw): Drop this field, instead look it up from the charm config
    type_id INT NOT NULL,
    value TEXT,
    CONSTRAINT fk_application_config_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_config_charm_config_type
    FOREIGN KEY (type_id)
    REFERENCES charm_config_type (id),
    PRIMARY KEY (application_uuid, "key")
);

CREATE VIEW v_application_config AS
SELECT
    a.uuid,
    ac."key",
    ac.value,
    cct.name AS type
FROM application AS a
LEFT JOIN application_config AS ac ON a.uuid = ac.application_uuid
JOIN charm_config_type AS cct ON ac.type_id = cct.id;

CREATE TABLE application_constraint (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    constraint_uuid TEXT,
    CONSTRAINT fk_application_constraint_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_constraint_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid)
);

CREATE TABLE application_setting (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    trust BOOLEAN DEFAULT FALSE,
    CONSTRAINT fk_application_setting_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

CREATE TABLE application_platform (
    application_uuid TEXT NOT NULL,
    os_id TEXT NOT NULL,
    channel TEXT,
    architecture_id INT NOT NULL,
    CONSTRAINT fk_application_platform_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_application_platform_os
    FOREIGN KEY (os_id)
    REFERENCES os (id),
    CONSTRAINT fk_application_platform_architecture
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

CREATE TABLE application_channel (
    application_uuid TEXT NOT NULL,
    track TEXT,
    risk TEXT NOT NULL,
    branch TEXT,
    CONSTRAINT fk_application_origin_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    PRIMARY KEY (application_uuid, track, risk, branch)
);

CREATE TABLE application_status (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    data TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_application_status_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_workload_status_value_status
    FOREIGN KEY (status_id)
    REFERENCES workload_status_value (id)
);

CREATE TABLE device_constraint (
    uuid TEXT NOT NULL PRIMARY KEY,
    application_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    count INT,
    CONSTRAINT fk_device_constraint_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

CREATE UNIQUE INDEX idx_device_constraint_application_name
ON device_constraint (application_uuid, name);

CREATE TABLE device_constraint_attribute (
    device_constraint_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    CONSTRAINT fk_device_constraint_attribute_device_constraint
    FOREIGN KEY (device_constraint_uuid)
    REFERENCES device_constraint (uuid),
    PRIMARY KEY (device_constraint_uuid, "key")
);

CREATE VIEW v_application_constraint AS
SELECT
    ac.application_uuid,
    c.arch,
    c.cpu_cores,
    c.cpu_power,
    c.mem,
    c.root_disk,
    c.root_disk_source,
    c.instance_role,
    c.instance_type,
    ctype.value AS container_type,
    c.virt_type,
    c.allocate_public_ip,
    c.image_id,
    ctag.tag,
    cspace.space AS space_name,
    cspace."exclude" AS space_exclude,
    czone.zone
FROM application_constraint AS ac
JOIN "constraint" AS c ON ac.constraint_uuid = c.uuid
LEFT JOIN container_type AS ctype ON c.container_type_id = ctype.id
LEFT JOIN constraint_tag AS ctag ON c.uuid = ctag.constraint_uuid
LEFT JOIN constraint_space AS cspace ON c.uuid = cspace.constraint_uuid
LEFT JOIN constraint_zone AS czone ON c.uuid = czone.constraint_uuid;

CREATE VIEW v_application_platform_channel AS
SELECT
    ap.application_uuid,
    os.name AS platform_os,
    os.id AS platform_os_id,
    ap.channel AS platform_channel,
    a.name AS platform_architecture,
    a.id AS platform_architecture_id,
    ac.track AS channel_track,
    ac.risk AS channel_risk,
    ac.branch AS channel_branch
FROM application_platform AS ap
JOIN os ON ap.os_id = os.id
JOIN architecture AS a ON ap.architecture_id = a.id
LEFT JOIN application_channel AS ac ON ap.application_uuid = ac.application_uuid;

CREATE VIEW v_application_origin AS
SELECT
    a.uuid,
    c.reference_name,
    c.source_id,
    c.revision,
    cdi.charmhub_identifier,
    ch.hash
FROM application AS a
JOIN charm AS c ON a.charm_uuid = c.uuid
LEFT JOIN charm_download_info AS cdi ON c.uuid = cdi.charm_uuid
JOIN charm_hash AS ch ON c.uuid = ch.charm_uuid;

CREATE VIEW v_application_export AS
SELECT
    a.uuid,
    a.name,
    a.life_id,
    a.charm_uuid,
    a.charm_modified_version,
    a.charm_upgrade_on_error,
    cm.subordinate,
    c.reference_name,
    c.source_id,
    c.revision,
    c.architecture_id,
    k8s.provider_id AS k8s_provider_id
FROM application AS a
JOIN charm AS c ON a.charm_uuid = c.uuid
JOIN charm_metadata AS cm ON c.uuid = cm.charm_uuid
LEFT JOIN k8s_service AS k8s ON a.uuid = k8s.application_uuid;

CREATE VIEW v_application_endpoint_uuid AS
SELECT
    a.uuid,
    c.name,
    a.application_uuid
FROM application_endpoint AS a
JOIN charm_relation AS c ON a.charm_relation_uuid = c.uuid;

-- v_application_subordinate provides an application, whether its charm is a
-- subordinate, and a relation_uuid if it exists. It's possible the application
-- is in zero or multiple relations.
CREATE VIEW v_application_subordinate AS
SELECT
    a.uuid AS application_uuid,
    cm.subordinate,
    re.relation_uuid
FROM application AS a
JOIN charm AS c ON a.charm_uuid = c.uuid
JOIN charm_metadata AS cm ON c.uuid = cm.charm_uuid
JOIN charm_relation AS cr ON c.uuid = cr.charm_uuid
JOIN application_endpoint AS ae ON cr.uuid = ae.charm_relation_uuid
JOIN relation_endpoint AS re ON ae.uuid = re.endpoint_uuid;
