CREATE TABLE object_store_backend_type (
    id INT NOT NULL PRIMARY KEY,
    "type" TEXT NOT NULL UNIQUE 
);

INSERT INTO object_store_backend_type (id, "type") VALUES
(0, 'file'),
(1, 's3');

CREATE TABLE object_store_backend (
    uuid TEXT NOT NULL PRIMARY KEY,
    life_id INT NOT NULL,
    type_id INT NOT NULL,
    CONSTRAINT fk_object_store_backend_life_id
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_object_store_backend_type_id
    FOREIGN KEY (type_id)
    REFERENCES object_store_backend_type (id)
);

INSERT INTO object_store_backend (uuid, life_id, type_id) VALUES
('f44ea516-22ad-4161-b2bd-cbae9d7a9412', 0, 0);

-- A unique constraint over a constant index ensures only 1 entry matching the
-- condition can exist. In this case only 1 object store backend of type file
-- can exist, but multiple s3 backends can exist.
CREATE UNIQUE INDEX idx_singleton_object_store_backend ON object_store_backend ((1)) WHERE type_id = 0;

-- This index ensures only 1 object store backend can exist with a life_id of 0,
-- which is the life_id used for the file backend. This ensures only 1 file
-- backend can exist at a time.
CREATE UNIQUE INDEX idx_object_store_backend_life_id ON object_store_backend (life_id) WHERE life_id = 0;

CREATE TABLE object_store_backend_s3_credential (
    object_store_backend_uuid INT NOT NULL PRIMARY KEY,
    endpoint TEXT NOT NULL,
    static_key TEXT NOT NULL,
    static_secret TEXT NOT NULL,
    session_token TEXT,
    CONSTRAINT fk_object_store_backend_uuid_s3_credential_object_store_uuid
    FOREIGN KEY (object_store_backend_uuid)
    REFERENCES object_store_backend (uuid)
);

-- Note for merging. This should overwrite the existing object_store_drain_info.
DROP TRIGGER trg_log_object_store_drain_info_insert;
DROP TRIGGER trg_log_object_store_drain_info_update;
DROP TRIGGER trg_log_object_store_drain_info_delete;

DROP TABLE object_store_drain_info;

CREATE TABLE object_store_drain_info (
    uuid TEXT NOT NULL PRIMARY KEY,
    phase_type_id INT NOT NULL,
    from_backend_uuid TEXT,
    to_backend_uuid TEXT NOT NULL,
    CONSTRAINT fk_object_store_drain_info_object_store_drain_phase_type
    FOREIGN KEY (phase_type_id)
    REFERENCES object_store_drain_phase_type (id),
    CONSTRAINT fk_object_store_drain_info_from_object_store_backend_uuid
    FOREIGN KEY (from_backend_uuid)
    REFERENCES object_store_backend (uuid),
    CONSTRAINT fk_object_store_drain_info_to_object_store_backend_uuid
    FOREIGN KEY (to_backend_uuid)
    REFERENCES object_store_backend (uuid)
);

-- Insert the initial draining info for the file backend, which is not draining
-- and is active.
INSERT INTO object_store_drain_info (uuid, phase_type_id, from_backend_uuid, to_backend_uuid) VALUES
('1df5de05-c87b-4b81-b309-8cda26e7df98', 3, NULL, 'f44ea516-22ad-4161-b2bd-cbae9d7a9412');

-- Note for merging. This can be dropped entirely as the triggers will be added
-- to the triggergen tool.
CREATE TRIGGER trg_log_object_store_drain_info_insert
AFTER INSERT ON object_store_drain_info FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10013, NEW.uuid, DATETIME('now', 'utc'));
END;
CREATE TRIGGER trg_log_object_store_drain_info_update
AFTER UPDATE ON object_store_drain_info FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.phase_type_id != OLD.phase_type_id OR
    (NEW.from_backend_uuid != OLD.from_backend_uuid OR (NEW.from_backend_uuid IS NOT NULL AND OLD.from_backend_uuid IS NULL) OR (NEW.from_backend_uuid IS NULL AND OLD.from_backend_uuid IS NOT NULL)) OR
    NEW.to_backend_uuid != OLD.to_backend_uuid
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10013, OLD.uuid, DATETIME('now', 'utc'));
END;
CREATE TRIGGER trg_log_object_store_drain_info_delete
AFTER DELETE ON object_store_drain_info FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10013, OLD.uuid, DATETIME('now', 'utc'));
END;

-- Note for merging. This should be added to the object store triggers via
-- the triggergen tool.

-- insert namespace for ObjectStoreBackend
INSERT INTO change_log_namespace VALUES (10019, 'object_store_backend', 'ObjectStoreBackend changes based on uuid');

-- insert trigger for ObjectStoreBackend
CREATE TRIGGER trg_log_object_store_backend_insert
AFTER INSERT ON object_store_backend FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, 10019, NEW.uuid, DATETIME('now', 'utc'));
END;

-- update trigger for ObjectStoreBackend
CREATE TRIGGER trg_log_object_store_backend_update
AFTER UPDATE ON object_store_backend FOR EACH ROW
WHEN 
	NEW.uuid != OLD.uuid OR
	NEW.life_id != OLD.life_id OR
	NEW.type_id != OLD.type_id 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, 10019, OLD.uuid, DATETIME('now', 'utc'));
END;
-- delete trigger for ObjectStoreBackend
CREATE TRIGGER trg_log_object_store_backend_delete
AFTER DELETE ON object_store_backend FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, 10019, OLD.uuid, DATETIME('now', 'utc'));
END;