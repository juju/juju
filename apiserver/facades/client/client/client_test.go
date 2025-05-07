// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
)

type clientSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&clientSuite{})

func (s *clientSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- A correct status is returned for the controller model on a pre-seeded scenario.	
- A correct status with the controller timestamp is returned for the controller model on a pre-seeded scenario.
`)
}
