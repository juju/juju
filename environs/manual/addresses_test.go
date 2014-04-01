// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"errors"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing/testbase"
)

type addressesSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&addressesSuite{})

func (s *addressesSuite) TestHostAddress(c *gc.C) {
	var lookupHostArg string
	var lookupHostError error
	s.PatchValue(manual.NetLookupHost, func(host string) ([]string, error) {
		lookupHostArg = host
		return nil, lookupHostError
	})

	hostname := "boxen0"
	addr, err := manual.HostAddress(hostname)
	c.Assert(err, gc.IsNil)
	c.Assert(lookupHostArg, gc.Equals, hostname)
	c.Assert(addr, gc.Equals, instance.Address{
		Value:        hostname,
		Type:         instance.HostName,
		NetworkScope: instance.NetworkPublic,
	})

	lookupHostError = errors.New("whatever")
	addr, err = manual.HostAddress(hostname)
	c.Assert(err, gc.Equals, lookupHostError)
	c.Assert(addr, gc.Equals, instance.Address{})

	lookupHostArg = ""
	hostname = "127.0.0.1"
	addr, err = manual.HostAddress(hostname)
	c.Assert(err, gc.IsNil)
	c.Assert(lookupHostArg, gc.Equals, "") // no call to NetLookupHost
	c.Assert(addr, gc.Equals, instance.Address{
		Value:        hostname,
		Type:         instance.Ipv4Address,
		NetworkScope: instance.NetworkPublic,
	})

	lookupHostArg = ""
	hostname = "::1"
	addr, err = manual.HostAddress(hostname)
	c.Assert(err, gc.IsNil)
	c.Assert(lookupHostArg, gc.Equals, "") // no call to NetLookupHost
	c.Assert(addr, gc.Equals, instance.Address{
		Value:        hostname,
		Type:         instance.Ipv6Address,
		NetworkScope: instance.NetworkPublic,
	})
}
