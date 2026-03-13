// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/core/machine"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
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
	c.Check(exists, tc.IsTrue)

	exists, err = st.ModelExists(c.Context(), "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
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

func (s *modelSuite) TestEnsureModelNotAlive(c *tc.C) {
	svc := s.setupApplicationService(c)

	s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	err := st.EnsureModelNotAlive(c.Context(), modelUUID, false)
	c.Assert(err, tc.ErrorIsNil)

	s.checkModelLife(c, modelUUID, life.Dying)
}

func (s *modelSuite) TestEnsureModelNotAliveEmpty(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	err := st.EnsureModelNotAlive(c.Context(), modelUUID, false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestEnsureModelNotAliveCascade(c *tc.C) {
	svc := s.setupApplicationService(c)

	s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	artifacts, err := st.EnsureModelNotAliveCascade(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(artifacts.UnitUUIDs), tc.Equals, 1)
	c.Check(len(artifacts.ApplicationUUIDs), tc.Equals, 1)
	c.Check(len(artifacts.MachineUUIDs), tc.Equals, 1)
	c.Check(len(artifacts.RelationUUIDs), tc.Equals, 0)

	s.checkModelLife(c, modelUUID, life.Dying)
	s.checkUnitLife(c, artifacts.UnitUUIDs[0], life.Dying)
	s.checkMachineLife(c, artifacts.MachineUUIDs[0], life.Dying)
	s.checkInstanceLife(c, artifacts.MachineUUIDs[0], life.Dying)
	s.checkApplicationLife(c, artifacts.ApplicationUUIDs[0], life.Dying)
}

func (s *modelSuite) TestEnsureModelNotAliveCascadeRetryReturnsDyingArtifacts(c *tc.C) {
	svc := s.setupApplicationService(c)

	s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	modelUUID := s.getModelUUID(c)

	firstArtifacts, err := st.EnsureModelNotAliveCascade(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(firstArtifacts.UnitUUIDs), tc.Equals, 1)
	c.Check(len(firstArtifacts.ApplicationUUIDs), tc.Equals, 1)
	c.Check(len(firstArtifacts.MachineUUIDs), tc.Equals, 1)
	c.Check(len(firstArtifacts.RelationUUIDs), tc.Equals, 0)

	// Simulate retrying (e.g. destroy-model --force) while dependent entity
	// removal is still in progress. Dying entities must be returned again so
	// new jobs can be scheduled.
	secondArtifacts, err := st.EnsureModelNotAliveCascade(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(secondArtifacts, tc.DeepEquals, firstArtifacts)

	s.checkModelLife(c, modelUUID, life.Dying)
	s.checkUnitLife(c, secondArtifacts.UnitUUIDs[0], life.Dying)
	s.checkMachineLife(c, secondArtifacts.MachineUUIDs[0], life.Dying)
	s.checkInstanceLife(c, secondArtifacts.MachineUUIDs[0], life.Dying)
	s.checkApplicationLife(c, secondArtifacts.ApplicationUUIDs[0], life.Dying)
}

func (s *modelSuite) TestEnsureModelNotAliveCascadeRetryReturnsDyingRelations(c *tc.C) {
	relUUID := s.createRelation(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	modelUUID := s.getModelUUID(c)

	firstArtifacts, err := st.EnsureModelNotAliveCascade(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(firstArtifacts.RelationUUIDs), tc.Equals, 1)
	c.Check(len(firstArtifacts.ApplicationUUIDs), tc.Equals, 2)
	c.Check(len(firstArtifacts.UnitUUIDs), tc.Equals, 0)
	c.Check(len(firstArtifacts.MachineUUIDs), tc.Equals, 0)
	c.Check(firstArtifacts.RelationUUIDs[0], tc.Equals, relUUID.String())

	// Simulate retrying (e.g. destroy-model --force) while dependent entity
	// removal is still in progress. Dying entities must be returned again so
	// new jobs can be scheduled.
	secondArtifacts, err := st.EnsureModelNotAliveCascade(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(secondArtifacts, tc.DeepEquals, firstArtifacts)

	// The relation should still be in the dying state.
	row := s.DB().QueryRowContext(c.Context(), "SELECT life_id FROM relation where uuid = ?", relUUID.String())
	var relationLife life.Life
	err = row.Scan(&relationLife)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relationLife, tc.Equals, life.Dying)
}

func (s *modelSuite) TestEnsureModelNotAliveCascadeEmpty(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	artifacts, err := st.EnsureModelNotAliveCascade(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(artifacts.Empty(), tc.IsTrue)
}

func (s *modelSuite) TestModelRemovalNormalSuccess(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.ModelScheduleRemoval(
		c.Context(),
		"removal-uuid",
		modelUUID, false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	ensureRemovalJob := func(removalUUID string, idType int) {
		row := s.DB().QueryRow(
			"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal where uuid = ?",
			removalUUID,
		)
		var (
			removalTypeID int
			rUUID         string
			force         bool
			scheduledFor  time.Time
		)
		err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
		c.Assert(err, tc.ErrorIsNil)

		c.Check(removalTypeID, tc.Equals, idType)
		c.Check(rUUID, tc.Equals, modelUUID)
		c.Check(force, tc.IsFalse)
		c.Check(scheduledFor, tc.Equals, when)
	}

	ensureRemovalJob("removal-uuid", 4)
}

func (s *modelSuite) TestModelRemovalNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.ModelScheduleRemoval(
		c.Context(),
		"removal-uuid",
		"some-model-uuid", true, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have two removal jobs scheduled immediately.
	// It doesn't matter that the model does not exist.
	// We rely on the worker to handle that fact.
	ensureRemovalJob := func(removalUUID string) {
		row := s.DB().QueryRow(`
SELECT t.name, r.entity_uuid, r.force, r.scheduled_for 
FROM   removal r JOIN removal_type t ON r.removal_type_id = t.id
where  r.uuid = ?`, removalUUID,
		)

		var (
			removalType  string
			rUUID        string
			force        bool
			scheduledFor time.Time
		)
		err = row.Scan(&removalType, &rUUID, &force, &scheduledFor)
		c.Assert(err, tc.ErrorIsNil)

		c.Check(removalType, tc.Equals, "model")
		c.Check(rUUID, tc.Equals, "some-model-uuid")
		c.Check(force, tc.IsTrue)
		c.Check(scheduledFor, tc.Equals, when)
	}
	ensureRemovalJob("removal-uuid")
}

func (s *modelSuite) TestControllerModelRemovalNormalSuccess(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.ControllerModelScheduleRemoval(
		c.Context(),
		"removal-uuid",
		modelUUID, false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	ensureRemovalJob := func(removalUUID string, idType int) {
		row := s.DB().QueryRow(
			"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal where uuid = ?",
			removalUUID,
		)
		var (
			removalTypeID int
			rUUID         string
			force         bool
			scheduledFor  time.Time
		)
		err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
		c.Assert(err, tc.ErrorIsNil)

		c.Check(removalTypeID, tc.Equals, idType)
		c.Check(rUUID, tc.Equals, modelUUID)
		c.Check(force, tc.IsFalse)
		c.Check(scheduledFor, tc.Equals, when)
	}

	ensureRemovalJob("removal-uuid", 15)
}

func (s *modelSuite) TestControllerModelRemovalNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.ControllerModelScheduleRemoval(
		c.Context(),
		"removal-uuid",
		"some-model-uuid", true, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have two removal jobs scheduled immediately.
	// It doesn't matter that the controller model does not exist.
	// We rely on the worker to handle that fact.
	ensureRemovalJob := func(removalUUID string) {
		row := s.DB().QueryRow(`
SELECT t.name, r.entity_uuid, r.force, r.scheduled_for 
FROM   removal r JOIN removal_type t ON r.removal_type_id = t.id
where  r.uuid = ?`, removalUUID,
		)

		var (
			removalType  string
			rUUID        string
			force        bool
			scheduledFor time.Time
		)
		err = row.Scan(&removalType, &rUUID, &force, &scheduledFor)
		c.Assert(err, tc.ErrorIsNil)

		c.Check(removalType, tc.Equals, "controller-model")
		c.Check(rUUID, tc.Equals, "some-model-uuid")
		c.Check(force, tc.IsTrue)
		c.Check(scheduledFor, tc.Equals, when)
	}
	ensureRemovalJob("removal-uuid")
}

func (s *modelSuite) TestMarkModelAsDeadNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.MarkModelAsDead(c.Context(), "foo", false)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestMarkModelAsDeadStillAlive(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	err := st.MarkModelAsDead(c.Context(), modelUUID, false)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *modelSuite) TestMarkModelAsDeadStillDying(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	s.advanceModelLife(c, modelUUID, life.Dying)

	err := st.MarkModelAsDead(c.Context(), modelUUID, false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestMarkModelAsDead(c *tc.C) {
	svc := s.setupApplicationService(c)
	s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	err := st.MarkModelAsDead(c.Context(), modelUUID, false)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)

	s.advanceModelLife(c, modelUUID, life.Dying)

	err = st.MarkModelAsDead(c.Context(), modelUUID, true)
	c.Assert(err, tc.ErrorIsNil)

	s.checkModelLife(c, modelUUID, life.Dead)
}

func (s *modelSuite) TestMarkModelAsDeadApplicationsExists(c *tc.C) {
	svc := s.setupApplicationService(c)
	s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	s.advanceModelLife(c, modelUUID, life.Dying)

	err := st.MarkModelAsDead(c.Context(), modelUUID, false)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)

	err = st.MarkModelAsDead(c.Context(), modelUUID, true)
	c.Assert(err, tc.ErrorIsNil)

	s.checkModelLife(c, modelUUID, life.Dead)
}

func (s *modelSuite) TestMarkModelAsDeadMachinesExists(c *tc.C) {
	s.createMachine(c, machine.Name("0"))

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	s.advanceModelLife(c, modelUUID, life.Dying)

	err := st.MarkModelAsDead(c.Context(), modelUUID, false)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)

	err = st.MarkModelAsDead(c.Context(), modelUUID, true)
	c.Assert(err, tc.ErrorIsNil)

	s.checkModelLife(c, modelUUID, life.Dead)
}

func (s *modelSuite) TestMarkModelAsDeadModelFilesystemStorageExists(c *tc.C) {
	s.addModelProvisionedFilesystem(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	s.advanceModelLife(c, modelUUID, life.Dying)

	err := st.MarkModelAsDead(c.Context(), modelUUID, false)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)

	err = st.MarkModelAsDead(c.Context(), modelUUID, true)
	c.Assert(err, tc.ErrorIsNil)

	s.checkModelLife(c, modelUUID, life.Dead)
}

func (s *modelSuite) TestMarkModelAsDeadModelVolumeStorageExists(c *tc.C) {
	s.addModelProvisionedVolume(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	s.advanceModelLife(c, modelUUID, life.Dying)

	err := st.MarkModelAsDead(c.Context(), modelUUID, false)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)

	err = st.MarkModelAsDead(c.Context(), modelUUID, true)
	c.Assert(err, tc.ErrorIsNil)

	s.checkModelLife(c, modelUUID, life.Dead)
}

func (s *modelSuite) TestIsControllerModel(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	isController, err := st.IsControllerModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isController, tc.IsFalse)
}

func (s *modelSuite) TestIsControllerModelControllerModel(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Force the model to be a controller model.
		_, err := tx.ExecContext(ctx, `DROP TRIGGER IF EXISTS trg_model_immutable_update;`)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `UPDATE model SET is_controller_model = 1 WHERE uuid = ?`, s.getModelUUID(c))
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	isController, err := st.IsControllerModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isController, tc.IsTrue)
}

func (s *modelSuite) TestIsControllerModelNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.IsControllerModel(c.Context(), "not-a-model-uuid")
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
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

func (s *modelSuite) createMachine(c *tc.C, machineId machine.Name) {
	nodeUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	query := `
INSERT INTO machine (*)
VALUES ($createMachine.*)
`
	machine := createMachine{
		MachineUUID: machine.UUID(machineUUID.String()),
		NetNodeUUID: nodeUUID.String(),
		Name:        machineId,
		LifeID:      life.Alive,
	}

	createMachineStmt, err := sqlair.Prepare(query, machine)
	c.Assert(err, tc.ErrorIsNil)

	createNode := `INSERT INTO net_node (uuid) VALUES ($createMachine.net_node_uuid)`
	createNodeStmt, err := sqlair.Prepare(createNode, machine)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, createNodeStmt, machine).Run(); err != nil {
			return errors.Errorf("creating net node row for bootstrap machine %q: %w", machineId, err)
		}
		if err := tx.Query(ctx, createMachineStmt, machine).Run(); err != nil {
			return errors.Errorf("creating machine row for bootstrap machine %q: %w", machineId, err)
		}
		return nil
	})

	c.Assert(err, tc.ErrorIsNil)
}

type createMachine struct {
	MachineUUID machine.UUID `db:"uuid"`
	NetNodeUUID string       `db:"net_node_uuid"`
	Name        machine.Name `db:"name"`
	LifeID      life.Life    `db:"life_id"`
}
