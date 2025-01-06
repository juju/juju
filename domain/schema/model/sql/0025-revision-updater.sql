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
    ap.architecture_id AS platform_architecture_id
FROM application AS a
LEFT JOIN charm AS c ON a.charm_uuid = c.uuid
LEFT JOIN application_channel AS ac ON a.uuid = ac.application_uuid
LEFT JOIN application_platform AS ap ON a.uuid = ap.application_uuid
WHERE a.life_id == 0 AND c.source_id = 1;

CREATE VIEW v_revision_updater_application_unit AS
SELECT
    a.uuid,
    COUNT(u.uuid) AS num_units
FROM application AS a
LEFT JOIN unit AS u ON a.uuid = u.application_uuid
GROUP BY u.uuid;
