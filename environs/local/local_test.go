// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	stdtesting "testing"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
)

func TestLocal(t *stdtesting.T) {
	TestingT(t)
}

type localSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&localSuite{})

func (*localSuite) TestProviderRegistered(c *C) {
	_, error := environs.Provider("local")
	c.Assert(error, IsNil)
}
