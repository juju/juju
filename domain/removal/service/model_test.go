// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	removalinternal "github.com/juju/juju/domain/removal/internal"
	"github.com/juju/juju/internal/errors"
)

type modelSuite struct {
	baseSuite
}

func TestModelSuite(t *testing.T) {
	tc.Run(t, &modelSuite{})
}

func (s *modelSuite) TestRemoveModelNoForceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := tc.Must0(c, coremodel.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil)
	cExp.EnsureModelNotAlive(gomock.Any(), mUUID.String(), false).Return(nil)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), mUUID.String()).Return(false, nil)
	mExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil)
	mExp.EnsureModelNotAliveCascade(gomock.Any(), mUUID.String()).Return(removal.ModelArtifacts{
		RelationUUIDs:    []string{"some-relation-id"},
		UnitUUIDs:        []string{"some-unit-id"},
		MachineUUIDs:     []string{"some-machine-id"},
		ApplicationUUIDs: []string{"some-application-id"},
	}, nil)
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), false, when.UTC()).Return(nil)

	// We don't want to create all the machine, unit, relation and application
	// expectations here, so we'll assume that the
	// machine/unit/relation/application no longer exists, to prevent this test
	// from depending on the machine/unit/relation/application removal logic.
	mExp.MachineExists(gomock.Any(), "some-machine-id").Return(false, nil)
	mExp.UnitExists(gomock.Any(), "some-unit-id").Return(false, nil)
	mExp.RelationExists(gomock.Any(), "some-relation-id").Return(false, nil)
	mExp.ApplicationExists(gomock.Any(), "some-application-id").Return(false, nil)

	jobUUID, err := s.newService(c).RemoveModel(c.Context(), mUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *modelSuite) TestRemoveModelRetrySchedulesRemovalJobs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := tc.Must0(c, coremodel.NewUUID)
	when := time.Now()
	artifacts := removal.ModelArtifacts{
		RelationUUIDs:    []string{"some-relation-id"},
		UnitUUIDs:        []string{"some-unit-id"},
		MachineUUIDs:     []string{"some-machine-id"},
		ApplicationUUIDs: []string{"some-application-id"},
	}

	// Each call schedules model + relation + unit + machine + application.
	s.clock.EXPECT().Now().Return(when).Times(10)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil).Times(2)
	cExp.EnsureModelNotAlive(gomock.Any(), mUUID.String(), false).Return(nil).Times(2)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), mUUID.String()).Return(false, nil).Times(2)
	mExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil).Times(2)
	mExp.EnsureModelNotAliveCascade(gomock.Any(), mUUID.String()).Return(artifacts, nil).Times(2)
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), false, when.UTC()).Return(nil).Times(2)

	mExp.RelationExists(gomock.Any(), "some-relation-id").Return(true, nil).Times(2)
	mExp.EnsureRelationNotAlive(gomock.Any(), "some-relation-id").Return(nil).Times(2)
	mExp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), "some-relation-id", false, when.UTC()).Return(nil).Times(2)

	mExp.UnitExists(gomock.Any(), "some-unit-id").Return(true, nil).Times(2)
	mExp.EnsureUnitNotAliveCascade(gomock.Any(), "some-unit-id", true).Return(removalinternal.CascadedUnitLives{}, nil).Times(2)
	mExp.UnitScheduleRemoval(gomock.Any(), gomock.Any(), "some-unit-id", false, when.UTC()).Return(nil).Times(2)

	mExp.MachineExists(gomock.Any(), "some-machine-id").Return(true, nil).Times(2)
	mExp.EnsureMachineNotAliveCascade(gomock.Any(), "some-machine-id", false).Return(removalinternal.CascadedMachineLives{}, nil).Times(2)
	mExp.MachineScheduleRemoval(gomock.Any(), gomock.Any(), "some-machine-id", false, when.UTC()).Return(nil).Times(2)

	mExp.ApplicationExists(gomock.Any(), "some-application-id").Return(true, nil).Times(2)
	mExp.EnsureApplicationNotAliveCascade(gomock.Any(), "some-application-id", true, false).Return(removalinternal.CascadedApplicationLives{}, nil).Times(2)
	mExp.ApplicationScheduleRemoval(gomock.Any(), gomock.Any(), "some-application-id", false, when.UTC()).Return(nil).Times(2)

	jobUUID1, err := s.newService(c).RemoveModel(c.Context(), mUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID1.Validate(), tc.ErrorIsNil)

	// Simulate a second identical call, should be idempotent.
	jobUUID2, err := s.newService(c).RemoveModel(c.Context(), mUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID2.Validate(), tc.ErrorIsNil)
}

func (s *modelSuite) TestRemoveModelRetryWithForceSchedulesRemovalJobs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := tc.Must0(c, coremodel.NewUUID)
	when := time.Now()
	artifacts := removal.ModelArtifacts{
		RelationUUIDs:    []string{"some-relation-id"},
		UnitUUIDs:        []string{"some-unit-id"},
		MachineUUIDs:     []string{"some-machine-id"},
		ApplicationUUIDs: []string{"some-application-id"},
	}

	// Each call schedules model + relation + unit + machine + application.
	s.clock.EXPECT().Now().Return(when).Times(10)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil).Times(2)
	cExp.EnsureModelNotAlive(gomock.Any(), mUUID.String(), false).Return(nil)
	cExp.EnsureModelNotAlive(gomock.Any(), mUUID.String(), true).Return(nil)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), mUUID.String()).Return(false, nil).Times(2)
	mExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil).Times(2)
	mExp.EnsureModelNotAliveCascade(gomock.Any(), mUUID.String()).Return(artifacts, nil).Times(2)
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), false, when.UTC()).Return(nil)
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), true, when.UTC()).Return(nil)

	mExp.RelationExists(gomock.Any(), "some-relation-id").Return(true, nil).Times(2)
	mExp.EnsureRelationNotAlive(gomock.Any(), "some-relation-id").Return(nil).Times(2)
	mExp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), "some-relation-id", false, when.UTC()).Return(nil)
	mExp.RelationScheduleRemoval(gomock.Any(), gomock.Any(), "some-relation-id", true, when.UTC()).Return(nil)

	mExp.UnitExists(gomock.Any(), "some-unit-id").Return(true, nil).Times(2)
	mExp.EnsureUnitNotAliveCascade(gomock.Any(), "some-unit-id", true).Return(removalinternal.CascadedUnitLives{}, nil).Times(2)
	mExp.UnitScheduleRemoval(gomock.Any(), gomock.Any(), "some-unit-id", false, when.UTC()).Return(nil)
	mExp.UnitScheduleRemoval(gomock.Any(), gomock.Any(), "some-unit-id", true, when.UTC()).Return(nil)

	mExp.MachineExists(gomock.Any(), "some-machine-id").Return(true, nil).Times(2)
	mExp.EnsureMachineNotAliveCascade(gomock.Any(), "some-machine-id", false).Return(removalinternal.CascadedMachineLives{}, nil)
	mExp.EnsureMachineNotAliveCascade(gomock.Any(), "some-machine-id", true).Return(removalinternal.CascadedMachineLives{}, nil)
	mExp.MachineScheduleRemoval(gomock.Any(), gomock.Any(), "some-machine-id", false, when.UTC()).Return(nil)
	mExp.MachineScheduleRemoval(gomock.Any(), gomock.Any(), "some-machine-id", true, when.UTC()).Return(nil)

	mExp.ApplicationExists(gomock.Any(), "some-application-id").Return(true, nil).Times(2)
	mExp.EnsureApplicationNotAliveCascade(gomock.Any(), "some-application-id", true, false).Return(removalinternal.CascadedApplicationLives{}, nil)
	mExp.EnsureApplicationNotAliveCascade(gomock.Any(), "some-application-id", true, true).Return(removalinternal.CascadedApplicationLives{}, nil)
	mExp.ApplicationScheduleRemoval(gomock.Any(), gomock.Any(), "some-application-id", false, when.UTC()).Return(nil)
	mExp.ApplicationScheduleRemoval(gomock.Any(), gomock.Any(), "some-application-id", true, when.UTC()).Return(nil)

	jobUUID1, err := s.newService(c).RemoveModel(c.Context(), mUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID1.Validate(), tc.ErrorIsNil)

	// Simulate a second call with force, should also schedule the same removal
	// jobs.
	jobUUID2, err := s.newService(c).RemoveModel(c.Context(), mUUID, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID2.Validate(), tc.ErrorIsNil)
}

func (s *modelSuite) TestRemoveModelControllerModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := tc.Must0(c, coremodel.NewUUID)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), mUUID.String()).Return(true, nil)

	_, err := s.newService(c).RemoveModel(c.Context(), mUUID, false, 0)
	c.Assert(err, tc.ErrorIs, removalerrors.ForceRequired)
}

func (s *modelSuite) TestRemoveModelNoForceSuccessControllerModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := tc.Must0(c, coremodel.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil)
	cExp.EnsureModelNotAlive(gomock.Any(), mUUID.String(), true).Return(nil)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), mUUID.String()).Return(true, nil)
	mExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil)
	mExp.EnsureModelNotAliveCascade(gomock.Any(), mUUID.String()).Return(removal.ModelArtifacts{
		RelationUUIDs:    []string{"some-relation-id"},
		UnitUUIDs:        []string{"some-unit-id"},
		MachineUUIDs:     []string{"some-machine-id"},
		ApplicationUUIDs: []string{"some-application-id"},
	}, nil)
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), true, when.UTC()).Return(nil)

	// We don't want to create all the machine, unit, relation and application
	// expectations here, so we'll assume that the
	// machine/unit/relation/application no longer exists, to prevent this test
	// from depending on the machine/unit/relation/application removal logic.
	mExp.MachineExists(gomock.Any(), "some-machine-id").Return(false, nil)
	mExp.UnitExists(gomock.Any(), "some-unit-id").Return(false, nil)
	mExp.RelationExists(gomock.Any(), "some-relation-id").Return(false, nil)
	mExp.ApplicationExists(gomock.Any(), "some-application-id").Return(false, nil)

	jobUUID, err := s.newService(c).RemoveModel(c.Context(), mUUID, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *modelSuite) TestRemoveModelForceNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := tc.Must0(c, coremodel.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil)
	cExp.EnsureModelNotAlive(gomock.Any(), mUUID.String(), true).Return(nil)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), mUUID.String()).Return(false, nil)
	mExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil)
	mExp.EnsureModelNotAliveCascade(gomock.Any(), mUUID.String()).Return(removal.ModelArtifacts{}, nil)
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveModel(c.Context(), mUUID, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *modelSuite) TestRemoveModelForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := tc.Must0(c, coremodel.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil)
	cExp.EnsureModelNotAlive(gomock.Any(), mUUID.String(), true).Return(nil)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), mUUID.String()).Return(false, nil)
	mExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil)
	mExp.EnsureModelNotAliveCascade(gomock.Any(), mUUID.String()).Return(removal.ModelArtifacts{}, nil)

	// The first normal removal scheduled immediately.
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), false, when.UTC()).Return(nil)

	// The forced removal scheduled after the wait duration.
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), true, when.UTC().Add(time.Minute)).Return(nil)

	jobUUID, err := s.newService(c).RemoveModel(c.Context(), mUUID, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *modelSuite) TestRemoveModelNotFoundInModelButInController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := tc.Must0(c, coremodel.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil)
	cExp.EnsureModelNotAlive(gomock.Any(), mUUID.String(), false).Return(nil)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), mUUID.String()).Return(false, nil)
	mExp.ModelExists(gomock.Any(), mUUID.String()).Return(false, nil)
	mExp.EnsureModelNotAliveCascade(gomock.Any(), mUUID.String()).Return(removal.ModelArtifacts{}, nil)
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), false, when.UTC()).Return(nil)

	_, err := s.newService(c).RemoveModel(c.Context(), mUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestRemoveModelNotFoundInControllerButInModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := tc.Must0(c, coremodel.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), mUUID.String()).Return(false, nil)
	cExp.EnsureModelNotAlive(gomock.Any(), mUUID.String(), false).Return(nil)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), mUUID.String()).Return(false, nil)
	mExp.ModelExists(gomock.Any(), mUUID.String()).Return(true, nil)
	mExp.EnsureModelNotAliveCascade(gomock.Any(), mUUID.String()).Return(removal.ModelArtifacts{}, nil)
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), false, when.UTC()).Return(nil)

	_, err := s.newService(c).RemoveModel(c.Context(), mUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestRemoveModelNotFoundInBothControllerAndModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := tc.Must0(c, coremodel.NewUUID)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), mUUID.String()).Return(false, nil)
	cExp.EnsureModelNotAlive(gomock.Any(), mUUID.String(), false).Return(nil)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), mUUID.String()).Return(false, nil)
	mExp.ModelExists(gomock.Any(), mUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveModel(c.Context(), mUUID, false, 0)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestRemoveMigratingModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cExp := s.controllerState.EXPECT()
	cExp.IsMigratingModel(gomock.Any(), "some-model-uuid").Return(true, nil)
	cExp.MarkMigratingModelAsDead(gomock.Any(), "some-model-uuid").Return(nil)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), "some-model-uuid").Return(false, nil)

	err := s.newService(c).RemoveMigratingModel(c.Context(), "some-model-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestRemoveMigratingModelControllerModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), "some-model-uuid").Return(true, nil)

	err := s.newService(c).RemoveMigratingModel(c.Context(), "some-model-uuid")
	c.Assert(err, tc.ErrorMatches, `.*cannot remove controller model.*`)
}

func (s *modelSuite) TestRemoveMigratingModelNotImporting(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cExp := s.controllerState.EXPECT()
	cExp.IsMigratingModel(gomock.Any(), "some-model-uuid").Return(false, nil)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), "some-model-uuid").Return(false, nil)

	err := s.newService(c).RemoveMigratingModel(c.Context(), "some-model-uuid")
	c.Assert(err, tc.ErrorMatches, `.*is not importing`)
}

func (s *modelSuite) TestDeleteModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cExp := s.controllerState.EXPECT()
	cExp.IsMigratingModel(gomock.Any(), "some-model-uuid").Return(false, nil)
	cExp.GetModelLife(gomock.Any(), "some-model-uuid").Return(life.Dead, nil)
	cExp.DeleteModel(gomock.Any(), "some-model-uuid").Return(nil)

	s.provider.EXPECT().Destroy(gomock.Any()).Return(nil)

	err := s.newService(c).DeleteModel(c.Context(), "some-model-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestDeleteModelIsMigrating(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cExp := s.controllerState.EXPECT()
	cExp.IsMigratingModel(gomock.Any(), "some-model-uuid").Return(true, nil)
	cExp.DeleteModel(gomock.Any(), "some-model-uuid").Return(nil)

	err := s.newService(c).DeleteModel(c.Context(), "some-model-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestDeleteModelControllerAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cExp := s.controllerState.EXPECT()
	cExp.IsMigratingModel(gomock.Any(), "some-model-uuid").Return(false, nil)
	cExp.GetModelLife(gomock.Any(), "some-model-uuid").Return(life.Alive, nil)

	err := s.newService(c).DeleteModel(c.Context(), "some-model-uuid")
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *modelSuite) TestDeleteModelControllerGetModelLifeError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cExp := s.controllerState.EXPECT()
	cExp.IsMigratingModel(gomock.Any(), "some-model-uuid").Return(false, nil)
	cExp.GetModelLife(gomock.Any(), "some-model-uuid").Return(life.Dead, errors.Errorf("the front fell off"))

	err := s.newService(c).DeleteModel(c.Context(), "some-model-uuid")
	c.Assert(err, tc.ErrorMatches, `.*the front fell off`)
}

func (s *modelSuite) TestDeleteModelControllerGetModelLifeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cExp := s.controllerState.EXPECT()
	cExp.IsMigratingModel(gomock.Any(), "some-model-uuid").Return(false, nil)
	cExp.GetModelLife(gomock.Any(), "some-model-uuid").Return(-1, modelerrors.NotFound)
	cExp.DeleteModel(gomock.Any(), "some-model-uuid").Return(nil)

	s.provider.EXPECT().Destroy(gomock.Any()).Return(nil)

	err := s.newService(c).DeleteModel(c.Context(), "some-model-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestProcessJobInvalidJobType(c *tc.C) {
	var invalidJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: invalidJobType,
	}

	err := s.newService(c).processModelJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *modelSuite) TestExecuteJobForModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// This does not return model not found, instead it returns that the
	// model was removed successfully, which is a different error. That way
	// we can ensure anyone that is listening for removal jobs will
	// be able to handle the removal of the model.

	j := newModelJob(c)

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(-1, modelerrors.NotFound)
	mExp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(false, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalModelRemoved)
}

func (s *modelSuite) TestExecuteJobForModelError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(-1, errors.Errorf("the front fell off"))

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(true, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *modelSuite) TestExecuteJobForModelStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(life.Alive, nil)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(true, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *modelSuite) TestExecuteJobForModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(1, nil)
	mExp.IsControllerModel(gomock.Any(), j.EntityUUID).Return(false, nil)
	mExp.MarkModelAsDead(gomock.Any(), j.EntityUUID, false).Return(nil)
	mExp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	cExp.MarkModelAsDead(gomock.Any(), j.EntityUUID).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestExecuteJobForModelControllerModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(1, nil)
	mExp.IsControllerModel(gomock.Any(), j.EntityUUID).Return(true, nil)
	mExp.MarkModelAsDead(gomock.Any(), j.EntityUUID, false).Return(nil)
	mExp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{"model-1", "model-2"}, nil)
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	cExp.MarkModelAsDead(gomock.Any(), j.EntityUUID).Return(nil)

	cExp.GetModelLife(gomock.Any(), "model-1").Return(life.Dead, nil)
	cExp.GetModelLife(gomock.Any(), "model-2").Return(life.Dead, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestExecuteJobForModelControllerModelNotDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(1, nil)
	mExp.IsControllerModel(gomock.Any(), j.EntityUUID).Return(true, nil)

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{"model-1", "model-2"}, nil)
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(true, nil)

	// Notice that it doesn't attempt to get the life of model-2, because
	// the model-1 is not dead.

	cExp.GetModelLife(gomock.Any(), "model-1").Return(life.Dying, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestExecuteJobForModelControllerModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(1, nil)
	mExp.IsControllerModel(gomock.Any(), j.EntityUUID).Return(true, nil)
	mExp.MarkModelAsDead(gomock.Any(), j.EntityUUID, false).Return(nil)
	mExp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{"model-1", "model-2"}, nil)
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	cExp.MarkModelAsDead(gomock.Any(), j.EntityUUID).Return(nil)

	cExp.GetModelLife(gomock.Any(), "model-1").Return(-1, modelerrors.NotFound)
	cExp.GetModelLife(gomock.Any(), "model-2").Return(life.Dead, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestExecuteJobForModelControllerModelAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(1, nil)
	mExp.IsControllerModel(gomock.Any(), j.EntityUUID).Return(true, nil)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{"model-1", "model-2"}, nil)

	cExp.GetModelLife(gomock.Any(), "model-1").Return(life.Dead, nil)
	cExp.GetModelLife(gomock.Any(), "model-2").Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestExecuteJobForModelControllerModelAliveWithForce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)
	j.Force = true

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(1, nil)
	mExp.IsControllerModel(gomock.Any(), j.EntityUUID).Return(true, nil)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{"model-1", "model-2"}, nil)

	cExp.GetModelLife(gomock.Any(), "model-1").Return(-1, modelerrors.NotFound)
	cExp.GetModelLife(gomock.Any(), "model-2").Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestExecuteJobForModelControllerModelDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(1, nil)
	mExp.IsControllerModel(gomock.Any(), j.EntityUUID).Return(true, nil)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{"model-1", "model-2"}, nil)

	cExp.GetModelLife(gomock.Any(), "model-1").Return(-1, modelerrors.NotFound)
	cExp.GetModelLife(gomock.Any(), "model-2").Return(life.Dying, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestExecuteJobForModelControllerModelDyingWithForce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)
	j.Force = true

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(1, nil)
	mExp.IsControllerModel(gomock.Any(), j.EntityUUID).Return(true, nil)
	mExp.MarkModelAsDead(gomock.Any(), j.EntityUUID, true).Return(nil)
	mExp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{"model-1", "model-2"}, nil)
	cExp.MarkModelAsDead(gomock.Any(), j.EntityUUID).Return(nil)

	cExp.GetModelLife(gomock.Any(), "model-1").Return(life.Dead, nil)
	cExp.GetModelLife(gomock.Any(), "model-2").Return(life.Dying, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestExecuteJobForModelReenterantModelDeleted(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(1, nil)
	mExp.IsControllerModel(gomock.Any(), j.EntityUUID).Return(false, nil)
	mExp.MarkModelAsDead(gomock.Any(), j.EntityUUID, false).Return(modelerrors.NotFound)
	mExp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	cExp.MarkModelAsDead(gomock.Any(), j.EntityUUID).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestExecuteJobForModelReenterantControllerModelDeleted(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(1, nil)
	mExp.IsControllerModel(gomock.Any(), j.EntityUUID).Return(false, nil)
	mExp.MarkModelAsDead(gomock.Any(), j.EntityUUID, false).Return(nil)
	mExp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(true, nil)
	cExp.MarkModelAsDead(gomock.Any(), j.EntityUUID).Return(modelerrors.NotFound)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSuite) TestExecuteJobForModelReenterantControllerModelDeletedDoesNotExist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newModelJob(c)

	mExp := s.modelState.EXPECT()
	mExp.GetModelLife(gomock.Any(), j.EntityUUID).Return(1, nil)
	mExp.IsControllerModel(gomock.Any(), j.EntityUUID).Return(false, nil)
	mExp.MarkModelAsDead(gomock.Any(), j.EntityUUID, false).Return(nil)
	mExp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(false, nil)
	cExp.MarkModelAsDead(gomock.Any(), j.EntityUUID).Return(modelerrors.NotFound)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func newModelJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.ModelJob,
		EntityUUID:  tc.Must0(c, coremodel.NewUUID).String(),
	}
}
