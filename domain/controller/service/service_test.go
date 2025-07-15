// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

type serviceSuite struct {
	testhelpers.IsolationSuite
	state *MockState
}

func TestServiceSuite(t *testing.T) {
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

	s.state.EXPECT().GetControllerModelUUID(gomock.Any()).Return(controllerModelUUID, nil)

	uuid, err := st.ControllerModelUUID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, controllerModelUUID)
}

func (s *serviceSuite) TestGetControllerAgentInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()
	st := NewService(s.state)

	servingInfo := controller.ControllerAgentInfo{
		APIPort:        1234,
		Cert:           "test-cert",
		PrivateKey:     "test-private-key",
		CAPrivateKey:   "test-ca-private-key",
		SystemIdentity: "test-system-identity",
	}

	s.state.EXPECT().GetControllerAgentInfo(gomock.Any()).Return(servingInfo, nil)

	info, err := st.GetControllerAgentInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info, tc.Equals, servingInfo)
}
