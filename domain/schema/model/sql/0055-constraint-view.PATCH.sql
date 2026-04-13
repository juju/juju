DROP VIEW IF EXISTS v_application_constraint;

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
    ctag.rowid AS tag_order,
    cspace.space AS space_name,
    cspace."exclude" AS space_exclude,
    cspace.rowid AS space_order,
    czone.zone,
    czone.rowid AS zone_order
FROM application_constraint AS ac
JOIN "constraint" AS c ON ac.constraint_uuid = c.uuid
LEFT JOIN container_type AS ctype ON c.container_type_id = ctype.id
LEFT JOIN constraint_tag AS ctag ON c.uuid = ctag.constraint_uuid
LEFT JOIN constraint_space AS cspace ON c.uuid = cspace.constraint_uuid
LEFT JOIN constraint_zone AS czone ON c.uuid = czone.constraint_uuid;
