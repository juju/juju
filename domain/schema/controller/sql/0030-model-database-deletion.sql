-- model_database_deletion stages deletions of a purged model's dqlite
-- database for the model DB deleter worker to process asynchronously.
--
-- When a model is purged from the controller database while its dqlite
-- database must outlive the purge transaction (source-side model migration
-- REAP), a row is staged here inside that same transaction. The model DB
-- deleter worker on each controller node watches this table, deletes the
-- database, and removes the row on success, retrying on failure.
--
-- The table is deliberately standalone with no FK to model: the model row is
-- gone by the time a row exists here.
CREATE TABLE model_database_deletion (
    namespace TEXT NOT NULL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL
);
