// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
)

type AddressSuite struct{}

var _ = gc.Suite(&AddressSuite{})

func (s *AddressSuite) TestNewAddressIpv4(c *gc.C) {
	addr := instance.NewAddress("127.0.0.1")
	c.Check(addr.Value, gc.Equals, "127.0.0.1")
	c.Check(addr.Type, gc.Equals, instance.Ipv4Address)
}

func (s *AddressSuite) TestNewAddressIpv6(c *gc.C) {
	addr := instance.NewAddress("::1")
	c.Check(addr.Value, gc.Equals, "::1")
	c.Check(addr.Type, gc.Equals, instance.Ipv6Address)
}

func (s *AddressSuite) TestNewAddressHostname(c *gc.C) {
	addr := instance.NewAddress("localhost")
	c.Check(addr.Value, gc.Equals, "localhost")
	c.Check(addr.Type, gc.Equals, instance.HostName)
}
