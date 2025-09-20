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
    -- version is the unique version number that is incremented when the 
    -- consumer model changes the offerer application.
    version INT NOT NULL,
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

-- application_remote_consumer represents a remote consumer application
-- inside of the offering model.
CREATE TABLE application_remote_consumer (
    uuid TEXT NOT NULL PRIMARY KEY,
    life_id INT NOT NULL,
    -- offer_connection_uuid is the offer connection that links the remote
    -- consumer to the offer.
    offer_connection_uuid TEXT NOT NULL,
    -- version is the unique version number that is incremented when the
    -- consumer model changes the consumer application.
    version INT NOT NULL,
    CONSTRAINT fk_life_id
    FOREIGN KEY (life_id)
    REFERENCES life (id),
    CONSTRAINT fk_offer_connection_uuid
    FOREIGN KEY (offer_connection_uuid)
    REFERENCES offer_connection (uuid)
);

-- application_remote_relation represents a look up table to find the consumer
-- relation UUID for a given local relation.
CREATE TABLE application_remote_relation (
    uuid TEXT NOT NULL PRIMARY KEY,
    -- relation_uuid is the local relation UUID.
    relation_uuid TEXT NOT NULL,
    -- consumer_relation_uuid is the relation UUID in the consumer model.
    -- There is no FK constraint on it, because we don't have the relation
    -- locally in the model.
    consumer_relation_uuid TEXT NOT NULL,
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
