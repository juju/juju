-- Patch 0060: Add indexes used by model schema views.
--
-- Also makes application_platform and charm_hash single-row-per-application
-- and single-row-per-charm tables respectively. The domain already treats
-- application_uuid and charm_uuid as row identities for these tables.

DROP VIEW v_application_charm_download_info;
DROP VIEW v_application_origin;
DROP VIEW v_application_platform_channel;
DROP VIEW v_revision_updater_application;

CREATE TABLE application_platform_new (
    application_uuid TEXT NOT NULL PRIMARY KEY,
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

INSERT INTO application_platform_new (
    application_uuid,
    os_id,
    channel,
    architecture_id
)
SELECT
    application_uuid,
    os_id,
    channel,
    architecture_id
FROM application_platform;

DROP TABLE application_platform;
ALTER TABLE application_platform_new RENAME TO application_platform;

CREATE TABLE charm_hash_new (
    charm_uuid TEXT NOT NULL PRIMARY KEY,
    hash_kind_id INT NOT NULL DEFAULT 0,
    hash TEXT NOT NULL,
    CONSTRAINT fk_charm_hash_charm
    FOREIGN KEY (charm_uuid)
    REFERENCES charm (uuid),
    CONSTRAINT fk_charm_hash_kind
    FOREIGN KEY (hash_kind_id)
    REFERENCES hash_kind (id)
);

INSERT INTO charm_hash_new (
    charm_uuid,
    hash_kind_id,
    hash
)
SELECT
    charm_uuid,
    hash_kind_id,
    hash
FROM charm_hash;

DROP TABLE charm_hash;
ALTER TABLE charm_hash_new RENAME TO charm_hash;

-- noqa: disable=all
CREATE TRIGGER trg_charm_hash_immutable_update
BEFORE UPDATE ON charm_hash
FOR EACH ROW
BEGIN
    SELECT RAISE(FAIL, 'charm_hash table is unmodifiable, only insertions and deletions are allowed');
END;
-- noqa: enable=all

CREATE VIEW v_application_charm_download_info AS
SELECT
    a.uuid AS application_uuid,
    c.uuid AS charm_uuid,
    c.reference_name AS name,
    c.available,
    cs.id AS source_id,
    cp.name AS provenance,
    cdi.charmhub_identifier,
    cdi.download_url,
    cdi.download_size,
    ch.hash
FROM application AS a
LEFT JOIN charm AS c ON a.charm_uuid = c.uuid
LEFT JOIN charm_download_info AS cdi ON c.uuid = cdi.charm_uuid
LEFT JOIN charm_provenance AS cp ON cdi.provenance_id = cp.id
LEFT JOIN charm_source AS cs ON c.source_id = cs.id
LEFT JOIN charm_hash AS ch ON c.uuid = ch.charm_uuid;

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

CREATE VIEW v_revision_updater_application AS
SELECT
    a.uuid,
    a.name,
    c.reference_name,
    c.revision,
    c.architecture_id AS charm_architecture_id,
    ac.track AS channel_track,
    ac.risk AS channel_risk,
    ac.branch AS channel_branch,
    ap.os_id AS platform_os_id,
    ap.channel AS platform_channel,
    ap.architecture_id AS platform_architecture_id,
    cdi.charmhub_identifier
FROM application AS a
LEFT JOIN charm AS c ON a.charm_uuid = c.uuid
LEFT JOIN application_channel AS ac ON a.uuid = ac.application_uuid
LEFT JOIN application_platform AS ap ON a.uuid = ap.application_uuid
LEFT JOIN charm_download_info AS cdi ON c.uuid = cdi.charm_uuid
WHERE a.life_id = 0 AND c.source_id = 1;

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

CREATE INDEX idx_agent_binary_store_object_store_uuid
ON agent_binary_store (object_store_uuid);

CREATE INDEX idx_object_store_metadata_path_metadata_uuid
ON object_store_metadata_path (metadata_uuid);

CREATE INDEX idx_application_charm_uuid
ON application (charm_uuid);

CREATE INDEX idx_provider_link_layer_device_device_uuid
ON provider_link_layer_device (device_uuid);

CREATE INDEX idx_provider_ip_address_address_uuid
ON provider_ip_address (address_uuid);

CREATE INDEX idx_subnet_space_uuid
ON subnet (space_uuid);

CREATE INDEX idx_availability_zone_subnet_subnet_uuid
ON availability_zone_subnet (subnet_uuid);

CREATE INDEX idx_application_endpoint_charm_relation_uuid
ON application_endpoint (charm_relation_uuid);

CREATE INDEX idx_relation_endpoint_endpoint_uuid
ON relation_endpoint (endpoint_uuid);
