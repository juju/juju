/**
  this patch is a placeholder to remember to remove DEFAULT values from secret_metadata and secret_revision DATETIME
  fields. Those fields should be populated by the application, and needs to be removed from the schema.

  However, this is not trivial to delete as a PATCH a default value, because it can be done only by dropping and adding
  the column, which is not possible if the column is not nullable. Another way would be to drop the table and
  recreate it, but that would be a big pain.
 */
