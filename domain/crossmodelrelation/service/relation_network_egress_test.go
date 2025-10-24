// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
)

type relationNetworkEgressSuite struct {
	baseSuite
}

func TestRelationNetworkEgressSuite(t *testing.T) {
	tc.Run(t, &relationNetworkEgressSuite{})
}

func (s *relationNetworkEgressSuite) TestFilterPublicAddressesOnlyPublic(c *tc.C) {
	// Arrange
	svc := &WatchableService{
		Service: Service{
			logger: s.service(c).logger,
		},
	}

	addresses := map[string]network.SpaceAddresses{
		"unit-uuid-1": {
			{
				MachineAddress: network.MachineAddress{
					Value: "203.0.113.10",
					Scope: network.ScopePublic,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginProvider,
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "198.51.100.1",
					Scope: network.ScopeCloudLocal,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginProvider,
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "192.0.2.5",
					Scope: network.ScopeMachineLocal,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginMachine,
			},
		},
	}

	// Act
	result := svc.filterPublicAddresses(addresses)

	// Assert
	c.Logf("Result addresses: %v", result)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0], tc.Equals, "203.0.113.10")
}

func (s *relationNetworkEgressSuite) TestFilterPublicAddressesMultiplePublic(c *tc.C) {
	// Arrange
	svc := &WatchableService{
		Service: Service{
			logger: s.service(c).logger,
		},
	}

	addresses := map[string]network.SpaceAddresses{
		"unit-uuid-1": {
			{
				MachineAddress: network.MachineAddress{
					Value: "203.0.113.10",
					Scope: network.ScopePublic,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginProvider,
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "198.51.100.1",
					Scope: network.ScopeCloudLocal,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginProvider,
			},
		},
		"unit-uuid-2": {
			{
				MachineAddress: network.MachineAddress{
					Value: "198.51.100.5",
					Scope: network.ScopePublic,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginProvider,
			},
		},
	}

	// Act
	result := svc.filterPublicAddresses(addresses)

	// Assert
	c.Assert(result, tc.HasLen, 2)
	c.Check(result, tc.SameContents, []string{"203.0.113.10", "198.51.100.5"})
}

func (s *relationNetworkEgressSuite) TestFilterPublicAddressesNoPublic(c *tc.C) {
	// Arrange
	svc := &WatchableService{
		Service: Service{
			logger: s.service(c).logger,
		},
	}

	addresses := map[string]network.SpaceAddresses{
		"unit-uuid-1": {
			{
				MachineAddress: network.MachineAddress{
					Value: "198.51.100.1",
					Scope: network.ScopeCloudLocal,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginProvider,
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "192.0.2.5",
					Scope: network.ScopeMachineLocal,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginMachine,
			},
		},
	}

	// Act
	result := svc.filterPublicAddresses(addresses)

	// Assert
	c.Assert(result, tc.HasLen, 1)
	// cloud-local is the best available
	c.Check(result[0], tc.Equals, "198.51.100.1")
}

func (s *relationNetworkEgressSuite) TestFilterPublicAddressesEmpty(c *tc.C) {
	// Arrange
	svc := &WatchableService{
		Service: Service{
			logger: s.service(c).logger,
		},
	}

	addresses := map[string]network.SpaceAddresses{}

	// Act
	result := svc.filterPublicAddresses(addresses)

	// Assert
	c.Assert(result, tc.HasLen, 0)
}

func (s *relationNetworkEgressSuite) TestFilterPublicAddressesIPv6(c *tc.C) {
	// Arrange
	svc := &WatchableService{
		Service: Service{
			logger: s.service(c).logger,
		},
	}

	addresses := map[string]network.SpaceAddresses{
		"unit-uuid-1": {
			{
				MachineAddress: network.MachineAddress{
					Value: "2001:db8::1",
					Scope: network.ScopePublic,
					Type:  network.IPv6Address,
				},
				Origin: network.OriginProvider,
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "fc00::1",
					Scope: network.ScopeCloudLocal,
					Type:  network.IPv6Address,
				},
				Origin: network.OriginProvider,
			},
		},
	}

	// Act
	result := svc.filterPublicAddresses(addresses)

	// Assert
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0], tc.Equals, "2001:db8::1")
}

func (s *relationNetworkEgressSuite) TestFilterPublicAddressesMixedIPv4IPv6(c *tc.C) {
	// Arrange
	svc := &WatchableService{
		Service: Service{
			logger: s.service(c).logger,
		},
	}

	addresses := map[string]network.SpaceAddresses{
		"unit-uuid-1": {
			{
				MachineAddress: network.MachineAddress{
					Value: "203.0.113.10",
					Scope: network.ScopePublic,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginProvider,
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "2001:db8::1",
					Scope: network.ScopePublic,
					Type:  network.IPv6Address,
				},
				Origin: network.OriginProvider,
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "198.51.100.1",
					Scope: network.ScopeCloudLocal,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginProvider,
			},
		},
	}

	// Act
	result := svc.filterPublicAddresses(addresses)

	// Assert
	c.Assert(result, tc.HasLen, 1)
	// Prefer IPv4.
	c.Check(result[0], tc.Equals, "203.0.113.10")
}

func (s *relationNetworkEgressSuite) TestFilterPublicAddressesMachineLocalExcluded(c *tc.C) {
	// Arrange
	svc := &WatchableService{
		Service: Service{
			logger: s.service(c).logger,
		},
	}

	addresses := map[string]network.SpaceAddresses{
		"unit-uuid-1": {
			{
				MachineAddress: network.MachineAddress{
					Value: "203.0.113.10",
					Scope: network.ScopePublic,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginProvider,
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "192.0.2.1",
					Scope: network.ScopeMachineLocal,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginMachine,
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "198.51.100.1",
					Scope: network.ScopeCloudLocal,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginProvider,
			},
		},
	}

	// Act
	result := svc.filterPublicAddresses(addresses)

	// Assert
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0], tc.Equals, "203.0.113.10")
}

func (s *relationNetworkEgressSuite) TestFilterPublicAddressesScopePriority(c *tc.C) {
	// Arrange
	svc := &WatchableService{
		Service: Service{
			logger: s.service(c).logger,
		},
	}

	addresses := map[string]network.SpaceAddresses{
		"unit-uuid-1": {
			{
				MachineAddress: network.MachineAddress{
					Value: "192.0.2.1",
					Scope: network.ScopeFanLocal,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginProvider,
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "198.51.100.1",
					Scope: network.ScopeCloudLocal,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginProvider,
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "203.0.113.10",
					Scope: network.ScopePublic,
					Type:  network.IPv4Address,
				},
				Origin: network.OriginProvider,
			},
		},
	}

	// Act
	result := svc.filterPublicAddresses(addresses)

	// Assert
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0], tc.Equals, "203.0.113.10")
}
