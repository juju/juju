// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	applicationservice "github.com/juju/juju/domain/application/service"
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

func (s *modelSuite) TestEnsureModelNotAliveCascade(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)

	s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	artifacts, err := st.EnsureModelNotAliveCascade(c.Context(), modelUUID, false)
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

func (s *modelSuite) TestEnsureModelNotAliveCascadeEmpty(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	artifacts, err := st.EnsureModelNotAliveCascade(c.Context(), modelUUID, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(artifacts.Empty(), tc.Equals, true)
}

func (s *modelSuite) TestModelRemovalNormalSuccess(c *tc.C) {
	modelUUID := s.getModelUUID(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.ModelScheduleRemoval(
		c.Context(), "removal-uuid", modelUUID, false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	row := s.DB().QueryRow(
		"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal where uuid = ?",
		"removal-uuid",
	)
	var (
		removalTypeID int
		rUUID         string
		force         bool
		scheduledFor  time.Time
	)
	err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalTypeID, tc.Equals, 4)
	c.Check(rUUID, tc.Equals, modelUUID)
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *modelSuite) TestModelRemovalNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.ModelScheduleRemoval(
		c.Context(), "removal-uuid", "some-model-uuid", true, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	// It doesn't matter that the machine does not exist.
	// We rely on the worker to handle that fact.
	row := s.DB().QueryRow(`
SELECT t.name, r.entity_uuid, r.force, r.scheduled_for 
FROM   removal r JOIN removal_type t ON r.removal_type_id = t.id
where  r.uuid = ?`, "removal-uuid",
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
	c.Check(force, tc.Equals, true)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *modelSuite) TestDeleteModel(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupApplicationService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUID := s.getAllUnitUUIDs(c, appUUID)[0]
	machineUUID := s.getMachineUUIDFromApp(c, appUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := s.getModelUUID(c)

	err := st.DeleteModelArtifacts(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)

	s.advanceModelLife(c, modelUUID, life.Dead)
	s.advanceApplicationLife(c, appUUID, life.Dead)
	s.advanceUnitLife(c, unitUUID, life.Dead)
	s.advanceMachineLife(c, machineUUID, life.Dead)
	s.advanceInstanceLife(c, machineUUID, life.Dead)

	err = st.DeleteModelArtifacts(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)

	err = st.DeleteUnit(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteMachine(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteApplication(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteModelArtifacts(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// The model should be gone.
	exists, err := st.ModelExists(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *modelSuite) TestDeleteModelNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteModelArtifacts(c.Context(), "0")
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
