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

func (*localSuite) TestEnsureLocalPort(c *gc.C) {
	// Block some ports.
	for port := 65501; port < 65505; port++ {
		addr := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", addr)
		c.Assert(err, gc.IsNil)
		defer ln.Close()
	}

	port, err := local.EnsureLocalPort(65501)
	c.Assert(err, gc.IsNil)
	c.Assert(port, gc.Equals, 65505)

	port, err = local.EnsureLocalPort(65504)
	c.Assert(err, gc.IsNil)
	c.Assert(port, gc.Equals, 65505)

	port, err = local.EnsureLocalPort(65500)
	c.Assert(err, gc.IsNil)
	c.Assert(port, gc.Equals, 65500)
}
