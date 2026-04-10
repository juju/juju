CREATE TABLE IF NOT EXISTS application_k8s_resources_managed (
    application_uuid TEXT NOT NULL PRIMARY KEY,
    CONSTRAINT fk_application_k8s_resources_managed_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);
