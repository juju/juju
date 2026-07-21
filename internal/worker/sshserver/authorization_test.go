// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"errors"
	"testing"

	"github.com/juju/tc"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type authorizationSuite struct{}

func TestAuthorizationSuite(t *testing.T) {
	tc.Run(t, &authorizationSuite{})
}

func (s *authorizationSuite) TestJWTModelAdminAccess(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	token, err := jwt.NewBuilder().Claim("access", map[string]any{
		"model-" + destination.ModelUUID().String(): permission.AdminAccess.String(),
	}).Build()
	c.Assert(err, tc.ErrorIsNil)
	ctx := &stubAuthenticationContext{values: map[any]any{
		authenticatedViaPublicKey{}: false,
		userJWT{}:                   token,
	}}

	authorizer := authorizer{logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorIsNil)
	c.Check(authorized, tc.IsTrue)
}

func (s *authorizationSuite) TestPublicKeyAccessAllowed(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	access := &stubAccessService{allowed: true}
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{
		authenticatedViaPublicKey{}: true,
	}}

	authorizer := authorizer{access: access, logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorIsNil)
	c.Check(authorized, tc.IsTrue)
	c.Check(access.username, tc.Equals, "alice")
	c.Check(access.destination, tc.Equals, destination)
}

func (s *authorizationSuite) TestPublicKeyAccessDenied(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	access := &stubAccessService{allowed: false}
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{
		authenticatedViaPublicKey{}: true,
	}}

	authorizer := authorizer{access: access, logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorIsNil)
	c.Check(authorized, tc.IsFalse)
	c.Check(access.username, tc.Equals, "alice")
	c.Check(access.destination, tc.Equals, destination)
}

func (s *authorizationSuite) TestJWTAccessRejectsNonAdmin(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	token, err := jwt.NewBuilder().Claim("access", map[string]any{
		"model-" + destination.ModelUUID().String(): permission.WriteAccess.String(),
	}).Build()
	c.Assert(err, tc.ErrorIsNil)

	ctx := &stubAuthenticationContext{values: map[any]any{
		authenticatedViaPublicKey{}: false,
		userJWT{}:                   token,
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
		authenticatedViaPublicKey{}: false,
		userJWT{}:                   token,
	}}
	authorizer := authorizer{logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorMatches, "invalid SSH JWT token, missing access claim")
	c.Check(authorized, tc.IsFalse)
}

func (s *authorizationSuite) TestAuthorizeRejectsMissingAuthenticationMethod(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)

	authorized, err := authorizer{}.Authorize(&stubAuthenticationContext{values: map[any]any{}}, destination)
	c.Check(err, tc.ErrorMatches, "SSH authentication method is missing from connection context")
	c.Check(authorized, tc.IsFalse)
}

func (s *authorizationSuite) TestAuthorizeRejectsMissingJWT(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	ctx := &stubAuthenticationContext{values: map[any]any{authenticatedViaPublicKey{}: false}}

	authorized, err := authorizer{}.Authorize(ctx, destination)
	c.Check(err, tc.ErrorMatches, "SSH JWT is missing from connection context")
	c.Check(authorized, tc.IsFalse)
}

func (s *authorizationSuite) TestPublicKeyAccessReturnsError(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	access := &stubAccessService{err: errors.New("boom")}
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{authenticatedViaPublicKey{}: true}}

	authorized, err := (authorizer{access: access}).Authorize(ctx, destination)
	c.Check(err, tc.ErrorMatches, "checking SSH access: boom")
	c.Check(authorized, tc.IsFalse)
}

type stubAccessService struct {
	allowed     bool
	err         error
	username    string
	destination virtualhostname.Info
}

func (s *stubAccessService) HasSSHAccess(_ context.Context, username string, destination virtualhostname.Info) (bool, error) {
	s.username = username
	s.destination = destination
	return s.allowed, s.err
}
