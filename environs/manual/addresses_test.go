// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, network.Address{
		Value: validHost,
		Type:  network.HostName,
		Scope: network.ScopePublic,
	})
}

func (s *addressesSuite) TestHostAddressError(c *gc.C) {
	addr, err := manual.HostAddress(invalidHost)
	c.Assert(s.netLookupHostCalled, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "invalid host: "+invalidHost)
	c.Assert(addr, gc.Equals, network.Address{})
}

func (s *addressesSuite) TestHostAddressIPv4(c *gc.C) {
	addr, err := manual.HostAddress("127.0.0.1")
	c.Assert(s.netLookupHostCalled, gc.Equals, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, network.Address{
		Value: "127.0.0.1",
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
	})
}

func (s *addressesSuite) TestHostAddressIPv6(c *gc.C) {
	addr, err := manual.HostAddress("::1")
	c.Assert(s.netLookupHostCalled, gc.Equals, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, network.Address{
		Value: "::1",
		Type:  network.IPv6Address,
		Scope: network.ScopePublic,
	})
}
