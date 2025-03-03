// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	jujutesting "github.com/juju/juju/internal/testing"
)

type serviceSuite struct {
	testing.IsolationSuite
	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) TestControllerModelUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	st := NewService(s.state, nil)
	controllerModelUUID := model.UUID(jujutesting.ModelTag.Id())
	s.state.EXPECT().ControllerModelUUID(gomock.Any()).Return(controllerModelUUID, nil)
	uuid, err := st.ControllerModelUUID(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, controllerModelUUID)
}
