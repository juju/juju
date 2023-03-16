// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

// ControllerDDL is used to create the controller database schema at bootstrap.
func ControllerDDL() []string {
	schemas := []func() string{
		leaseSchema,
		changeLogSchema,
		cloudSchema,
	}

	var deltas []string
	for _, fn := range schemas {
		deltas = append(deltas, fn())
	}

	return deltas
}

func leaseSchema() string {
	return `
CREATE TABLE lease_type (
    id   INT PRIMARY KEY,
    type TEXT
);

CREATE UNIQUE INDEX idx_lease_type_type
ON lease_type (type);

INSERT INTO lease_type VALUES
    (0, 'singular-controller'),    -- The controller running singular controller/model workers.
    (1, 'application-leadership'); -- The unit that holds leadership for an application.

CREATE TABLE lease (
    uuid            TEXT PRIMARY KEY,
    lease_type_id   INT NOT NULL,
    model_uuid      TEXT,
    name            TEXT,
    holder          TEXT,
    start           TIMESTAMP,
    expiry          TIMESTAMP,
    CONSTRAINT      fk_lease_lease_type
        FOREIGN KEY (lease_type_id)
        REFERENCES  lease_type(id)
);

CREATE UNIQUE INDEX idx_lease_model_type_name
ON lease (model_uuid, lease_type_id, name);

CREATE INDEX idx_lease_expiry
ON lease (expiry);

CREATE TABLE lease_pin (
    -- The presence of entries in this table for a particular lease_uuid
    -- implies that the lease in question is pinned and cannot expire.
    uuid       TEXT PRIMARY KEY,
    lease_uuid TEXT,
    entity_id  TEXT,
    CONSTRAINT      fk_lease_pin_lease
        FOREIGN KEY (lease_uuid)
        REFERENCES  lease(uuid)
);

CREATE UNIQUE INDEX idx_lease_pin_lease_entity
ON lease_pin (lease_uuid, entity_id);

CREATE INDEX idx_lease_pin_lease
ON lease_pin (lease_uuid);
`[1:]
}

func changeLogSchema() string {
	return `
CREATE TABLE change_log_edit_type (
    id        INT PRIMARY KEY,
    edit_type TEXT
);

CREATE UNIQUE INDEX idx_change_log_edit_type_edit_type
ON change_log_edit_type (edit_type);

-- The change log type values are bitmasks, so that multiple types can be
-- expressed when looking for changes.
INSERT INTO change_log_edit_type VALUES
    (1, 'create'),
    (2, 'update'),
    (4, 'delete');

CREATE TABLE change_log_namespace (
    id        INT PRIMARY KEY,
    namespace TEXT
);

CREATE UNIQUE INDEX idx_change_log_namespace_namespace
ON change_log_namespace (namespace);

-- TODO (stickupkid): Add the namespaces we want to track.
-- INSERT INTO change_log_namespace VALUES
--    (1, 'foo');

CREATE TABLE change_log (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    edit_type_id        INT NOT NULL,
    namespace_id        INT NOT NULL,
    changed_uuid        TEXT NOT NULL,
    created_at          DATETIME NOT NULL DEFAULT(STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    CONSTRAINT          fk_change_log_edit_type
            FOREIGN KEY (edit_type_id)
            REFERENCES  change_log_edit_type(id),
    CONSTRAINT          fk_change_log_namespace
            FOREIGN KEY (namespace_id)
            REFERENCES  change_log_namespace(id)
);`[1:]
}

func cloudSchema() string {
	return `
CREATE TABLE cloud_type (
    id   INT PRIMARY KEY,
    type TEXT
);

CREATE UNIQUE INDEX idx_cloud_type_type
ON cloud_type (type);

-- The list of all the cloud types that are supported for this release. This
-- doesn't indicate whether the cloud type is supported for the current
-- controller, but rather the cloud type is supported in general.
INSERT INTO cloud_type VALUES
    (0, 'kubernetes'),
    (1, 'lxd'),
    (2, 'maas'),
    (3, 'manual'),
    (4, 'azure'),
    (5, 'ec2'),
    (6, 'equinix'),
    (7, 'gce'),
    (8, 'oci'),
    (9, 'openstack'),
    (10, 'vsphere');

CREATE TABLE cloud_auth_type (
    id   INT PRIMARY KEY,
    type TEXT
);

CREATE UNIQUE INDEX idx_cloud_auth_type_type
ON cloud_auth_type (type);

INSERT INTO cloud_auth_type VALUES
    (0, 'access-key'),
    (1, 'instance-role'),
    (2, 'userpass'),
    (3, 'oauth1'),
    (4, 'oauth2'),
    (5, 'jsonfile'),
    (6, 'clientcertificate'),
    (7, 'httpsig'),
    (8, 'interactive'),
    (9, 'empty'),
    (10, 'certificate'),
    (11, 'oauth2withcert');

CREATE TABLE cloud_auth_types (
    uuid            TEXT PRIMARY KEY,
    auth_type_id    INT NOT NULL
);

CREATE UNIQUE INDEX idx_cloud_auth_types_auth_type_id
ON cloud_auth_types (auth_type_id);

CREATE TABLE cloud_regions (
    uuid            TEXT PRIMARY KEY,
    region_id       INT NOT NULL
);

CREATE UNIQUE INDEX idx_cloud_regions_region_id
ON cloud_regions (region_id);

CREATE TABLE cloud_region (
    id                     INT PRIMARY KEY,
    region                 TEXT NOT NULL,
    endpoint               TEXT,
    identity_endpoint      TEXT,
    storage_endpoint       TEXT
);

CREATE UNIQUE INDEX idx_cloud_region_region
ON cloud_region (region);

CREATE TABLE cloud_certificates (
    uuid            TEXT PRIMARY KEY,
    certificate     TEXT NOT NULL
);

CREATE TABLE cloud (
    uuid                TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    cloud_type_id       INT NOT NULL,
    endpoint            TEXT NOT NULL,
    identity_endpoint   TEXT,
    storage_endpoint    TEXT,
    skip_tls_verify     BOOLEAN NOT NULL
);`[1:]
}
