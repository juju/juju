// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type clientSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) TestStub(c *gc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- A correct status is returned for the controller model on a pre-seeded scenario.	
- A correct status with the controller timestamp is returned for the controller model on a pre-seeded scenario.
`)
}
