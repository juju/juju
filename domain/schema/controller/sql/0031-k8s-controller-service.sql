-- k8s_controller_service records the Kubernetes controller Service
-- bootstrap configuration (controller-service-type, controller-external-name
-- and controller-external-ips): exactly one row for controllers bootstrapped
-- on Kubernetes, referencing the controller it belongs to. Machine controllers
-- have no row. Empty values mean the provider default service spec for the
-- cloud type. Controller recovery reads the row from the backup dump to
-- provision a replacement Service with the same exposure as the source
-- controller.
CREATE TABLE k8s_controller_service (
    controller_uuid TEXT NOT NULL PRIMARY KEY,
    service_type TEXT NOT NULL,
    external_name TEXT NOT NULL,
    external_ips TEXT NOT NULL,
    CONSTRAINT fk_k8s_controller_service_controller_uuid
    FOREIGN KEY (controller_uuid)
    REFERENCES controller (uuid)
);
