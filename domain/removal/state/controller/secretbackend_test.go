// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/secretbackend"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type secretBackendSuite struct {
	baseSuite
}

func TestSecretBackendSuite(t *testing.T) {
	tc.Run(t, &secretBackendSuite{})
}

func (s *secretBackendSuite) TestRemoveBuiltInSecretBackendForModel(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db, err := st.DB(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	modelName := "test-model"
	backendName := secretbackend.MakeBuiltInK8sSecretBackendName(modelName)
	backendUUID := "backend-uuid"

	// 1. Setup: Create a secret backend and some references.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO secret_backend (uuid, name, backend_type_id, origin_id) 
VALUES (?, ?, 1, 0)`, backendUUID, backendName)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO secret_backend_config (backend_uuid, name, content) VALUES (?, 'name', 'content')", backendUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Verify it exists.
	var res count
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, sqlair.MustPrepare("SELECT COUNT(*) AS &count.count FROM secret_backend WHERE uuid = $entityUUID.uuid", count{}, entityUUID{}), entityUUID{UUID: backendUUID}).Get(&res)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Count, tc.Equals, 1)

	// 2. Execute: removeBuiltInSecretBackendForModel.
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.removeBuiltInSecretBackendForModel(ctx, tx, modelName)
	})
	c.Assert(err, tc.ErrorIsNil)

	// 3. Verify: backend and references are gone.
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, sqlair.MustPrepare("SELECT COUNT(*) AS &count.count FROM secret_backend WHERE uuid = $entityUUID.uuid", count{}, entityUUID{}), entityUUID{UUID: backendUUID}).Get(&res)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Count, tc.Equals, 0)

	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, sqlair.MustPrepare("SELECT COUNT(*) AS &count.count FROM secret_backend_config WHERE backend_uuid = $entityUUID.uuid", count{}, entityUUID{}), entityUUID{UUID: backendUUID}).Get(&res)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Count, tc.Equals, 0)
}

func (s *secretBackendSuite) TestRemoveBuiltInSecretBackendForModelNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db, err := st.DB(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Execute on non-existent backend.
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return st.removeBuiltInSecretBackendForModel(ctx, tx, "non-existent")
	})
	c.Assert(err, tc.ErrorIsNil)
}
