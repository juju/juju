// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"errors"

	"github.com/juju/juju/environs/manual/common"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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
	s.PatchValue(common.NetLookupHost, func(host string) ([]string, error) {
		s.netLookupHostCalled++
		if host == invalidHost {
			return nil, errors.New("invalid host: " + invalidHost)
		}
		return []string{"127.0.0.1"}, nil
	})
}

func (s *addressesSuite) TestHostAddress(c *gc.C) {
	addr, err := common.HostAddress(validHost)
	c.Assert(s.netLookupHostCalled, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, network.Address{
		Value: validHost,
		Type:  network.HostName,
		Scope: network.ScopePublic,
	})
}

func (s *addressesSuite) TestHostAddressError(c *gc.C) {
	addr, err := common.HostAddress(invalidHost)
	c.Assert(s.netLookupHostCalled, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "invalid host: "+invalidHost)
	c.Assert(addr, gc.Equals, network.Address{})
}

func (s *addressesSuite) TestHostAddressIPv4(c *gc.C) {
	addr, err := common.HostAddress("127.0.0.1")
	c.Assert(s.netLookupHostCalled, gc.Equals, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, network.Address{
		Value: "127.0.0.1",
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
	})
}

func (s *addressesSuite) TestHostAddressIPv6(c *gc.C) {
	addr, err := common.HostAddress("::1")
	c.Assert(s.netLookupHostCalled, gc.Equals, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, network.Address{
		Value: "::1",
		Type:  network.IPv6Address,
		Scope: network.ScopePublic,
	})
}