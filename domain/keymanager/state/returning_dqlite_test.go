// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build dqlite && linux

package state

import (
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"
)

// TestReturningWriteConflictWorkaroundStillRequired detects when the bundled
// dqlite includes canonical/dqlite#918. When it fails, review the keymanager
// insert/lookup workaround and other RETURNING callers before removing it.
// Once fixed, test the correct behaviour and remove the dqlite build tag so
// the test also runs on SQLite.
func (s *stateSuite) TestReturningWriteConflictWorkaroundStillRequired(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keys := generatePublicKeys(c, testingPublicKeys[:2])
	stmt, err := state.Prepare(`
INSERT INTO user_public_ssh_key (comment, fingerprint, public_key, user_uuid,
                                 fingerprint_hash_algorithm_id)
SELECT $userPublicKeyInsert.comment,
       $userPublicKeyInsert.fingerprint,
       $userPublicKeyInsert.public_key,
       $userPublicKeyInsert.user_uuid,
       s.id
FROM ssh_fingerprint_hash_algorithm AS s
WHERE s.algorithm = $userPublicKeyInsert.algorithm
RETURNING &userPublicKeyId.id
`, userPublicKeyInsert{}, userPublicKeyId{})
	c.Assert(err, tc.ErrorIsNil)

	tx, err := sqlair.NewDB(s.DB()).Begin(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() {
		if err := tx.Rollback(); err != sql.ErrTxDone {
			c.Check(err, tc.ErrorIsNil)
		}
	})
	err = state.checkUserExists(c.Context(), s.userId, tx)
	c.Assert(err, tc.ErrorIsNil)

	// Establish a stale snapshot by committing a competing write.
	err = state.EnsurePublicKeysForUser(c.Context(), s.modelId, s.userId, keys[:1])
	c.Assert(err, tc.ErrorIsNil)

	key := keys[1]
	var row userPublicKeyId
	err = tx.Query(c.Context(), stmt, userPublicKeyInsert{
		Comment:                  key.Comment,
		FingerprintHashAlgorithm: key.FingerprintHash.String(),
		Fingerprint:              key.Fingerprint,
		PublicKey:                key.Key,
		UserId:                   s.userId.String(),
	}).Get(&row)
	c.Check(err, tc.ErrorMatches, ".*_sqlair_0.*", tc.Commentf(
		"dqlite RETURNING error behaviour changed; review workarounds for canonical/dqlite#918",
	))
}
