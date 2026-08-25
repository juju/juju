-- Patch 0062: enforce uniqueness on storage_filesystem.provider_id.
--
-- Required so that two concurrent import-filesystem calls cannot both
-- succeed and create distinct storage instances pointing at the same
-- provider filesystem. The old non-unique index is dropped and replaced
-- by a unique one.
--
-- Pre-flight (operators must run manually before upgrading if duplicates
-- may exist):
--     SELECT provider_id, COUNT(*) FROM storage_filesystem
--     GROUP BY provider_id HAVING COUNT(*) > 1;

DROP INDEX idx_storage_filesystem_provider_id;
CREATE UNIQUE INDEX idx_storage_filesystem_provider_id
ON storage_filesystem (provider_id);
