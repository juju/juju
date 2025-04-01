// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/unit"
	passworderrors "github.com/juju/juju/domain/password/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type agentAuthenticatorSuite struct {
	testing.IsolationSuite

	passwordService *MockPasswordService
}

var _ = gc.Suite(&agentAuthenticatorSuite{})

func (s *agentAuthenticatorSuite) TestStub(c *gc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Valid machine login.
- Invalid login for machine not provisioned.
- Login for invalid relation entity.
`)
}

func (s *agentAuthenticatorSuite) TestUserLogin(c *gc.C) {
	defer s.setupMocks(c).Finish()

	authTag := names.NewUserTag("joeblogs")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.passwordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(context.Background(), authentication.AuthParams{
		AuthTag: authTag,
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrBadRequest)
}

func (s *agentAuthenticatorSuite) TestUnitLogin(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.passwordService.EXPECT().IsValidUnitPassword(gomock.Any(), unit.Name("foo/0"), "password").Return(true, nil)

	authTag := names.NewUnitTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.passwordService, nil, loggertesting.WrapCheckLog(c))
	entity, err := authenticatorGetter.Authenticator().Authenticate(context.Background(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "password",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(entity.Tag(), gc.DeepEquals, authTag)
}

func (s *agentAuthenticatorSuite) TestUnitLoginEmptyCredentials(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.passwordService.EXPECT().IsValidUnitPassword(gomock.Any(), unit.Name("foo/0"), "").Return(false, passworderrors.EmptyPassword)

	authTag := names.NewUnitTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.passwordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(context.Background(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrBadRequest)
}

func (s *agentAuthenticatorSuite) TestUnitLoginInvalidCredentials(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.passwordService.EXPECT().IsValidUnitPassword(gomock.Any(), unit.Name("foo/0"), "").Return(false, passworderrors.InvalidPassword)

	authTag := names.NewUnitTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.passwordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(context.Background(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrUnauthorized)
}

func (s *agentAuthenticatorSuite) TestUnitLoginUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.passwordService.EXPECT().IsValidUnitPassword(gomock.Any(), unit.Name("foo/0"), "").Return(false, passworderrors.UnitNotFound)

	authTag := names.NewUnitTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.passwordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(context.Background(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrUnauthorized)
}

func (s *agentAuthenticatorSuite) TestUnitLoginUnitError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.passwordService.EXPECT().IsValidUnitPassword(gomock.Any(), unit.Name("foo/0"), "").Return(false, errors.Errorf("boom"))

	authTag := names.NewUnitTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.passwordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(context.Background(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "",
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *agentAuthenticatorSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.passwordService = NewMockPasswordService(ctrl)

	return ctrl
}
