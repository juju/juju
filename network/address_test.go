// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"errors"
	"fmt"
	"net"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
		Value:       "0.1.2.3",
		Type:        "ipv4",
		Scope:       "public",
		NetworkName: "",
		SpaceName:   "foo",
	})
	c.Check(addr2, jc.DeepEquals, network.Address{
		Value:       "2001:db8::123",
		Type:        "ipv6",
		Scope:       "public",
		NetworkName: "",
		SpaceName:   "",
	})
}

func (s *AddressSuite) TestNewAddressesOnSpace(c *gc.C) {
	addrs := network.NewAddressesOnSpace("bar", "0.2.3.4", "fc00::1")
	c.Check(addrs, jc.DeepEquals, []network.Address{{
		Value:           "0.2.3.4",
		Type:            "ipv4",
		Scope:           "public",
		NetworkName:     "",
		SpaceName:       "bar",
		SpaceProviderId: network.Id(""),
	}, {
		Value:           "fc00::1",
		Type:            "ipv6",
		Scope:           "local-cloud",
		NetworkName:     "",
		SpaceName:       "bar",
		SpaceProviderId: network.Id(""),
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
	preferIPv6    bool
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
	false,
}, {
	"a public IPv4 address is selected",
	[]network.Address{
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	0,
	false,
}, {
	"a public IPv6 address is selected",
	[]network.Address{
		{"2001:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	0,
	false,
}, {
	"first public address is selected",
	[]network.Address{
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"2001:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	0,
	false,
}, {
	"the first public address is selected when cloud local fallbacks exist",
	[]network.Address{
		{"172.16.1.1", network.IPv4Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"fc00:1", network.IPv6Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"2001:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	1,
	false,
}, {
	"IPv6 public address is preferred to a cloud local one when preferIPv6 is true",
	[]network.Address{
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"172.16.1.1", network.IPv4Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"fc00:1", network.IPv6Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"2001:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	3,
	true,
}, {
	"a machine IPv4 local address is not selected",
	[]network.Address{
		{"127.0.0.1", network.IPv4Address, "machine", network.ScopeMachineLocal, network.DefaultSpace, ""},
	},
	-1,
	false,
}, {
	"a machine IPv6 local address is not selected",
	[]network.Address{
		{"::1", network.IPv6Address, "machine", network.ScopeMachineLocal, network.DefaultSpace, ""},
	},
	-1,
	false,
}, {
	"a link-local IPv4 address is not selected",
	[]network.Address{
		{"169.254.1.1", network.IPv4Address, "link", network.ScopeLinkLocal, network.DefaultSpace, ""},
	},
	-1,
	false,
}, {
	"a link-local (multicast or not) IPv6 address is not selected",
	[]network.Address{
		{"fe80::1", network.IPv6Address, "link", network.ScopeLinkLocal, network.DefaultSpace, ""},
		{"ff01::2", network.IPv6Address, "link", network.ScopeLinkLocal, network.DefaultSpace, ""},
		{"ff02::1:1", network.IPv6Address, "link", network.ScopeLinkLocal, network.DefaultSpace, ""},
	},
	-1,
	false,
}, {
	"a public name is preferred to an unknown or cloud local address",
	[]network.Address{
		{"127.0.0.1", network.IPv4Address, "local", network.ScopeUnknown, network.DefaultSpace, ""},
		{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"fc00::1", network.IPv6Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"public.invalid.testing", network.HostName, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	3,
	false,
}, {
	"first unknown address selected",
	[]network.Address{
		{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeUnknown, network.DefaultSpace, ""},
		{"8.8.8.8", network.IPv4Address, "floating", network.ScopeUnknown, network.DefaultSpace, ""},
	},
	0,
	false,
}, {
	"first public address is picked when both public IPs and public hostnames exist",
	[]network.Address{
		{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeUnknown, network.DefaultSpace, ""},
		{"example.com", network.HostName, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"8.8.8.8", network.IPv4Address, "floating", network.ScopePublic, network.DefaultSpace, ""},
	},
	1,
	false,
}, {
	"first public IPv6 address is picked when both public IPs and public hostnames exist when preferIPv6 is true",
	[]network.Address{
		{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeUnknown, network.DefaultSpace, ""},
		{"8.8.8.8", network.IPv4Address, "floating", network.ScopePublic, network.DefaultSpace, ""},
		{"example.com", network.HostName, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"2001:db8::1", network.IPv6Address, "other", network.ScopePublic, network.DefaultSpace, ""},
	},
	3,
	true,
}}

func (s *AddressSuite) TestSelectPublicAddress(c *gc.C) {
	oldValue := network.PreferIPv6()
	defer func() {
		network.SetPreferIPv6(oldValue)
	}()
	for i, t := range selectPublicTests {
		c.Logf("test %d: %s", i, t.about)
		network.SetPreferIPv6(t.preferIPv6)
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
	false,
}, {
	"a public IPv4 address is selected",
	[]network.Address{
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	0,
	false,
}, {
	"a public IPv6 address is selected",
	[]network.Address{
		{"2001:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	0,
	false,
}, {
	"a public IPv6 address is selected when both IPv4 and IPv6 addresses exist and preferIPv6 is true",
	[]network.Address{
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"2001:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	1,
	true,
}, {
	"the first public IPv4 address is selected when preferIPv6 is true and no IPv6 addresses",
	[]network.Address{
		{"127.0.0.1", network.IPv4Address, "machine", network.ScopeMachineLocal, network.DefaultSpace, ""},
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"169.254.1.1", network.IPv4Address, "link", network.ScopeLinkLocal, network.DefaultSpace, ""},
		{"8.8.4.4", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	1,
	true,
}, {
	"a cloud local IPv4 address is selected",
	[]network.Address{
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"10.0.0.1", network.IPv4Address, "private", network.ScopeCloudLocal, network.DefaultSpace, ""},
	},
	1,
	false,
}, {
	"a cloud local IPv6 address is selected",
	[]network.Address{
		{"fc00::1", network.IPv6Address, "private", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"2001:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	0,
	false,
}, {
	"a machine local or link-local address is not selected",
	[]network.Address{
		{"127.0.0.1", network.IPv4Address, "machine", network.ScopeMachineLocal, network.DefaultSpace, ""},
		{"::1", network.IPv6Address, "machine", network.ScopeMachineLocal, network.DefaultSpace, ""},
		{"fe80::1", network.IPv6Address, "machine", network.ScopeLinkLocal, network.DefaultSpace, ""},
	},
	-1,
	false,
}, {
	"a cloud local address is preferred to a public address",
	[]network.Address{
		{"2001:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"fc00::1", network.IPv6Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
	},
	1,
	false,
}, {
	"an IPv6 cloud local address is preferred to a public address when preferIPv6 is true",
	[]network.Address{
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"2001:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"fc00::1", network.IPv6Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
	},
	2,
	false,
}}

func (s *AddressSuite) TestSelectInternalAddress(c *gc.C) {
	oldValue := network.PreferIPv6()
	defer func() {
		network.SetPreferIPv6(oldValue)
	}()
	for i, t := range selectInternalTests {
		c.Logf("test %d: %s", i, t.about)
		network.SetPreferIPv6(t.preferIPv6)
		expectAddr, expectOK := t.expected()
		actualAddr, actualOK := network.SelectInternalAddress(t.addresses, false)
		c.Check(actualOK, gc.Equals, expectOK)
		c.Check(actualAddr, gc.Equals, expectAddr)
	}
}

var selectInternalMachineTests = []selectTest{{
	"first cloud local address is selected",
	[]network.Address{
		{"fc00::1", network.IPv6Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"2001:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	0,
	false,
}, {
	"first cloud local hostname is selected when preferIPv6 is false",
	[]network.Address{
		{"example.com", network.HostName, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"cloud1.internal", network.HostName, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"cloud2.internal", network.HostName, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"example.org", network.HostName, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	1,
	false,
}, {
	"first cloud local hostname is selected when preferIPv6 is true (public first)",
	[]network.Address{
		{"example.org", network.HostName, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"example.com", network.HostName, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"cloud1.internal", network.HostName, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"cloud2.internal", network.HostName, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
	},
	2,
	true,
}, {
	"first cloud local hostname is selected when preferIPv6 is true (public last)",
	[]network.Address{
		{"cloud1.internal", network.HostName, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"cloud2.internal", network.HostName, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"example.org", network.HostName, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"example.com", network.HostName, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	0,
	true,
}, {
	"first IPv6 cloud local address is selected when preferIPv6 is true",
	[]network.Address{
		{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"fc00::1", network.IPv6Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"2001:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""},
	},
	2,
	true,
}, {
	"first IPv4 cloud local address is selected when preferIPv6 is true and no IPv6 addresses",
	[]network.Address{
		{"169.254.1.1", network.IPv4Address, "link", network.ScopeLinkLocal, network.DefaultSpace, ""},
		{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"172.16.1.1", network.IPv4Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
	},
	1,
	true,
}, {
	"first machine local address is selected",
	[]network.Address{
		{"127.0.0.1", network.IPv4Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
		{"::1", network.IPv6Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
	},
	0,
	false,
}, {
	"first IPv6 machine local address is selected when preferIPv6 is true",
	[]network.Address{
		{"127.0.0.1", network.IPv4Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
		{"::1", network.IPv6Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
		{"fe80::1", network.IPv6Address, "container", network.ScopeLinkLocal, network.DefaultSpace, ""},
	},
	1,
	true,
}, {
	"first IPv4 machine local address is selected when preferIPv6 is true and no IPv6 addresses",
	[]network.Address{
		{"169.254.1.1", network.IPv4Address, "link", network.ScopeLinkLocal, network.DefaultSpace, ""},
		{"127.0.0.1", network.IPv4Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
		{"127.0.0.2", network.IPv4Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
	},
	1,
	true,
}, {
	"first machine local address is selected when preferIPv6 is false even with public/cloud hostnames",
	[]network.Address{
		{"public.example.com", network.HostName, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"::1", network.IPv6Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
		{"unknown.example.com", network.HostName, "public", network.ScopeUnknown, network.DefaultSpace, ""},
		{"cloud.internal", network.HostName, "public", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"127.0.0.1", network.IPv4Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
		{"fe80::1", network.IPv6Address, "container", network.ScopeLinkLocal, network.DefaultSpace, ""},
		{"127.0.0.2", network.IPv4Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
	},
	1,
	false,
}, {
	"first cloud local hostname is selected when preferIPv6 is false even with other machine/cloud addresses",
	[]network.Address{
		{"169.254.1.1", network.IPv4Address, "link", network.ScopeLinkLocal, network.DefaultSpace, ""},
		{"cloud-unknown.internal", network.HostName, "container", network.ScopeUnknown, network.DefaultSpace, ""},
		{"cloud-local.internal", network.HostName, "container", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"fc00::1", network.IPv6Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"127.0.0.1", network.IPv4Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
		{"127.0.0.2", network.IPv4Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
	},
	2,
	false,
}, {
	"first IPv6 machine local address is selected when preferIPv6 is true even with public/cloud hostnames",
	[]network.Address{
		{"127.0.0.1", network.IPv4Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
		{"public.example.com", network.HostName, "public", network.ScopePublic, network.DefaultSpace, ""},
		{"unknown.example.com", network.HostName, "public", network.ScopeUnknown, network.DefaultSpace, ""},
		{"cloud.internal", network.HostName, "public", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"::1", network.IPv6Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
		{"fe80::1", network.IPv6Address, "container", network.ScopeLinkLocal, network.DefaultSpace, ""},
	},
	4,
	true,
}, {
	"first IPv6 cloud local address is selected when preferIPv6 is true even with cloud local hostnames",
	[]network.Address{
		{"169.254.1.1", network.IPv4Address, "link", network.ScopeLinkLocal, network.DefaultSpace, ""},
		{"cloud-unknown.internal", network.HostName, "container", network.ScopeUnknown, network.DefaultSpace, ""},
		{"cloud-local.internal", network.HostName, "container", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"fc00::1", network.IPv6Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"127.0.0.1", network.IPv4Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
		{"fc00::2", network.IPv6Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""},
		{"127.0.0.2", network.IPv4Address, "container", network.ScopeMachineLocal, network.DefaultSpace, ""},
	},
	3,
	true,
}}

func (s *AddressSuite) TestSelectInternalMachineAddress(c *gc.C) {
	oldValue := network.PreferIPv6()
	defer func() {
		network.SetPreferIPv6(oldValue)
	}()
	for i, t := range selectInternalMachineTests {
		c.Logf("test %d: %s", i, t.about)
		network.SetPreferIPv6(t.preferIPv6)
		expectAddr, expectOK := t.expected()
		actualAddr, actualOK := network.SelectInternalAddress(t.addresses, true)
		c.Check(actualOK, gc.Equals, expectOK)
		c.Check(actualAddr, gc.Equals, expectAddr)
	}
}

type selectInternalHostPortsTest struct {
	about      string
	addresses  []network.HostPort
	expected   []string
	preferIPv6 bool
}

var selectInternalHostPortsTests = []selectInternalHostPortsTest{{
	"no addresses gives empty string result",
	[]network.HostPort{},
	[]string{},
	false,
}, {
	"a public IPv4 address is selected",
	[]network.HostPort{
		{network.Address{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""}, 9999},
	},
	[]string{"8.8.8.8:9999"},
	false,
}, {
	"public IPv6 addresses selected when both IPv4 and IPv6 addresses exist and preferIPv6 is true",
	[]network.HostPort{
		{network.Address{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""}, 9999},
		{network.Address{"2001:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""}, 8888},
		{network.Address{"2002:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""}, 9999},
	},
	[]string{"[2001:db8::1]:8888", "[2002:db8::1]:9999"},
	true,
}, {
	"a cloud local IPv4 addresses are selected",
	[]network.HostPort{
		{network.Address{"10.1.0.1", network.IPv4Address, "private", network.ScopeCloudLocal, network.DefaultSpace, ""}, 8888},
		{network.Address{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""}, 123},
		{network.Address{"10.0.0.1", network.IPv4Address, "private", network.ScopeCloudLocal, network.DefaultSpace, ""}, 1234},
	},
	[]string{"10.1.0.1:8888", "10.0.0.1:1234"},
	false,
}, {
	"a machine local or link-local address is not selected",
	[]network.HostPort{
		{network.Address{"127.0.0.1", network.IPv4Address, "machine", network.ScopeMachineLocal, network.DefaultSpace, ""}, 111},
		{network.Address{"::1", network.IPv6Address, "machine", network.ScopeMachineLocal, network.DefaultSpace, ""}, 222},
		{network.Address{"fe80::1", network.IPv6Address, "machine", network.ScopeLinkLocal, network.DefaultSpace, ""}, 333},
	},
	[]string{},
	false,
}, {
	"cloud local addresses are preferred to a public addresses",
	[]network.HostPort{
		{network.Address{"2001:db8::1", network.IPv6Address, "public", network.ScopePublic, network.DefaultSpace, ""}, 123},
		{network.Address{"fc00::1", network.IPv6Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""}, 123},
		{network.Address{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""}, 123},
		{network.Address{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""}, 4444},
	},
	[]string{"[fc00::1]:123", "10.0.0.1:4444"},
	false,
}, {
	"IPv4 addresses are used when prefer-IPv6 is set but no IPv6 addresses are available",
	[]network.HostPort{
		{network.Address{"8.8.8.8", network.IPv4Address, "public", network.ScopePublic, network.DefaultSpace, ""}, 123},
		{network.Address{"10.0.0.1", network.IPv4Address, "cloud", network.ScopeCloudLocal, network.DefaultSpace, ""}, 4444},
	},
	[]string{"10.0.0.1:4444"},
	true,
}}

func (s *AddressSuite) TestSelectInternalHostPorts(c *gc.C) {
	oldValue := network.PreferIPv6()
	defer func() {
		network.SetPreferIPv6(oldValue)
	}()
	for i, t := range selectInternalHostPortsTests {
		c.Logf("test %d: %s", i, t.about)
		network.SetPreferIPv6(t.preferIPv6)
		c.Check(network.SelectInternalHostPorts(t.addresses, false), gc.DeepEquals, t.expected)
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
		SpaceProviderId: network.Id("3"),
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
		NetworkName:     "netname",
		SpaceProviderId: network.Id("3"),
	},
	str: "public:foo.com(netname)@(id:3)",
}, {
	addr: network.Address{
		Type:            network.HostName,
		Value:           "foo.com",
		Scope:           network.ScopePublic,
		NetworkName:     "netname",
		SpaceName:       "badlands",
		SpaceProviderId: network.Id("3"),
	},
	str: "public:foo.com(netname)@badlands(id:3)",
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
		"2001:db8::1",
		"fe80::2",
		"7.8.8.8",
		"172.16.0.1",
		"example.com",
		"8.8.8.8",
	)
	// Simulate prefer-ipv6: false first.
	network.SortAddresses(addrs, false)
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
		// Then machine-local IPv4 addresses.
		"127.0.0.1",
		// Then machine-local IPv6 addresses.
		"::1",
		// Then link-local IPv4 addresses.
		"169.254.1.2",
		// Finally, link-local IPv6 addresses.
		"fe80::2",
	))

	// Now, simulate prefer-ipv6: true.
	network.SortAddresses(addrs, true)
	c.Assert(addrs, jc.DeepEquals, network.NewAddresses(
		// Public IPv6 addresses on top.
		"2001:db8::1",
		// After that public IPv4 addresses.
		"7.8.8.8",
		"8.8.8.8",
		// Then hostnames.
		"example.com",
		"localhost",
		// Then IPv6 cloud-local addresses.
		"fc00::1",
		// Then IPv4 cloud-local addresses.
		"172.16.0.1",
		// Then machine-local IPv6 addresses.
		"::1",
		// Then machine-local IPv4 addresses.
		"127.0.0.1",
		// Then link-local IPv6 addresses.
		"fe80::2",
		// Finally, link-local IPv4 addresses.
		"169.254.1.2",
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
	network.SetPreferIPv6(false)
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

func (*AddressSuite) TestExactScopeMatchHonoursPreferIPv6(c *gc.C) {
	network.SetPreferIPv6(true)
	addr := network.NewScopedAddress("10.0.0.2", network.ScopeCloudLocal)
	match := network.ExactScopeMatch(addr, network.ScopeCloudLocal)
	c.Assert(match, jc.IsFalse)
	match = network.ExactScopeMatch(addr, network.ScopePublic)
	c.Assert(match, jc.IsFalse)

	addr = network.NewScopedAddress("8.8.8.8", network.ScopePublic)
	match = network.ExactScopeMatch(addr, network.ScopeCloudLocal)
	c.Assert(match, jc.IsFalse)
	match = network.ExactScopeMatch(addr, network.ScopePublic)
	c.Assert(match, jc.IsFalse)

	addr = network.NewScopedAddress("2001:db8::ff00:42:8329", network.ScopePublic)
	match = network.ExactScopeMatch(addr, network.ScopeCloudLocal)
	c.Assert(match, jc.IsFalse)
	match = network.ExactScopeMatch(addr, network.ScopePublic)
	c.Assert(match, jc.IsTrue)

	addr = network.NewScopedAddress("fc00::1", network.ScopeCloudLocal)
	match = network.ExactScopeMatch(addr, network.ScopeCloudLocal)
	c.Assert(match, jc.IsTrue)
	match = network.ExactScopeMatch(addr, network.ScopePublic)
	c.Assert(match, jc.IsFalse)
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
