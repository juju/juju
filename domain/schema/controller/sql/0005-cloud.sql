-- The cloud and accompanying tables drive the provider tracker. It is not safe
-- to modify the cloud or other tables in a patch/build release. Only make 
-- changes to this table during a major/minor release. Changes to the cloud
-- table will cause undefined behaviour in the provider tracker.
CREATE TABLE cloud_type (
    id INTEGER PRIMARY KEY,
    type TEXT NOT NULL
) STRICT;

CREATE UNIQUE INDEX idx_cloud_type_type
ON cloud_type (type);

-- The list of all the cloud types that are supported for this release. This
-- doesn't indicate whether the cloud type is supported for the current
-- controller, but rather the cloud type is supported in general.
INSERT INTO cloud_type VALUES
(0, 'kubernetes'),
(1, 'lxd'),
(2, 'maas'),
(3, 'unmanaged'),
(4, 'azure'),
(5, 'ec2'),
(6, 'gce'),
(7, 'oci'),
(8, 'openstack'),
(9, 'vsphere');

CREATE TABLE auth_type (
    id INTEGER PRIMARY KEY,
    type TEXT
) STRICT;

CREATE UNIQUE INDEX idx_auth_type_type
ON auth_type (type);

INSERT INTO auth_type VALUES
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
(11, 'oauth2withcert'),
(12, 'service-principal-secret'),
(13, 'managed-identity'),
(14, 'service-account');

CREATE TABLE cloud (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    cloud_type_id INT NOT NULL,
    endpoint TEXT NOT NULL,
    identity_endpoint TEXT,
    storage_endpoint TEXT,
    skip_tls_verify INTEGER NOT NULL,
    CONSTRAINT chk_name_empty CHECK (name != ''),
    CONSTRAINT fk_cloud_type
    FOREIGN KEY (cloud_type_id)
    REFERENCES cloud_type (id)
) STRICT;

CREATE TABLE cloud_defaults (
    cloud_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT,
    PRIMARY KEY (cloud_uuid, "key"),
    CONSTRAINT chk_key_empty CHECK ("key" != ''),
    CONSTRAINT fk_cloud_uuid
    FOREIGN KEY (cloud_uuid)
    REFERENCES cloud (uuid)
) STRICT;

CREATE TABLE cloud_auth_type (
    cloud_uuid TEXT NOT NULL,
    auth_type_id INT NOT NULL,
    CONSTRAINT fk_cloud_auth_type_cloud
    FOREIGN KEY (cloud_uuid)
    REFERENCES cloud (uuid),
    CONSTRAINT fk_cloud_auth_type_auth_type
    FOREIGN KEY (auth_type_id)
    REFERENCES auth_type (id),
    PRIMARY KEY (cloud_uuid, auth_type_id)
);

CREATE UNIQUE INDEX idx_cloud_auth_type_cloud_uuid_auth_type_id
ON cloud_auth_type (cloud_uuid, auth_type_id);

CREATE TABLE cloud_region (
    uuid TEXT NOT NULL PRIMARY KEY,
    cloud_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    endpoint TEXT,
    identity_endpoint TEXT,
    storage_endpoint TEXT,
    CONSTRAINT fk_cloud_region_cloud
    FOREIGN KEY (cloud_uuid)
    REFERENCES cloud (uuid)
) STRICT;

CREATE UNIQUE INDEX idx_cloud_region_cloud_uuid_name
ON cloud_region (cloud_uuid, name);

CREATE INDEX idx_cloud_region_cloud_uuid
ON cloud_region (cloud_uuid);

CREATE TABLE cloud_region_defaults (
    region_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value TEXT,
    PRIMARY KEY (region_uuid, "key"),
    CONSTRAINT chk_key_empty CHECK ("key" != ''),
    CONSTRAINT fk_region_uuid
    FOREIGN KEY (region_uuid)
    REFERENCES cloud_region (uuid)
) STRICT;

CREATE TABLE cloud_ca_cert (
    cloud_uuid TEXT NOT NULL,
    ca_cert TEXT NOT NULL,
    CONSTRAINT fk_cloud_ca_cert_cloud
    FOREIGN KEY (cloud_uuid)
    REFERENCES cloud (uuid),
    PRIMARY KEY (cloud_uuid, ca_cert)
) STRICT;

CREATE UNIQUE INDEX idx_cloud_ca_cert_cloud_uuid_ca_cert
ON cloud_ca_cert (cloud_uuid, ca_cert);

CREATE TABLE cloud_credential (
    uuid TEXT NOT NULL PRIMARY KEY,
    cloud_uuid TEXT NOT NULL,
    auth_type_id TEXT NOT NULL,
    owner_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    revoked INTEGER,
    invalid INTEGER,
    invalid_reason TEXT,
    CONSTRAINT chk_name_empty CHECK (name != ''),
    CONSTRAINT fk_cloud_credential_cloud
    FOREIGN KEY (cloud_uuid)
    REFERENCES cloud (uuid),
    CONSTRAINT fk_cloud_credential_auth_type
    FOREIGN KEY (auth_type_id)
    REFERENCES auth_type (id),
    CONSTRAINT fk_cloud_credential_user
    FOREIGN KEY (owner_uuid)
    REFERENCES user (uuid)
) STRICT;

CREATE UNIQUE INDEX idx_cloud_credential_cloud_uuid_owner_uuid
ON cloud_credential (cloud_uuid, owner_uuid, name);

-- view_cloud_credential is a convenience view for accessing a
-- credential UUID based on the natural key used to display the
-- credential to users.
CREATE VIEW v_cloud_credential
AS
SELECT
    cc.uuid,
    cc.cloud_uuid,
    c.name AS cloud_name,
    cc.auth_type_id,
    at.type AS auth_type,
    cc.owner_uuid,
    cc.name,
    cc.revoked,
    cc.invalid,
    cc.invalid_reason,
    u.name AS owner_name
FROM cloud_credential AS cc
JOIN cloud AS c ON cc.cloud_uuid = c.uuid
JOIN user AS u ON cc.owner_uuid = u.uuid
JOIN auth_type AS at ON cc.auth_type_id = at.id;

CREATE TABLE cloud_credential_attribute (
    cloud_credential_uuid TEXT NOT NULL,
    "key" TEXT NOT NULL,
    value BLOB,
    PRIMARY KEY (cloud_credential_uuid, "key"),
    CONSTRAINT chk_key_empty CHECK ("key" != ''),
    CONSTRAINT fk_cloud_credential_uuid
    FOREIGN KEY (cloud_credential_uuid)
    REFERENCES cloud_credential (uuid)
) STRICT;

-- v_cloud_credential_attribute returns a view of all cloud credentials
-- and their attributes repeated for every attribute.
CREATE VIEW v_cloud_credential_attribute
AS
SELECT
    cc.uuid,
    cc.cloud_uuid,
    cc.auth_type_id,
    cc.auth_type,
    cc.owner_uuid,
    cc.name,
    cc.revoked,
    cc.invalid,
    cc.invalid_reason,
    cc.cloud_name,
    cc.owner_name,
    cca."key" AS attribute_key,
    cca.value AS attribute_value
FROM v_cloud_credential AS cc
JOIN cloud_credential_attribute AS cca ON cc.uuid = cca.cloud_credential_uuid;
