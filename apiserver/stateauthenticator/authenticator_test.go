// Copyright 2014-2018 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/stateauthenticator"
	coreuser "github.com/juju/juju/core/user"
	statetesting "github.com/juju/juju/state/testing"
)

// TODO update these tests (moved from apiserver) to test
// via the public interface, and then get rid of export_test.go.
type agentAuthenticatorSuite struct {
	statetesting.StateSuite
	authenticator           *stateauthenticator.Authenticator
	controllerConfigService *MockControllerConfigService
	userService             *MockUserService
}

var _ = gc.Suite(&agentAuthenticatorSuite{})

func (s *agentAuthenticatorSuite) TestAuthenticateLoginRequestHandleNotSupportedRequests(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.authenticator.AuthenticateLoginRequest(context.Background(), "", "", authentication.AuthParams{Token: "token"})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *agentAuthenticatorSuite) TestAuthenticatorForTag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	user := coreuser.User{
		Name: "user",
	}
	tag := names.NewUserTag("user")

	authenticator, err := stateauthenticator.EntityAuthenticator(context.Background(), s.authenticator, tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(authenticator, gc.NotNil)

	s.userService.EXPECT().GetUserByAuth(context.Background(), "user", "password").Return(user, nil).AnyTimes()

	entity, err := authenticator.Authenticate(context.Background(), authentication.AuthParams{
		AuthTag:     tag,
		Credentials: "password",
		Nonce:       "nonce",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Tag(), gc.DeepEquals, tag)
}

func (s *agentAuthenticatorSuite) TestMachineGetsAgentAuthenticator(c *gc.C) {
	defer s.setupMocks(c).Finish()

	authenticator, err := stateauthenticator.EntityAuthenticator(context.Background(), s.authenticator, names.NewMachineTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	_, ok := authenticator.(*authentication.AgentAuthenticator)
	c.Assert(ok, jc.IsTrue)
}

func (s *agentAuthenticatorSuite) TestModelGetsAgentAuthenticator(c *gc.C) {
	defer s.setupMocks(c).Finish()

	authenticator, err := stateauthenticator.EntityAuthenticator(context.Background(), s.authenticator, names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f00d"))
	c.Assert(err, jc.ErrorIsNil)
	_, ok := authenticator.(*authentication.AgentAuthenticator)
	c.Assert(ok, jc.IsTrue)
}

func (s *agentAuthenticatorSuite) TestUnitGetsAgentAuthenticator(c *gc.C) {
	defer s.setupMocks(c).Finish()

	authenticator, err := stateauthenticator.EntityAuthenticator(context.Background(), s.authenticator, names.NewUnitTag("wordpress/0"))
	c.Assert(err, jc.ErrorIsNil)
	_, ok := authenticator.(*authentication.AgentAuthenticator)
	c.Assert(ok, jc.IsTrue)
}

func (s *agentAuthenticatorSuite) TestNotSupportedTag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	authenticator, err := stateauthenticator.EntityAuthenticator(context.Background(), s.authenticator, names.NewCloudTag("not-support"))
	c.Assert(err, gc.ErrorMatches, "unexpected login entity tag: invalid request")
	c.Assert(authenticator, gc.IsNil)
}

func (s *agentAuthenticatorSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(s.ControllerConfig, nil).AnyTimes()

	s.userService = NewMockUserService(ctrl)

	authenticator, err := stateauthenticator.NewAuthenticator(s.StatePool, s.controllerConfigService, s.userService, clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.authenticator = authenticator

	return ctrl
}
