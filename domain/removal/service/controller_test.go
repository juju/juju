// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/model"
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
	s.clock.EXPECT().Now().Return(when)

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), s.modelUUID.String()).Return(true, nil)

	cExp := s.controllerState.EXPECT()
	cExp.GetModelUUIDs(gomock.Any()).Return([]string{"model-1"}, nil)

	cExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(true, nil)
	cExp.EnsureModelNotAliveCascade(gomock.Any(), s.modelUUID.String(), false).Return(nil)

	mExp.ModelExists(gomock.Any(), s.modelUUID.String()).Return(false, nil)
	mExp.EnsureModelNotAliveCascade(gomock.Any(), s.modelUUID.String(), false).Return(removal.ModelArtifacts{}, nil)
	mExp.ModelScheduleRemoval(gomock.Any(), gomock.Any(), s.modelUUID.String(), false, when.UTC()).Return(nil)

	modelUUIDs, err := s.newService(c).RemoveController(c.Context(), false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelUUIDs, tc.DeepEquals, []model.UUID{"model-1"})
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

	modelUUID, err := s.newService(c).RemoveController(c.Context(), false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelUUID, tc.DeepEquals, []model.UUID{})
}

func (s *controllerSuite) TestRemoveControllerNotController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mExp := s.modelState.EXPECT()
	mExp.IsControllerModel(gomock.Any(), s.modelUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveController(c.Context(), false, 0)
	c.Assert(err, tc.ErrorMatches, `.*not the controller model.*`)
}
