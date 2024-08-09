CREATE TABLE net_node (
    uuid TEXT PRIMARY KEY
);

CREATE TABLE cloud_service (
    net_node_uuid TEXT NOT NULL PRIMARY KEY,
    application_uuid TEXT NOT NULL,
    CONSTRAINT fk_cloud_service_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid),
    CONSTRAINT fk_cloud_application
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid)
);

CREATE UNIQUE INDEX idx_cloud_service_application
ON cloud_service (application_uuid);

CREATE TABLE cloud_container (
    net_node_uuid TEXT NOT NULL PRIMARY KEY,
    provider_id TEXT NOT NULL,
    CONSTRAINT fk_cloud_container_net_node
    FOREIGN KEY (net_node_uuid)
    REFERENCES net_node (uuid)
);
