// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"fmt"
	"net"

	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/provider/local"
	"launchpad.net/juju-core/testing/testbase"
)

func TestLocal(t *stdtesting.T) {
	gc.TestingT(t)
}

type localSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&localSuite{})

func (*localSuite) TestProviderRegistered(c *gc.C) {
	provider, error := environs.Provider(provider.Local)
	c.Assert(error, gc.IsNil)
	c.Assert(provider, gc.DeepEquals, local.Provider)
}

func (*localSuite) TestCheckLocalPort(c *gc.C) {
	// Block a ports
	addr := fmt.Sprintf(":%d", 65501)
	ln, err := net.Listen("tcp", addr)
	c.Assert(err, gc.IsNil)
	defer ln.Close()

	err = local.CheckLocalPort(65501, "test port")
	c.Assert(err, gc.ErrorMatches, "cannot use 65501 as test port, already in use")

	err = local.CheckLocalPort(65502, "another test port")
	c.Assert(err, gc.IsNil)
}
