// Copyright 2014-2018 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

// TODO update these tests (moved from apiserver) to test
// via the public interface, and then get rid of export_test.go.
type agentAuthenticatorSuite struct {
	statetesting.StateSuite
	authenticator          *stateauthenticator.Authenticator
	controllerConfigGetter *MockControllerConfigGetter
}

var _ = gc.Suite(&agentAuthenticatorSuite{})

func (s *agentAuthenticatorSuite) TestAuthenticateLoginRequestHandleNotSupportedRequests(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.authenticator.AuthenticateLoginRequest(context.TODO(), "", "", authentication.AuthParams{Token: "token"})
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *agentAuthenticatorSuite) TestAuthenticatorForTag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "password"})

	authenticator, err := stateauthenticator.EntityAuthenticator(context.Background(), s.authenticator, user.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(authenticator, gc.NotNil)
	userFinder := userFinder{user}

	entity, err := authenticator.Authenticate(context.TODO(), userFinder, authentication.AuthParams{
		AuthTag:     user.Tag(),
		Credentials: "password",
		Nonce:       "nonce",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity, gc.DeepEquals, user)
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

	s.controllerConfigGetter = NewMockControllerConfigGetter(ctrl)
	s.controllerConfigGetter.EXPECT().ControllerConfig(gomock.Any()).Return(s.ControllerConfig, nil).AnyTimes()

	authenticator, err := stateauthenticator.NewAuthenticator(s.StatePool, s.controllerConfigGetter, clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.authenticator = authenticator

	return ctrl
}

type userFinder struct {
	user state.Entity
}

func (u userFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	return u.user, nil
}
