// Copyright 2014-2018 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/auth"
	statetesting "github.com/juju/juju/state/testing"
)

// TODO update these tests (moved from apiserver) to test
// via the public interface, and then get rid of export_test.go.
type agentAuthenticatorSuite struct {
	statetesting.StateSuite

	authenticator             *Authenticator
	entityAuthenticator       *MockEntityAuthenticator
	agentAuthenticatorFactory *MockAgentAuthenticatorFactory
	controllerConfigService   *MockControllerConfigService
	userService               *MockUserService
	bakeryConfigService       *MockBakeryConfigService
}

var _ = gc.Suite(&agentAuthenticatorSuite{})

func (s *agentAuthenticatorSuite) TestAuthenticateLoginRequestHandleNotSupportedRequests(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.agentAuthenticatorFactory.EXPECT().AuthenticatorForState(gomock.Any()).Return(s.entityAuthenticator)

	_, err := s.authenticator.AuthenticateLoginRequest(context.Background(), "", "", authentication.AuthParams{Token: "token"})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *agentAuthenticatorSuite) TestAuthenticatorForTag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	user := coreuser.User{
		Name: "user",
	}
	tag := names.NewUserTag("user")

	s.agentAuthenticatorFactory.EXPECT().Authenticator().Return(s.entityAuthenticator)

	authenticator, err := s.authenticatorForTag(context.Background(), s.authenticator, tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(authenticator, gc.NotNil)

	s.userService.EXPECT().GetUserByAuth(context.Background(), "user", auth.NewPassword("password")).Return(user, nil).AnyTimes()

	entity, err := authenticator.Authenticate(context.Background(), authentication.AuthParams{
		AuthTag:     tag,
		Credentials: "password",
		Nonce:       "nonce",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(entity.Tag(), gc.DeepEquals, tag)
}

func (s *agentAuthenticatorSuite) TestNotSupportedTag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.agentAuthenticatorFactory.EXPECT().Authenticator().Return(s.entityAuthenticator)

	authenticator, err := s.authenticatorForTag(context.Background(), s.authenticator, names.NewCloudTag("not-support"))
	c.Assert(err, gc.ErrorMatches, "unexpected login entity tag: invalid request")
	c.Check(authenticator, gc.IsNil)
}

func (s *agentAuthenticatorSuite) TestMachineGetsAgentAuthenticator(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("0")

	s.agentAuthenticatorFactory.EXPECT().Authenticator().Return(s.entityAuthenticator)
	s.entityAuthenticator.EXPECT().Authenticate(gomock.Any(), authentication.AuthParams{}).Return(authentication.TagToEntity(tag), nil)

	authenticator, err := s.authenticatorForTag(context.Background(), s.authenticator, tag)
	c.Assert(err, jc.ErrorIsNil)
	entity, err := authenticator.Authenticate(context.Background(), authentication.AuthParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(entity.Tag(), gc.Equals, tag)
}

func (s *agentAuthenticatorSuite) TestModelGetsAgentAuthenticator(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f00d")

	s.agentAuthenticatorFactory.EXPECT().Authenticator().Return(s.entityAuthenticator)
	s.entityAuthenticator.EXPECT().Authenticate(gomock.Any(), authentication.AuthParams{}).Return(authentication.TagToEntity(tag), nil)

	authenticator, err := s.authenticatorForTag(context.Background(), s.authenticator, tag)
	c.Assert(err, jc.ErrorIsNil)
	entity, err := authenticator.Authenticate(context.Background(), authentication.AuthParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(entity.Tag(), gc.Equals, tag)
}

func (s *agentAuthenticatorSuite) TestUnitGetsAgentAuthenticator(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("wordpress/0")

	s.agentAuthenticatorFactory.EXPECT().Authenticator().Return(s.entityAuthenticator)
	s.entityAuthenticator.EXPECT().Authenticate(gomock.Any(), authentication.AuthParams{}).Return(authentication.TagToEntity(tag), nil)

	authenticator, err := s.authenticatorForTag(context.Background(), s.authenticator, tag)
	c.Assert(err, jc.ErrorIsNil)
	entity, err := authenticator.Authenticate(context.Background(), authentication.AuthParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(entity.Tag(), gc.Equals, tag)
}

func (s *agentAuthenticatorSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agentAuthenticatorFactory = NewMockAgentAuthenticatorFactory(ctrl)
	s.entityAuthenticator = NewMockEntityAuthenticator(ctrl)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(s.ControllerConfig, nil).AnyTimes()

	s.userService = NewMockUserService(ctrl)

	s.bakeryConfigService = NewMockBakeryConfigService(ctrl)
	s.bakeryConfigService.EXPECT().GetLocalUsersKey(gomock.Any()).Return(bakery.MustGenerateKey(), nil).MinTimes(1)
	s.bakeryConfigService.EXPECT().GetLocalUsersThirdPartyKey(gomock.Any()).Return(bakery.MustGenerateKey(), nil).MinTimes(1)

	authenticator, err := NewAuthenticator(context.Background(), s.StatePool, s.State, s.controllerConfigService, s.userService, s.bakeryConfigService, s.agentAuthenticatorFactory, clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.authenticator = authenticator

	return ctrl
}

func (s *agentAuthenticatorSuite) authenticatorForTag(ctx context.Context, authenticator *Authenticator, tag names.Tag) (authentication.EntityAuthenticator, error) {
	return authenticator.authContext.authenticator("testing.invalid:1234").authenticatorForTag(ctx, tag)
}
