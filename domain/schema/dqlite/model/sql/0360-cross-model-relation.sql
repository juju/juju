-- application_remote_offerer represents a remote offerer application
-- inside of the consumer model.
CREATE TABLE application_remote_offerer (
    uuid TEXT NOT NULL PRIMARY KEY,
    life_id INT NOT NULL,
    -- application_uuid is the synthetic application in the consumer model.
    -- Locating charm is done through the application.
    application_uuid TEXT NOT NULL,
    -- offer_uuid is the offer uuid that ties both the offerer and the consumer
    -- together.
    offer_uuid TEXT NOT NULL,
    -- offer_url is the URL of the offer that the remote application is
    -- consuming.
    offer_url TEXT NOT NULL,
    -- offerer_controller_uuid is the offering controller where the
    -- offerer application is located. There is no FK constraint on it,
    -- because that information is located in the controller DB.
    offerer_controller_uuid TEXT,
    -- offerer_model_uuid is the model in the offering controller where
    -- the offerer application is located. There is no FK constraint on it,
    -- because we don't have the model locally.
    offerer_model_uuid TEXT NOT NULL,
    -- macaroon represents the credentials to access the offering model.
    macaroon TEXT NOT NULL,
    CONSTRAINT fk_application_uuid
    FOREIGN KEY (application_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_life_id
    FOREIGN KEY (life_id)
    REFERENCES life (id)
);

-- Ensure that an offer can only be consumed once in a model.
CREATE UNIQUE INDEX idx_application_remote_offerer_offer_uuid
ON application_remote_offerer (offer_uuid);

-- Ensure that an application can only be used once as a remote offerer.
CREATE UNIQUE INDEX idx_application_remote_offerer_application_uuid
ON application_remote_offerer (application_uuid);

CREATE INDEX idx_application_remote_offerer_model
ON application_remote_offerer (offerer_model_uuid);

-- application_remote_offerer_status represents the status of the remote
-- offerer application inside of the consumer model.
CREATE TABLE application_remote_offerer_status (
    application_remote_offerer_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    data TEXT,
    updated_at DATETIME,
    CONSTRAINT fk_application_remote_offerer_status
    FOREIGN KEY (application_remote_offerer_uuid)
    REFERENCES application_remote_offerer (uuid),
    CONSTRAINT fk_workload_status_value_status
    FOREIGN KEY (status_id)
    REFERENCES workload_status_value (id)
);

-- application_remote_offerer_relation_macaroon represents the macaroon
-- used to authenticate against the offering model for a given relation.
CREATE TABLE application_remote_offerer_relation_macaroon (
    relation_uuid TEXT NOT NULL PRIMARY KEY,
    macaroon TEXT NOT NULL,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid)
);

-- offer connection links the application remote consumer to the offer.
CREATE TABLE offer_connection (
    uuid TEXT NOT NULL PRIMARY KEY,
    -- offer_uuid is the offer that the remote application is using.
    offer_uuid TEXT NOT NULL,
    -- remote_relation_uuid is the relation for which the offer connection
    -- is made. It uses the relation, as we can identify both the
    -- relation id and the relation key from it.
    remote_relation_uuid TEXT NOT NULL,
    -- username is the user in the consumer model that created the offer
    -- connection. This is not a user, but an offer user for which offers are
    -- granted permissions on.
    username TEXT NOT NULL,
    CONSTRAINT fk_offer_uuid
    FOREIGN KEY (offer_uuid)
    REFERENCES offer (uuid),
    CONSTRAINT fk_remote_relation_uuid
    FOREIGN KEY (remote_relation_uuid)
    REFERENCES relation (uuid)
);


CREATE VIEW v_offer_detail AS
WITH total_conn AS (
    SELECT
        offer_uuid,
        COUNT(*) AS total_connections
    FROM offer_connection
    GROUP BY offer_uuid
),

active_conn AS (
    SELECT
        oc.offer_uuid,
        COUNT(*) AS total_active_connections
    FROM offer_connection AS oc
    JOIN relation_status AS rs ON oc.remote_relation_uuid = rs.relation_uuid AND rs.relation_status_type_id = 1
    GROUP BY oc.offer_uuid
)

SELECT
    o.uuid AS offer_uuid,
    o.name AS offer_name,
    a.name AS application_name,
    c.reference_name AS charm_name,
    c.revision AS charm_revision,
    cs.name AS charm_source,
    c.architecture_id AS charm_architecture,
    cr.name AS endpoint_name,
    crr.name AS endpoint_role,
    cr.interface AS endpoint_interface,
    cr.capacity AS endpoint_limit,
    COALESCE(tc.total_connections, 0) AS total_connections,
    COALESCE(ac.total_active_connections, 0) AS total_active_connections,
    COALESCE(o.description, cm.description) AS application_description
FROM offer AS o
JOIN offer_endpoint AS oe ON o.uuid = oe.offer_uuid
JOIN application_endpoint AS ae ON oe.endpoint_uuid = ae.uuid
JOIN application AS a ON ae.application_uuid = a.uuid
JOIN charm AS c ON a.charm_uuid = c.uuid
JOIN charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
JOIN charm_relation_role AS crr ON cr.role_id = crr.id
JOIN charm_source AS cs ON c.source_id = cs.id
JOIN charm_metadata AS cm ON c.uuid = cm.charm_uuid
LEFT JOIN total_conn AS tc ON o.uuid = tc.offer_uuid
LEFT JOIN active_conn AS ac ON o.uuid = ac.offer_uuid;

CREATE INDEX IF NOT EXISTS idx_offer_connection_offer_uuid
ON offer_connection (offer_uuid);

CREATE INDEX IF NOT EXISTS idx_offer_connection_remote_relation_offer
ON offer_connection (remote_relation_uuid, offer_uuid);


-- application_remote_consumer represents a remote consumer application
-- inside of the offering model.
CREATE TABLE application_remote_consumer (
    -- offer_connection_uuid is the offer connection that links the remote
    -- consumer to the offer. This is the same UUID as the synthetic application
    -- that represents the remote application in the consumer model.
    offer_connection_uuid TEXT NOT NULL PRIMARY KEY,
    -- offerer_application_uuid is application UUID of the offer in the offering
    -- model.
    offerer_application_uuid TEXT NOT NULL,
    -- consumed_application_uuid is the (remote token) application UUID in 
    -- the consumer model.
    consumer_application_uuid TEXT NOT NULL,
    -- consumer_model_uuid is the model in the consuming controller where
    -- the consumer application is located. There is no FK constraint on it,
    -- because we don't have the model locally.
    consumer_model_uuid TEXT NOT NULL,
    life_id INT NOT NULL,
    CONSTRAINT fk_life_id
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_offerer_application_uuid
    FOREIGN KEY (offerer_application_uuid)
    REFERENCES application (uuid),
    -- This is correct, the offer_connection_uuid is both the connection_uuid
    -- and the application uuid representing the synth application. The
    CONSTRAINT fk_offer_connection_uuid
    FOREIGN KEY (offer_connection_uuid)
    REFERENCES application (uuid),
    CONSTRAINT fk_offer_connection_uuid
    FOREIGN KEY (offer_connection_uuid)
    REFERENCES offer_connection (uuid)
);

CREATE INDEX idx_application_remote_consumer_consumer
ON application_remote_consumer (consumer_application_uuid);

CREATE INDEX idx_application_remote_consumer_life
ON application_remote_consumer (life_id);

CREATE INDEX idx_application_remote_consumer_offerer
ON application_remote_consumer (offerer_application_uuid);

-- relation_network_ingress holds information about ingress CIDRs for a 
-- relation.
CREATE TABLE relation_network_ingress (
    relation_uuid TEXT NOT NULL,
    cidr TEXT NOT NULL,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    PRIMARY KEY (relation_uuid, cidr)
);

-- relation_network_egress holds information about egress CIDRs for a relation.
CREATE TABLE relation_network_egress (
    relation_uuid TEXT NOT NULL,
    cidr TEXT NOT NULL,
    CONSTRAINT fk_relation_uuid
    FOREIGN KEY (relation_uuid)
    REFERENCES relation (uuid),
    PRIMARY KEY (relation_uuid, cidr)
);
