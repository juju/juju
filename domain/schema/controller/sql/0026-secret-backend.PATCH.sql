/*
 * Copyright 2025 Canonical Ltd.
 * Licensed under the AGPLv3, see LICENCE file for details.
 */

-- Patch 0026: Add origin to secret backends.
--
-- This patch introduces a new table to capture the origin of a
-- secret backend and augments the secret_backend table with an origin_id
-- that references it.

-- Drop the triggers that enforce immutability on the secret_backend table.
DROP TRIGGER trg_secret_backend_immutable_update;
DROP TRIGGER trg_secret_backend_immutable_delete;

-- Create the origin table with the two supported origins.
CREATE TABLE secret_backend_origin (
    id INT NOT NULL PRIMARY KEY,
    origin TEXT NOT NULL UNIQUE,
    CONSTRAINT chk_secret_backend_origin_not_empty
    CHECK (origin <> '')
);

-- Seed known origins.
INSERT INTO secret_backend_origin (id, origin) VALUES
(0, 'built-in'),
(1, 'user');

-- Add the origin_id column to secret_backend; Defaulted to 1 (user).
-- Include a column-level FK reference for SQLite.
ALTER TABLE secret_backend
-- ideally origin_id should be set NOT NULL DEFAULT 1 but can't since sqlite doesn't allows that while ALTER TABLE
ADD COLUMN origin_id INT -- NOT NULL DEFAULT 1
REFERENCES secret_backend_origin (id);

-- Backfill existing secret backends: the built-in ones are named
-- 'internal' (the Juju controller backend) and 'kubernetes'.
UPDATE secret_backend
SET origin_id = 0
WHERE name IN ('internal', 'kubernetes');

-- The rest are user-created.
UPDATE secret_backend
SET origin_id = 1
WHERE name NOT IN ('internal', 'kubernetes');

-- When merging into main, update the file domain/schema/controller.go:
--  - triggersForImmutableTable "secret_backend" needs to be updated to below condition and message
--  - this file needs to be removed
-- Note: the immutability is enforced by business logic, so those triggers are not strictly necessary, but it's
-- nice to have.
CREATE TRIGGER trg_secret_backend_immutable_update -- noqa: PRS
    BEFORE UPDATE ON secret_backend
    FOR EACH ROW
    WHEN OLD.origin_id = 0
BEGIN
    SELECT RAISE(FAIL, 'built-in secret backends are immutable');
END;

CREATE TRIGGER trg_secret_backend_immutable_delete -- noqa: PRS
    BEFORE DELETE ON secret_backend
    FOR EACH ROW
    WHEN OLD.origin_id = 0
BEGIN
    SELECT RAISE(FAIL, 'built-in secret backends are immutable');
END;
