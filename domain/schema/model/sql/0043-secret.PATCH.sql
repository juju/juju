/**
  IMPORTANT FOR MERGE:

  This comment is a placeholder to remember to remove DEFAULT values from secret_metadata and secret_revision DATETIME
  fields. Those fields should be populated by the application and need to be removed from the schema.

  However, this is not trivial to delete as a PATCH a default value, because it can be done only by dropping and adding
  the column, which is not possible if the column is not nullable. Another way would be to drop the table and
  recreate it, but that would be a big pain.
 */

-- Secret revision have update time too. A revision is updated when the expiry time is updated, even if the revision
-- content is not changed.
 ALTER TABLE secret_revision_expire ADD COLUMN update_time DATETIME; -- NOT NULL should be added on merge
