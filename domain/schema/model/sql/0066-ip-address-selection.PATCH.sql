-- Share address metadata and public-address preference ranks between unit,
-- machine and status queries. Callers apply their own filters and ORDER BY.
CREATE VIEW v_ip_address_selection AS
SELECT
    ipa.net_node_uuid,
    ipa.device_uuid,
    ipa.address_value,
    iact.name AS config_type_name,
    iat.name AS type_name,
    iao.name AS origin_name,
    ias.name AS scope_name,
    sn.space_uuid,
    sn.cidr,
    ipa.scope_id,
    ipa.type_id,
    ipa.is_secondary,
    CASE
        WHEN ipa.scope_id = 1 /* public */ THEN 0
        WHEN ipa.scope_id = 2 /* local-cloud */ THEN 1
        WHEN ipa.scope_id = 0 /* unknown */ THEN 2
        WHEN ipa.scope_id = 3 /* local-machine */ THEN 3
        WHEN ipa.scope_id = 4 /* link-local */ THEN 4
        ELSE 5
    END AS scope_rank,
    -- Discovery identifies the IPv4 default route. It provides no preference
    -- between IPv6 addresses, or between secondary addresses on its device.
    CASE
        WHEN ipa.type_id = 0 /* ipv4 */
            AND ipa.is_secondary IS NOT TRUE
            AND lld.is_default_gateway THEN 0
        ELSE 1
    END AS default_route_rank,
    CASE
        WHEN ipa.origin_id = 1 /* provider */ THEN 0
        WHEN ipa.origin_id = 0 /* machine */ THEN 1
        ELSE 2
    END AS origin_rank
FROM ip_address AS ipa
JOIN link_layer_device AS lld ON ipa.device_uuid = lld.uuid
JOIN ip_address_config_type AS iact ON ipa.config_type_id = iact.id
JOIN ip_address_type AS iat ON ipa.type_id = iat.id
JOIN ip_address_origin AS iao ON ipa.origin_id = iao.id
JOIN ip_address_scope AS ias ON ipa.scope_id = ias.id
LEFT JOIN subnet AS sn ON ipa.subnet_uuid = sn.uuid;
