-- Move SSH key comments from global storage (user_public_ssh_key.comment)
-- to per-model storage (model_authorized_keys.comment).
--
-- This allows the same SSH key to be added to multiple models with different comments,
-- fixing the issue where Terraform would detect diffs simply due to the comment
-- when the same key is used in different models.

-- Add comment column to model_authorized_keys.
ALTER TABLE model_authorized_keys
ADD COLUMN comment TEXT;

-- Migrate existing comments from user_public_ssh_key to model_authorized_keys.
UPDATE model_authorized_keys
SET comment = (
    SELECT comment FROM user_public_ssh_key
    WHERE user_public_ssh_key.id = model_authorized_keys.user_public_ssh_key_id
)
WHERE comment IS NULL;

-- Clean up public_key values by removing comments from the end, as we're
-- storing them on the model_authorized_keys table now there's no
-- need to ever store the key with the comment.
UPDATE user_public_ssh_key
SET public_key = (
    SUBSTR(public_key, 1, 
        CASE 
            WHEN INSTR(SUBSTR(public_key, INSTR(public_key, ' ') + 1), ' ') > 0
            THEN INSTR(public_key, ' ') + INSTR(SUBSTR(public_key, INSTR(public_key, ' ') + 1), ' ') - 1
            ELSE LENGTH(public_key)
        END
    )
)
WHERE comment IS NOT NULL;

-- Remove the comment column from user_public_ssh_key as it's no longer needed.
DROP INDEX IF EXISTS idx_user_public_ssh_key_user_comment;
ALTER TABLE user_public_ssh_key
DROP COLUMN comment;

-- Drop the old view..
DROP VIEW IF EXISTS v_model_authorized_keys;

-- Create unique constraint as it was because we don't want 
-- to allow the same key with a new comment adding to the model again.
CREATE UNIQUE INDEX idx_model_authorized_keys_composite
ON model_authorized_keys (model_uuid, user_public_ssh_key_id);

-- Recreate the view with per-model comments.
CREATE VIEW v_model_authorized_keys AS
SELECT
    mak.model_uuid,
    CASE
        WHEN mak.comment IS NOT NULL AND mak.comment != ''
        THEN upsk.public_key || ' ' || mak.comment
        ELSE upsk.public_key
    END AS public_key,
    upsk.user_uuid
FROM model_authorized_keys AS mak
JOIN user_public_ssh_key AS upsk ON mak.user_public_ssh_key_id = upsk.id
JOIN user AS u ON upsk.user_uuid = u.uuid
JOIN user_authentication AS ua ON u.uuid = ua.user_uuid
WHERE
    u.removed = FALSE
    AND ua.disabled = FALSE;
