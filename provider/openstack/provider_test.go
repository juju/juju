// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/goose.v1/nova"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/testing"
)

// localTests contains tests which do not require a live service or test double to run.
type localTests struct{}

var _ = gc.Suite(&localTests{})

// ported from lp:juju/juju/providers/openstack/tests/test_machine.py
var addressTests = []struct {
	summary    string
	floatingIP string
	private    []nova.IPAddress
	public     []nova.IPAddress
	networks   []string
	expected   string
	failure    error
}{{
	summary:  "missing",
	expected: "",
}, {
	summary:  "empty",
	private:  []nova.IPAddress{},
	networks: []string{"private"},
	expected: "",
}, {
	summary:  "private IPv4 only",
	private:  []nova.IPAddress{{4, "192.168.0.1"}},
	networks: []string{"private"},
	expected: "192.168.0.1",
}, {
	summary:  "private IPv6 only",
	private:  []nova.IPAddress{{6, "fc00::1"}},
	networks: []string{"private"},
	expected: "fc00::1",
}, {
	summary:  "private only, both IPv4 and IPv6",
	private:  []nova.IPAddress{{4, "192.168.0.1"}, {6, "fc00::1"}},
	networks: []string{"private"},
	expected: "192.168.0.1",
}, {
	summary:  "private only, both IPv6 and IPv4",
	private:  []nova.IPAddress{{6, "fc00::1"}, {4, "192.168.0.1"}},
	networks: []string{"private"},
	expected: "fc00::1",
}, {
	summary:  "private IPv4 plus (HP cloud)",
	private:  []nova.IPAddress{{4, "10.0.0.1"}, {4, "8.8.4.4"}},
	networks: []string{"private"},
	expected: "8.8.4.4",
}, {
	summary:  "public IPv4 only",
	public:   []nova.IPAddress{{4, "8.8.8.8"}},
	networks: []string{"", "public"},
	expected: "8.8.8.8",
}, {
	summary:  "public IPv6 only",
	public:   []nova.IPAddress{{6, "2001:db8::1"}},
	networks: []string{"", "public"},
	expected: "2001:db8::1",
}, {
	summary:  "public only, both IPv4 and IPv6",
	public:   []nova.IPAddress{{4, "8.8.8.8"}, {6, "2001:db8::1"}},
	networks: []string{"", "public"},
	expected: "8.8.8.8",
}, {
	summary:  "public only, both IPv6 and IPv4",
	public:   []nova.IPAddress{{6, "2001:db8::1"}, {4, "8.8.8.8"}},
	networks: []string{"", "public"},
	expected: "2001:db8::1",
}, {
	summary:  "public and private both IPv4",
	private:  []nova.IPAddress{{4, "10.0.0.4"}},
	public:   []nova.IPAddress{{4, "8.8.4.4"}},
	networks: []string{"private", "public"},
	expected: "8.8.4.4",
}, {
	summary:  "public and private both IPv6",
	private:  []nova.IPAddress{{6, "fc00::1"}},
	public:   []nova.IPAddress{{6, "2001:db8::1"}},
	networks: []string{"private", "public"},
	expected: "2001:db8::1",
}, {
	summary:  "public, private, and localhost IPv4",
	private:  []nova.IPAddress{{4, "127.0.0.4"}, {4, "192.168.0.1"}},
	public:   []nova.IPAddress{{4, "8.8.8.8"}},
	networks: []string{"private", "public"},
	expected: "8.8.8.8",
}, {
	summary:  "public, private, and localhost IPv6",
	private:  []nova.IPAddress{{6, "::1"}, {6, "fc00::1"}},
	public:   []nova.IPAddress{{6, "2001:db8::1"}},
	networks: []string{"private", "public"},
	expected: "2001:db8::1",
}, {
	summary:  "public, private, and localhost - both IPv4 and IPv6",
	private:  []nova.IPAddress{{4, "127.0.0.4"}, {4, "192.168.0.1"}, {6, "::1"}, {6, "fc00::1"}},
	public:   []nova.IPAddress{{4, "8.8.8.8"}, {6, "2001:db8::1"}},
	networks: []string{"private", "public"},
	expected: "8.8.8.8",
}, {
	summary:  "public, private, and localhost - both IPv6 and IPv4",
	private:  []nova.IPAddress{{6, "::1"}, {6, "fc00::1"}, {4, "127.0.0.4"}, {4, "192.168.0.1"}},
	public:   []nova.IPAddress{{6, "2001:db8::1"}, {4, "8.8.8.8"}},
	networks: []string{"private", "public"},
	expected: "2001:db8::1",
}, {
	summary:  "custom only IPv4",
	private:  []nova.IPAddress{{4, "192.168.0.1"}},
	networks: []string{"special"},
	expected: "192.168.0.1",
}, {
	summary:  "custom only IPv6",
	private:  []nova.IPAddress{{6, "fc00::1"}},
	networks: []string{"special"},
	expected: "fc00::1",
}, {
	summary:  "custom only - both IPv4 and IPv6",
	private:  []nova.IPAddress{{4, "192.168.0.1"}, {6, "fc00::1"}},
	networks: []string{"special"},
	expected: "192.168.0.1",
}, {
	summary:  "custom only - both IPv6 and IPv4",
	private:  []nova.IPAddress{{6, "fc00::1"}, {4, "192.168.0.1"}},
	networks: []string{"special"},
	expected: "fc00::1",
}, {
	summary:  "custom and public IPv4",
	private:  []nova.IPAddress{{4, "172.16.0.1"}},
	public:   []nova.IPAddress{{4, "8.8.8.8"}},
	networks: []string{"special", "public"},
	expected: "8.8.8.8",
}, {
	summary:  "custom and public IPv6",
	private:  []nova.IPAddress{{6, "fc00::1"}},
	public:   []nova.IPAddress{{6, "2001:db8::1"}},
	networks: []string{"special", "public"},
	expected: "2001:db8::1",
}, {
	summary:  "custom and public - both IPv4 and IPv6",
	private:  []nova.IPAddress{{4, "172.16.0.1"}, {6, "fc00::1"}},
	public:   []nova.IPAddress{{4, "8.8.8.8"}, {6, "2001:db8::1"}},
	networks: []string{"special", "public"},
	expected: "8.8.8.8",
}, {
	summary:  "custom and public - both IPv6 and IPv4",
	private:  []nova.IPAddress{{6, "fc00::1"}, {4, "172.16.0.1"}},
	public:   []nova.IPAddress{{6, "2001:db8::1"}, {4, "8.8.8.8"}},
	networks: []string{"special", "public"},
	expected: "2001:db8::1",
}, {
	summary:    "floating and public, same address",
	floatingIP: "8.8.8.8",
	public:     []nova.IPAddress{{4, "8.8.8.8"}},
	networks:   []string{"", "public"},
	expected:   "8.8.8.8",
}, {
	summary:    "floating and public, different address",
	floatingIP: "8.8.4.4",
	public:     []nova.IPAddress{{4, "8.8.8.8"}},
	networks:   []string{"", "public"},
	expected:   "8.8.4.4",
}, {
	summary:    "floating and private",
	floatingIP: "8.8.4.4",
	private:    []nova.IPAddress{{4, "10.0.0.1"}},
	networks:   []string{"private"},
	expected:   "8.8.4.4",
}, {
	summary:    "floating, custom and public",
	floatingIP: "8.8.4.4",
	private:    []nova.IPAddress{{4, "172.16.0.1"}},
	public:     []nova.IPAddress{{4, "8.8.8.8"}},
	networks:   []string{"special", "public"},
	expected:   "8.8.4.4",
}}

func (t *localTests) TestGetServerAddresses(c *gc.C) {
	for i, t := range addressTests {
		c.Logf("#%d. %s -> %s (%v)", i, t.summary, t.expected, t.failure)
		addresses := make(map[string][]nova.IPAddress)
		if t.private != nil {
			if len(t.networks) < 1 {
				addresses["private"] = t.private
			} else {
				addresses[t.networks[0]] = t.private
			}
		}
		if t.public != nil {
			if len(t.networks) < 2 {
				addresses["public"] = t.public
			} else {
				addresses[t.networks[1]] = t.public
			}
		}
		addr := openstack.InstanceAddress(t.floatingIP, addresses)
		c.Assert(addr, gc.Equals, t.expected)
	}
}

func (*localTests) TestPortsToRuleInfo(c *gc.C) {
	groupId := "groupid"
	testCases := []struct {
		about    string
		ports    []network.PortRange
		expected []nova.RuleInfo
	}{{
		about: "single port",
		ports: []network.PortRange{{
			FromPort: 80,
			ToPort:   80,
			Protocol: "tcp",
		}},
		expected: []nova.RuleInfo{{
			IPProtocol:    "tcp",
			FromPort:      80,
			ToPort:        80,
			Cidr:          "0.0.0.0/0",
			ParentGroupId: groupId,
		}},
	}, {
		about: "multiple ports",
		ports: []network.PortRange{{
			FromPort: 80,
			ToPort:   82,
			Protocol: "tcp",
		}},
		expected: []nova.RuleInfo{{
			IPProtocol:    "tcp",
			FromPort:      80,
			ToPort:        82,
			Cidr:          "0.0.0.0/0",
			ParentGroupId: groupId,
		}},
	}, {
		about: "multiple port ranges",
		ports: []network.PortRange{{
			FromPort: 80,
			ToPort:   82,
			Protocol: "tcp",
		}, {
			FromPort: 100,
			ToPort:   120,
			Protocol: "tcp",
		}},
		expected: []nova.RuleInfo{{
			IPProtocol:    "tcp",
			FromPort:      80,
			ToPort:        82,
			Cidr:          "0.0.0.0/0",
			ParentGroupId: groupId,
		}, {
			IPProtocol:    "tcp",
			FromPort:      100,
			ToPort:        120,
			Cidr:          "0.0.0.0/0",
			ParentGroupId: groupId,
		}},
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		rules := openstack.PortsToRuleInfo(groupId, t.ports)
		c.Check(len(rules), gc.Equals, len(t.expected))
		c.Check(rules, gc.DeepEquals, t.expected)
	}
}

func (*localTests) TestRuleMatchesPortRange(c *gc.C) {
	proto_tcp := "tcp"
	proto_udp := "udp"
	port_80 := 80
	port_85 := 85

	testCases := []struct {
		about    string
		ports    network.PortRange
		rule     nova.SecurityGroupRule
		expected bool
	}{{
		about: "single port",
		ports: network.PortRange{
			FromPort: 80,
			ToPort:   80,
			Protocol: "tcp",
		},
		rule: nova.SecurityGroupRule{
			IPProtocol: &proto_tcp,
			FromPort:   &port_80,
			ToPort:     &port_80,
		},
		expected: true,
	}, {
		about: "multiple port",
		ports: network.PortRange{
			FromPort: port_80,
			ToPort:   port_85,
			Protocol: proto_tcp,
		},
		rule: nova.SecurityGroupRule{
			IPProtocol: &proto_tcp,
			FromPort:   &port_80,
			ToPort:     &port_85,
		},
		expected: true,
	}, {
		about: "nil rule components",
		ports: network.PortRange{
			FromPort: port_80,
			ToPort:   port_85,
			Protocol: proto_tcp,
		},
		rule: nova.SecurityGroupRule{
			IPProtocol: nil,
			FromPort:   nil,
			ToPort:     nil,
		},
		expected: false,
	}, {
		about: "mismatched port range and rule",
		ports: network.PortRange{
			FromPort: port_80,
			ToPort:   port_85,
			Protocol: proto_tcp,
		},
		rule: nova.SecurityGroupRule{
			IPProtocol: &proto_udp,
			FromPort:   &port_80,
			ToPort:     &port_80,
		},
		expected: false,
	}}
	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		c.Check(openstack.RuleMatchesPortRange(t.rule, t.ports), gc.Equals, t.expected)
	}
}

func (t *localTests) TestPrepareSetsControlBucket(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "openstack",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err = openstack.ProviderInstance.PrepareForCreateEnvironment(cfg)
	c.Assert(err, jc.ErrorIsNil)

	bucket := cfg.UnknownAttrs()["control-bucket"]
	c.Assert(bucket, gc.Matches, "[a-f0-9]{32}")
}
