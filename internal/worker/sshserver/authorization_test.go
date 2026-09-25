// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"errors"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

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

func (s *authorizationSuite) TestPublicKeyAccessAllowed(c *tc.C) {
	s.SetUpMocks(c)

	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	signer := newSigner(c)
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{
		authenticatedPublicKey{}: publicKeyWithComment{
			PublicKey: signer.PublicKey(),
		},
	}}
	access := NewMockAccessService(s.ctrl)
	access.EXPECT().HasSSHAccessToModel(gomock.Any(), "alice", destination).Return(true, nil)
	access.EXPECT().HasPublicKeyInModel(gomock.Any(), "alice", signer.PublicKey(), destination).Return(true, nil)

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
		authenticatedPublicKey{}: publicKeyWithComment{
			PublicKey: newSigner(c).PublicKey(),
			Comment:   "my laptop",
		},
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
		authenticatedPublicKey{}: publicKeyWithComment{
			PublicKey: newSigner(c).PublicKey(),
			Comment:   "my laptop",
		},
	}}
	access := NewMockAccessService(s.ctrl)
	access.EXPECT().HasSSHAccessToModel(gomock.Any(), "alice", destination).Return(true, nil)
	access.EXPECT().HasPublicKeyInModel(gomock.Any(), "alice", gomock.Any(), destination).Return(false, nil)

	authorizer := authorizer{access: access, logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorMatches, `public key "my laptop" used to authenticate is not associated with model "8419cd78-4993-4c3a-928e-c646226beeee", add the key to the model or specify a different key`)
	c.Check(authorized, tc.IsFalse)
}

func (s *authorizationSuite) TestPublicKeyModelKeyCheckError(c *tc.C) {
	s.SetUpMocks(c)

	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{
		authenticatedPublicKey{}: publicKeyWithComment{
			PublicKey: newSigner(c).PublicKey(),
		},
	}}
	access := NewMockAccessService(s.ctrl)
	access.EXPECT().HasSSHAccessToModel(gomock.Any(), "alice", destination).Return(true, nil)
	access.EXPECT().HasPublicKeyInModel(gomock.Any(), "alice", gomock.Any(), destination).Return(false, errors.New("boom"))

	authorizer := authorizer{access: access, logger: loggertesting.WrapCheckLog(c)}
	authorized, err := authorizer.Authorize(ctx, destination)
	c.Check(err, tc.ErrorMatches, "checking SSH key for model: boom")
	c.Check(authorized, tc.IsFalse)
}

func (s *authorizationSuite) TestAuthorizeRejectsMissingPublicKey(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	ctx := &stubAuthenticationContext{values: map[any]any{}}

	authorized, err := authorizer{}.Authorize(ctx, destination)
	c.Check(err, tc.ErrorMatches, "SSH connection is not authenticated via public key")
	c.Check(authorized, tc.IsFalse)
}

func (s *authorizationSuite) TestPublicKeyAccessReturnsError(c *tc.C) {
	s.SetUpMocks(c)

	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{
		authenticatedPublicKey{}: publicKeyWithComment{
			PublicKey: newSigner(c).PublicKey(),
		},
	}}
	access := NewMockAccessService(s.ctrl)
	access.EXPECT().HasSSHAccessToModel(gomock.Any(), "alice", destination).Return(false, errors.New("boom"))

	authorized, err := (authorizer{access: access}).Authorize(ctx, destination)
	c.Check(err, tc.ErrorMatches, "checking SSH access: boom")
	c.Check(authorized, tc.IsFalse)
}
