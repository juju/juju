// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/removal"
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
	s.clock.EXPECT().Now().Return(when).Times(2)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	mExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	mExp.EnsureModelNotAliveCascade(gomock.Any(), s.modelUUID.String(), false).Return(removal.ModelArtifacts{}, nil)
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), s.modelUUID.String(), false, when.UTC()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{"model-1"}, nil)
	cExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	cExp.EnsureModelNotAliveCascade(gomock.Any(), s.modelUUID.String(), false).Return(nil)

	mExp.ModelExists(gomock.Any(), "model-1").Return(true, nil)
	mExp.EnsureModelNotAliveCascade(gomock.Any(), "model-1", false).Return(removal.ModelArtifacts{}, nil)
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), "model-1", false, when.UTC()).Return(nil)

	cExp.ModelExists(gomock.Any(), "model-1").Return(false, nil)
	cExp.EnsureModelNotAliveCascade(gomock.Any(), "model-1", false).Return(nil)

	err := s.newService(c).RemoveController(c.Context(), false, 0)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *controllerSuite) TestRemoveControllerEmptyController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	mExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	mExp.EnsureModelNotAliveCascade(gomock.Any(), s.modelUUID.String(), false).Return(removal.ModelArtifacts{}, nil)
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), s.modelUUID.String(), false, when.UTC()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{}, nil)
	cExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	cExp.EnsureModelNotAliveCascade(gomock.Any(), s.modelUUID.String(), false).Return(nil)

	err := s.newService(c).RemoveController(c.Context(), false, 0)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *controllerSuite) TestRemoveControllerNotController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), s.modelUUID.String()).Return(false, nil)

	err := s.newService(c).RemoveController(c.Context(), false, 0)
	c.Assert(err, tc.ErrorMatches, `.*not the controller model.*`)
}

// Ensures that RemoveController ignores model-not-found errors when attempting
// to remove non-controller models. The controller model is scheduled for
// removal, while a secondary model disappears between controller and model DB
// checks causing removeModel to return modelerrors.NotFound, which must be
// ignored by RemoveController.
func (s *controllerSuite) TestRemoveControllerIgnoresModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	when := time.Now()
	// Only the controller model removal schedules a job, so Now() is called once.
	s.clock.EXPECT().Now().Return(when).Times(1)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	mExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	mExp.EnsureModelNotAliveCascade(gomock.Any(), s.modelUUID.String(), false).Return(removal.ModelArtifacts{}, nil)
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), s.modelUUID.String(), false, when.UTC()).Return(nil)

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{"model-1"}, nil)
	// Controller model still exists in controller DB and can be cascaded.
	cExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	cExp.EnsureModelNotAliveCascade(gomock.Any(), s.modelUUID.String(), false).Return(nil)
	// For the non-controller model, the controller DB no longer has the model.
	cExp.ModelExists(gomock.Any(), "model-1").Return(false, nil)
	// EnsureModelNotAliveCascade is always invoked; treat as no-op success.
	cExp.EnsureModelNotAliveCascade(gomock.Any(), "model-1", false).Return(nil)
	// The model DB also doesn't have the model, triggering NotFound path inside removeModel.
	mExp.ModelExists(gomock.Any(), "model-1").Return(false, nil)

	err := s.newService(c).RemoveController(c.Context(), false, 0)
	c.Assert(err, tc.ErrorIsNil)
}
