-- The application_provider_storage_id table stores information about a
-- migrated CAAS application's storage unique ID from prior Juju 4.0 versions.
-- For new applications deployed on 4.0, the application will have no entry here.
CREATE TABLE application_provider_storage_id (
    application_uuid TEXT NOT NULL,
    storage_name TEXT NOT NULL,
    storage_unique_id TEXT NOT NULL,
    PRIMARY KEY (storage_name, storage_unique_id),
    CONSTRAINT fk_application_provider_storage_id_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
)
