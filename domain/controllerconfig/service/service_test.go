// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucontroller "github.com/juju/juju/controller"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestUpdateControllerConfigSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	//rawControllerConfig := map[string]interface{}{
	//	jujucontroller.AuditingEnabled:     "1",
	//	jujucontroller.AuditLogCaptureArgs: "0",
	//	jujucontroller.AuditLogMaxBackups:  "10",
	//	jujucontroller.PublicDNSAddress:    "controller.test.com:1234",
	//	jujucontroller.APIPortOpenDelay:    "100ms",
	//}

	coercedControllerConfig := jujucontroller.Config{
		jujucontroller.AuditingEnabled:     true,
		jujucontroller.AuditLogCaptureArgs: false,
		jujucontroller.AuditLogMaxBackups:  "10",
		jujucontroller.PublicDNSAddress:    "controller.test.com:1234",
		jujucontroller.APIPortOpenDelay:    "100ms",
	}

	k1 := jujucontroller.AuditingEnabled
	k2 := jujucontroller.APIPortOpenDelay

	s.state.EXPECT().UpdateControllerConfig(gomock.Any(), coercedControllerConfig, []string{k1, k2}).Return(nil)

	err := NewService(s.state).UpdateControllerConfig(context.Background(), coercedControllerConfig, []string{k1, k2})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateExternalControllerError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cc := map[string]interface{}{
		jujucontroller.AuditingEnabled:     "1",
		jujucontroller.AuditLogCaptureArgs: "0",
		jujucontroller.AuditLogMaxBackups:  "10",
		jujucontroller.PublicDNSAddress:    "controller.test.com:1234",
		jujucontroller.APIPortOpenDelay:    "100ms",
	}

	s.state.EXPECT().UpdateControllerConfig(gomock.Any(), cc, nil).Return(errors.New("boom"))

	err := NewService(s.state).UpdateControllerConfig(context.Background(), cc, nil)
	c.Assert(err, gc.ErrorMatches, "updating controller config state: boom")
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}
