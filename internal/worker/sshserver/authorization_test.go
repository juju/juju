// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"errors"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type authorizationSuite struct {
	testhelpers.IsolationSuite

	ctrl *gomock.Controller
}

func TestAuthorizationSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &authorizationSuite{})
	})
}

func (s *authorizationSuite) SetUpMocks(c *tc.C) *gomock.Controller {
	s.ctrl = gomock.NewController(c)
	return s.ctrl
}

func (s *authorizationSuite) TestJWTModelAdminAccess(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	token, err := jwt.NewBuilder().Claim("access", map[string]any{
		"model-" + destination.ModelUUID().String(): permission.AdminAccess.String(),
	}).Build()
	c.Assert(err, tc.ErrorIsNil)
	ctx := &stubAuthenticationContext{values: map[any]any{

		userJWT{}: token,
	}}

	authorizer := authorizer{logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorIsNil)
	c.Check(authorized, tc.IsTrue)
}

func (s *authorizationSuite) TestPublicKeyAccessAllowed(c *tc.C) {
	s.SetUpMocks(c)

	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	authKey := newSigner(c).PublicKey()
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{
		authenticatedPublicKey{}: newSigner(c).PublicKey(),
		authenticatedPublicKey{}: authKey,
	}}
	access := NewMockAccessService(s.ctrl)
	access.EXPECT().HasSSHAccessToModel(gomock.Any(), "alice", destination).Return(true, nil)
	access.EXPECT().PublicKeyInModel(gomock.Any(), "alice", authKey, destination).Return(true, nil)

	authorizer := authorizer{access: access, logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorIsNil)
	c.Check(authorized, tc.IsTrue)
}

func (s *authorizationSuite) TestPublicKeyAccessDenied(c *tc.C) {
	s.SetUpMocks(c)

	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{
		authenticatedPublicKey{}: newSigner(c).PublicKey(),
		authenticatedPublicKey{}: newSigner(c).PublicKey(),
	}}
	access := NewMockAccessService(s.ctrl)
	access.EXPECT().HasSSHAccessToModel(gomock.Any(), "alice", destination).Return(false, nil)

	authorizer := authorizer{access: access, logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorIsNil)
	c.Check(authorized, tc.IsFalse)
}

func (s *authorizationSuite) TestPublicKeyNotInModelRejected(c *tc.C) {
	s.SetUpMocks(c)

	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{
		authenticatedPublicKey{}: newSigner(c).PublicKey(),
		authenticatedPublicKey{}: newSigner(c).PublicKey(),
	}}
	access := NewMockAccessService(s.ctrl)
	access.EXPECT().HasSSHAccessToModel(gomock.Any(), "alice", destination).Return(true, nil)
	access.EXPECT().PublicKeyInModel(gomock.Any(), "alice", gomock.Any(), destination).Return(false, nil)

	authorizer := authorizer{access: access, logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorMatches, `public key used to authenticate is not associated with model "8419cd78-4993-4c3a-928e-c646226beeee", add the key to the model to access it`)
	c.Check(authorized, tc.IsFalse)
}

func (s *authorizationSuite) TestPublicKeyModelKeyCheckError(c *tc.C) {
	s.SetUpMocks(c)

	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{
		authenticatedPublicKey{}: newSigner(c).PublicKey(),
		authenticatedPublicKey{}: newSigner(c).PublicKey(),
	}}
	access := NewMockAccessService(s.ctrl)
	access.EXPECT().HasSSHAccessToModel(gomock.Any(), "alice", destination).Return(true, nil)
	access.EXPECT().PublicKeyInModel(gomock.Any(), "alice", gomock.Any(), destination).Return(false, errors.New("boom"))

	authorizer := authorizer{access: access, logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorMatches, "checking SSH key for model: boom")
	c.Check(authorized, tc.IsFalse)
}

func (s *authorizationSuite) TestJWTAccessRejectsNonAdmin(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	token, err := jwt.NewBuilder().Claim("access", map[string]any{
		"model-" + destination.ModelUUID().String(): permission.WriteAccess.String(),
	}).Build()
	c.Assert(err, tc.ErrorIsNil)

	ctx := &stubAuthenticationContext{values: map[any]any{

		userJWT{}: token,
	}}
	authorizer := authorizer{logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorIsNil)
	c.Check(authorized, tc.IsFalse)
}

func (s *authorizationSuite) TestJWTAccessRejectsJWTWithMissingAccessClaim(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	token, err := jwt.NewBuilder().Build()
	c.Assert(err, tc.ErrorIsNil)

	ctx := &stubAuthenticationContext{values: map[any]any{

		userJWT{}: token,
	}}
	authorizer := authorizer{logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorMatches, "invalid SSH JWT token, missing access claim")
	c.Check(authorized, tc.IsFalse)
}

func (s *authorizationSuite) TestJWTAccessRejectsJWTWithInvalidAccessClaim(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	token, err := jwt.NewBuilder().Claim("access", "invalid").Build()
	c.Assert(err, tc.ErrorIsNil)

	ctx := &stubAuthenticationContext{values: map[any]any{

		userJWT{}: token,
	}}
	authorizer := authorizer{logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorMatches, "invalid SSH JWT token, invalid access claim")
	c.Check(authorized, tc.IsFalse)
}

func (s *authorizationSuite) TestAuthorizeRejectsMissingJWT(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	ctx := &stubAuthenticationContext{values: map[any]any{}}

	authorized, err := authorizer{}.Authorize(ctx, destination)
	c.Check(err, tc.ErrorMatches, "SSH JWT is missing from connection context")
	c.Check(authorized, tc.IsFalse)
}

func (s *authorizationSuite) TestPublicKeyAccessReturnsError(c *tc.C) {
	s.SetUpMocks(c)

	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{authenticatedPublicKey{}: newSigner(c).PublicKey()}}
	access := NewMockAccessService(s.ctrl)
	access.EXPECT().HasSSHAccessToModel(gomock.Any(), "alice", destination).Return(false, errors.New("boom"))

	authorized, err := (authorizer{access: access}).Authorize(ctx, destination)
	c.Check(err, tc.ErrorMatches, "checking SSH access: boom")
	c.Check(authorized, tc.IsFalse)
}
