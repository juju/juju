// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
)

type controllerSuite struct {
	baseSuite
}

func TestControllerSuite(t *testing.T) {
	tc.Run(t, &controllerSuite{})
}

func (s *controllerSuite) TestRemoveController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	mExp.GetModelLife(gomock.Any(), s.modelUUID.String()).Return(life.Alive, nil)

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{"model-1"}, nil)

	cExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	cExp.EnsureModelNotAlive(gomock.Any(), s.modelUUID.String(), false).Return(nil)

	mExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(false, nil)
	mExp.EnsureModelNotAlive(gomock.Any(), s.modelUUID.String(), false).Return(nil)
	mExp.ControllerModelScheduleRemoval(gomock.Any(), gomock.Any(), s.modelUUID.String(), false, when.UTC()).Return(nil)

	modelUUIDs, modelForce, err := s.newService(c).RemoveController(c.Context(), 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelUUIDs, tc.DeepEquals, []model.UUID{"model-1"})
	c.Check(modelForce, tc.IsFalse)
}

func (s *controllerSuite) TestRemoveControllerAlreadyDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	mExp.GetModelLife(gomock.Any(), s.modelUUID.String()).Return(life.Dying, nil)

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{"model-1"}, nil)

	cExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	cExp.EnsureModelNotAlive(gomock.Any(), s.modelUUID.String(), true).Return(nil)

	mExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(false, nil)
	mExp.EnsureModelNotAlive(gomock.Any(), s.modelUUID.String(), true).Return(nil)
	mExp.ControllerModelScheduleRemoval(gomock.Any(), gomock.Any(), s.modelUUID.String(), true, when.UTC()).Return(nil)

	modelUUIDs, modelForce, err := s.newService(c).RemoveController(c.Context(), 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelUUIDs, tc.DeepEquals, []model.UUID{"model-1"})
	c.Check(modelForce, tc.IsTrue)
}

func (s *controllerSuite) TestRemoveControllerEmptyController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	mExp.GetModelLife(gomock.Any(), s.modelUUID.String()).Return(life.Alive, nil)

	mExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	mExp.EnsureModelNotAlive(gomock.Any(), s.modelUUID.String(), false).Return(nil)
	mExp.ControllerModelScheduleRemoval(gomock.Any(), gomock.Any(), s.modelUUID.String(), false, when.UTC()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{}, nil)
	cExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	cExp.EnsureModelNotAlive(gomock.Any(), s.modelUUID.String(), false).Return(nil)

	modelUUID, modelForce, err := s.newService(c).RemoveController(c.Context(), 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelUUID, tc.DeepEquals, []model.UUID{})
	c.Check(modelForce, tc.IsFalse)
}

func (s *controllerSuite) TestRemoveControllerNotController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{}, nil)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), s.modelUUID.String()).Return(false, nil)

	_, _, err := s.newService(c).RemoveController(c.Context(), 0)
	c.Assert(err, tc.ErrorMatches, `.*not the controller model.*`)
}

func (s *controllerSuite) TestProcessJobInvalidJobType(c *tc.C) {
	var invalidJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: invalidJobType,
	}

	err := s.newService(c).processControllerModelJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *controllerSuite) TestExecuteJobForControllerModelWithHostedModels(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newControllerModelJob(c)

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{j.EntityUUID, "foo", "bar"}, nil)

	err := s.newService(c).processControllerModelJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)
}

func (s *controllerSuite) TestExecuteJobForControllerModelWithNoModels(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newControllerModelJob(c)

	s.clock.EXPECT().Now().Return(time.Now())

	mExp := s.modelState.EXPECT()
	mExp.ModelExists(gomock.Any(), j.EntityUUID).Return(false, nil)
	mExp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{}, nil)
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(false, nil)
	cExp.EnsureModelNotAlive(gomock.Any(), j.EntityUUID, false).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalModelRemoved)
}

func (s *controllerSuite) TestExecuteJobForControllerModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newControllerModelJob(c)

	s.clock.EXPECT().Now().Return(time.Now())

	mExp := s.modelState.EXPECT()
	mExp.ModelExists(gomock.Any(), j.EntityUUID).Return(false, nil)
	mExp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{j.EntityUUID}, nil)
	cExp.ModelExists(gomock.Any(), j.EntityUUID).Return(false, nil)
	cExp.EnsureModelNotAlive(gomock.Any(), j.EntityUUID, false).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalModelRemoved)
}

func newControllerModelJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.ControllerModelJob,
		EntityUUID:  modeltesting.GenModelUUID(c).String(),
	}
}
