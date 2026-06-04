-- Patch 0032: Add indexes used by controller schema views.

CREATE INDEX idx_agent_binary_store_object_store_uuid
ON agent_binary_store (object_store_uuid);

CREATE INDEX idx_object_store_metadata_path_metadata_uuid
ON object_store_metadata_path (metadata_uuid);

CREATE INDEX idx_permission_grant_to
ON permission (grant_to);

CREATE INDEX idx_permission_object_type_grant_on
ON permission (object_type_id, grant_on);

CREATE INDEX idx_user_name
ON user (name);
