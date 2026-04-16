// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jaas

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
)

// authenticatorSuite is a collection of tests to assert the inteface and
// contracts on offer by [Authenticator].
type authenticatorSuite struct {
	tokenVerifier *MockTokenVerifier
	userService   *MockUserService
}

// TestAuthenticatorSuite runs all of the tests contained within
// [authenticatorSuite].
func TestAuthenticatorSuite(t *testing.T) {
	tc.Run(t, &authenticatorSuite{})
}

// SetupMocks sets up the mocks for the [authenticatorSuite].
func (s *authenticatorSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.tokenVerifier = NewMockTokenVerifier(ctrl)
	s.userService = NewMockUserService(ctrl)

	c.Cleanup(func() {
		s.tokenVerifier = nil
		s.userService = nil
	})

	return ctrl
}

func (s *authenticatorSuite) TestAuthentication(c *tc.C) {
	defer s.setupMocks(c).Finish()
}
