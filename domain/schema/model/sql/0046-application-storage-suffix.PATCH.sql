-- The application_storage_suffix table stores information about a
-- migrated CAAS application's storage unique ID from prior Juju 4.0 versions.
-- For new applications deployed on 4.0, the application will have no entry here.
CREATE TABLE application_storage_suffix (
    application_uuid TEXT NOT NULL,
    storage_unique_id TEXT NOT NULL,
    PRIMARY KEY (application_uuid, storage_unique_id),
    CONSTRAINT fk_application_storage_suffix_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
)
