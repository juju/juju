/*
 * Copyright 2026 Canonical Ltd.
 * Licensed under the AGPLv3, see LICENCE file for details.
 */

-- Patch 0030: Add secret_id to secret_backend_reference.
--
-- This patch adds the secret_id column (the logical secret identifier, not
-- the revision UUID) to the secret_backend_reference table so that
-- listing backends can count unique secrets rather than unique revisions.
--
-- SQLite does not allow NOT NULL without a DEFAULT in ALTER TABLE, so the
-- column is nullable. Existing rows will have NULL; new rows will always
-- carry the value set by the application. The listing query uses
-- COALESCE(secret_id, secret_revision_uuid) so that pre-existing rows
-- continue to produce a count rather than silently disappearing.
--
-- TODO WHILE MERGING into main:
--    * Add a NOT NULL constraint to secret_id column,
--    * remove the COALESCE(secret_id, secret_revision_uuid) expression,
--    * update the listing query to use only secret_id rather than COALESCE,
ALTER TABLE secret_backend_reference ADD COLUMN secret_id TEXT;
