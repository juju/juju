// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"slices"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
)

type controllerKeySuite struct {
	state *MockControllerKeyState
}

var (
	_ = gc.Suite(&controllerKeySuite{})

	controllerConfigKeys = `
ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBG00bYFLb/sxPcmVRMg8NXZK/ldefElAkC9wD41vABdHZiSRvp+2y9BMNVYzE/FnzKObHtSvGRX65YQgRn7k5p0= juju@example.com
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju@example.com
`
)

func (s *controllerKeySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockControllerKeyState(ctrl)
	return ctrl
}

// TestNoControllerKeys asserts that if no controller public keys exists we get
// back a safe empty slice and no errors.
func (s *controllerKeySuite) TestNoControllerKeys(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetControllerConfigKeys(
		gomock.Any(), []string{controller.SystemSSHKeys},
	).Return(map[string]string{}, nil)

	keys, err := NewControllerKeyService(s.state).ControllerAuthorisedKeys(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(keys, gc.NotNil)
	c.Check(len(keys), gc.Equals, 0)
}

// TestControllerKeys is asserting the happy path of controller config keys.
func (s *controllerKeySuite) TestControllerKeys(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetControllerConfigKeys(
		gomock.Any(), []string{controller.SystemSSHKeys},
	).Return(map[string]string{
		controller.SystemSSHKeys: controllerConfigKeys,
	}, nil)

	expectedKeys := []string{
		"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBG00bYFLb/sxPcmVRMg8NXZK/ldefElAkC9wD41vABdHZiSRvp+2y9BMNVYzE/FnzKObHtSvGRX65YQgRn7k5p0= juju@example.com",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju@example.com",
	}

	keys, err := NewControllerKeyService(s.state).ControllerAuthorisedKeys(context.Background())
	c.Check(err, jc.ErrorIsNil)

	// Sort expected v actual so we not hardcoded onto implementation anymore
	// then we have to be.
	slices.Sort(expectedKeys)
	slices.Sort(keys)
	c.Check(keys, jc.DeepEquals, expectedKeys)
}
