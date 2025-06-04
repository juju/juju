// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"fmt"
	"net"
	"sort"
	stdtesting "testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testing"
)

type AddressSuite struct {
	testing.BaseSuite
}

func TestAddressSuite(t *stdtesting.T) {
	tc.Run(t, &AddressSuite{})
}

func (s *AddressSuite) TestNewScopedAddressIPv4(c *tc.C) {
	type test struct {
		value         string
		scope         network.Scope
		expectedScope network.Scope
	}

	tests := []test{{
		value:         "127.0.0.1",
		scope:         network.ScopeUnknown,
		expectedScope: network.ScopeMachineLocal,
	}, {
		value:         "127.0.0.1",
		scope:         network.ScopePublic,
		expectedScope: network.ScopePublic, // don't second guess != Unknown
	}, {
		value:         "10.0.3.1",
		scope:         network.ScopeUnknown,
		expectedScope: network.ScopeCloudLocal,
	}, {
		value:         "172.16.15.14",
		scope:         network.ScopeUnknown,
		expectedScope: network.ScopeCloudLocal,
	}, {
		value:         "192.168.0.1",
		scope:         network.ScopeUnknown,
		expectedScope: network.ScopeCloudLocal,
	}, {
		value:         "169.254.1.1",
		scope:         network.ScopeUnknown,
		expectedScope: network.ScopeLinkLocal,
	}, {
		value:         "8.8.8.8",
		scope:         network.ScopeUnknown,
		expectedScope: network.ScopePublic,
	}, {
		value:         "241.1.2.3",
		scope:         network.ScopeUnknown,
		expectedScope: network.ScopeFanLocal,
	}}

	for i, t := range tests {
		c.Logf("test %d: %s %s", i, t.value, t.scope)
		addr := network.NewSpaceAddress(t.value, network.WithScope(t.scope))
		c.Check(addr.Value, tc.Equals, t.value)
		c.Check(addr.Type, tc.Equals, network.IPv4Address)
		c.Check(addr.Scope, tc.Equals, t.expectedScope)
	}
}

func (s *AddressSuite) TestNewScopedAddressIPv6(c *tc.C) {
	// Examples below taken from
	// http://en.wikipedia.org/wiki/IPv6_address
	testAddresses := []struct {
		value string
		scope network.Scope
	}{
		// IPv6 loopback address
		{"::1", network.ScopeMachineLocal},
		// used documentation examples
		{"2001:db8::1", network.ScopePublic},
		// link-local
		{"fe80::1", network.ScopeLinkLocal},
		// unique local address (ULA) - first group
		{"fc00::1", network.ScopeCloudLocal},
		// unique local address (ULA) - second group
		{"fd00::1", network.ScopeCloudLocal},
		// IPv4-mapped IPv6 address
		{"::ffff:0:0:1", network.ScopePublic},
		// IPv4-translated IPv6 address (SIIT)
		{"::ffff:0:0:0:1", network.ScopePublic},
		// "well-known" prefix for IPv4/IPv6 auto translation
		{"64:ff9b::1", network.ScopePublic},
		// used for 6to4 addressing
		{"2002::1", network.ScopePublic},
		// used for Teredo tunneling
		{"2001::1", network.ScopePublic},
		// used for IPv6 benchmarking
		{"2001:2::1", network.ScopePublic},
		// used for cryptographic hash identifiers
		{"2001:10::1", network.ScopePublic},
		// interface-local multicast (all nodes)
		{"ff01::1", network.ScopeLinkLocal},
		// link-local multicast (all nodes)
		{"ff02::1", network.ScopeLinkLocal},
		// interface-local multicast (all routers)
		{"ff01::2", network.ScopeLinkLocal},
		// link-local multicast (all routers)
		{"ff02::2", network.ScopeLinkLocal},
	}
	for i, test := range testAddresses {
		c.Logf("test %d: %q -> %q", i, test.value, test.scope)
		addr := network.NewSpaceAddress(test.value)
		c.Check(addr.Value, tc.Equals, test.value)
		c.Check(addr.Type, tc.Equals, network.IPv6Address)
		c.Check(addr.Scope, tc.Equals, test.scope)
	}
}

func (s *AddressSuite) TestAsProviderAddress(c *tc.C) {
	addr1 := network.NewMachineAddress("0.1.2.3").AsProviderAddress(
		network.WithSpaceName("foo"),
		network.WithProviderSpaceID("3"),
		network.WithProviderID("523"),
		network.WithProviderSubnetID("5"),
		network.WithProviderVLANID("5001"),
		network.WithVLANTag(50),
	)
	addr2 := network.NewMachineAddress("2001:db8::123").AsProviderAddress(
		network.WithSpaceName(""),
	)
	c.Check(addr1, tc.DeepEquals, network.ProviderAddress{
		MachineAddress: network.MachineAddress{
			Value: "0.1.2.3",
			Type:  "ipv4",
			Scope: "public",
		},
		SpaceName:        "foo",
		ProviderSpaceID:  "3",
		ProviderID:       "523",
		ProviderSubnetID: "5",
		ProviderVLANID:   "5001",
		VLANTag:          50,
	})
	c.Check(addr2, tc.DeepEquals, network.ProviderAddress{
		MachineAddress: network.MachineAddress{
			Value: "2001:db8::123",
			Type:  "ipv6",
			Scope: "public",
		},
		SpaceName: "",
	})
}

func (s *AddressSuite) TestAsProviderAddresses(c *tc.C) {
	addrs := network.NewMachineAddresses([]string{"0.2.3.4", "fc00::1"}).AsProviderAddresses(
		network.WithSpaceName("bar"),
		network.WithProviderSpaceID("4"),
		network.WithProviderSubnetID("6"),
		network.WithProviderVLANID("5002"),
		network.WithVLANTag(100),
	)
	c.Check(addrs, tc.DeepEquals, network.ProviderAddresses{{
		MachineAddress: network.MachineAddress{
			Value: "0.2.3.4",
			Type:  "ipv4",
			Scope: "public",
		},
		SpaceName:        "bar",
		ProviderSpaceID:  "4",
		ProviderSubnetID: "6",
		ProviderVLANID:   "5002",
		VLANTag:          100,
	}, {
		MachineAddress: network.MachineAddress{
			Value: "fc00::1",
			Type:  "ipv6",
			Scope: "local-cloud",
		},
		SpaceName:        "bar",
		ProviderSpaceID:  "4",
		ProviderSubnetID: "6",
		ProviderVLANID:   "5002",
		VLANTag:          100,
	}})
}

func (s *AddressSuite) TestNewAddressIPv4(c *tc.C) {
	value := "0.1.2.3"
	addr1 := network.NewSpaceAddress(value)
	addr2 := network.NewSpaceAddress(value, network.WithScope(network.ScopeLinkLocal))
	c.Assert(addr1.Scope, tc.Equals, network.ScopePublic) // derived from value
	c.Assert(addr1.Value, tc.Equals, value)
	c.Assert(addr1.Type, tc.Equals, network.IPv4Address)
	c.Assert(addr2.Scope, tc.Equals, network.ScopeLinkLocal)
}

func (s *AddressSuite) TestNewAddressIPv6(c *tc.C) {
	value := "2001:db8::1"
	addr1 := network.NewSpaceAddress(value)
	addr2 := network.NewSpaceAddress(value, network.WithScope(network.ScopeLinkLocal))
	c.Assert(addr1.Scope, tc.Equals, network.ScopePublic) // derived from value
	c.Assert(addr1.Value, tc.Equals, value)
	c.Assert(addr1.Type, tc.Equals, network.IPv6Address)
	c.Assert(addr2.Scope, tc.Equals, network.ScopeLinkLocal)
}

func (s *AddressSuite) TestNewAddresses(c *tc.C) {
	testAddresses := []struct {
		values   []string
		addrType network.AddressType
		scope    network.Scope
	}{{
		[]string{"127.0.0.1", "127.0.1.2"},
		network.IPv4Address,
		network.ScopeMachineLocal,
	}, {
		[]string{"::1"},
		network.IPv6Address,
		network.ScopeMachineLocal,
	}, {
		[]string{"192.168.1.1", "192.168.178.255", "10.5.1.1", "172.16.1.1"},
		network.IPv4Address,
		network.ScopeCloudLocal,
	}, {
		[]string{"fc00::1", "fd00::2"},
		network.IPv6Address,
		network.ScopeCloudLocal,
	}, {
		[]string{"8.8.8.8", "8.8.4.4"},
		network.IPv4Address,
		network.ScopePublic,
	}, {
		[]string{"2001:db8::1", "64:ff9b::1", "2002::1"},
		network.IPv6Address,
		network.ScopePublic,
	}, {
		[]string{"169.254.1.23", "169.254.1.1"},
		network.IPv4Address,
		network.ScopeLinkLocal,
	}, {
		[]string{"243.1.5.7", "245.3.1.2"},
		network.IPv4Address,
		network.ScopeFanLocal,
	}, {
		[]string{"ff01::2", "ff01::1"},
		network.IPv6Address,
		network.ScopeLinkLocal,
	}, {
		[]string{"example.com", "example.org"},
		network.HostName,
		network.ScopeUnknown,
	}}

	for i, test := range testAddresses {
		c.Logf("test %d: %v -> %q", i, test.values, test.scope)
		addresses := network.NewSpaceAddresses(test.values...)
		c.Check(addresses, tc.HasLen, len(test.values))
		for j, addr := range addresses {
			c.Check(addr.Value, tc.Equals, test.values[j])
			c.Check(addr.Type, tc.Equals, test.addrType)
			c.Check(addr.Scope, tc.Equals, test.scope)
		}
	}
}

func (s *AddressSuite) TestNewScopedAddressHostname(c *tc.C) {
	addr := network.NewSpaceAddress("localhost")
	c.Check(addr.Value, tc.Equals, "localhost")
	c.Check(addr.Type, tc.Equals, network.HostName)
	c.Check(addr.Scope, tc.Equals, network.ScopeUnknown)
	addr = network.NewSpaceAddress("example.com")
	c.Check(addr.Value, tc.Equals, "example.com")
	c.Check(addr.Type, tc.Equals, network.HostName)
	c.Check(addr.Scope, tc.Equals, network.ScopeUnknown)
}

type selectTest struct {
	about         string
	addresses     network.SpaceAddresses
	expectedIndex int
}

// expected returns the expected address for the test.
func (t selectTest) expected() (network.SpaceAddress, bool) {
	if t.expectedIndex == -1 {
		return network.SpaceAddress{}, false
	}
	return t.addresses[t.expectedIndex], true
}

var selectPublicTests = []selectTest{{
	"no addresses gives empty string result",
	[]network.SpaceAddress{},
	-1,
}, {
	"a public IPv4 address is selected",
	[]network.SpaceAddress{
		network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)),
	},
	0,
}, {
	"a public IPv6 address is selected",
	[]network.SpaceAddress{
		network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopePublic)),
	},
	0,
}, {
	"first public address is selected",
	[]network.SpaceAddress{
		network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopePublic)),
	},
	0,
}, {
	"the first public address is selected when cloud local fallbacks exist",
	[]network.SpaceAddress{
		network.NewSpaceAddress("172.16.1.1", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("fc00:1", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopePublic)),
	},
	1,
}, {
	"the cloud local address is selected when a fan-local fallback exists",
	[]network.SpaceAddress{
		network.NewSpaceAddress("243.1.1.1", network.WithScope(network.ScopeFanLocal)),
		network.NewSpaceAddress("172.16.1.1", network.WithScope(network.ScopeCloudLocal)),
	},
	1,
},
	{
		"a machine IPv4 local address is not selected",
		[]network.SpaceAddress{
			network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
		},
		-1,
	}, {
		"a machine IPv6 local address is not selected",
		[]network.SpaceAddress{
			network.NewSpaceAddress("::1", network.WithScope(network.ScopeMachineLocal)),
		},
		-1,
	}, {
		"a link-local IPv4 address is not selected",
		[]network.SpaceAddress{
			network.NewSpaceAddress("169.254.1.1", network.WithScope(network.ScopeLinkLocal)),
		},
		-1,
	}, {
		"a link-local (multicast or not) IPv6 address is not selected",
		[]network.SpaceAddress{
			network.NewSpaceAddress("fe80::1", network.WithScope(network.ScopeLinkLocal)),
			network.NewSpaceAddress("ff01::2", network.WithScope(network.ScopeLinkLocal)),
			network.NewSpaceAddress("ff02::1:1", network.WithScope(network.ScopeLinkLocal)),
		},
		-1,
	}, {
		"a public name is preferred to an unknown or cloud local address",
		[]network.SpaceAddress{
			network.NewSpaceAddress("127.0.0.1"),
			network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("fc00::1", network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("public.invalid.testing", network.WithScope(network.ScopePublic)),
		},
		3,
	}, {
		"first unknown address selected",
		// NOTE(dimitern): Not using NewSpaceAddress() below as it derives the
		// scope internally from the value when given ScopeUnknown.
		[]network.SpaceAddress{
			{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
					Scope: network.ScopeUnknown,
				},
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "8.8.8.8",
					Scope: network.ScopeUnknown,
				},
			},
		},
		0,
	}, {
		"public IP address is picked when both public IPs and public hostnames exist",
		[]network.SpaceAddress{
			network.NewSpaceAddress("10.0.0.1"),
			network.NewSpaceAddress("example.com", network.WithScope(network.ScopePublic)),
			network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)),
		},
		2,
	}, {
		"hostname is picked over cloud local address",
		[]network.SpaceAddress{
			network.NewSpaceAddress("10.0.0.1"),
			network.NewSpaceAddress("example.com", network.WithScope(network.ScopePublic)),
		},
		1,
	}, {
		"IPv4 preferred over IPv6",
		[]network.SpaceAddress{
			network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopePublic)),
			network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)),
		},
		1,
	}}

func (s *AddressSuite) TestSelectPublicAddress(c *tc.C) {
	for i, t := range selectPublicTests {
		c.Logf("test %d: %s", i, t.about)
		expectAddr, expectOK := t.expected()
		actualAddr, actualOK := t.addresses.OneMatchingScope(network.ScopeMatchPublic)
		c.Check(actualOK, tc.Equals, expectOK)
		c.Check(actualAddr, tc.Equals, expectAddr)
	}
}

var selectInternalTests = []selectTest{{
	"no addresses gives empty string result",
	[]network.SpaceAddress{},
	-1,
}, {
	"a public IPv4 address is selected",
	[]network.SpaceAddress{
		network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)),
	},
	0,
}, {
	"a public IPv6 address is selected",
	[]network.SpaceAddress{
		network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopePublic)),
	},
	0,
}, {
	"a cloud local IPv4 address is selected",
	[]network.SpaceAddress{
		network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)),
	},
	1,
}, {
	"a cloud local IPv6 address is selected",
	[]network.SpaceAddress{
		network.NewSpaceAddress("fc00::1", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopePublic)),
	},
	0,
}, {
	"a machine local or link-local address is not selected",
	[]network.SpaceAddress{
		network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
		network.NewSpaceAddress("::1", network.WithScope(network.ScopeMachineLocal)),
		network.NewSpaceAddress("fe80::1", network.WithScope(network.ScopeLinkLocal)),
	},
	-1,
}, {
	"a cloud local address is preferred to a public address",
	[]network.SpaceAddress{
		network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("fc00::1", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)),
	},
	1,
}, {
	"an IPv6 cloud local address is preferred to a public address if the former appears first",
	[]network.SpaceAddress{
		network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("fc00::1", network.WithScope(network.ScopeCloudLocal)),
	},
	2,
}}

func (s *AddressSuite) TestSelectInternalAddress(c *tc.C) {
	for i, t := range selectInternalTests {
		c.Logf("test %d: %s", i, t.about)
		expectAddr, expectOK := t.expected()
		actualAddr, actualOK := t.addresses.OneMatchingScope(network.ScopeMatchCloudLocal)
		c.Check(actualOK, tc.Equals, expectOK)
		c.Check(actualAddr, tc.Equals, expectAddr)
	}
}

var selectInternalMachineTests = []selectTest{{
	"first cloud local IPv4 address is selected",
	[]network.SpaceAddress{
		network.NewSpaceAddress("fc00::1", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)),
	},
	2,
}, {
	"first cloud local address is selected",
	[]network.SpaceAddress{
		network.NewSpaceAddress("fc00::1", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)),
	},
	0,
}, {
	"first cloud local hostname is selected",
	[]network.SpaceAddress{
		network.NewSpaceAddress("example.com", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("cloud1.internal", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("cloud2.internal", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("example.org", network.WithScope(network.ScopePublic)),
	},
	1,
}, {
	"first machine local address is selected",
	[]network.SpaceAddress{
		network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
		network.NewSpaceAddress("::1", network.WithScope(network.ScopeMachineLocal)),
	},
	0,
}, {
	"first machine local IPv4 address is selected even with public/cloud hostnames",
	[]network.SpaceAddress{
		network.NewSpaceAddress("public.example.com", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("::1", network.WithScope(network.ScopeMachineLocal)),
		network.NewSpaceAddress("unknown.example.com"),
		network.NewSpaceAddress("cloud.internal", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
		network.NewSpaceAddress("fe80::1", network.WithScope(network.ScopeLinkLocal)),
		network.NewSpaceAddress("127.0.0.2", network.WithScope(network.ScopeMachineLocal)),
	},
	4,
}, {
	"first machine local non-IPv4 address is selected even with public/cloud hostnames",
	[]network.SpaceAddress{
		network.NewSpaceAddress("public.example.com", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("::1", network.WithScope(network.ScopeMachineLocal)),
		network.NewSpaceAddress("unknown.example.com"),
		network.NewSpaceAddress("cloud.internal", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("fe80::1", network.WithScope(network.ScopeLinkLocal)),
	},
	1,
}, {
	"cloud local IPv4 is selected even with other machine/cloud addresses",
	[]network.SpaceAddress{
		network.NewSpaceAddress("169.254.1.1", network.WithScope(network.ScopeLinkLocal)),
		network.NewSpaceAddress("cloud-unknown.internal"),
		network.NewSpaceAddress("cloud-local.internal", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("fc00::1", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
		network.NewSpaceAddress("127.0.0.2", network.WithScope(network.ScopeMachineLocal)),
	},
	4,
}, {
	"first cloud local hostname is selected even with other machine/cloud addresses",
	[]network.SpaceAddress{
		network.NewSpaceAddress("169.254.1.1", network.WithScope(network.ScopeLinkLocal)),
		network.NewSpaceAddress("cloud-unknown.internal"),
		network.NewSpaceAddress("cloud-local.internal", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("fc00::1", network.WithScope(network.ScopeCloudLocal)),
	},
	2,
}}

func (s *AddressSuite) TestSelectInternalMachineAddress(c *tc.C) {
	for i, t := range selectInternalMachineTests {
		c.Logf("test %d: %s", i, t.about)
		expectAddr, expectOK := t.expected()
		actualAddr, actualOK := t.addresses.OneMatchingScope(network.ScopeMatchMachineOrCloudLocal)
		c.Check(actualOK, tc.Equals, expectOK)
		c.Check(actualAddr, tc.Equals, expectAddr)
	}
}

type selectInternalAddressesTest struct {
	about     string
	addresses network.SpaceAddresses
	matcher   network.ScopeMatchFunc
	expected  network.SpaceAddresses
}

var selectInternalAddressesTests = []selectInternalAddressesTest{
	{
		about: "machine/cloud-local addresses are selected when machineLocal is true",
		addresses: []network.SpaceAddress{
			network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
			network.NewSpaceAddress("10.0.0.9", network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("fc00::1", network.WithScope(network.ScopePublic)),
		},
		matcher: network.ScopeMatchMachineOrCloudLocal,
		expected: []network.SpaceAddress{
			network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
			network.NewSpaceAddress("10.0.0.9", network.WithScope(network.ScopeCloudLocal)),
		},
	},
	{
		about: "cloud-local addresses are selected when machineLocal is false",
		addresses: []network.SpaceAddress{
			network.NewSpaceAddress("169.254.1.1", network.WithScope(network.ScopeLinkLocal)),
			network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
			network.NewSpaceAddress("cloud-local.internal", network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("cloud-local2.internal", network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("fc00::1", network.WithScope(network.ScopePublic)),
		},
		matcher: network.ScopeMatchCloudLocal,
		expected: []network.SpaceAddress{
			network.NewSpaceAddress("cloud-local.internal", network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("cloud-local2.internal", network.WithScope(network.ScopeCloudLocal)),
		},
	},
	{
		about: "nil is returned when no cloud-local addresses are found",
		addresses: []network.SpaceAddress{
			network.NewSpaceAddress("169.254.1.1", network.WithScope(network.ScopeLinkLocal)),
			network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
		},
		matcher:  network.ScopeMatchCloudLocal,
		expected: nil,
	},
}

func (s *AddressSuite) TestSelectInternalAddresses(c *tc.C) {
	for i, t := range selectInternalAddressesTests {
		c.Logf("test %d: %s", i, t.about)
		actualAddr := t.addresses.AllMatchingScope(t.matcher)
		c.Check(actualAddr, tc.DeepEquals, t.expected)
	}
}

// stringer wraps Stringer and GoStringer for convenience.
type stringer interface {
	fmt.Stringer
	fmt.GoStringer
}

var stringTests = []struct {
	addr stringer
	str  string
}{{
	addr: network.MachineAddress{
		Type:  network.IPv4Address,
		Value: "127.0.0.1",
	},
	str: "127.0.0.1",
}, {
	addr: network.ProviderAddress{
		MachineAddress: network.MachineAddress{
			Type:  network.IPv4Address,
			Value: "127.0.0.1",
		},
		SpaceName: "storage-data",
	},
	str: "127.0.0.1@storage-data",
}, {
	addr: network.MachineAddress{
		Type:  network.IPv6Address,
		Value: "2001:db8::1",
		Scope: network.ScopePublic,
	},
	str: "public:2001:db8::1",
}, {
	addr: network.MachineAddress{
		Type:  network.HostName,
		Value: "foo.com",
	},
	str: "foo.com",
}, {
	addr: network.MachineAddress{
		Type:  network.HostName,
		Value: "foo.com",
		Scope: network.ScopeUnknown,
	},
	str: "foo.com",
}, {
	addr: network.ProviderAddress{
		MachineAddress: network.MachineAddress{
			Type:  network.HostName,
			Value: "foo.com",
			Scope: network.ScopePublic,
		},
		ProviderSpaceID: network.Id("3"),
	},
	str: "public:foo.com@(id:3)",
}, {
	addr: network.ProviderAddress{
		MachineAddress: network.MachineAddress{
			Type:  network.HostName,
			Value: "foo.com",
			Scope: network.ScopePublic,
		},
		SpaceName: "default",
	},
	str: "public:foo.com@default",
}, {
	addr: network.ProviderAddress{
		MachineAddress: network.MachineAddress{
			Type:  network.HostName,
			Value: "foo.com",
			Scope: network.ScopePublic,
		},
		SpaceName:       "badlands",
		ProviderSpaceID: network.Id("3"),
	},
	str: "public:foo.com@badlands(id:3)",
}, {
	addr: network.ProviderAddress{
		MachineAddress: network.MachineAddress{
			Type:  network.HostName,
			Value: "foo.com",
			Scope: network.ScopePublic,
		},
		SpaceName:       "badlands",
		ProviderSpaceID: network.Id("3"),
		ProviderID:      "523",
	},
	str: "public:foo.com@badlands(id:3)",
}, {
	addr: network.ProviderAddress{
		MachineAddress: network.MachineAddress{
			Type:  network.HostName,
			Value: "foo.com",
			Scope: network.ScopePublic,
		},
		SpaceName:        "badlands",
		ProviderSpaceID:  network.Id("3"),
		ProviderSubnetID: "5",
	},
	str: "public:foo.com@badlands(id:3)",
}, {
	addr: network.ProviderAddress{
		MachineAddress: network.MachineAddress{
			Type:  network.HostName,
			Value: "foo.com",
			Scope: network.ScopePublic,
		},
		SpaceName:       "badlands",
		ProviderSpaceID: network.Id("3"),
		ProviderVLANID:  "5001",
	},
	str: "public:foo.com@badlands(id:3)",
}, {
	addr: network.ProviderAddress{
		MachineAddress: network.MachineAddress{
			Type:  network.HostName,
			Value: "foo.com",
			Scope: network.ScopePublic,
		},
		SpaceName:       "badlands",
		ProviderSpaceID: network.Id("3"),
		VLANTag:         50,
	},
	str: "public:foo.com@badlands(id:3)",
}}

func (s *AddressSuite) TestString(c *tc.C) {
	for i, test := range stringTests {
		c.Logf("test %d: %#v", i, test.addr)
		c.Check(test.addr.String(), tc.Equals, test.str)
		c.Check(test.addr.GoString(), tc.Equals, test.str)
	}
}

func (*AddressSuite) TestSortAddresses(c *tc.C) {
	addrs := network.NewSpaceAddresses(
		"127.0.0.1",
		"::1",
		"fc00::1",
		"169.254.1.2",
		"localhost",
		"243.5.1.2",
		"2001:db8::1",
		"fe80::2",
		"7.8.8.8",
		"172.16.0.2",
		"example.com",
		"8.8.8.8",
	)

	// Public and local-cloud secondary addresses.
	addrs = append(addrs, network.NewSpaceAddress("6.8.8.8", network.WithSecondary(true)))
	addrs = append(addrs, network.NewSpaceAddress("172.16.0.1", network.WithSecondary(true)))

	sort.Sort(addrs)
	c.Assert(addrs.Values(), tc.DeepEquals, []string{
		// Public IPv4 addresses on top.
		"7.8.8.8",
		"8.8.8.8",
		// After that public IPv6 addresses.
		"2001:db8::1",
		// Then hostname.
		"example.com",
		// Secondary public address follows the others.
		"6.8.8.8",
		// With localhost last.
		"localhost",
		// Then IPv4 cloud-local addresses.
		"172.16.0.2",
		// Then IPv6 cloud-local addresses.
		"fc00::1",
		// Then secondary cloud-local addresses.
		"172.16.0.1",
		// Then fan-local addresses.
		"243.5.1.2",
		// Then machine-local IPv4 addresses.
		"127.0.0.1",
		// Then machine-local IPv6 addresses.
		"::1",
		// Then link-local IPv4 addresses.
		"169.254.1.2",
		// Finally, link-local IPv6 addresses.
		"fe80::2",
	})
}

func (*AddressSuite) TestExactScopeMatch(c *tc.C) {
	var addr network.Address

	addr = network.NewMachineAddress("10.0.0.2", network.WithScope(network.ScopeCloudLocal))
	match := network.ExactScopeMatch(addr, network.ScopeCloudLocal)
	c.Assert(match, tc.IsTrue)
	match = network.ExactScopeMatch(addr, network.ScopePublic)
	c.Assert(match, tc.IsFalse)

	addr = network.NewMachineAddress("8.8.8.8", network.WithScope(network.ScopePublic)).AsProviderAddress()
	match = network.ExactScopeMatch(addr, network.ScopeCloudLocal)
	c.Assert(match, tc.IsFalse)
	match = network.ExactScopeMatch(addr, network.ScopePublic)
	c.Assert(match, tc.IsTrue)
}

func (s *AddressSuite) TestSelectAddressesBySpaceNamesFiltered(c *tc.C) {
	sp := network.SpaceInfo{
		ID:         "666",
		Name:       "thaSpace",
		ProviderId: "",
		Subnets:    nil,
	}

	// Only the first address has a space.
	addr := network.NewSpaceAddress("192.168.5.5")
	addr.SpaceID = sp.ID
	addrs := network.SpaceAddresses{
		addr,
		network.NewSpaceAddress("127.0.0.1"),
	}

	filtered, ok := addrs.InSpaces(sp)
	c.Check(ok, tc.IsTrue)
	c.Check(filtered, tc.DeepEquals, network.SpaceAddresses{addr})
}

func (s *AddressSuite) TestSelectAddressesBySpaceNoSpaceFalse(c *tc.C) {
	addrs := network.SpaceAddresses{network.NewSpaceAddress("127.0.0.1")}
	filtered, ok := addrs.InSpaces()
	c.Check(ok, tc.IsFalse)
	c.Check(filtered, tc.DeepEquals, addrs)
}

func (s *AddressSuite) TestSelectAddressesBySpaceNoneFound(c *tc.C) {
	sp := network.SpaceInfo{
		ID:         "666",
		Name:       "noneSpace",
		ProviderId: "",
		Subnets:    nil,
	}

	addrs := network.SpaceAddresses{network.NewSpaceAddress("127.0.0.1")}
	filtered, ok := addrs.InSpaces(sp)
	c.Check(ok, tc.IsFalse)
	c.Check(filtered, tc.DeepEquals, addrs)
}

func (s *AddressSuite) TestProviderAddressesToSpaceAddressesByName(c *tc.C) {
	// Check success.
	addrs := network.ProviderAddresses{
		network.NewMachineAddress("1.2.3.4").AsProviderAddress(network.WithSpaceName("space-one")),
		network.NewMachineAddress("2.3.4.5").AsProviderAddress(network.WithSpaceName("space-two")),
		network.NewMachineAddress("3.4.5.6").AsProviderAddress(),
	}

	allSpaces := network.SpaceInfos{
		{ID: "1", Name: "space-one", ProviderId: "p1"},
		{ID: "2", Name: "space-two"},
	}
	exp := network.NewSpaceAddresses("1.2.3.4", "2.3.4.5", "3.4.5.6")
	exp[0].SpaceID = "1"
	exp[1].SpaceID = "2"

	res, err := addrs.ToSpaceAddresses(allSpaces)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.SameContents, exp)

	// Add an address in a space that the lookup will not resolve.
	addrs = append(addrs, network.NewMachineAddress("4.5.6.7").AsProviderAddress(network.WithSpaceName("space-denied")))
	_, err = addrs.ToSpaceAddresses(allSpaces)
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *AddressSuite) TestProviderAddressesToSpaceAddressesBySubnet(c *tc.C) {
	// Check success.
	addrs := network.ProviderAddresses{
		network.NewMachineAddress(
			"10.0.0.6",
			network.WithCIDR("10.0.0.0/24"),
		).AsProviderAddress(network.WithProviderSubnetID("61")),
	}

	allSpaces := network.SpaceInfos{
		{ID: "1", Name: "space-one", ProviderId: "p1"},
		{ID: "2", Name: "space-two"},
		{
			ID:   "6",
			Name: "space-six",
			Subnets: network.SubnetInfos{
				{
					ID:         "666",
					CIDR:       "10.0.0.0/24",
					ProviderId: "61",
				},
			},
		},
	}
	res, err := addrs.ToSpaceAddresses(allSpaces)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.HasLen, 1)
	c.Check(res[0].SpaceID, tc.Equals, network.SpaceUUID("6"))
}

func (s *AddressSuite) TestSpaceAddressesToProviderAddresses(c *tc.C) {
	// Check success.
	addrs := network.NewSpaceAddresses("1.2.3.4", "2.3.4.5", "3.4.5.6")
	addrs[0].SpaceID = "1"
	addrs[1].SpaceID = "2"

	allSpaces := network.SpaceInfos{
		{ID: "1", Name: "space-one", ProviderId: "p1"},
		{ID: "2", Name: "space-two"},
	}
	exp := network.ProviderAddresses{
		network.NewMachineAddress("1.2.3.4").AsProviderAddress(network.WithSpaceName("space-one")),
		network.NewMachineAddress("2.3.4.5").AsProviderAddress(network.WithSpaceName("space-two")),
		network.NewMachineAddress("3.4.5.6").AsProviderAddress(),
	}
	// Only the first address in the lookup has a provider ID.
	exp[0].ProviderSpaceID = "p1"

	res, err := addrs.ToProviderAddresses(allSpaces)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.SameContents, exp)

	// Add an address in a space that the lookup will not resolve.
	addrs = append(addrs, network.NewSpaceAddress("4.5.6.7"))
	addrs[3].SpaceID = "3"
	_, err = addrs.ToProviderAddresses(allSpaces)
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *AddressSuite) TestSpaceAddressesValues(c *tc.C) {
	values := []string{"1.2.3.4", "2.3.4.5", "3.4.5.6"}
	addrs := network.NewSpaceAddresses(values...)
	c.Check(addrs.Values(), tc.DeepEquals, values)
}

func (s *AddressSuite) TestAddressValueForCIDR(c *tc.C) {
	_, err := network.NewMachineAddress("172.31.37.53").ValueWithMask()
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)

	_, err = network.NewMachineAddress("", network.WithCIDR("172.31.37.0/20")).ValueWithMask()
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)

	val, err := network.NewMachineAddress("172.31.37.53", network.WithCIDR("172.31.37.0/20")).ValueWithMask()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(val, tc.Equals, "172.31.37.53/20")
}

func (s *AddressSuite) TestCIDRAddressType(c *tc.C) {
	tests := []struct {
		descr  string
		CIDR   string
		exp    network.AddressType
		expErr string
	}{
		{
			descr: "IPV4 CIDR",
			CIDR:  "10.0.0.0/24",
			exp:   network.IPv4Address,
		},
		{
			descr: "IPV6 CIDR",
			CIDR:  "2002::1234:abcd:ffff:c0a8:101/64",
			exp:   network.IPv6Address,
		},
		{
			descr: "IPV6 with 4in6 prefix",
			CIDR:  "0:0:0:0:0:ffff:c0a8:2a00/120",
			// The Go stdlib interprets this as an IPV4
			exp: network.IPv4Address,
		},
		{
			descr:  "bogus CIDR",
			CIDR:   "catastrophe",
			expErr: ".*invalid CIDR address.*",
		},
	}

	for i, t := range tests {
		c.Logf("test %d: %s", i, t.descr)
		got, err := network.CIDRAddressType(t.CIDR)
		if t.expErr != "" {
			c.Check(got, tc.Equals, network.AddressType(""))
			c.Check(err, tc.ErrorMatches, t.expErr)
		} else {
			c.Check(err, tc.ErrorIsNil)
			c.Check(got, tc.Equals, t.exp)
		}
	}
}

func (s *AddressSuite) TestNoAddressError(c *tc.C) {
	err := network.NoAddressError("fake")
	c.Assert(err, tc.ErrorMatches, `no fake address\(es\)`)
	c.Assert(network.IsNoAddressError(err), tc.IsTrue)
	c.Assert(network.IsNoAddressError(errors.New("address found")), tc.IsFalse)
}

func (s *AddressSuite) TestNetworkCIDRFromIPAndMask(c *tc.C) {
	specs := []struct {
		descr   string
		ip      net.IP
		mask    net.IPMask
		expCIDR string
	}{
		{
			descr:   "nil ip",
			mask:    net.IPv4Mask(0, 0, 0, 255),
			expCIDR: "",
		},
		{
			descr:   "nil mask",
			ip:      net.ParseIP("10.1.2.42"),
			expCIDR: "",
		},
		{
			descr:   "network IP",
			ip:      net.ParseIP("10.1.0.0"),
			mask:    net.IPv4Mask(255, 255, 0, 0),
			expCIDR: "10.1.0.0/16",
		},
		{
			descr:   "host IP",
			ip:      net.ParseIP("10.1.2.42"),
			mask:    net.IPv4Mask(255, 255, 255, 0),
			expCIDR: "10.1.2.0/24",
		},
	}

	for i, spec := range specs {
		c.Logf("%d: %s", i, spec.descr)
		gotCIDR := network.NetworkCIDRFromIPAndMask(spec.ip, spec.mask)
		c.Assert(gotCIDR, tc.Equals, spec.expCIDR)
	}
}

func (s *AddressSuite) TestIsValidAddressConfigTypeWithValidValues(c *tc.C) {
	validTypes := []network.AddressConfigType{
		network.ConfigLoopback,
		network.ConfigStatic,
		network.ConfigDHCP,
		network.ConfigManual,
	}

	for _, value := range validTypes {
		result := network.IsValidAddressConfigType(string(value))
		c.Check(result, tc.IsTrue)
	}
}

func (s *AddressSuite) TestIsValidAddressConfigTypeWithInvalidValues(c *tc.C) {
	result := network.IsValidAddressConfigType("")
	c.Check(result, tc.IsFalse)

	result = network.IsValidAddressConfigType("anything")
	c.Check(result, tc.IsFalse)

	result = network.IsValidAddressConfigType(" ")
	c.Check(result, tc.IsFalse)
}

// spaceAddressCandidate implements the SpaceAddressCandidate
// interface from the core/network package.
type spaceAddressCandidate struct {
	value        string
	configMethod network.AddressConfigType
	subnetCIDR   string
	isSecondary  bool
}

func (s spaceAddressCandidate) Value() string {
	return s.value
}

func (s spaceAddressCandidate) ConfigMethod() network.AddressConfigType {
	return s.configMethod
}

func (s spaceAddressCandidate) SubnetCIDR() string {
	return s.subnetCIDR
}

func (s spaceAddressCandidate) IsSecondary() bool {
	return s.isSecondary
}

func (s *AddressSuite) TestConvertToSpaceAddresses(c *tc.C) {
	subs := network.SubnetInfos{
		{ID: "1", CIDR: "192.168.0.0/24", SpaceID: "666"},
		{ID: "2", CIDR: "252.80.0.0/12", SpaceID: "999"},
		{ID: "3", CIDR: "10.0.0.10/32", SpaceID: "333"},
	}

	candidates := []network.SpaceAddressCandidate{
		spaceAddressCandidate{
			value:        "252.80.0.100",
			configMethod: network.ConfigStatic,
			subnetCIDR:   "252.80.0.0/12",
			isSecondary:  true,
		},
		spaceAddressCandidate{
			value:        "192.168.0.66",
			configMethod: network.ConfigDHCP,
			subnetCIDR:   "192.168.0.0/24",
		},
		// This simulates an address added to the loopback device,
		// such as done by BGP.
		spaceAddressCandidate{
			value:        "10.0.0.10",
			configMethod: network.ConfigLoopback,
			subnetCIDR:   "10.0.0.10/32",
		},
	}

	addrs := make(network.SpaceAddresses, len(candidates))
	for i, ca := range candidates {
		var err error
		addrs[i], err = network.ConvertToSpaceAddress(ca, subs)
		c.Assert(err, tc.ErrorIsNil)
	}

	sort.Sort(addrs)
	c.Check(addrs, tc.DeepEquals, network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value:      "10.0.0.10",
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
				ConfigType: network.ConfigLoopback,
				CIDR:       "10.0.0.10/32",
			},
			SpaceID: "333",
		},
		{
			MachineAddress: network.MachineAddress{
				Value:      "192.168.0.66",
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
				ConfigType: network.ConfigDHCP,
				CIDR:       "192.168.0.0/24",
			},
			SpaceID: "666",
		},
		{
			MachineAddress: network.MachineAddress{
				Value:       "252.80.0.100",
				Type:        network.IPv4Address,
				Scope:       network.ScopeFanLocal,
				ConfigType:  network.ConfigStatic,
				CIDR:        "252.80.0.0/12",
				IsSecondary: true,
			},
			SpaceID: "999",
		},
	})
}

func (s *AddressSuite) TestSortOrderScope(c *tc.C) {
	sas := network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.0",
				Type:  network.IPv4Address,
				Scope: network.ScopeCloudLocal,
			},
			Origin:  network.OriginMachine,
			SpaceID: "0",
		},
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			Origin:  network.OriginProvider,
			SpaceID: "0",
		},
	}

	sort.Sort(sas)
	// Public addresses first.
	c.Check(sas, tc.DeepEquals, network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			Origin:  network.OriginProvider,
			SpaceID: "0",
		},
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.0",
				Type:  network.IPv4Address,
				Scope: network.ScopeCloudLocal,
			},
			Origin:  network.OriginMachine,
			SpaceID: "0",
		},
	})
}

func (s *AddressSuite) TestSortOrderSameScopeDiffOrigin(c *tc.C) {
	sas := network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.0",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			Origin:  network.OriginMachine,
			SpaceID: "0",
		},
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			Origin:  network.OriginProvider,
			SpaceID: "0",
		},
	}

	sort.Sort(sas)
	// Public addresses first.
	c.Check(sas, tc.DeepEquals, network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			Origin:  network.OriginProvider,
			SpaceID: "0",
		},
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.0",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			Origin:  network.OriginMachine,
			SpaceID: "0",
		},
	})
}

func (s *AddressSuite) TestSortOrderSameScopeSameOriginDiffValue(c *tc.C) {
	sas := network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.0",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			Origin:  network.OriginProvider,
			SpaceID: "0",
		},
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			Origin:  network.OriginProvider,
			SpaceID: "0",
		},
	}

	sort.Sort(sas)
	// Public addresses first.
	c.Check(sas, tc.DeepEquals, network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.0",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			Origin:  network.OriginProvider,
			SpaceID: "0",
		},
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
			Origin:  network.OriginProvider,
			SpaceID: "0",
		},
	})
}

type testFindSubnetAddr struct {
	val string
}

func (a testFindSubnetAddr) Network() string {
	return "ip+net"
}

func (a testFindSubnetAddr) String() string {
	return a.val
}

func testAddresses(c *tc.C, networks ...string) ([]net.Addr, error) {
	addrs := make([]net.Addr, 0)
	for _, n := range networks {
		_, _, err := net.ParseCIDR(n)
		if err != nil {
			return nil, err
		}
		c.Assert(err, tc.IsNil)
		addrs = append(addrs, testFindSubnetAddr{n})
	}
	return addrs, nil
}

func (s *AddressSuite) TestIsLocalAddress(c *tc.C) {

	tests := []struct {
		descr            string
		ip               net.IP
		ifaceAddrsResult []string
		ifaceAddrsErr    error
		expected         bool
		expectedErr      string
	}{
		{
			descr:         "error returned from net.InterfaceAddrs()",
			ifaceAddrsErr: errors.New("interface addrs - some error"),
			expectedErr:   "interface addrs - some error",
			expected:      false,
		},
		{
			descr:            "IPv4 is local",
			ifaceAddrsResult: []string{"192.168.0.0/16"},
			ip:               net.IPv4(192, 168, 0, 0),
			expected:         true,
		},
		{
			descr:            "IPv4 is not local",
			ifaceAddrsResult: []string{"192.168.0.0/16"},
			ip:               net.IPv4(8, 8, 8, 8),
			expected:         false,
		},
		{
			descr:            "IPv6 is local",
			ifaceAddrsResult: []string{"fc00::/7"},
			ip:               net.ParseIP("fc00::0"),
			expected:         true,
		},
		{
			descr:            "IPv6 is not local",
			ifaceAddrsResult: []string{"fc00::/7"},
			ip:               net.ParseIP("2606::1"),
			expected:         false,
		},
	}

	for i, tt := range tests {
		c.Logf("Test %d: %s", i, tt.descr)

		s.PatchValue(&network.InterfaceAddrs, func() ([]net.Addr, error) {
			if tt.ifaceAddrsErr != nil {
				return []net.Addr{}, tt.ifaceAddrsErr
			}
			return testAddresses(c, tt.ifaceAddrsResult...)
		})
		isLocal, err := network.IsLocalAddress(tt.ip)
		if err != nil {
			c.Check(err, tc.ErrorMatches, tt.expectedErr)
			// when err is returned, isLocal is false
			c.Check(isLocal, tc.Equals, false)
			continue
		}
		c.Check(err, tc.ErrorIsNil)
		c.Check("", tc.Equals, tt.expectedErr)

		c.Check(isLocal, tc.Equals, tt.expected)
	}
}
