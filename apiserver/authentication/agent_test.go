// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	agentpassworderrors "github.com/juju/juju/domain/agentpassword/errors"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type agentAuthenticatorSuite struct {
	testhelpers.IsolationSuite

	agentPasswordService *MockAgentPasswordService
}

func TestAgentAuthenticatorSuite(t *testing.T) {
	tc.Run(t, &agentAuthenticatorSuite{})
}

func (s *agentAuthenticatorSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Valid machine login.
- Invalid login for machine not provisioned.
- Login for invalid relation entity.
`)
}

func (s *agentAuthenticatorSuite) TestUserLogin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authTag := names.NewUserTag("joeblogs")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.agentPasswordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(c.Context(), authentication.AuthParams{
		AuthTag: authTag,
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrBadRequest)
}

func (s *agentAuthenticatorSuite) TestUnitLogin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().MatchesUnitPasswordHash(gomock.Any(), unit.Name("foo/0"), "password").Return(true, nil)

	authTag := names.NewUnitTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.agentPasswordService, nil, loggertesting.WrapCheckLog(c))
	entity, err := authenticatorGetter.Authenticator().Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "password",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entity.Tag(), tc.DeepEquals, authTag)
}

func (s *agentAuthenticatorSuite) TestUnitLoginEmptyCredentials(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().MatchesUnitPasswordHash(gomock.Any(), unit.Name("foo/0"), "").Return(false, agentpassworderrors.EmptyPassword)

	authTag := names.NewUnitTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.agentPasswordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrBadRequest)
}

func (s *agentAuthenticatorSuite) TestUnitLoginInvalidCredentials(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().MatchesUnitPasswordHash(gomock.Any(), unit.Name("foo/0"), "").Return(false, agentpassworderrors.InvalidPassword)

	authTag := names.NewUnitTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.agentPasswordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrUnauthorized)
}

func (s *agentAuthenticatorSuite) TestUnitLoginUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().MatchesUnitPasswordHash(gomock.Any(), unit.Name("foo/0"), "").Return(false, applicationerrors.UnitNotFound)

	authTag := names.NewUnitTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.agentPasswordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrUnauthorized)
}

func (s *agentAuthenticatorSuite) TestUnitLoginUnitError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().MatchesUnitPasswordHash(gomock.Any(), unit.Name("foo/0"), "").Return(false, errors.Errorf("boom"))

	authTag := names.NewUnitTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.agentPasswordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "",
	})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *agentAuthenticatorSuite) TestMachineLogin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().MatchesMachinePasswordHashWithNonce(gomock.Any(), machine.Name("0"), "password", "nonce").Return(true, nil)

	authTag := names.NewMachineTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.agentPasswordService, nil, loggertesting.WrapCheckLog(c))
	entity, err := authenticatorGetter.Authenticator().Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "password",
		Nonce:       "nonce",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entity.Tag(), tc.DeepEquals, authTag)
}

func (s *agentAuthenticatorSuite) TestMachineLoginEmptyCredentials(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().MatchesMachinePasswordHashWithNonce(gomock.Any(), machine.Name("0"), "", "").Return(false, agentpassworderrors.EmptyPassword)

	authTag := names.NewMachineTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.agentPasswordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "",
		Nonce:       "",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrBadRequest)
}

func (s *agentAuthenticatorSuite) TestMachineLoginEmptyNonce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().MatchesMachinePasswordHashWithNonce(gomock.Any(), machine.Name("0"), "password", "").Return(false, agentpassworderrors.EmptyNonce)

	authTag := names.NewMachineTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.agentPasswordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "password",
		Nonce:       "",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrBadRequest)
}

func (s *agentAuthenticatorSuite) TestMachineLoginInvalidCredentials(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().MatchesMachinePasswordHashWithNonce(gomock.Any(), machine.Name("0"), "", "").Return(false, agentpassworderrors.InvalidPassword)

	authTag := names.NewMachineTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.agentPasswordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "",
		Nonce:       "",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrUnauthorized)
}

func (s *agentAuthenticatorSuite) TestMachineLoginMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().MatchesMachinePasswordHashWithNonce(gomock.Any(), machine.Name("0"), "", "").Return(false, applicationerrors.MachineNotFound)

	authTag := names.NewMachineTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.agentPasswordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "",
		Nonce:       "",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrUnauthorized)
}

func (s *agentAuthenticatorSuite) TestMachineLoginMachineError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().MatchesMachinePasswordHashWithNonce(gomock.Any(), machine.Name("0"), "", "").Return(false, errors.Errorf("boom"))

	authTag := names.NewMachineTag("foo/0")

	authenticatorGetter := authentication.NewAgentAuthenticatorGetter(s.agentPasswordService, nil, loggertesting.WrapCheckLog(c))
	_, err := authenticatorGetter.Authenticator().Authenticate(c.Context(), authentication.AuthParams{
		AuthTag:     authTag,
		Credentials: "",
	})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *agentAuthenticatorSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agentPasswordService = NewMockAgentPasswordService(ctrl)

	return ctrl
}
