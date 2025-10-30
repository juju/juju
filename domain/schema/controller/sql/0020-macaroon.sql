CREATE TABLE bakery_config (
    local_users_private_key BLOB NOT NULL,
    local_users_public_key BLOB NOT NULL,
    local_users_third_party_private_key BLOB NOT NULL,
    local_users_third_party_public_key BLOB NOT NULL,
    external_users_third_party_private_key BLOB NOT NULL,
    external_users_third_party_public_key BLOB NOT NULL,
    offers_third_party_private_key BLOB NOT NULL,
    offers_third_party_public_key BLOB NOT NULL
) STRICT;

-- A unique constraint over a constant index ensures only 1 entry matching the
-- condition can exist.
CREATE UNIQUE INDEX idx_singleton_bakery_config ON bakery_config ((1));

CREATE TABLE macaroon_root_key (
    id TEXT NOT NULL PRIMARY KEY,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    root_key BLOB NOT NULL
) STRICT;
