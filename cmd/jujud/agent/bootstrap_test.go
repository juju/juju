// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

type BootstrapSuite struct {
}

func TestBootstrapSuite(t *stdtesting.T) {
	tc.Run(t, &BootstrapSuite{})
}

func (s *BootstrapSuite) TestStub(c *tc.C) {
	c.Skipf(`This suite is missing tests for the following scenarios:

  - Test that the initial model and all of the machines are created.
  - Test JWKS reachable check.
  - Test that the initial model is created with the correct UUID.
  - Test initial password.
  - Test set constraints for bootstrap machine and model.
  - Test system identity is written to the data directory.
`)
}
