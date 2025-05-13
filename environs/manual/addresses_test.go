// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"errors"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/internal/testing"
)

const (
	invalidHost = "testing.invalid"
	validHost   = "testing.valid"
)

type addressesSuite struct {
	testing.BaseSuite
	netLookupHostCalled int
}

var _ = tc.Suite(&addressesSuite{})

func (s *addressesSuite) SetUpTest(c *tc.C) {
	s.netLookupHostCalled = 0
	s.PatchValue(manual.NetLookupHost, func(host string) ([]string, error) {
		s.netLookupHostCalled++
		if host == invalidHost {
			return nil, errors.New("invalid host: " + invalidHost)
		}
		return []string{"127.0.0.1"}, nil
	})
}

func (s *addressesSuite) TestHostAddress(c *tc.C) {
	addr, err := manual.HostAddress(validHost)
	c.Assert(s.netLookupHostCalled, tc.Equals, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.Equals, network.NewMachineAddress(validHost, network.WithScope(network.ScopePublic)).AsProviderAddress())
}

func (s *addressesSuite) TestHostAddressError(c *tc.C) {
	addr, err := manual.HostAddress(invalidHost)
	c.Assert(s.netLookupHostCalled, tc.Equals, 1)
	c.Assert(err, tc.ErrorMatches, "invalid host: "+invalidHost)
	c.Assert(addr, tc.Equals, network.ProviderAddress{})
}

func (s *addressesSuite) TestHostAddressIPv4(c *tc.C) {
	addr, err := manual.HostAddress("127.0.0.1")
	c.Assert(s.netLookupHostCalled, tc.Equals, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.Equals, network.NewMachineAddress("127.0.0.1", network.WithScope(network.ScopePublic)).AsProviderAddress())
}

func (s *addressesSuite) TestHostAddressIPv6(c *tc.C) {
	addr, err := manual.HostAddress("::1")
	c.Assert(s.netLookupHostCalled, tc.Equals, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.Equals, network.NewMachineAddress("::1", network.WithScope(network.ScopePublic)).AsProviderAddress())
}
