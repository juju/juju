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

// TestMarkMigratingModelAsDeadTakesAbortLock verifies that marking an importing
// model dead atomically transitions its import claim to the aborting phase. This
// is what closes the abort/activate split brain: with the claim in aborting, a
// concurrent activation's importing->activating compare-and-set can no longer
// win and resurrect the just-killed model.
func (s *modelSuite) TestMarkMigratingModelAsDeadTakesAbortLock(c *tc.C) {
	modelUUID := s.getModelUUID(c)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// phase_type_id defaults to 0 (importing).
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid)
VALUES ('claim', ?, 'source-migration-uuid')`, modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.MarkMigratingModelAsDead(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	lifeID, phaseID := s.modelLifeAndClaimPhase(c, modelUUID)
	c.Check(lifeID, tc.Equals, int(life.Dead))
	c.Check(phaseID, tc.Equals, 2) // aborting
}

// TestMarkMigratingModelAsDeadAbortingClaimIsIdempotent verifies that a retried
// abort, whose claim is already aborting, still marks the model dead and leaves
// the claim aborting for the undertaker to reap.
func (s *modelSuite) TestMarkMigratingModelAsDeadAbortingClaimIsIdempotent(c *tc.C) {
	modelUUID := s.getModelUUID(c)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid, phase_type_id)
VALUES ('claim', ?, 'source-migration-uuid', 2)`, modelUUID) // aborting
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.MarkMigratingModelAsDead(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	lifeID, phaseID := s.modelLifeAndClaimPhase(c, modelUUID)
	c.Check(lifeID, tc.Equals, int(life.Dead))
	c.Check(phaseID, tc.Equals, 2) // still aborting
}

// TestMarkMigratingModelAsDeadRefusesActivatingClaim verifies that a model whose
// import claim has reached the activating phase is not marked dead by the abort
// path: activation has crossed the point of no return and owns the model's
// teardown, so killing it here would strand an activated model.
func (s *modelSuite) TestMarkMigratingModelAsDeadRefusesActivatingClaim(c *tc.C) {
	modelUUID := s.getModelUUID(c)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid, phase_type_id)
VALUES ('claim', ?, 'source-migration-uuid', 1)`, modelUUID) // activating
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.MarkMigratingModelAsDead(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIs, removalerrors.MigrationImportPastImporting)

	// The model is left alive and the claim untouched.
	lifeID, phaseID := s.modelLifeAndClaimPhase(c, modelUUID)
	c.Check(lifeID, tc.Equals, int(life.Alive))
	c.Check(phaseID, tc.Equals, 1) // still activating
}

// TestDeleteModelDeletesAbortingClaim verifies the generic (undertaker) removal
// path reaps an aborting import claim - the abort lock taken by a v7/legacy
// abort - once the model is torn down and no model-database drop is still
// staged.
func (s *modelSuite) TestDeleteModelDeletesAbortingClaim(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE model SET life_id = 2 WHERE uuid = ?", modelUUID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid, phase_type_id)
VALUES ('claim', ?, 'source-migration-uuid', 2)`, modelUUID) // aborting
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.claimCount(c, modelUUID), tc.Equals, 0)
}

// TestDeleteModelPreservesStagedAbortingClaim verifies that when a v8 abort
// finalizer has staged the model-database drop, the generic removal path leaves
// the aborting claim in place: the v8 finalizer owns releasing it once the drop
// is proven complete.
func (s *modelSuite) TestDeleteModelPreservesStagedAbortingClaim(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE model SET life_id = 2 WHERE uuid = ?", modelUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid, phase_type_id)
VALUES ('claim', ?, 'source-migration-uuid', 2)`, modelUUID); err != nil { // aborting
			return err
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_database_deletion (namespace, created_at)
VALUES (?, DATETIME('now'))`, modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.claimCount(c, modelUUID), tc.Equals, 1)
}

// TestDeleteModelPreservesActivatingClaim verifies the generic removal path
// never reaps an activating claim - it is owned by the v8 activation finalizer.
func (s *modelSuite) TestDeleteModelPreservesActivatingClaim(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE model SET life_id = 2 WHERE uuid = ?", modelUUID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid, phase_type_id)
VALUES ('claim', ?, 'source-migration-uuid', 1)`, modelUUID) // activating
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.claimCount(c, modelUUID), tc.Equals, 1)
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
	c.Check(exists, tc.IsFalse)

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

// modelLifeAndClaimPhase returns the model's life_id and its import claim's
// phase_type_id.
func (s *modelSuite) modelLifeAndClaimPhase(c *tc.C, modelUUID string) (lifeID, phaseID int) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx,
			"SELECT life_id FROM model WHERE uuid = ?", modelUUID).Scan(&lifeID); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx,
			"SELECT phase_type_id FROM model_migration_import WHERE model_uuid = ?", modelUUID).Scan(&phaseID)
	})
	c.Assert(err, tc.ErrorIsNil)
	return lifeID, phaseID
}

// claimCount returns the number of import claims for the model.
func (s *modelSuite) claimCount(c *tc.C, modelUUID string) int {
	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?", modelUUID).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	return count
}
