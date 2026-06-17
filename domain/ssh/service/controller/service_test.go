// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"

	controllersshservice "github.com/juju/juju/domain/ssh/service/controller"
)

type serviceSuite struct{}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestSSHServerHostKeyReturnsExisting(c *tc.C) {
	controllerState := &stubControllerState{
		key:   testPrivateKey,
		found: true,
	}

	svc := controllersshservice.NewService(controllerState)
	key, err := svc.SSHServerHostKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *serviceSuite) TestSSHServerHostKeyErrorsWhenMissing(c *tc.C) {
	svc := controllersshservice.NewService(&stubControllerState{})

	key, err := svc.SSHServerHostKey(c.Context())
	c.Check(key, tc.Equals, "")
	c.Assert(err, tc.ErrorMatches, `controller SSH server host key not found`)
}

type stubControllerState struct {
	key    string
	found  bool
	getErr error
}

func (s *stubControllerState) GetSSHServerHostKey(_ context.Context) (string, bool, error) {
	return s.key, s.found, s.getErr
}

const testPrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\n" +
	"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz\n" +
	"c2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7VHoJY7LZ7yXzuWlSVYAAA\n" +
	"AIiZq0wRmatMEQAAAAtzc2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7V\n" +
	"HoJY7LZ7yXzuWlSVYAAAAEBYRsJTytYJUidtOuv3s3tdjyDA+4TSdCz9+hFKjyqz\n" +
	"v1PxSJ2ipSalQUUIYSFmEdYYTtUegljstnvJfO5aVJVgAAAAAAECAwQF\n" +
	"-----END OPENSSH PRIVATE KEY-----\n"
