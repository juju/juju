// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"errors"
	"fmt"
	"net"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type AddressSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&AddressSuite{})

func (s *AddressSuite) TestNewScopedAddressIPv4(c *gc.C) {
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
		addr := network.NewScopedAddress(t.value, t.scope)
		c.Check(addr.Value, gc.Equals, t.value)
		c.Check(addr.Type, gc.Equals, network.IPv4Address)
		c.Check(addr.Scope, gc.Equals, t.expectedScope)
	}
}

func (s *AddressSuite) TestNewScopedAddressIPv6(c *gc.C) {
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
		addr := network.NewScopedAddress(test.value, network.ScopeUnknown)
		c.Check(addr.Value, gc.Equals, test.value)
		c.Check(addr.Type, gc.Equals, network.IPv6Address)
		c.Check(addr.Scope, gc.Equals, test.scope)
	}
}

func (s *AddressSuite) TestNewAddressOnSpace(c *gc.C) {
	addr1 := network.NewAddressOnSpace("foo", "0.1.2.3")
	addr2 := network.NewAddressOnSpace("", "2001:db8::123")
	c.Check(addr1, jc.DeepEquals, network.Address{
		Value:     "0.1.2.3",
		Type:      "ipv4",
		Scope:     "public",
		SpaceName: "foo",
	})
	c.Check(addr2, jc.DeepEquals, network.Address{
		Value:     "2001:db8::123",
		Type:      "ipv6",
		Scope:     "public",
		SpaceName: "",
	})
}

func (s *AddressSuite) TestNewAddressesOnSpace(c *gc.C) {
	addrs := network.NewAddressesOnSpace("bar", "0.2.3.4", "fc00::1")
	c.Check(addrs, jc.DeepEquals, []network.Address{{
		Value:           "0.2.3.4",
		Type:            "ipv4",
		Scope:           "public",
		SpaceName:       "bar",
		SpaceProviderId: corenetwork.Id(""),
	}, {
		Value:           "fc00::1",
		Type:            "ipv6",
		Scope:           "local-cloud",
		SpaceName:       "bar",
		SpaceProviderId: corenetwork.Id(""),
	}})
}

func (s *AddressSuite) TestNewAddressIPv4(c *gc.C) {
	value := "0.1.2.3"
	addr1 := network.NewScopedAddress(value, network.ScopeUnknown)
	addr2 := network.NewAddress(value)
	addr3 := network.NewScopedAddress(value, network.ScopeLinkLocal)
	// NewAddress behaves exactly like NewScopedAddress with ScopeUnknown
	c.Assert(addr1, jc.DeepEquals, addr2)
	c.Assert(addr2.Scope, gc.Equals, network.ScopePublic) // derived from value
	c.Assert(addr2.Value, gc.Equals, value)
	c.Assert(addr2.Type, gc.Equals, network.IPv4Address)
	c.Assert(addr2.Scope, gc.Not(gc.Equals), addr3.Scope) // different scope
	c.Assert(addr3.Scope, gc.Equals, network.ScopeLinkLocal)
}

func (s *AddressSuite) TestNewAddressIPv6(c *gc.C) {
	value := "2001:db8::1"
	addr1 := network.NewScopedAddress(value, network.ScopeUnknown)
	addr2 := network.NewAddress(value)
	addr3 := network.NewScopedAddress(value, network.ScopeLinkLocal)
	// NewAddress behaves exactly like NewScopedAddress with ScopeUnknown
	c.Assert(addr1, jc.DeepEquals, addr2)
	c.Assert(addr2.Scope, gc.Equals, network.ScopePublic) // derived from value
	c.Assert(addr2.Value, gc.Equals, value)
	c.Assert(addr2.Type, gc.Equals, network.IPv6Address)
	c.Assert(addr2.Scope, gc.Not(gc.Equals), addr3.Scope) // different scope
	c.Assert(addr3.Scope, gc.Equals, network.ScopeLinkLocal)
}

func (s *AddressSuite) TestNewAddresses(c *gc.C) {
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
		addresses := network.NewAddresses(test.values...)
		c.Check(addresses, gc.HasLen, len(test.values))
		for j, addr := range addresses {
			c.Check(addr.Value, gc.Equals, test.values[j])
			c.Check(addr.Type, gc.Equals, test.addrType)
			c.Check(addr.Scope, gc.Equals, test.scope)
		}
	}
}

func (s *AddressSuite) TestNewScopedAddressHostname(c *gc.C) {
	addr := network.NewScopedAddress("localhost", network.ScopeUnknown)
	c.Check(addr.Value, gc.Equals, "localhost")
	c.Check(addr.Type, gc.Equals, network.HostName)
	c.Check(addr.Scope, gc.Equals, network.ScopeUnknown)
	addr = network.NewScopedAddress("example.com", network.ScopeUnknown)
	c.Check(addr.Value, gc.Equals, "example.com")
	c.Check(addr.Type, gc.Equals, network.HostName)
	c.Check(addr.Scope, gc.Equals, network.ScopeUnknown)
}

type selectTest struct {
	about         string
	addresses     []network.Address
	expectedIndex int
}

// expected returns the expected address for the test.
func (t selectTest) expected() (network.Address, bool) {
	if t.expectedIndex == -1 {
		return network.Address{}, false
	}
	return t.addresses[t.expectedIndex], true
}

var selectPublicTests = []selectTest{{
	"no addresses gives empty string result",
	[]network.Address{},
	-1,
}, {
	"a public IPv4 address is selected",
	[]network.Address{
		network.NewScopedAddress("8.8.8.8", network.ScopePublic),
	},
	0,
}, {
	"a public IPv6 address is selected",
	[]network.Address{
		network.NewScopedAddress("2001:db8::1", network.ScopePublic),
	},
	0,
}, {
	"first public address is selected",
	[]network.Address{
		network.NewScopedAddress("8.8.8.8", network.ScopePublic),
		network.NewScopedAddress("2001:db8::1", network.ScopePublic),
	},
	0,
}, {
	"the first public address is selected when cloud local fallbacks exist",
	[]network.Address{
		network.NewScopedAddress("172.16.1.1", network.ScopeCloudLocal),
		network.NewScopedAddress("8.8.8.8", network.ScopePublic),
		network.NewScopedAddress("fc00:1", network.ScopeCloudLocal),
		network.NewScopedAddress("2001:db8::1", network.ScopePublic),
	},
	1,
}, {
	"the cloud local address is selected when a fan-local fallback exists",
	[]network.Address{
		network.NewScopedAddress("243.1.1.1", network.ScopeFanLocal),
		network.NewScopedAddress("172.16.1.1", network.ScopeCloudLocal),
	},
	1,
},
	{
		"a machine IPv4 local address is not selected",
		[]network.Address{
			network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal),
		},
		-1,
	}, {
		"a machine IPv6 local address is not selected",
		[]network.Address{
			network.NewScopedAddress("::1", network.ScopeMachineLocal),
		},
		-1,
	}, {
		"a link-local IPv4 address is not selected",
		[]network.Address{
			network.NewScopedAddress("169.254.1.1", network.ScopeLinkLocal),
		},
		-1,
	}, {
		"a link-local (multicast or not) IPv6 address is not selected",
		[]network.Address{
			network.NewScopedAddress("fe80::1", network.ScopeLinkLocal),
			network.NewScopedAddress("ff01::2", network.ScopeLinkLocal),
			network.NewScopedAddress("ff02::1:1", network.ScopeLinkLocal),
		},
		-1,
	}, {
		"a public name is preferred to an unknown or cloud local address",
		[]network.Address{
			network.NewScopedAddress("127.0.0.1", network.ScopeUnknown),
			network.NewScopedAddress("10.0.0.1", network.ScopeCloudLocal),
			network.NewScopedAddress("fc00::1", network.ScopeCloudLocal),
			network.NewScopedAddress("public.invalid.testing", network.ScopePublic),
		},
		3,
	}, {
		"first unknown address selected",
		// NOTE(dimitern): Not using NewScopedAddress() below as it derives the
		// scope internally from the value when given ScopeUnknown.
		[]network.Address{
			{Value: "10.0.0.1", Scope: network.ScopeUnknown},
			{Value: "8.8.8.8", Scope: network.ScopeUnknown},
		},
		0,
	}, {
		"public IP address is picked when both public IPs and public hostnames exist",
		[]network.Address{
			network.NewScopedAddress("10.0.0.1", network.ScopeUnknown),
			network.NewScopedAddress("example.com", network.ScopePublic),
			network.NewScopedAddress("8.8.8.8", network.ScopePublic),
		},
		2,
	}, {
		"hostname is picked over cloud local address",
		[]network.Address{
			network.NewScopedAddress("10.0.0.1", network.ScopeUnknown),
			network.NewScopedAddress("example.com", network.ScopePublic),
		},
		1,
	}, {
		"IPv4 preferred over IPv6",
		[]network.Address{
			network.NewScopedAddress("2001:db8::1", network.ScopePublic),
			network.NewScopedAddress("8.8.8.8", network.ScopePublic),
		},
		1,
	}}

func (s *AddressSuite) TestSelectPublicAddress(c *gc.C) {
	for i, t := range selectPublicTests {
		c.Logf("test %d: %s", i, t.about)
		expectAddr, expectOK := t.expected()
		actualAddr, actualOK := network.SelectPublicAddress(t.addresses)
		c.Check(actualOK, gc.Equals, expectOK)
		c.Check(actualAddr, gc.Equals, expectAddr)
	}
}

var selectInternalTests = []selectTest{{
	"no addresses gives empty string result",
	[]network.Address{},
	-1,
}, {
	"a public IPv4 address is selected",
	[]network.Address{
		network.NewScopedAddress("8.8.8.8", network.ScopePublic),
	},
	0,
}, {
	"a public IPv6 address is selected",
	[]network.Address{
		network.NewScopedAddress("2001:db8::1", network.ScopePublic),
	},
	0,
}, {
	"a cloud local IPv4 address is selected",
	[]network.Address{
		network.NewScopedAddress("8.8.8.8", network.ScopePublic),
		network.NewScopedAddress("10.0.0.1", network.ScopeCloudLocal),
	},
	1,
}, {
	"a cloud local IPv6 address is selected",
	[]network.Address{
		network.NewScopedAddress("fc00::1", network.ScopeCloudLocal),
		network.NewScopedAddress("2001:db8::1", network.ScopePublic),
	},
	0,
}, {
	"a machine local or link-local address is not selected",
	[]network.Address{
		network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal),
		network.NewScopedAddress("::1", network.ScopeMachineLocal),
		network.NewScopedAddress("fe80::1", network.ScopeLinkLocal),
	},
	-1,
}, {
	"a cloud local address is preferred to a public address",
	[]network.Address{
		network.NewScopedAddress("2001:db8::1", network.ScopePublic),
		network.NewScopedAddress("fc00::1", network.ScopeCloudLocal),
		network.NewScopedAddress("8.8.8.8", network.ScopePublic),
	},
	1,
}, {
	"an IPv6 cloud local address is preferred to a public address if the former appears first",
	[]network.Address{
		network.NewScopedAddress("8.8.8.8", network.ScopePublic),
		network.NewScopedAddress("2001:db8::1", network.ScopePublic),
		network.NewScopedAddress("fc00::1", network.ScopeCloudLocal),
	},
	2,
}}

func (s *AddressSuite) TestSelectInternalAddress(c *gc.C) {
	for i, t := range selectInternalTests {
		c.Logf("test %d: %s", i, t.about)
		expectAddr, expectOK := t.expected()
		actualAddr, actualOK := network.SelectInternalAddress(t.addresses, false)
		c.Check(actualOK, gc.Equals, expectOK)
		c.Check(actualAddr, gc.Equals, expectAddr)
	}
}

var selectInternalMachineTests = []selectTest{{
	"first cloud local IPv4 address is selected",
	[]network.Address{
		network.NewScopedAddress("fc00::1", network.ScopeCloudLocal),
		network.NewScopedAddress("2001:db8::1", network.ScopePublic),
		network.NewScopedAddress("10.0.0.1", network.ScopeCloudLocal),
		network.NewScopedAddress("8.8.8.8", network.ScopePublic),
	},
	2,
}, {
	"first cloud local address is selected",
	[]network.Address{
		network.NewScopedAddress("fc00::1", network.ScopeCloudLocal),
		network.NewScopedAddress("2001:db8::1", network.ScopePublic),
		network.NewScopedAddress("8.8.8.8", network.ScopePublic),
	},
	0,
}, {
	"first cloud local hostname is selected",
	[]network.Address{
		network.NewScopedAddress("example.com", network.ScopePublic),
		network.NewScopedAddress("cloud1.internal", network.ScopeCloudLocal),
		network.NewScopedAddress("cloud2.internal", network.ScopeCloudLocal),
		network.NewScopedAddress("example.org", network.ScopePublic),
	},
	1,
}, {
	"first machine local address is selected",
	[]network.Address{
		network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal),
		network.NewScopedAddress("::1", network.ScopeMachineLocal),
	},
	0,
}, {
	"first machine local IPv4 address is selected even with public/cloud hostnames",
	[]network.Address{
		network.NewScopedAddress("public.example.com", network.ScopePublic),
		network.NewScopedAddress("::1", network.ScopeMachineLocal),
		network.NewScopedAddress("unknown.example.com", network.ScopeUnknown),
		network.NewScopedAddress("cloud.internal", network.ScopeCloudLocal),
		network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal),
		network.NewScopedAddress("fe80::1", network.ScopeLinkLocal),
		network.NewScopedAddress("127.0.0.2", network.ScopeMachineLocal),
	},
	4,
}, {
	"first machine local non-IPv4 address is selected even with public/cloud hostnames",
	[]network.Address{
		network.NewScopedAddress("public.example.com", network.ScopePublic),
		network.NewScopedAddress("::1", network.ScopeMachineLocal),
		network.NewScopedAddress("unknown.example.com", network.ScopeUnknown),
		network.NewScopedAddress("cloud.internal", network.ScopeCloudLocal),
		network.NewScopedAddress("fe80::1", network.ScopeLinkLocal),
	},
	1,
}, {
	"cloud local IPv4 is selected even with other machine/cloud addresses",
	[]network.Address{
		network.NewScopedAddress("169.254.1.1", network.ScopeLinkLocal),
		network.NewScopedAddress("cloud-unknown.internal", network.ScopeUnknown),
		network.NewScopedAddress("cloud-local.internal", network.ScopeCloudLocal),
		network.NewScopedAddress("fc00::1", network.ScopeCloudLocal),
		network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal),
		network.NewScopedAddress("127.0.0.2", network.ScopeMachineLocal),
	},
	4,
}, {
	"first cloud local hostname is selected even with other machine/cloud addresses",
	[]network.Address{
		network.NewScopedAddress("169.254.1.1", network.ScopeLinkLocal),
		network.NewScopedAddress("cloud-unknown.internal", network.ScopeUnknown),
		network.NewScopedAddress("cloud-local.internal", network.ScopeCloudLocal),
		network.NewScopedAddress("fc00::1", network.ScopeCloudLocal),
	},
	2,
}}

func (s *AddressSuite) TestSelectInternalMachineAddress(c *gc.C) {
	for i, t := range selectInternalMachineTests {
		c.Logf("test %d: %s", i, t.about)
		expectAddr, expectOK := t.expected()
		actualAddr, actualOK := network.SelectInternalAddress(t.addresses, true)
		c.Check(actualOK, gc.Equals, expectOK)
		c.Check(actualAddr, gc.Equals, expectAddr)
	}
}

type selectInternalAddressesTest struct {
	about        string
	addresses    []network.Address
	machineLocal bool
	expected     []network.Address
}

var selectInternalAddressesTests = []selectInternalAddressesTest{
	{
		about: "machine/cloud-local addresses are selected when machineLocal is true",
		addresses: []network.Address{
			network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal),
			network.NewScopedAddress("10.0.0.9", network.ScopeCloudLocal),
			network.NewScopedAddress("fc00::1", network.ScopePublic),
		},
		machineLocal: true,
		expected: []network.Address{
			network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal),
			network.NewScopedAddress("10.0.0.9", network.ScopeCloudLocal),
		},
	},
	{
		about: "cloud-local addresses are selected when machineLocal is false",
		addresses: []network.Address{
			network.NewScopedAddress("169.254.1.1", network.ScopeLinkLocal),
			network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal),
			network.NewScopedAddress("cloud-local.internal", network.ScopeCloudLocal),
			network.NewScopedAddress("cloud-local2.internal", network.ScopeCloudLocal),
			network.NewScopedAddress("fc00::1", network.ScopePublic),
		},
		machineLocal: false,
		expected: []network.Address{
			network.NewScopedAddress("cloud-local.internal", network.ScopeCloudLocal),
			network.NewScopedAddress("cloud-local2.internal", network.ScopeCloudLocal),
		},
	},
	{
		about: "nil is returned when no cloud-local addresses are found",
		addresses: []network.Address{
			network.NewScopedAddress("169.254.1.1", network.ScopeLinkLocal),
			network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal),
		},
		machineLocal: false,
		expected:     nil,
	},
}

func (s *AddressSuite) TestSelectInternalAddresses(c *gc.C) {
	for i, t := range selectInternalAddressesTests {
		c.Logf("test %d: %s", i, t.about)
		actualAddr := network.SelectInternalAddresses(t.addresses, t.machineLocal)
		c.Check(actualAddr, gc.DeepEquals, t.expected)
	}
}

type selectInternalHostPortsTest struct {
	about     string
	addresses []network.HostPort
	expected  []string
}

var selectInternalHostPortsTests = []selectInternalHostPortsTest{{
	"no addresses gives empty string result",
	[]network.HostPort{},
	[]string{},
}, {
	"a public IPv4 address is selected",
	[]network.HostPort{
		{network.NewScopedAddress("8.8.8.8", network.ScopePublic), 9999},
	},
	[]string{"8.8.8.8:9999"},
}, {
	"cloud local IPv4 addresses are selected",
	[]network.HostPort{
		{network.NewScopedAddress("10.1.0.1", network.ScopeCloudLocal), 8888},
		{network.NewScopedAddress("8.8.8.8", network.ScopePublic), 123},
		{network.NewScopedAddress("10.0.0.1", network.ScopeCloudLocal), 1234},
	},
	[]string{"10.1.0.1:8888", "10.0.0.1:1234"},
}, {
	"a machine local or link-local address is not selected",
	[]network.HostPort{
		{network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal), 111},
		{network.NewScopedAddress("::1", network.ScopeMachineLocal), 222},
		{network.NewScopedAddress("fe80::1", network.ScopeLinkLocal), 333},
	},
	[]string{},
}, {
	"cloud local IPv4 addresses are preferred to a public addresses",
	[]network.HostPort{
		{network.NewScopedAddress("2001:db8::1", network.ScopePublic), 123},
		{network.NewScopedAddress("fc00::1", network.ScopeCloudLocal), 123},
		{network.NewScopedAddress("8.8.8.8", network.ScopePublic), 123},
		{network.NewScopedAddress("10.0.0.1", network.ScopeCloudLocal), 4444},
	},
	[]string{"10.0.0.1:4444"},
}, {
	"cloud local IPv6 addresses are preferred to a public addresses",
	[]network.HostPort{
		{network.NewScopedAddress("2001:db8::1", network.ScopePublic), 123},
		{network.NewScopedAddress("fc00::1", network.ScopeCloudLocal), 123},
		{network.NewScopedAddress("8.8.8.8", network.ScopePublic), 123},
	},
	[]string{"[fc00::1]:123"},
}}

func (s *AddressSuite) TestSelectInternalHostPorts(c *gc.C) {
	for i, t := range selectInternalHostPortsTests {
		c.Logf("test %d: %s", i, t.about)
		c.Check(network.SelectInternalHostPorts(t.addresses, false), gc.DeepEquals, t.expected)
	}
}

var prioritizeInternalHostPortsTests = []selectInternalHostPortsTest{{
	"no addresses gives empty string result",
	[]network.HostPort{},
	[]string{},
}, {
	"a public IPv4 address is selected",
	[]network.HostPort{
		{network.NewScopedAddress("8.8.8.8", network.ScopePublic), 9999},
	},
	[]string{"8.8.8.8:9999"},
}, {
	"cloud local IPv4 addresses are selected",
	[]network.HostPort{
		{network.NewScopedAddress("10.1.0.1", network.ScopeCloudLocal), 8888},
		{network.NewScopedAddress("8.8.8.8", network.ScopePublic), 123},
		{network.NewScopedAddress("10.0.0.1", network.ScopeCloudLocal), 1234},
	},
	[]string{"10.1.0.1:8888", "10.0.0.1:1234", "8.8.8.8:123"},
}, {
	"a machine local or link-local address is not selected",
	[]network.HostPort{
		{network.NewScopedAddress("127.0.0.1", network.ScopeMachineLocal), 111},
		{network.NewScopedAddress("::1", network.ScopeMachineLocal), 222},
		{network.NewScopedAddress("fe80::1", network.ScopeLinkLocal), 333},
	},
	[]string{},
}, {
	"cloud local addresses are preferred to a public addresses",
	[]network.HostPort{
		{network.NewScopedAddress("2001:db8::1", network.ScopePublic), 123},
		{network.NewScopedAddress("fc00::1", network.ScopeCloudLocal), 123},
		{network.NewScopedAddress("8.8.8.8", network.ScopePublic), 123},
		{network.NewScopedAddress("10.0.0.1", network.ScopeCloudLocal), 4444},
	},
	[]string{"10.0.0.1:4444", "[fc00::1]:123", "8.8.8.8:123", "[2001:db8::1]:123"},
}}

func (s *AddressSuite) TestPrioritizeInternalHostPorts(c *gc.C) {
	for i, t := range prioritizeInternalHostPortsTests {
		c.Logf("test %d: %s", i, t.about)
		prioritized := network.PrioritizeInternalHostPorts(t.addresses, false)
		c.Check(prioritized, gc.DeepEquals, t.expected)
	}
}

var stringTests = []struct {
	addr network.Address
	str  string
}{{
	addr: network.Address{
		Type:  network.IPv4Address,
		Value: "127.0.0.1",
	},
	str: "127.0.0.1",
}, {
	addr: network.Address{
		Type:      network.IPv4Address,
		Value:     "127.0.0.1",
		SpaceName: "storage-data",
	},
	str: "127.0.0.1@storage-data",
}, {
	addr: network.Address{
		Type:  network.IPv6Address,
		Value: "2001:db8::1",
		Scope: network.ScopePublic,
	},
	str: "public:2001:db8::1",
}, {
	addr: network.Address{
		Type:  network.HostName,
		Value: "foo.com",
	},
	str: "foo.com",
}, {
	addr: network.Address{
		Type:  network.HostName,
		Value: "foo.com",
		Scope: network.ScopeUnknown,
	},
	str: "foo.com",
}, {
	addr: network.Address{
		Type:            network.HostName,
		Value:           "foo.com",
		Scope:           network.ScopePublic,
		SpaceProviderId: corenetwork.Id("3"),
	},
	str: "public:foo.com@(id:3)",
}, {
	addr: network.Address{
		Type:      network.HostName,
		Value:     "foo.com",
		Scope:     network.ScopePublic,
		SpaceName: "default",
	},
	str: "public:foo.com@default",
}, {
	addr: network.Address{
		Type:            network.HostName,
		Value:           "foo.com",
		Scope:           network.ScopePublic,
		SpaceName:       "badlands",
		SpaceProviderId: corenetwork.Id("3"),
	},
	str: "public:foo.com@badlands(id:3)",
}}

func (s *AddressSuite) TestString(c *gc.C) {
	for i, test := range stringTests {
		c.Logf("test %d: %#v", i, test.addr)
		c.Check(test.addr.String(), gc.Equals, test.str)
		c.Check(test.addr.GoString(), gc.Equals, test.str)
	}
}

func (*AddressSuite) TestSortAddresses(c *gc.C) {
	addrs := network.NewAddresses(
		"127.0.0.1",
		"::1",
		"fc00::1",
		"169.254.1.2",
		"localhost",
		"243.5.1.2",
		"2001:db8::1",
		"fe80::2",
		"7.8.8.8",
		"172.16.0.1",
		"example.com",
		"8.8.8.8",
	)
	network.SortAddresses(addrs)
	c.Assert(addrs, jc.DeepEquals, network.NewAddresses(
		// Public IPv4 addresses on top.
		"7.8.8.8",
		"8.8.8.8",
		// After that public IPv6 addresses.
		"2001:db8::1",
		// Then hostnames.
		"example.com",
		"localhost",
		// Then IPv4 cloud-local addresses.
		"172.16.0.1",
		// Then IPv6 cloud-local addresses.
		"fc00::1",
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
	))
}

func (*AddressSuite) TestIPv4ToDecimal(c *gc.C) {
	zeroIP, err := network.IPv4ToDecimal(net.ParseIP("0.0.0.0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zeroIP, gc.Equals, uint32(0))

	nilIP := net.ParseIP("bad format")
	_, err = network.IPv4ToDecimal(nilIP)
	c.Assert(err, gc.ErrorMatches, `"<nil>" is not a valid IPv4 address`)

	_, err = network.IPv4ToDecimal(net.ParseIP("2001:db8::1"))
	c.Assert(err, gc.ErrorMatches, `"2001:db8::1" is not a valid IPv4 address`)

	nonZeroIP, err := network.IPv4ToDecimal(net.ParseIP("192.168.1.1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nonZeroIP, gc.Equals, uint32(3232235777))
}

func (*AddressSuite) TestDecimalToIPv4(c *gc.C) {
	addr := network.DecimalToIPv4(uint32(0))
	c.Assert(addr.String(), gc.Equals, "0.0.0.0")

	addr = network.DecimalToIPv4(uint32(3232235777))
	c.Assert(addr.String(), gc.Equals, "192.168.1.1")
}

func (*AddressSuite) TestExactScopeMatch(c *gc.C) {
	addr := network.NewScopedAddress("10.0.0.2", network.ScopeCloudLocal)
	match := network.ExactScopeMatch(addr, network.ScopeCloudLocal)
	c.Assert(match, jc.IsTrue)
	match = network.ExactScopeMatch(addr, network.ScopePublic)
	c.Assert(match, jc.IsFalse)

	addr = network.NewScopedAddress("8.8.8.8", network.ScopePublic)
	match = network.ExactScopeMatch(addr, network.ScopeCloudLocal)
	c.Assert(match, jc.IsFalse)
	match = network.ExactScopeMatch(addr, network.ScopePublic)
	c.Assert(match, jc.IsTrue)
}

func (s *AddressSuite) TestResolvableHostnames(c *gc.C) {
	seq := 0

	s.PatchValue(network.NetLookupIP, func(host string) ([]net.IP, error) {
		if host == "not-resolvable.com" {
			return nil, errors.New("no such host")
		}
		seq++
		return []net.IP{net.ParseIP(fmt.Sprintf("1.1.1.%d", seq))}, nil
	})

	// Test empty input yields empty output.
	empty := []network.Address{}
	c.Assert(empty, jc.DeepEquals, network.ResolvableHostnames(empty))

	// Test unresolvable inputs yields empty output.
	unresolvable := []network.Address{{Value: "not-resolvable.com", Type: network.HostName}}
	c.Assert(empty, jc.DeepEquals, network.ResolvableHostnames(unresolvable))

	// Test resolvable inputs yields identical outputs.
	resolvable := []network.Address{{Value: "localhost", Type: network.HostName}}
	c.Assert(resolvable, jc.DeepEquals, network.ResolvableHostnames(resolvable))

	unscopedAddrs := []network.Address{
		network.NewAddress("localhost"),
		network.NewAddress("127.0.0.1"),
		network.NewAddress("fe80::d806:dbff:fe23:1199"),
		network.NewAddress("not-resolvable.com"),
		network.NewAddress("not-resolvable.com"),
		network.NewAddress("fe80::1"),
		network.NewAddress("resolvable.com"),
		network.NewAddress("localhost"),
		network.NewAddress("resolvable.com"),
		network.NewAddress("ubuntu.com"),
	}

	unscopedAddrsExpected := []network.Address{
		unscopedAddrs[0], unscopedAddrs[1],
		unscopedAddrs[2], unscopedAddrs[5],
		unscopedAddrs[6], unscopedAddrs[7],
		unscopedAddrs[8], unscopedAddrs[9],
	}

	c.Assert(unscopedAddrsExpected, jc.DeepEquals, network.ResolvableHostnames(unscopedAddrs))

	// Test multiple inputs have their order preserved, that
	// duplicates are preserved but unresolvable hostnames (except
	// 'localhost') are removed.
	scopedAddrs := []network.Address{
		network.NewScopedAddress("172.16.1.1", network.ScopeCloudLocal),
		network.NewScopedAddress("not-resolvable.com", network.ScopePublic),
		network.NewScopedAddress("8.8.8.8", network.ScopePublic),
		network.NewScopedAddress("resolvable.com", network.ScopePublic),
		network.NewScopedAddress("fc00:1", network.ScopeCloudLocal),
		network.NewScopedAddress("localhost", network.ScopePublic),
		network.NewScopedAddress("192.168.1.1", network.ScopeCloudLocal),
		network.NewScopedAddress("localhost", network.ScopeCloudLocal),
		network.NewScopedAddress("not-resolvable.com", network.ScopePublic),
		network.NewScopedAddress("resolvable.com", network.ScopePublic),
	}

	scopedAddrsExpected := []network.Address{
		scopedAddrs[0], scopedAddrs[2], scopedAddrs[3], scopedAddrs[4],
		scopedAddrs[5], scopedAddrs[6], scopedAddrs[7], scopedAddrs[9],
	}

	c.Assert(scopedAddrsExpected, jc.DeepEquals, network.ResolvableHostnames(scopedAddrs))
}

func (s *AddressSuite) TestSelectAddressesBySpaceNamesFiltered(c *gc.C) {
	sp := "thaSpace"
	addrsSpace := []network.Address{network.NewAddressOnSpace(sp, "192.168.5.5")}
	addrsNoSpace := []network.Address{network.NewAddress("127.0.0.1")}

	filtered, ok := network.SelectAddressesBySpaceNames(append(addrsSpace, addrsNoSpace...), network.SpaceName(sp))
	c.Check(ok, jc.IsTrue)
	c.Check(filtered, jc.DeepEquals, addrsSpace)
}

func (s *AddressSuite) TestSelectAddressesBySpaceNoSpaceFalse(c *gc.C) {
	addrs := []network.Address{network.NewAddress("127.0.0.1")}
	filtered, ok := network.SelectAddressesBySpaceNames(addrs)
	c.Check(ok, jc.IsFalse)
	c.Check(filtered, jc.DeepEquals, addrs)
}

func (s *AddressSuite) TestSelectAddressesBySpaceNoneFound(c *gc.C) {
	addrs := []network.Address{network.NewAddress("127.0.0.1")}
	filtered, ok := network.SelectAddressesBySpaceNames(addrs, "noneSpace")
	c.Check(ok, jc.IsFalse)
	c.Check(filtered, jc.DeepEquals, addrs)
}
