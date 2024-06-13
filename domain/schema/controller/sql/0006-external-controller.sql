CREATE TABLE external_controller (
    uuid TEXT NOT NULL PRIMARY KEY,
    alias TEXT,
    ca_cert TEXT NOT NULL
);

CREATE TABLE external_controller_address (
    uuid TEXT NOT NULL PRIMARY KEY,
    controller_uuid TEXT NOT NULL,
    address TEXT NOT NULL,
    CONSTRAINT fk_external_controller_address_external_controller_uuid
    FOREIGN KEY (controller_uuid)
    REFERENCES external_controller (uuid)
);

CREATE UNIQUE INDEX idx_external_controller_address
ON external_controller_address (controller_uuid, address);

CREATE TABLE external_model (
    uuid TEXT NOT NULL PRIMARY KEY,
    controller_uuid TEXT NOT NULL,
    CONSTRAINT fk_external_model_external_controller_uuid
    FOREIGN KEY (controller_uuid)
    REFERENCES external_controller (uuid)
);
