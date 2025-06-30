CREATE TABLE permission_access_type (
    id INT PRIMARY KEY,
    type TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_permission_access_type
ON permission_access_type (type);

-- Maps to the Access type in core/permission package.
INSERT INTO permission_access_type VALUES
(0, 'read'),
(1, 'write'),
(2, 'consume'),
(3, 'admin'),
(4, 'login'),
(5, 'add-model'),
(6, 'superuser');

CREATE TABLE permission_object_type (
    id INT PRIMARY KEY,
    type TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_permission_object_type
ON permission_object_type (type);

-- Maps to the ObjectType type in core/permission package.
INSERT INTO permission_object_type VALUES
(0, 'cloud'),
(1, 'controller'),
(2, 'model'),
(3, 'offer');

CREATE TABLE permission_object_access (
    id INT PRIMARY KEY,
    access_type_id INT NOT NULL,
    object_type_id INT NOT NULL,
    CONSTRAINT fk_permission_access_type
    FOREIGN KEY (access_type_id)
    REFERENCES permission_access_type (id),
    CONSTRAINT fk_permission_object_type
    FOREIGN KEY (object_type_id)
    REFERENCES permission_object_type (id)
);

CREATE UNIQUE INDEX idx_permission_object_access
ON permission_object_access (access_type_id, object_type_id);

INSERT INTO permission_object_access VALUES
(0, 3, 0), -- admin, cloud
(1, 5, 0), -- add-model, cloud
(2, 4, 1), -- login, controller
(3, 6, 1), -- superuser, controller
(4, 0, 2), -- read, model
(5, 1, 2), -- write, model
(6, 3, 2), -- admin, model
(7, 0, 3), -- read, offer
(8, 2, 3), -- consume, offer
(9, 3, 3); -- admin, offer

-- Column grant_to may extend to entities beyond users.
-- The name of the column is general, but for now we retain the FK constraint.
-- We will need to remove/replace it in the event of change
CREATE TABLE permission (
    uuid TEXT NOT NULL PRIMARY KEY,
    access_type_id INT NOT NULL,
    object_type_id INT NOT NULL,
    grant_on TEXT NOT NULL, -- name or uuid of the object
    grant_to TEXT NOT NULL,
    CONSTRAINT fk_permission_user_uuid
    FOREIGN KEY (grant_to)
    REFERENCES user (uuid),
    CONSTRAINT fk_permission_object_access
    FOREIGN KEY (access_type_id, object_type_id)
    REFERENCES permission_object_access (access_type_id, object_type_id)
);

-- Allow only 1 combination of grant_on and grant_to
-- Otherwise we will get conflicting permissions.
CREATE UNIQUE INDEX idx_permission_type_to
ON permission (grant_on, grant_to);

-- All permissions
CREATE VIEW v_permission AS
SELECT
    p.uuid,
    p.grant_on,
    p.grant_to,
    at.type AS access_type,
    ot.type AS object_type
FROM permission AS p
JOIN permission_access_type AS at ON p.access_type_id = at.id
JOIN permission_object_type AS ot ON p.object_type_id = ot.id;

-- All model permissions, verifying the model does exist.
CREATE VIEW v_permission_model AS
SELECT
    p.uuid,
    p.grant_on,
    p.grant_to,
    p.access_type,
    p.object_type
FROM v_permission AS p
JOIN model ON p.grant_on = model.uuid
WHERE p.object_type = 'model';

-- All controller cloud, verifying the cloud does exist.
CREATE VIEW v_permission_cloud AS
SELECT
    p.uuid,
    p.grant_on,
    p.grant_to,
    p.access_type,
    p.object_type
FROM v_permission AS p
JOIN cloud ON p.grant_on = cloud.name
WHERE p.object_type = 'cloud';

-- All controller permissions, verifying the controller does exists.
CREATE VIEW v_permission_controller AS
SELECT
    p.uuid,
    p.grant_on,
    p.grant_to,
    p.access_type,
    p.object_type
FROM v_permission AS p
JOIN controller ON p.grant_on = controller.uuid
WHERE p.object_type = 'controller';

-- All offer permissions, NOT verifying the offer does exist.
CREATE VIEW v_permission_offer AS
SELECT
    p.uuid,
    p.grant_on,
    p.grant_to,
    p.access_type,
    p.object_type
FROM v_permission AS p
WHERE p.object_type = 'offer';

-- The permissions for the special user everyone@external.
CREATE VIEW v_everyone_external AS
SELECT
    p.uuid,
    p.grant_on,
    p.grant_to,
    p.access_type,
    p.object_type
FROM v_permission AS p
JOIN user AS u ON p.grant_to = u.uuid
WHERE u.name = 'everyone@external';
