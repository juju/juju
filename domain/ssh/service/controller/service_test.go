// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	stdtesting "testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/user"
	controllersshservice "github.com/juju/juju/domain/ssh/service/controller"
)

type serviceSuite struct{}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestSSHServerHostKeyReturnsExisting(c *tc.C) {
	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().GetSSHServerHostKey(gomock.Any()).Return(testPrivateKey, nil)

	svc := controllersshservice.NewService(controllerState)
	key, err := svc.SSHServerHostKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *serviceSuite) TestSSHServerHostKeyErrorsWhenMissing(c *tc.C) {
	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().GetSSHServerHostKey(gomock.Any()).Return("", context.Canceled)

	svc := controllersshservice.NewService(controllerState)

	key, err := svc.SSHServerHostKey(c.Context())
	c.Check(key, tc.Equals, "")
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

// TestSSHServerHostPublicKeyReturnsStored checks the public key is returned
// directly from state without deriving it from the private key.
func (s *serviceSuite) TestSSHServerHostPublicKeyReturnsStored(c *tc.C) {
	signer, err := gossh.ParsePrivateKey([]byte(testPrivateKey))
	c.Assert(err, tc.ErrorIsNil)
	want := signer.PublicKey().Marshal()

	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().GetSSHServerHostPublicKey(gomock.Any()).Return(want, nil)
	svc := controllersshservice.NewService(controllerState)

	got, err := svc.SSHServerHostPublicKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, want)

}

// TestSSHServerHostPublicKeyErrorsWhenMissing checks that a state error
// fetching the public key is propagated to the caller.
func (s *serviceSuite) TestSSHServerHostPublicKeyErrorsWhenMissing(c *tc.C) {
	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().GetSSHServerHostPublicKey(gomock.Any()).Return(nil, context.Canceled)

	svc := controllersshservice.NewService(controllerState)

	got, err := svc.SSHServerHostPublicKey(c.Context())
	c.Check(got, tc.IsNil)
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *serviceSuite) TestGetPublicKeysForUser(c *tc.C) {
	keys := []coressh.PublicKey{{Key: "ssh-ed25519 AAAA"}}
	controllerState := NewMockState(gomock.NewController(c))
	username, err := user.NewName("alice")
	c.Assert(err, tc.ErrorIsNil)
	controllerState.EXPECT().GetPublicKeysForUser(gomock.Any(), username).Return(keys, nil)

	got, err := controllersshservice.NewService(controllerState).GetPublicKeysForUser(c.Context(), username)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, keys)
}

const testPrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\n" +
	"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz\n" +
	"c2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7VHoJY7LZ7yXzuWlSVYAAA\n" +
	"AIiZq0wRmatMEQAAAAtzc2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7V\n" +
	"HoJY7LZ7yXzuWlSVYAAAAEBYRsJTytYJUidtOuv3s3tdjyDA+4TSdCz9+hFKjyqz\n" +
	"v1PxSJ2ipSalQUUIYSFmEdYYTtUegljstnvJfO5aVJVgAAAAAAECAwQF\n" +
	"-----END OPENSSH PRIVATE KEY-----\n"
