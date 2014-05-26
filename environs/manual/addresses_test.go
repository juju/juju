// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"errors"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing"
)

const (
	invalidHost = "testing.invalid"
	validHost   = "testing.valid"
)

type addressesSuite struct {
	testing.BaseSuite
	netLookupHostCalled int
}

var _ = gc.Suite(&addressesSuite{})

func (s *addressesSuite) SetUpTest(c *gc.C) {
	s.netLookupHostCalled = 0
	s.PatchValue(manual.NetLookupHost, func(host string) ([]string, error) {
		s.netLookupHostCalled++
		if host == invalidHost {
			return nil, errors.New("invalid host: " + invalidHost)
		}
		return []string{"127.0.0.1"}, nil
	})
}

func (s *addressesSuite) TestHostAddress(c *gc.C) {
	addr, err := manual.HostAddress(validHost)
	c.Assert(s.netLookupHostCalled, gc.Equals, 1)
	c.Assert(err, gc.IsNil)
	c.Assert(addr, gc.Equals, instance.Address{
		Value:        validHost,
		Type:         instance.HostName,
		NetworkScope: instance.NetworkPublic,
	})
}

func (s *addressesSuite) TestHostAddressError(c *gc.C) {
	addr, err := manual.HostAddress(invalidHost)
	c.Assert(s.netLookupHostCalled, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "invalid host: "+invalidHost)
	c.Assert(addr, gc.Equals, instance.Address{})
}

func (s *addressesSuite) TestHostAddressIPv4(c *gc.C) {
	addr, err := manual.HostAddress("127.0.0.1")
	c.Assert(s.netLookupHostCalled, gc.Equals, 0)
	c.Assert(err, gc.IsNil)
	c.Assert(addr, gc.Equals, instance.Address{
		Value:        "127.0.0.1",
		Type:         instance.Ipv4Address,
		NetworkScope: instance.NetworkPublic,
	})
}

func (s *addressesSuite) TestHostAddressIPv6(c *gc.C) {
	addr, err := manual.HostAddress("::1")
	c.Assert(s.netLookupHostCalled, gc.Equals, 0)
	c.Assert(err, gc.IsNil)
	c.Assert(addr, gc.Equals, instance.Address{
		Value:        "::1",
		Type:         instance.Ipv6Address,
		NetworkScope: instance.NetworkPublic,
	})
}
