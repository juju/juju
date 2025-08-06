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
