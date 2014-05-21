// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
)

type AddressSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&AddressSuite{})

func (s *AddressSuite) TestNewAddressIpv4(c *gc.C) {
	type test struct {
		value         string
		scope         NetworkScope
		expectedScope NetworkScope
	}

	tests := []test{{
		value:         "127.0.0.1",
		scope:         NetworkUnknown,
		expectedScope: NetworkMachineLocal,
	}, {
		value:         "127.0.0.1",
		scope:         NetworkPublic,
		expectedScope: NetworkPublic, // don't second guess != Unknown
	}, {
		value:         "10.0.3.1",
		scope:         NetworkUnknown,
		expectedScope: NetworkCloudLocal,
	}, {
		value:         "172.16.15.14",
		scope:         NetworkUnknown,
		expectedScope: NetworkCloudLocal,
	}, {
		value:         "192.168.0.1",
		scope:         NetworkUnknown,
		expectedScope: NetworkCloudLocal,
	}, {
		value:         "8.8.8.8",
		scope:         NetworkUnknown,
		expectedScope: NetworkPublic,
	}}

	for _, t := range tests {
		c.Logf("test %s %s", t.value, t.scope)
		addr := NewAddress(t.value, t.scope)
		c.Check(addr.Value, gc.Equals, t.value)
		c.Check(addr.Type, gc.Equals, Ipv4Address)
		c.Check(addr.NetworkScope, gc.Equals, t.expectedScope)
	}
}

func (s *AddressSuite) TestNewAddressIpv6(c *gc.C) {
	addr := NewAddress("::1", NetworkUnknown)
	c.Check(addr.Value, gc.Equals, "::1")
	c.Check(addr.Type, gc.Equals, Ipv6Address)
	c.Check(addr.NetworkScope, gc.Equals, NetworkMachineLocal)

	addr = NewAddress("2001:DB8::1", NetworkUnknown)
	c.Check(addr.Value, gc.Equals, "2001:DB8::1")
	c.Check(addr.Type, gc.Equals, Ipv6Address)
	c.Check(addr.NetworkScope, gc.Equals, NetworkUnknown)
}

func (s *AddressSuite) TestNewAddresses(c *gc.C) {
	addresses := NewAddresses("127.0.0.1", "192.168.1.1", "192.168.178.255")
	c.Assert(len(addresses), gc.Equals, 3)
	c.Assert(addresses[0].Value, gc.Equals, "127.0.0.1")
	c.Assert(addresses[0].NetworkScope, gc.Equals, NetworkMachineLocal)
	c.Assert(addresses[1].Value, gc.Equals, "192.168.1.1")
	c.Assert(addresses[1].NetworkScope, gc.Equals, NetworkCloudLocal)
	c.Assert(addresses[2].Value, gc.Equals, "192.168.178.255")
	c.Assert(addresses[2].NetworkScope, gc.Equals, NetworkCloudLocal)
}

func (s *AddressSuite) TestNewAddressHostname(c *gc.C) {
	addr := NewAddress("localhost", NetworkUnknown)
	c.Check(addr.Value, gc.Equals, "localhost")
	c.Check(addr.Type, gc.Equals, HostName)
	c.Check(addr.NetworkScope, gc.Equals, NetworkUnknown)
}

type selectTest struct {
	about         string
	addresses     []Address
	expectedIndex int
}

// expected returns the expected address for the test.
func (t selectTest) expected() string {
	if t.expectedIndex == -1 {
		return ""
	}
	return t.addresses[t.expectedIndex].Value
}

type hostPortTest struct {
	about         string
	hostPorts     []HostPort
	expectedIndex int
}

// hostPortTest returns the HostPort equivalent test to the
// receiving selectTest.
func (t selectTest) hostPortTest() hostPortTest {
	hps := AddressesWithPort(t.addresses, 9999)
	for i := range hps {
		hps[i].Port = i + 1
	}
	return hostPortTest{
		about:         t.about,
		hostPorts:     hps,
		expectedIndex: t.expectedIndex,
	}
}

// expected returns the expected host:port result
// of the test.
func (t hostPortTest) expected() string {
	if t.expectedIndex == -1 {
		return ""
	}
	return t.hostPorts[t.expectedIndex].NetAddr()
}

var selectPublicTests = []selectTest{{
	"no addresses gives empty string result",
	[]Address{},
	-1,
}, {
	"a public address is selected",
	[]Address{
		{"8.8.8.8", Ipv4Address, "public", NetworkPublic},
	},
	0,
}, {
	"a machine local address is not selected",
	[]Address{
		{"127.0.0.1", Ipv4Address, "machine", NetworkMachineLocal},
	},
	-1,
}, {
	"an ipv6 address is not selected",
	[]Address{
		{"2001:DB8::1", Ipv6Address, "", NetworkPublic},
	},
	-1,
}, {
	"a public name is preferred to an unknown or cloud local address",
	[]Address{
		{"127.0.0.1", Ipv4Address, "local", NetworkUnknown},
		{"10.0.0.1", Ipv4Address, "cloud", NetworkCloudLocal},
		{"public.invalid.testing", HostName, "public", NetworkPublic},
	},
	2,
}, {
	"first unknown address selected",
	[]Address{
		{"10.0.0.1", Ipv4Address, "cloud", NetworkUnknown},
		{"8.8.8.8", Ipv4Address, "floating", NetworkUnknown},
	},
	0,
}}

func (s *AddressSuite) TestSelectPublicAddress(c *gc.C) {
	for i, t := range selectPublicTests {
		c.Logf("test %d. %s", i, t.about)
		c.Check(SelectPublicAddress(t.addresses), gc.Equals, t.expected())
	}
}

func (s *AddressSuite) TestSelectPublicHostPort(c *gc.C) {
	for i, t0 := range selectPublicTests {
		t := t0.hostPortTest()
		c.Logf("test %d. %s", i, t.about)
		c.Assert(SelectPublicHostPort(t.hostPorts), gc.DeepEquals, t.expected())
	}
}

var selectInternalTests = []selectTest{{
	"no addresses gives empty string result",
	[]Address{},
	-1,
}, {
	"a public address is selected",
	[]Address{
		{"8.8.8.8", Ipv4Address, "public", NetworkPublic},
	},
	0,
}, {
	"a cloud local address is selected",
	[]Address{
		{"10.0.0.1", Ipv4Address, "private", NetworkCloudLocal},
	},
	0,
}, {
	"a machine local address is not selected",
	[]Address{
		{"127.0.0.1", Ipv4Address, "machine", NetworkMachineLocal},
	},
	-1,
}, {
	"ipv6 addresses are not selected",
	[]Address{
		{"::1", Ipv6Address, "", NetworkCloudLocal},
	},
	-1,
}, {
	"a cloud local address is preferred to a public address",
	[]Address{
		{"10.0.0.1", Ipv4Address, "cloud", NetworkCloudLocal},
		{"8.8.8.8", Ipv4Address, "public", NetworkPublic},
	},
	0,
}}

func (s *AddressSuite) TestSelectInternalAddress(c *gc.C) {
	for i, t := range selectInternalTests {
		c.Logf("test %d. %s", i, t.about)
		c.Check(SelectInternalAddress(t.addresses, false), gc.Equals, t.expected())
	}
}

func (s *AddressSuite) TestSelectInternalHostPort(c *gc.C) {
	for i, t0 := range selectInternalTests {
		t := t0.hostPortTest()
		c.Logf("test %d. %s", i, t.about)
		c.Assert(SelectInternalHostPort(t.hostPorts, false), gc.DeepEquals, t.expected())
	}
}

var selectInternalMachineTests = []selectTest{{
	"a cloud local address is selected",
	[]Address{
		{"10.0.0.1", Ipv4Address, "cloud", NetworkCloudLocal},
	},
	0,
}, {
	"a machine local address is selected",
	[]Address{
		{"127.0.0.1", Ipv4Address, "container", NetworkMachineLocal},
	},
	0,
}}

func (s *AddressSuite) TestSelectInternalMachineAddress(c *gc.C) {
	for i, t := range selectInternalMachineTests {
		c.Logf("test %d. %s", i, t.about)
		c.Check(SelectInternalAddress(t.addresses, true), gc.Equals, t.expected())
	}
}

func (s *AddressSuite) TestSelectInternalMachineHostPort(c *gc.C) {
	for i, t0 := range selectInternalMachineTests {
		t := t0.hostPortTest()
		c.Logf("test %d. %s", i, t.about)
		c.Assert(SelectInternalHostPort(t.hostPorts, true), gc.DeepEquals, t.expected())
	}
}

var stringTests = []struct {
	addr Address
	str  string
}{{
	addr: Address{
		Type:  Ipv4Address,
		Value: "127.0.0.1",
	},
	str: "127.0.0.1",
}, {
	addr: Address{
		Type:  HostName,
		Value: "foo.com",
	},
	str: "foo.com",
}, {
	addr: Address{
		Type:         HostName,
		Value:        "foo.com",
		NetworkScope: NetworkUnknown,
	},
	str: "foo.com",
}, {
	addr: Address{
		Type:         HostName,
		Value:        "foo.com",
		NetworkScope: NetworkPublic,
	},
	str: "public:foo.com",
}, {
	addr: Address{
		Type:         HostName,
		Value:        "foo.com",
		NetworkScope: NetworkPublic,
		NetworkName:  "netname",
	},
	str: "public:foo.com(netname)",
}}

func (s *AddressSuite) TestString(c *gc.C) {
	for _, test := range stringTests {
		c.Check(test.addr.String(), gc.Equals, test.str)
	}
}

func (*AddressSuite) TestAddressesWithPort(c *gc.C) {
	addrs := NewAddresses("0.1.2.3", "0.2.4.6")
	hps := AddressesWithPort(addrs, 999)
	c.Assert(hps, jc.DeepEquals, []HostPort{{
		Address: NewAddress("0.1.2.3", NetworkUnknown),
		Port:    999,
	}, {
		Address: NewAddress("0.2.4.6", NetworkUnknown),
		Port:    999,
	}})
}

var netAddrTests = []struct {
	addr   Address
	port   int
	expect string
}{{
	addr:   NewAddress("0.1.2.3", NetworkUnknown),
	port:   99,
	expect: "0.1.2.3:99",
}, {
	addr:   NewAddress("2001:DB8::1", NetworkUnknown),
	port:   100,
	expect: "[2001:DB8::1]:100",
}}

func (*AddressSuite) TestNetAddr(c *gc.C) {
	for i, test := range netAddrTests {
		c.Logf("test %d: %q", i, test.addr)
		hp := HostPort{
			Address: test.addr,
			Port:    test.port,
		}
		c.Assert(hp.NetAddr(), gc.Equals, test.expect)
	}
}
