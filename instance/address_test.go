// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
)

type AddressSuite struct{}

var _ = Suite(&AddressSuite{})

func (s *AddressSuite) TestNewAddressIpv4(c *C) {
	addr := instance.NewAddress("127.0.0.1")
	c.Check(addr.Value, Equals, "127.0.0.1")
	c.Check(addr.Type, Equals, instance.Ipv4Address)
}

func (s *AddressSuite) TestNewAddressIpv6(c *C) {
	addr := instance.NewAddress("::1")
	c.Check(addr.Value, Equals, "::1")
	c.Check(addr.Type, Equals, instance.Ipv6Address)
}

func (s *AddressSuite) TestNewAddressHostname(c *C) {
	addr := instance.NewAddress("localhost")
	c.Check(addr.Value, Equals, "localhost")
	c.Check(addr.Type, Equals, instance.HostName)
}
