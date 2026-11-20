CREATE TABLE offer (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT
);

CREATE INDEX idx_offer_name
ON offer (name);

-- The offer_endpoint table is a join table to indicate which application
-- endpoints are included in the offer.
--
-- Note: trg_ensure_single_app_per_offer ensures that for every offer,
-- each endpoint_uuid is for the same application.
CREATE TABLE offer_endpoint (
    offer_uuid TEXT NOT NULL,
    endpoint_uuid TEXT NOT NULL,
    PRIMARY KEY (offer_uuid, endpoint_uuid),
    CONSTRAINT fk_endpoint_uuid
    FOREIGN KEY (endpoint_uuid)
    REFERENCES application_endpoint (uuid),
    CONSTRAINT fk_offer_uuid
    FOREIGN KEY (offer_uuid)
    REFERENCES offer (uuid)
);

CREATE UNIQUE INDEX idx_offer_endpoint_endpoint_offer
ON offer_endpoint (endpoint_uuid, offer_uuid);
