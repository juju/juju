// Copyright 2014-2018 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"
	stdtesting "testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/core/model"
	coreuser "github.com/juju/juju/core/user"
	coreusertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/testing"
	statetesting "github.com/juju/juju/state/testing"
)

// TODO update these tests (moved from apiserver) to test
// via the public interface, and then get rid of export_test.go.
type agentAuthenticatorSuite struct {
	statetesting.StateSuite

	authenticator              *Authenticator
	entityAuthenticator        *MockEntityAuthenticator
	agentAuthenticatorGetter   *MockAgentAuthenticatorGetter
	agentPasswordServiceGetter *MockAgentPasswordServiceGetter
	agentPasswordService       *MockAgentPasswordService
	controllerConfigService    *MockControllerConfigService
	accessService              *MockAccessService
	macaroonService            *MockMacaroonService
}

func TestAgentAuthenticatorSuite(t *stdtesting.T) {
	tc.Run(t, &agentAuthenticatorSuite{})
}

func (s *agentAuthenticatorSuite) TestAuthenticateLoginRequestHandleNotSupportedRequests(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordServiceGetter.EXPECT().GetAgentPasswordServiceForModel(gomock.Any(), gomock.Any()).Return(s.agentPasswordService, nil)
	s.agentAuthenticatorGetter.EXPECT().AuthenticatorForModel(gomock.Any(), gomock.Any()).Return(s.entityAuthenticator)

	_, err := s.authenticator.AuthenticateLoginRequest(c.Context(), "", "", authentication.AuthParams{Token: "token"})
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *agentAuthenticatorSuite) TestAuthenticatorForTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	user := coreuser.User{
		Name: coreusertesting.GenNewName(c, "user"),
	}
	tag := names.NewUserTag("user")

	s.agentAuthenticatorGetter.EXPECT().Authenticator().Return(s.entityAuthenticator)

	authenticator, err := s.authenticatorForTag(c.Context(), s.authenticator, tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(authenticator, tc.NotNil)

	s.accessService.EXPECT().GetUserByAuth(gomock.Any(), coreusertesting.GenNewName(c, "user"), auth.NewPassword("password")).Return(user, nil).AnyTimes()

	entity, err := authenticator.Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     tag,
		Credentials: "password",
		Nonce:       "nonce",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entity.Tag(), tc.DeepEquals, tag)
}

func (s *agentAuthenticatorSuite) TestNotSupportedTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentAuthenticatorGetter.EXPECT().Authenticator().Return(s.entityAuthenticator)

	authenticator, err := s.authenticatorForTag(c.Context(), s.authenticator, names.NewCloudTag("not-support"))
	c.Assert(err, tc.ErrorMatches, "unexpected login entity tag: invalid request")
	c.Check(authenticator, tc.IsNil)
}

func (s *agentAuthenticatorSuite) TestMachineGetsAgentAuthenticator(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("0")

	s.agentAuthenticatorGetter.EXPECT().Authenticator().Return(s.entityAuthenticator)
	s.entityAuthenticator.EXPECT().Authenticate(gomock.Any(), authentication.AuthParams{}).Return(authentication.TagToEntity(tag), nil)

	authenticator, err := s.authenticatorForTag(c.Context(), s.authenticator, tag)
	c.Assert(err, tc.ErrorIsNil)
	entity, err := authenticator.Authenticate(c.Context(), authentication.AuthParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entity.Tag(), tc.Equals, tag)
}

func (s *agentAuthenticatorSuite) TestMachineGetsAgentAuthenticatorController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("0")

	s.agentAuthenticatorGetter.EXPECT().Authenticator().Return(s.entityAuthenticator)
	s.entityAuthenticator.EXPECT().Authenticate(gomock.Any(), authentication.AuthParams{}).Return(authentication.TagToEntity(tag), nil)

	authenticator, err := s.authenticatorForTag(c.Context(), s.authenticator, tag)
	c.Assert(err, tc.ErrorIsNil)
	entity, err := authenticator.Authenticate(c.Context(), authentication.AuthParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entity.Tag(), tc.Equals, tag)
}

func (s *agentAuthenticatorSuite) TestModelGetsAgentAuthenticator(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f00d")

	s.agentAuthenticatorGetter.EXPECT().Authenticator().Return(s.entityAuthenticator)
	s.entityAuthenticator.EXPECT().Authenticate(gomock.Any(), authentication.AuthParams{}).Return(authentication.TagToEntity(tag), nil)

	authenticator, err := s.authenticatorForTag(c.Context(), s.authenticator, tag)
	c.Assert(err, tc.ErrorIsNil)
	entity, err := authenticator.Authenticate(c.Context(), authentication.AuthParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entity.Tag(), tc.Equals, tag)
}

func (s *agentAuthenticatorSuite) TestUnitGetsAgentAuthenticator(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("wordpress/0")

	s.agentAuthenticatorGetter.EXPECT().Authenticator().Return(s.entityAuthenticator)
	s.entityAuthenticator.EXPECT().Authenticate(gomock.Any(), authentication.AuthParams{}).Return(authentication.TagToEntity(tag), nil)

	authenticator, err := s.authenticatorForTag(c.Context(), s.authenticator, tag)
	c.Assert(err, tc.ErrorIsNil)
	entity, err := authenticator.Authenticate(c.Context(), authentication.AuthParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entity.Tag(), tc.Equals, tag)
}

func (s *agentAuthenticatorSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agentAuthenticatorGetter = NewMockAgentAuthenticatorGetter(ctrl)
	s.entityAuthenticator = NewMockEntityAuthenticator(ctrl)

	s.agentPasswordService = NewMockAgentPasswordService(ctrl)

	s.agentPasswordServiceGetter = NewMockAgentPasswordServiceGetter(ctrl)
	s.agentPasswordServiceGetter.EXPECT().GetAgentPasswordServiceForModel(gomock.Any(), gomock.Any()).Return(s.agentPasswordService, nil)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(s.ControllerConfig, nil).AnyTimes()

	s.accessService = NewMockAccessService(ctrl)

	s.macaroonService = NewMockMacaroonService(ctrl)
	s.macaroonService.EXPECT().GetLocalUsersKey(gomock.Any()).Return(bakery.MustGenerateKey(), nil).MinTimes(1)
	s.macaroonService.EXPECT().GetLocalUsersThirdPartyKey(gomock.Any()).Return(bakery.MustGenerateKey(), nil).MinTimes(1)

	authenticator, err := NewAuthenticator(
		c.Context(),
		s.StatePool,
		model.UUID(testing.ModelTag.Id()),
		s.controllerConfigService,
		s.agentPasswordServiceGetter,
		s.accessService,
		s.macaroonService,
		s.agentAuthenticatorGetter,
		clock.WallClock,
	)
	c.Assert(err, tc.ErrorIsNil)
	s.authenticator = authenticator

	return ctrl
}

func (s *agentAuthenticatorSuite) authenticatorForTag(ctx context.Context, authenticator *Authenticator, tag names.Tag) (authentication.EntityAuthenticator, error) {
	return authenticator.authContext.authenticator("testing.invalid:1234").authenticatorForTag(ctx, tag)
}
