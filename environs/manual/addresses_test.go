// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing/testbase"
)

type addressesSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&addressesSuite{})

func (s *addressesSuite) TestHostAddresses(c *gc.C) {
	const hostname = "boxen0"
	s.PatchValue(manual.InstanceHostAddresses, func(host string) ([]instance.Address, error) {
		c.Check(host, gc.Equals, hostname)
		return []instance.Address{
			instance.NewAddress("192.168.0.1"),
			instance.NewAddress("nickname"),
			instance.NewAddress(hostname),
		}, nil
	})
	addrs, err := manual.HostAddresses(hostname)
	c.Assert(err, gc.IsNil)
	c.Assert(addrs, gc.HasLen, 3)
	// The last address is marked public, all others are unknown.
	c.Assert(addrs[0].NetworkScope, gc.Equals, instance.NetworkUnknown)
	c.Assert(addrs[1].NetworkScope, gc.Equals, instance.NetworkUnknown)
	c.Assert(addrs[2].NetworkScope, gc.Equals, instance.NetworkPublic)
}
