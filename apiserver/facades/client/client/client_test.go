// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type clientSuite struct {
	testhelpers.IsolationSuite
}

func TestClientSuite(t *stdtesting.T) {
	tc.Run(t, &clientSuite{})
}

func (s *clientSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- A correct status is returned for the controller model on a pre-seeded scenario. 	
- A correct status with the controller timestamp is returned for the controller model on a pre-seeded scenario.
`)
}
