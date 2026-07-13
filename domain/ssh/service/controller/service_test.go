// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

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

// TestSSHServerHostPublicKeyDerivesAndCaches checks the public key is derived
// from the stored private key and that the derivation is cached: the private
// key is only parsed once, so repeated calls do not re-parse it.
func (s *serviceSuite) TestSSHServerHostPublicKeyDerivesAndCaches(c *tc.C) {
	controllerState := &stubControllerState{key: testPrivateKey}
	svc := controllersshservice.NewService(controllerState)

	signer, err := gossh.ParsePrivateKey([]byte(testPrivateKey))
	c.Assert(err, tc.ErrorIsNil)
	want := signer.PublicKey().Marshal()

	got, err := svc.SSHServerHostPublicKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, want)

	// A second call returns the same cached key and does not re-fetch or
	// re-derive it: the private key is fetched from state exactly once.
	got2, err := svc.SSHServerHostPublicKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got2, tc.DeepEquals, want)
	c.Check(controllerState.gets, tc.Equals, 1)
}

// TestSSHServerHostPublicKeyErrorsWhenMissing checks that a state error fetching
// the host key is propagated to the caller.
func (s *serviceSuite) TestSSHServerHostPublicKeyErrorsWhenMissing(c *tc.C) {
	svc := controllersshservice.NewService(&stubControllerState{getErr: context.Canceled})

	got, err := svc.SSHServerHostPublicKey(c.Context())
	c.Check(got, tc.IsNil)
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

type stubControllerState struct {
	key    string
	getErr error
	gets   int
}

func (s *stubControllerState) GetSSHServerHostKey(_ context.Context) (string, error) {
	s.gets++
	return s.key, s.getErr
}

const testPrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\n" +
	"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz\n" +
	"c2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7VHoJY7LZ7yXzuWlSVYAAA\n" +
	"AIiZq0wRmatMEQAAAAtzc2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7V\n" +
	"HoJY7LZ7yXzuWlSVYAAAAEBYRsJTytYJUidtOuv3s3tdjyDA+4TSdCz9+hFKjyqz\n" +
	"v1PxSJ2ipSalQUUIYSFmEdYYTtUegljstnvJfO5aVJVgAAAAAAECAwQF\n" +
	"-----END OPENSSH PRIVATE KEY-----\n"
