// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"errors"
	"net"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

type AddressSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&AddressSuite{})

func (s *AddressSuite) TestNewAddressIpv4(c *gc.C) {
	addr := NewAddress("127.0.0.1")
	c.Check(addr.Value, gc.Equals, "127.0.0.1")
	c.Check(addr.Type, gc.Equals, Ipv4Address)
}

func (s *AddressSuite) TestNewAddressIpv6(c *gc.C) {
	addr := NewAddress("::1")
	c.Check(addr.Value, gc.Equals, "::1")
	c.Check(addr.Type, gc.Equals, Ipv6Address)
}

func (s *AddressSuite) TestNewAddressHostname(c *gc.C) {
	addr := NewAddress("localhost")
	c.Check(addr.Value, gc.Equals, "localhost")
	c.Check(addr.Type, gc.Equals, HostName)
}

type selectTests struct {
	about     string
	addresses []Address
	expected  string
}

var selectPublicTests = []selectTests{{
	"no addresses gives empty string result",
	[]Address{},
	"",
}, {
	"a public address is selected",
	[]Address{
		{"8.8.8.8", Ipv4Address, "public", NetworkPublic},
	},
	"8.8.8.8",
}, {
	"a machine local address is not selected",
	[]Address{
		{"127.0.0.1", Ipv4Address, "machine", NetworkMachineLocal},
	},
	"",
}, {
	"an ipv6 address is not selected",
	[]Address{
		{"2001:DB8::1", Ipv6Address, "", NetworkPublic},
	},
	"",
}, {
	"a public name is preferred to an unknown or cloud local address",
	[]Address{
		{"127.0.0.1", Ipv4Address, "local", NetworkUnknown},
		{"10.0.0.1", Ipv4Address, "cloud", NetworkCloudLocal},
		{"public.invalid.testing", HostName, "public", NetworkPublic},
	},
	"public.invalid.testing",
}, {
	"last unknown address selected",
	[]Address{
		{"10.0.0.1", Ipv4Address, "cloud", NetworkUnknown},
		{"8.8.8.8", Ipv4Address, "floating", NetworkUnknown},
	},
	"8.8.8.8",
}}

func (s *AddressSuite) TestSelectPublicAddressEmpty(c *gc.C) {
	for i, t := range selectPublicTests {
		c.Logf("test %d. %s", i, t.about)
		c.Check(SelectPublicAddress(t.addresses), gc.Equals, t.expected)
	}
}

var selectInternalTests = []selectTests{{
	"no addresses gives empty string result",
	[]Address{},
	"",
}, {
	"a public address is selected",
	[]Address{
		{"8.8.8.8", Ipv4Address, "public", NetworkPublic},
	},
	"8.8.8.8",
}, {
	"a cloud local address is selected",
	[]Address{
		{"10.0.0.1", Ipv4Address, "private", NetworkCloudLocal},
	},
	"10.0.0.1",
}, {
	"a machine local address is not selected",
	[]Address{
		{"127.0.0.1", Ipv4Address, "machine", NetworkMachineLocal},
	},
	"",
}, {
	"ipv6 addresses are not selected",
	[]Address{
		{"::1", Ipv6Address, "", NetworkCloudLocal},
	},
	"",
}, {
	"a cloud local address is preferred to a public address",
	[]Address{
		{"10.0.0.1", Ipv4Address, "cloud", NetworkCloudLocal},
		{"8.8.8.8", Ipv4Address, "public", NetworkPublic},
	},
	"10.0.0.1",
}}

func (s *AddressSuite) TestSelectInternalAddress(c *gc.C) {
	for i, t := range selectInternalTests {
		c.Logf("test %d. %s", i, t.about)
		c.Check(SelectInternalAddress(t.addresses, false), gc.Equals, t.expected)
	}
}

var selectInternalMachineTests = []selectTests{{
	"a cloud local address is selected",
	[]Address{
		{"10.0.0.1", Ipv4Address, "cloud", NetworkCloudLocal},
	},
	"10.0.0.1",
}, {
	"a machine local address is selected",
	[]Address{
		{"127.0.0.1", Ipv4Address, "container", NetworkMachineLocal},
	},
	"127.0.0.1",
}}

func (s *AddressSuite) TestSelectInternalMachineAddress(c *gc.C) {
	for i, t := range selectInternalMachineTests {
		c.Logf("test %d. %s", i, t.about)
		c.Check(SelectInternalAddress(t.addresses, true), gc.Equals, t.expected)
	}
}

func (s *AddressSuite) TestHostAddresses(c *gc.C) {
	// Mock the call to net.LookupIP made from HostAddresses.
	var lookupIPs []net.IP
	var lookupErr error
	lookupIP := func(addr string) ([]net.IP, error) {
		return append([]net.IP{}, lookupIPs...), lookupErr
	}
	s.PatchValue(&netLookupIP, lookupIP)

	// err is only non-nil if net.LookupIP fails.
	addrs, err := HostAddresses("")
	c.Assert(err, gc.IsNil)
	// addrs always contains the input address.
	c.Assert(addrs, gc.HasLen, 1)
	c.Assert(addrs[0], gc.Equals, NewAddress(""))

	loopback := net.ParseIP("127.0.0.1").To4()
	lookupIPs = []net.IP{net.IPv6loopback, net.IPv4zero, loopback}
	addrs, err = HostAddresses("localhost")
	c.Assert(err, gc.IsNil)
	c.Assert(addrs, gc.HasLen, 4)
	c.Assert(addrs[0], gc.Equals, NewAddress(net.IPv6loopback.String()))
	c.Assert(addrs[1], gc.Equals, NewAddress(net.IPv4zero.String()))
	c.Assert(addrs[2], gc.Equals, NewAddress(loopback.String()))
	c.Assert(addrs[3], gc.Equals, NewAddress("localhost"))

	lookupErr = errors.New("what happened?")
	addrs, err = HostAddresses("localhost")
	c.Assert(err, gc.Equals, lookupErr)

	// If the input address is an IP, the call to net.LookupIP is elided.
	addrs, err = HostAddresses("127.0.0.1")
	c.Assert(err, gc.IsNil)
	c.Assert(addrs, gc.HasLen, 1)
	c.Assert(addrs[0], gc.Equals, NewAddress("127.0.0.1"))
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
