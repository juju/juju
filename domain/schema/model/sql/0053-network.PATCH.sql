CREATE VIEW v_unit_relation_network AS
WITH unit_net_node AS (
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
),

candidate AS (
    SELECT
        unn.uuid AS unit_uuid,
        ipa.address_value,
        ipa.device_uuid,
        sn.space_uuid,
        sn.cidr,
        iact.name AS config_type_name,
        iat.name AS type_name,
        iao.name AS origin_name,
        ias.name AS scope_name,
        ipa.origin_id,
        ipa.is_secondary,
        lld.device_type_id,
        CASE
            WHEN ipa.scope_id = 2 /* local-cloud */
                AND ipa.type_id = 0 /* ipv4 */
                THEN 1
            WHEN ipa.scope_id = 2 /* local-cloud */
                AND ipa.type_id = 1 /* ipv6 */
                THEN 2
            WHEN ipa.scope_id IN (1 /* public */, 0 /* unknown */)
                AND ipa.type_id = 0 /* ipv4 */
                THEN 3
            WHEN ipa.scope_id IN (1 /* public */, 0 /* unknown */)
                AND ipa.type_id = 1 /* ipv6 */
                THEN 4
        END AS scope_rank
    FROM unit_net_node AS unn
    JOIN ip_address AS ipa ON unn.net_node_uuid = ipa.net_node_uuid
    JOIN link_layer_device AS lld ON ipa.device_uuid = lld.uuid
    JOIN ip_address_config_type AS iact ON ipa.config_type_id = iact.id
    JOIN ip_address_type AS iat ON ipa.type_id = iat.id
    JOIN ip_address_origin AS iao ON ipa.origin_id = iao.id
    JOIN ip_address_scope AS ias ON ipa.scope_id = ias.id
    LEFT JOIN subnet AS sn ON ipa.subnet_uuid = sn.uuid
)

SELECT
    candidate.unit_uuid,
    candidate.address_value,
    candidate.device_uuid,
    candidate.space_uuid,
    candidate.cidr,
    candidate.config_type_name,
    candidate.type_name,
    candidate.origin_name,
    candidate.scope_name,
    candidate.scope_rank,
    candidate.origin_id,
    candidate.is_secondary,
    candidate.device_type_id
FROM candidate;
