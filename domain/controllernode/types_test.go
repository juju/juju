// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllernode

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testing"
)

type typesSuite struct {
	testing.BaseSuite
}

func TestTypesSuite(t *stdtesting.T) {
	tc.Run(t, &typesSuite{})
}

type selectInternalHostPortsTest struct {
	about     string
	addresses APIAddresses
	expected  []string
}

var prioritizeInternalHostPortsTests = []selectInternalHostPortsTest{{
	"no addresses gives empty string result",
	APIAddresses{},
	[]string{},
}, {
	"a public IPv4 address is selected",
	APIAddresses{
		{Address: "8.8.8.8:9999", Scope: network.ScopePublic},
	},
	[]string{"8.8.8.8:9999"},
}, {
	"cloud local IPv4 addresses are selected",
	APIAddresses{
		{Address: "10.1.0.1:8888", Scope: network.ScopeCloudLocal},
		{Address: "8.8.8.8:123", Scope: network.ScopePublic},
		{Address: "10.0.0.1:1234", Scope: network.ScopeCloudLocal},
	},
	[]string{"10.1.0.1:8888", "10.0.0.1:1234", "8.8.8.8:123"},
}, {
	"cloud local IPv4 then public addresses are selected",
	APIAddresses{
		{Address: "10.1.0.1:8888", Scope: network.ScopeMachineLocal},
		{Address: "8.8.8.8:123", Scope: network.ScopePublic},
		{Address: "10.0.0.1:1234", Scope: network.ScopeCloudLocal},
	},
	[]string{"10.0.0.1:1234", "8.8.8.8:123"},
}, {
	"a machine local or link-local address is not selected",
	APIAddresses{
		{Address: "127.0.0.1:111", Scope: network.ScopeMachineLocal},
		{Address: "::1:222", Scope: network.ScopeMachineLocal},
		{Address: "fe80::1:333", Scope: network.ScopeLinkLocal},
	},
	[]string{},
}, {
	"cloud local addresses are preferred to a public addresses",
	APIAddresses{
		{Address: "[2001:db8::1]:123", Scope: network.ScopePublic},
		{Address: "[fc00::1]:123", Scope: network.ScopeCloudLocal},
		{Address: "8.8.8.8:123", Scope: network.ScopePublic},
		{Address: "10.0.0.1:4444", Scope: network.ScopeCloudLocal},
	},
	[]string{"10.0.0.1:4444", "[fc00::1]:123", "8.8.8.8:123", "[2001:db8::1]:123"},
}}

func (s *typesSuite) TestPrioritizeInternalHostPorts(c *tc.C) {
	for i, t := range prioritizeInternalHostPortsTests {
		c.Logf("test %d: %s", i, t.about)
		prioritized := t.addresses.PrioritizedForScope(ScopeMatchCloudLocal)
		c.Check(prioritized, tc.DeepEquals, t.expected)
	}
}

type selectNoProxyStringTest struct {
	about     string
	addresses APIAddresses
	expected  string
}

var toNoProxyStringTests = []selectNoProxyStringTest{
	{
		about: "skip machine local",
		addresses: APIAddresses{
			{Address: "0.1.2.3:17070", Scope: network.ScopePublic},
			{Address: "0.1.2.4:17070", Scope: network.ScopePublic},
			{ // This address should be ignored
				Address: "42.1.2.4:17070",
				Scope:   network.ScopeMachineLocal,
			},
			{Address: "0.1.2.5:17070", Scope: network.ScopePublic},
		},
		expected: "0.1.2.3,0.1.2.4,0.1.2.5",
	}, {
		about: "skip link local",
		addresses: APIAddresses{
			{Address: "0.1.2.3:17070", Scope: network.ScopePublic},
			{Address: "0.1.2.4:17070", Scope: network.ScopePublic},
			{ // This address should be ignored
				Address: "42.1.2.4:17070",
				Scope:   network.ScopeLinkLocal,
			},
			{Address: "0.1.2.5:17070", Scope: network.ScopePublic},
		},
		expected: "0.1.2.3,0.1.2.4,0.1.2.5",
	}, {
		about: "out of order input",
		addresses: APIAddresses{
			{Address: "0.1.2.5:17070", Scope: network.ScopePublic},
			{Address: "0.1.2.3:17070", Scope: network.ScopePublic},
			{Address: "0.1.2.4:17070", Scope: network.ScopePublic},
		},
		expected: "0.1.2.3,0.1.2.4,0.1.2.5",
	},
}

func (s *typesSuite) TestToNoProxyString(c *tc.C) {
	for i, t := range toNoProxyStringTests {
		c.Logf("test %d: %s", i, t.about)
		prioritized := t.addresses.ToNoProxyString()
		c.Check(prioritized, tc.DeepEquals, t.expected)
	}
}

var selectPublicTests = []selectInternalHostPortsTest{{
	"no addresses gives empty string result",
	[]APIAddress{},
	[]string{},
}, {
	"a public IPv4 address is selected",
	[]APIAddress{
		{Address: "8.8.8.8:17070", Scope: network.ScopePublic},
	},
	[]string{"8.8.8.8:17070"},
}, {
	"a public IPv6 address is selected",
	[]APIAddress{
		{Address: "[2001:db8::1]:17070", Scope: network.ScopePublic},
	},
	[]string{"[2001:db8::1]:17070"},
}, {
	"first public address is selected",
	[]APIAddress{
		{Address: "8.8.8.8:17070", Scope: network.ScopePublic},
		{Address: "[2001:db8::1]:17070", Scope: network.ScopePublic},
	},
	[]string{"8.8.8.8:17070", "[2001:db8::1]:17070"},
}, {
	"the first public address is selected when cloud local fallbacks exist",
	[]APIAddress{
		{Address: "172.16.1.1:17070", Scope: network.ScopeCloudLocal},
		{Address: "8.8.8.8:17070", Scope: network.ScopePublic},
		{Address: "[fc00:1]:17070", Scope: network.ScopeCloudLocal},
		{Address: "[2001:db8::1]:17070", Scope: network.ScopePublic},
	},
	[]string{"8.8.8.8:17070", "[2001:db8::1]:17070", "172.16.1.1:17070", "[fc00:1]:17070"},
}, {
	"the cloud local address is selected when a fan-local fallback exists",
	[]APIAddress{
		{Address: "243.1.1.1:17070", Scope: network.ScopeFanLocal},
		{Address: "172.16.1.1:17070", Scope: network.ScopeCloudLocal},
	},
	[]string{"172.16.1.1:17070", "243.1.1.1:17070"},
}, {
	"a machine IPv4 local address is not selected",
	[]APIAddress{
		{Address: "127.0.0.1:17070", Scope: network.ScopeMachineLocal},
	},
	[]string{},
}, {
	"a machine IPv6 local address is not selected",
	[]APIAddress{
		{Address: "[::1]:17070", Scope: network.ScopeMachineLocal},
	},
	[]string{},
}, {
	"a link-local IPv4 address is not selected",
	[]APIAddress{
		{Address: "169.254.1.1:17070", Scope: network.ScopeLinkLocal},
	},
	[]string{},
}, {
	"a link-local (multicast or not) IPv6 address is not selected",
	[]APIAddress{
		{Address: "[fe80::1]:17070", Scope: network.ScopeLinkLocal},
		{Address: "[ff01::2]:17070", Scope: network.ScopeLinkLocal},
		{Address: "[ff02::1:1]:17070", Scope: network.ScopeLinkLocal},
	},
	[]string{},
}, {
	"a public name is preferred to an unknown or cloud local address",
	[]APIAddress{
		{Address: "127.0.0.1:170702", Scope: network.ScopeMachineLocal},
		{Address: "10.0.0.1:17070", Scope: network.ScopeCloudLocal},
		{Address: "[fc00::1]:17070", Scope: network.ScopeCloudLocal},
		{Address: "public.invalid.testing", Scope: network.ScopePublic},
	},
	[]string{"public.invalid.testing", "10.0.0.1:17070", "[fc00::1]:17070"},
}, {
	"first unknown address selected",
	[]APIAddress{
		{Address: "10.0.0.1:17070", Scope: network.ScopeUnknown},
		{Address: "8.8.8.8:17070", Scope: network.ScopeUnknown},
	},
	[]string{"10.0.0.1:17070", "8.8.8.8:17070"},
}, {
	"public IP address is picked when both public IPs and public hostnames exist",
	[]APIAddress{
		{Address: "10.0.0.1:17070", Scope: network.ScopeCloudLocal},
		{Address: "example.com", Scope: network.ScopePublic},
		{Address: "8.8.8.8:17070", Scope: network.ScopePublic},
	},
	[]string{"8.8.8.8:17070", "example.com", "10.0.0.1:17070"},
}, {
	"hostname is picked over cloud local address",
	[]APIAddress{
		{Address: "10.0.0.1:17070", Scope: network.ScopeCloudLocal},
		{Address: "example.com", Scope: network.ScopePublic},
	},
	[]string{"example.com", "10.0.0.1:17070"},
}, {
	"IPv4 preferred over IPv6",
	[]APIAddress{
		{Address: "[2001:db8::1]:17070", Scope: network.ScopePublic},
		{Address: "8.8.8.8:17070", Scope: network.ScopePublic},
	},
	[]string{"8.8.8.8:17070", "[2001:db8::1]:17070"},
}}

func (s *typesSuite) TestSelectPublicAddress(c *tc.C) {
	for i, t := range selectPublicTests {
		c.Logf("test %d: %s", i, t.about)
		prioritized := t.addresses.PrioritizedForScope(ScopeMatchPublic)
		c.Check(prioritized, tc.DeepEquals, t.expected)
	}
}
