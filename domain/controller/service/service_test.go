// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

type serviceSuite struct {
	testhelpers.IsolationSuite
	state *MockState
}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) TestControllerModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()
	st := NewService(s.state)
	controllerModelUUID := model.UUID(jujutesting.ModelTag.Id())
	s.state.EXPECT().ControllerModelUUID(gomock.Any()).Return(controllerModelUUID, nil)
	uuid, err := st.ControllerModelUUID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid, tc.Equals, controllerModelUUID)
}
