-- controller_introduction_nonce stores a per-ordinal nonce used to
-- authenticate a controller pod during UnitIntroduction. Nonces are
-- generated before the StatefulSet is created and bound to a specific
-- ordinal in the ConfigMap. The correct nonce must be presented by the
-- pod's init container to prove it is the legitimate pod for that
-- ordinal. The row is verified (not consumed) on each introduction
-- attempt. Idempotency is provided by the password insert-if-absent
-- guard, not by nonce consumption.
CREATE TABLE controller_introduction_nonce (
    controller_id TEXT NOT NULL PRIMARY KEY,
    nonce TEXT NOT NULL
);