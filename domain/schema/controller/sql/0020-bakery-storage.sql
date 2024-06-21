CREATE TABLE bakery_config (
    local_users_private_key BLOB NOT NULL,
    local_users_public_key BLOB NOT NULL,
    local_users_third_party_private_key BLOB NOT NULL,
    local_users_third_party_public_key BLOB NOT NULL,
    external_users_third_party_private_key BLOB NOT NULL,
    external_users_third_party_public_key BLOB NOT NULL,
    offers_third_party_private_key BLOB NOT NULL,
    offers_third_party_public_key BLOB NOT NULL
);

-- A unique constraint over a constant index ensures only 1 entry matching the
-- condition can exist.
CREATE UNIQUE INDEX idx_singleton_bakery_config ON bakery_config ((1));
