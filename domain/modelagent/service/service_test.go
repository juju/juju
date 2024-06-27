// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
)

type suite struct {
	state *MockState
}

var _ = gc.Suite(&suite{})

func (s *suite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

// TestGetModelAgentVersionSuccess tests the happy path for
// Service.GetModelAgentVersion.
func (s *suite) TestGetModelAgentVersionSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelID := modeltesting.GenModelUUID(c)
	expectedVersion, err := version.Parse("4.21.65")
	c.Assert(err, jc.ErrorIsNil)
	s.state.EXPECT().GetModelAgentVersion(gomock.Any(), modelID).
		Return(expectedVersion, nil)

	svc := NewService(s.state)
	ver, err := svc.GetModelAgentVersion(context.Background(), modelID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(ver, jc.DeepEquals, expectedVersion)
}

// TestGetModelAgentVersionInvalidModelID tests that if an invalid model ID is
// passed to Service.GetModelAgentVersion, we return errors.NotValid.
func (s *suite) TestGetModelAgentVersionInvalidModelID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state)
	_, err := svc.GetModelAgentVersion(context.Background(), "invalid")
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

// TestGetModelAgentVersionModelNotFound tests that Service.GetModelAgentVersion
// returns an appropriate error when the specified model cannot be found.
func (s *suite) TestGetModelAgentVersionModelNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelID := modeltesting.GenModelUUID(c)
	modelNotFoundErr := fmt.Errorf("%w for id %q", modelerrors.NotFound, modelID)
	s.state.EXPECT().GetModelAgentVersion(gomock.Any(), modelID).
		Return(version.Zero, modelNotFoundErr)

	svc := NewService(s.state)
	_, err := svc.GetModelAgentVersion(context.Background(), modelID)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}
