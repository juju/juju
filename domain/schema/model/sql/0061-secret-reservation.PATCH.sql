-- secret_reservation tracks secret IDs that have been minted by
-- CreateSecretURIs but not yet persisted as charm secrets. Each
-- reservation is keyed to the unit that requested it so that
-- backend write authority can be granted only for IDs the
-- requesting unit actually reserved. Rows are consumed (deleted)
-- when the secret is created during the unit-state commit hook.
CREATE TABLE secret_reservation (
    secret_id TEXT NOT NULL PRIMARY KEY,
    unit_uuid TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT fk_secret_reservation_unit_uuid
    FOREIGN KEY (unit_uuid)
    REFERENCES unit (uuid)
);

CREATE INDEX idx_secret_reservation_unit_uuid
ON secret_reservation (unit_uuid);
