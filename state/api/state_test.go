// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type stateSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestCloseMultipleOk(c *gc.C) {
	c.Assert(s.APIState.Close(), gc.IsNil)
	c.Assert(s.APIState.Close(), gc.IsNil)
	c.Assert(s.APIState.Close(), gc.IsNil)
}
