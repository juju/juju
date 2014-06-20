// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type AddressSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&AddressSuite{})

func (s *AddressSuite) TestNewAddressIpv4(c *gc.C) {
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
		value:         "8.8.8.8",
		scope:         network.ScopeUnknown,
		expectedScope: network.ScopePublic,
	}}

	for i, t := range tests {
		c.Logf("test %d: %s %s", i, t.value, t.scope)
		addr := network.NewAddress(t.value, t.scope)
		c.Check(addr.Value, gc.Equals, t.value)
		c.Check(addr.Type, gc.Equals, network.IPv4Address)
		c.Check(addr.Scope, gc.Equals, t.expectedScope)
	}
}

func (s *AddressSuite) TestNewAddressIpv6(c *gc.C) {
	addr := network.NewAddress("::1", network.ScopeUnknown)
	c.Check(addr.Value, gc.Equals, "::1")
	c.Check(addr.Type, gc.Equals, network.IPv6Address)
	c.Check(addr.Scope, gc.Equals, network.ScopeMachineLocal)

	addr = network.NewAddress("2001:DB8::1", network.ScopeUnknown)
	c.Check(addr.Value, gc.Equals, "2001:DB8::1")
	c.Check(addr.Type, gc.Equals, network.IPv6Address)
	c.Check(addr.Scope, gc.Equals, network.ScopeUnknown)
}

func (s *AddressSuite) TestNewAddresses(c *gc.C) {
	addresses := network.NewAddresses("127.0.0.1", "192.168.1.1", "192.168.178.255")
	c.Assert(len(addresses), gc.Equals, 3)
	c.Assert(addresses[0].Value, gc.Equals, "127.0.0.1")
	c.Assert(addresses[0].Scope, gc.Equals, network.ScopeMachineLocal)
	c.Assert(addresses[1].Value, gc.Equals, "192.168.1.1")
	c.Assert(addresses[1].Scope, gc.Equals, network.ScopeCloudLocal)
	c.Assert(addresses[2].Value, gc.Equals, "192.168.178.255")
	c.Assert(addresses[2].Scope, gc.Equals, network.ScopeCloudLocal)
}

func (s *AddressSuite) TestNewAddressHostname(c *gc.C) {
	addr := network.NewAddress("localhost", network.ScopeUnknown)
	c.Check(addr.Value, gc.Equals, "localhost")
	c.Check(addr.Type, gc.Equals, network.HostName)
	c.Check(addr.Scope, gc.Equals, network.ScopeUnknown)
}

type selectTest struct {
	about         string
	addresses     []network.Address
	expectedIndex int
}

// expected returns the expected address for the test.
func (t selectTest) expected() string {
	if t.expectedIndex == -1 {
		return ""
	}
	return t.addresses[t.expectedIndex].Value
}

var selectPublicTests = []selectTest{{
	"no addresses gives empty string result",
	[]network.Address{},
	-1,
}, {
	"a public address is selected",
	[]network.Address{
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic},
	},
	0,
}, {
	"empty addresses are ignored",
	[]network.Address{
		{"", network.IPv4Address, "public", network.ScopeUnknown},
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic},
	},
	1,
}, {
	"a machine local address is not selected",
	[]network.Address{
		{"127.0.0.1", network.IPv4Address, "machine", network.ScopeMachineLocal},
	},
	-1,
}, {
	"an ipv6 address is not selected",
	[]network.Address{
		{"2001:DB8::1", network.IPv6Address, "", network.ScopePublic},
	},
	-1,
}, {
	"a public name is preferred to an unknown or cloud local address",
	[]network.Address{
		{"127.0.0.1", network.IPv4Address, "local", network.ScopeUnknown},
		{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeCloudLocal},
		{"public.invalid.testing", network.HostName, "public", network.ScopePublic},
	},
	2,
}, {
	"first unknown address selected",
	[]network.Address{
		{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeUnknown},
		{"8.8.8.8", network.IPv4Address, "floating", network.ScopeUnknown},
	},
	0,
}}

func (s *AddressSuite) TestSelectPublicAddress(c *gc.C) {
	for i, t := range selectPublicTests {
		c.Logf("test %d: %s", i, t.about)
		c.Check(network.SelectPublicAddress(t.addresses), gc.Equals, t.expected())
	}
}

var selectInternalTests = []selectTest{{
	"no addresses gives empty string result",
	[]network.Address{},
	-1,
}, {
	"empty addresses are ignored",
	[]network.Address{
		{"", network.IPv4Address, "public", network.ScopeUnknown},
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic},
	},
	1,
}, {
	"a public address is selected",
	[]network.Address{
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic},
	},
	0,
}, {
	"a cloud local address is selected",
	[]network.Address{
		{"10.0.0.1", network.IPv4Address, "private", network.ScopeCloudLocal},
	},
	0,
}, {
	"a machine local address is not selected",
	[]network.Address{
		{"127.0.0.1", network.IPv4Address, "machine", network.ScopeMachineLocal},
	},
	-1,
}, {
	"ipv6 addresses are not selected",
	[]network.Address{
		{"::1", network.IPv6Address, "", network.ScopeCloudLocal},
	},
	-1,
}, {
	"a cloud local address is preferred to a public address",
	[]network.Address{
		{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeCloudLocal},
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic},
	},
	0,
}}

func (s *AddressSuite) TestSelectInternalAddress(c *gc.C) {
	for i, t := range selectInternalTests {
		c.Logf("test %d: %s", i, t.about)
		c.Check(network.SelectInternalAddress(t.addresses, false), gc.Equals, t.expected())
	}
}

var selectInternalMachineTests = []selectTest{{
	"a cloud local address is selected",
	[]network.Address{
		{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeCloudLocal},
	},
	0,
}, {
	"a machine local address is selected",
	[]network.Address{
		{"127.0.0.1", network.IPv4Address, "container", network.ScopeMachineLocal},
	},
	0,
}}

func (s *AddressSuite) TestSelectInternalMachineAddress(c *gc.C) {
	for i, t := range selectInternalMachineTests {
		c.Logf("test %d: %s", i, t.about)
		c.Check(network.SelectInternalAddress(t.addresses, true), gc.Equals, t.expected())
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
		Type:  network.HostName,
		Value: "foo.com",
		Scope: network.ScopePublic,
	},
	str: "public:foo.com",
}, {
	addr: network.Address{
		Type:        network.HostName,
		Value:       "foo.com",
		Scope:       network.ScopePublic,
		NetworkName: "netname",
	},
	str: "public:foo.com(netname)",
}}

func (s *AddressSuite) TestString(c *gc.C) {
	for i, test := range stringTests {
		c.Logf("test %d: %#v", i, test.addr)
		c.Check(test.addr.String(), gc.Equals, test.str)
	}
}

var netAddrTests = []struct {
	addr   network.Address
	port   int
	expect string
}{{
	addr:   network.NewAddress("0.1.2.3", network.ScopeUnknown),
	port:   99,
	expect: "0.1.2.3:99",
}, {
	addr:   network.NewAddress("2001:DB8::1", network.ScopeUnknown),
	port:   100,
	expect: "[2001:DB8::1]:100",
}}

func (*AddressSuite) TestNetAddr(c *gc.C) {
	for i, test := range netAddrTests {
		c.Logf("test %d: %q", i, test.addr)
		hp := network.HostPort{
			Address: test.addr,
			Port:    test.port,
		}
		c.Assert(hp.NetAddr(), gc.Equals, test.expect)
	}
}
