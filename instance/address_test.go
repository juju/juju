// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
)

type AddressSuite struct{}

var _ = gc.Suite(&AddressSuite{})

func (s *AddressSuite) TestNewAddressIpv4(c *gc.C) {
	addr := instance.NewAddress("127.0.0.1")
	c.Check(addr.Value, gc.Equals, "127.0.0.1")
	c.Check(addr.Type, gc.Equals, instance.Ipv4Address)
}

func (s *AddressSuite) TestNewAddressIpv6(c *gc.C) {
	addr := instance.NewAddress("::1")
	c.Check(addr.Value, gc.Equals, "::1")
	c.Check(addr.Type, gc.Equals, instance.Ipv6Address)
}

func (s *AddressSuite) TestNewAddressHostname(c *gc.C) {
	addr := instance.NewAddress("localhost")
	c.Check(addr.Value, gc.Equals, "localhost")
	c.Check(addr.Type, gc.Equals, instance.HostName)
}

type selectTests struct {
	about     string
	addresses []instance.Address
	expected  string
}

var selectPublicTests = []selectTests{{
	"no addresses gives empty string result",
	[]instance.Address{},
	"",
}, {
	"a public address is selected",
	[]instance.Address{
		{"8.8.8.8", instance.Ipv4Address, "public", instance.NetworkPublic},
	},
	"8.8.8.8",
}, {
	"a machine local address is not selected",
	[]instance.Address{
		{"127.0.0.1", instance.Ipv4Address, "machine", instance.NetworkMachineLocal},
	},
	"",
}, {
	"an ipv6 address is not selected",
	[]instance.Address{
		{"2001:DB8::1", instance.Ipv6Address, "", instance.NetworkPublic},
	},
	"",
}, {
	"a public name is preferred to an unknown or cloud local address",
	[]instance.Address{
		{"127.0.0.1", instance.Ipv4Address, "local", instance.NetworkUnknown},
		{"10.0.0.1", instance.Ipv4Address, "cloud", instance.NetworkCloudLocal},
		{"public.invalid.testing", instance.HostName, "public", instance.NetworkPublic},
	},
	"public.invalid.testing",
}, {
	"last unknown address selected",
	[]instance.Address{
		{"10.0.0.1", instance.Ipv4Address, "cloud", instance.NetworkUnknown},
		{"8.8.8.8", instance.Ipv4Address, "floating", instance.NetworkUnknown},
	},
	"8.8.8.8",
}}

func (s *AddressSuite) TestSelectPublicAddressEmpty(c *gc.C) {
	for i, t := range selectPublicTests {
		c.Logf("test %d. %s", i, t.about)
		c.Check(instance.SelectPublicAddress(t.addresses), gc.Equals, t.expected)
	}
}

var selectInternalTests = []selectTests{{
	"no addresses gives empty string result",
	[]instance.Address{},
	"",
}, {
	"a public address is selected",
	[]instance.Address{
		{"8.8.8.8", instance.Ipv4Address, "public", instance.NetworkPublic},
	},
	"8.8.8.8",
}, {
	"a cloud local address is selected",
	[]instance.Address{
		{"10.0.0.1", instance.Ipv4Address, "private", instance.NetworkCloudLocal},
	},
	"10.0.0.1",
}, {
	"a machine local address is not selected",
	[]instance.Address{
		{"127.0.0.1", instance.Ipv4Address, "machine", instance.NetworkMachineLocal},
	},
	"",
}, {
	"ipv6 addresses are not selected",
	[]instance.Address{
		{"::1", instance.Ipv6Address, "", instance.NetworkCloudLocal},
	},
	"",
}, {
	"a cloud local address is preferred to a public address",
	[]instance.Address{
		{"10.0.0.1", instance.Ipv4Address, "cloud", instance.NetworkCloudLocal},
		{"8.8.8.8", instance.Ipv4Address, "public", instance.NetworkPublic},
	},
	"10.0.0.1",
}}

func (s *AddressSuite) TestSelectInternalAddress(c *gc.C) {
	for i, t := range selectInternalTests {
		c.Logf("test %d. %s", i, t.about)
		c.Check(instance.SelectInternalAddress(t.addresses, false), gc.Equals, t.expected)
	}
}

var selectInternalMachineTests = []selectTests{{
	"a cloud local address is selected",
	[]instance.Address{
		{"10.0.0.1", instance.Ipv4Address, "cloud", instance.NetworkCloudLocal},
	},
	"10.0.0.1",
}, {
	"a machine local address is selected",
	[]instance.Address{
		{"127.0.0.1", instance.Ipv4Address, "container", instance.NetworkMachineLocal},
	},
	"127.0.0.1",
}}

func (s *AddressSuite) TestSelectInternalMachineAddress(c *gc.C) {
	for i, t := range selectInternalMachineTests {
		c.Logf("test %d. %s", i, t.about)
		c.Check(instance.SelectInternalAddress(t.addresses, true), gc.Equals, t.expected)
	}
}
