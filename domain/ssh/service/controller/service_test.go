// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	stdtesting "testing"

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
	controllerState := &stubControllerState{
		key: testPrivateKey,
	}

	svc := controllersshservice.NewService(controllerState)
	key, err := svc.SSHServerHostKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *serviceSuite) TestSSHServerHostKeyErrorsWhenMissing(c *tc.C) {
	svc := controllersshservice.NewService(&stubControllerState{getErr: context.Canceled})

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

	controllerState := &stubControllerState{
		key:       testPrivateKey,
		publicKey: want,
	}
	svc := controllersshservice.NewService(controllerState)

	got, err := svc.SSHServerHostPublicKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, want)

	// The public key is read from state; the private key is never fetched.
	c.Check(controllerState.gets, tc.Equals, 0)
	c.Check(controllerState.publicKeyGets, tc.Equals, 1)
}

// TestSSHServerHostPublicKeyErrorsWhenMissing checks that a state error
// fetching the public key is propagated to the caller.
func (s *serviceSuite) TestSSHServerHostPublicKeyErrorsWhenMissing(c *tc.C) {
	svc := controllersshservice.NewService(&stubControllerState{publicKeyGetErr: context.Canceled})

	got, err := svc.SSHServerHostPublicKey(c.Context())
	c.Check(got, tc.IsNil)
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *serviceSuite) TestGetPublicKeysForUser(c *tc.C) {
	keys := []coressh.PublicKey{{Key: "ssh-ed25519 AAAA"}}
	controllerState := &stubControllerState{publicKeys: keys}
	username, err := user.NewName("alice")
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllersshservice.NewService(controllerState).GetPublicKeysForUser(c.Context(), username)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, keys)
}

type stubControllerState struct {
	key             string
	getErr          error
	gets            int
	publicKey       []byte
	publicKeyGetErr error
	publicKeyGets   int
	publicKeys      []coressh.PublicKey
}

func (s *stubControllerState) GetSSHServerHostKey(_ context.Context) (string, error) {
	s.gets++
	return s.key, s.getErr
}

func (s *stubControllerState) GetSSHServerHostPublicKey(_ context.Context) ([]byte, error) {
	s.publicKeyGets++
	return s.publicKey, s.publicKeyGetErr
}

func (s *stubControllerState) GetPublicKeysForUser(context.Context, user.Name) ([]coressh.PublicKey, error) {
	return s.publicKeys, nil
}

const testPrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\n" +
	"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz\n" +
	"c2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7VHoJY7LZ7yXzuWlSVYAAA\n" +
	"AIiZq0wRmatMEQAAAAtzc2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7V\n" +
	"HoJY7LZ7yXzuWlSVYAAAAEBYRsJTytYJUidtOuv3s3tdjyDA+4TSdCz9+hFKjyqz\n" +
	"v1PxSJ2ipSalQUUIYSFmEdYYTtUegljstnvJfO5aVJVgAAAAAAECAwQF\n" +
	"-----END OPENSSH PRIVATE KEY-----\n"
