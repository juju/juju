// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type secretSuite struct {
	baseSuite
}

func TestSecretSuite(t *testing.T) {
	tc.Run(t, &secretSuite{})
}

func (s *secretSuite) TestRemoveSecretBackendReference(c *tc.C) {
	backendID := s.insertSecretBackend(c)

	modelUUID := s.uuid.String()
	secretRevisionID1 := uuid.MustNewUUID().String()
	secretRevisionID2 := uuid.MustNewUUID().String()

	s.insertSecretBackendReference(c, backendID, modelUUID, secretRevisionID1)
	s.insertSecretBackendReference(c, backendID, modelUUID, secretRevisionID2)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	assertSecretBackendReferenceCount(c, s.DB(), backendID, 2)

	err := st.RemoveSecretBackendReference(c.Context(), secretRevisionID1)
	c.Assert(err, tc.ErrorIsNil)
	assertSecretBackendReferenceCount(c, s.DB(), backendID, 1)

	err = st.RemoveSecretBackendReference(c.Context(), secretRevisionID2)
	c.Assert(err, tc.ErrorIsNil)
	assertSecretBackendReferenceCount(c, s.DB(), backendID, 0)
}

// insertSecretBackend inserts a vault-type secret backend into the DB and
// returns its UUID.
func (s *secretSuite) insertSecretBackend(c *tc.C) string {
	backendUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO secret_backend (uuid, name, backend_type_id)
VALUES (?, ?, (SELECT id FROM secret_backend_type WHERE type = 'vault'))`,
			backendUUID, "test-vault-backend")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return backendUUID
}

// insertSecretBackendReference inserts a single row into
// secret_backend_reference for the given backendID, modelUUID and
// secretRevisionID.
func (s *secretSuite) insertSecretBackendReference(c *tc.C, backendID, modelUUID, secretRevisionID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO secret_backend_reference (secret_backend_uuid, model_uuid, secret_revision_uuid, secret_id)
VALUES (?, ?, ?, ?)`,
			backendID, modelUUID, secretRevisionID, secretRevisionID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// assertSecretBackendReferenceCount asserts that the number of references to the
// given secret backend in the DB matches expected.
func assertSecretBackendReferenceCount(c *tc.C, db *sql.DB, backendID string, expected int) {
	row := db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_reference
WHERE secret_backend_uuid = ?`, backendID)
	var refCount int
	err := row.Scan(&refCount)
	c.Assert(err, tc.IsNil)
	c.Assert(refCount, tc.Equals, expected)
}
