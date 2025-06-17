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
