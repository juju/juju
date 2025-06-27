// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

type bootstrapSuite struct{}

func TestBootstrapSuite(t *stdtesting.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:

- Initializing agent bootstrap
- Initializing agent bootstrap twice - should fail
`)
}
