// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type modelSuite struct {
	baseSuite
}

func TestModelSuite(t *testing.T) {
	tc.Run(t, &modelSuite{})
}

func (s *modelSuite) TestModelExists(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.ModelExists(c.Context(), s.getModelUUID(c))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)

	exists, err = st.ModelExists(c.Context(), "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *modelSuite) TestGetModelLifeSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	l, err := st.GetModelLife(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Alive)

	// Set the unit to "dying" manually.
	s.advanceModelLife(c, modelUUID, life.Dying)

	l, err = st.GetModelLife(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Dying)
}

func (s *modelSuite) TestGetModelLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetModelLife(c.Context(), "some-unit-uuid")
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestIsMigratingModel(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	isMigrating, err := st.IsMigratingModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isMigrating, tc.IsFalse)

	// Set the model as migrating.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid)
VALUES ('foo', ?, 'source-migration-uuid')`, modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	isMigrating, err = st.IsMigratingModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isMigrating, tc.IsTrue)
}

func (s *modelSuite) TestEnsureModelNotAlive(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	err := st.EnsureModelNotAlive(c.Context(), modelUUID, false)
	c.Assert(err, tc.ErrorIsNil)

	s.checkModelLife(c, modelUUID, life.Dying)
}

func (s *modelSuite) TestGetModelUUIDs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUIDs, err := st.GetModelUUIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelUUIDs, tc.HasLen, 1)

	expectedUUID := s.getModelUUID(c)
	c.Check(modelUUIDs[0], tc.DeepEquals, expectedUUID)
}

func (s *modelSuite) TestMarkMigratingModelAsDeadNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.MarkMigratingModelAsDead(c.Context(), "non-existent-model-uuid")
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestMarkMigratingModelAsDeadStillAlive(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid)
VALUES ('foo', ?, 'source-migration-uuid')`, modelUUID); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.MarkMigratingModelAsDead(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestMarkMigratingModelAsDeadDying(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE model SET life_id = 1 WHERE uuid = ?", modelUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid)
VALUES ('foo', ?, 'source-migration-uuid')`, modelUUID); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.MarkMigratingModelAsDead(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestMarkMigratingModelAsDeadAlreadyDead(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE model SET life_id = 2 WHERE uuid = ?", modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.MarkMigratingModelAsDead(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestMarkModelAsDeadNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.MarkModelAsDead(c.Context(), "non-existent-model-uuid")
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestMarkModelAsDeadStillAlive(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.MarkModelAsDead(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *modelSuite) TestMarkModelAsDeadDying(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE model SET life_id = 1 WHERE uuid = ?", modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.MarkModelAsDead(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestMarkModelAsDeadAlreadyDead(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE model SET life_id = 2 WHERE uuid = ?", modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.MarkModelAsDead(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestDeleteModel(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE model SET life_id = 2 WHERE uuid = ?", modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.DeleteModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure the model is gone.
	exists, err := st.ModelExists(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *modelSuite) TestDeleteModelDyingModel(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE model SET life_id = 1 WHERE uuid = ?", modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.DeleteModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)
}

func (s *modelSuite) TestDeleteMigratingModel(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE model SET life_id = 2 WHERE uuid = ?", modelUUID); err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, "INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES ('blah', ?, 'source-migration-uuid')", modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.DeleteModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure the model is gone.
	exists, err := st.ModelExists(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	// Ensure the migration entry is also gone.
	var count int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?", modelUUID).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestDeleteMigratingModelWithImportCompanions ensures that deleting a
// migrating model whose v8 import recorded offer permissions and external
// controllers (rows in the model_migration_import companion tables) succeeds.
// Those tables FK onto model_migration_import, so the claim row cannot be
// deleted before them without violating an enforced foreign-key constraint.
func (s *modelSuite) TestDeleteMigratingModelWithImportCompanions(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	const claimUUID = "import-claim-uuid"
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE model SET life_id = 2 WHERE uuid = ?", modelUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES (?, ?, 'source-migration-uuid')",
			claimUUID, modelUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO model_migration_import_offer (migration_uuid, offer_uuid) VALUES (?, 'offer-uuid')",
			claimUUID); err != nil {
			return err
		}
		// The external-controller companion FKs onto external_controller, so
		// seed that parent row first.
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO external_controller (uuid, ca_cert) VALUES ('ext-ctrl-uuid', 'cert')"); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			"INSERT INTO model_migration_import_external_controller_model (migration_uuid, offerer_model_uuid, controller_uuid) VALUES (?, 'offerer-model-uuid', 'ext-ctrl-uuid')",
			claimUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.DeleteModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure the model is gone.
	exists, err := st.ModelExists(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	// Ensure the claim and both companion rows are also gone.
	for _, table := range []string{
		"model_migration_import",
		"model_migration_import_offer",
		"model_migration_import_external_controller_model",
	} {
		var count int
		err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			return tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count)
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Check(count, tc.Equals, 0, tc.Commentf("table %q still has rows", table))
	}
}

func (s *modelSuite) getModelUUID(c *tc.C) string {
	var modelUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM model").Scan(&modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return modelUUID
}
