// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package iptables_test

import (
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/network/iptables"
)

type IptablesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&IptablesSuite{})

func (*IptablesSuite) TestDropCommand(c *gc.C) {
	assertRender(c,
		iptables.DropCommand{},
		"sudo iptables -I INPUT -m state --state NEW -j DROP -m comment --comment 'juju internal'",
	)
	assertRender(c,
		iptables.DropCommand{DestinationAddress: "1.2.3.4"},
		"sudo iptables -I INPUT -m state --state NEW -j DROP -m comment --comment 'juju internal' -d 1.2.3.4",
	)
	assertRender(c,
		iptables.DropCommand{Interface: "eth0"},
		"sudo iptables -I INPUT -m state --state NEW -j DROP -m comment --comment 'juju internal' -i eth0",
	)
}

func (*IptablesSuite) TestAcceptInternalPortCommand(c *gc.C) {
	assertRender(c,
		iptables.AcceptInternalCommand{},
		"sudo iptables -I INPUT -j ACCEPT -m comment --comment 'juju internal'",
	)
	assertRender(c,
		iptables.AcceptInternalCommand{
			DestinationAddress: "1.2.3.4",
			DestinationPort:    17070,
			Protocol:           "tcp",
		},
		"sudo iptables -I INPUT -j ACCEPT -m comment --comment 'juju internal' -p tcp -d 1.2.3.4 --dport 17070",
	)
}

func (*IptablesSuite) TestIngressRuleCommand(c *gc.C) {
	assertRender(c,
		iptables.IngressRuleCommand{
			Rule: network.IngressRule{
				PortRange: network.PortRange{Protocol: "icmp"},
			},
		},
		"(sudo iptables -C INPUT -j ACCEPT -p icmp --icmp-type 8 -m comment --comment 'juju ingress') || "+
			"(sudo iptables -I INPUT -j ACCEPT -p icmp --icmp-type 8 -m comment --comment 'juju ingress')",
	)

	// Same as above, but with "Delete: true". The only difference in
	// output is that "-D" is specified in place of "-I".
	assertRender(c,
		iptables.IngressRuleCommand{
			Rule: network.IngressRule{
				PortRange: network.PortRange{Protocol: "icmp"},
			},
			Delete: true,
		},
		"(sudo iptables -C INPUT -j ACCEPT -p icmp --icmp-type 8 -m comment --comment 'juju ingress') && "+
			"(sudo iptables -D INPUT -j ACCEPT -p icmp --icmp-type 8 -m comment --comment 'juju ingress')",
	)

	// If SourceCIDRs is non-empty, then the CIDRs will be
	// specified in the rule with "-s". Multiple CIDRs are
	// joined with a comma.
	assertRender(c,
		iptables.IngressRuleCommand{
			Rule: network.IngressRule{
				PortRange:   network.PortRange{Protocol: "icmp"},
				SourceCIDRs: []string{"1.2.3.4", "5.6.7.8"},
			},
		},
		"(sudo iptables -C INPUT -j ACCEPT -p icmp --icmp-type 8 -s 1.2.3.4,5.6.7.8 -m comment --comment 'juju ingress') || "+
			"(sudo iptables -I INPUT -j ACCEPT -p icmp --icmp-type 8 -s 1.2.3.4,5.6.7.8 -m comment --comment 'juju ingress')",
	)

	// UDP, single port.
	assertRender(c,
		iptables.IngressRuleCommand{
			Rule: network.IngressRule{
				PortRange: network.PortRange{
					Protocol: "udp",
					FromPort: 53,
					ToPort:   53,
				},
			},
		},
		"(sudo iptables -C INPUT -j ACCEPT -p udp --dport 53 -m comment --comment 'juju ingress') || "+
			"(sudo iptables -I INPUT -j ACCEPT -p udp --dport 53 -m comment --comment 'juju ingress')",
	)

	// TCP, port range.
	assertRender(c,
		iptables.IngressRuleCommand{
			Rule: network.IngressRule{
				PortRange: network.PortRange{
					Protocol: "tcp",
					FromPort: 6001,
					ToPort:   6007,
				},
			},
		},
		"(sudo iptables -C INPUT -j ACCEPT -p tcp -m multiport --dports 6001:6007 -m comment --comment 'juju ingress') || "+
			"(sudo iptables -I INPUT -j ACCEPT -p tcp -m multiport --dports 6001:6007 -m comment --comment 'juju ingress')",
	)
}

func (*IptablesSuite) TestParseIngressRulesEmpty(c *gc.C) {
	assertParseIngressRules(c, ``, []network.IngressRule{})
}

func (*IptablesSuite) TestParseIngressRulesGarbage(c *gc.C) {
	assertParseIngressRules(c, `a
b
ACCEPT zing
blargh

`, []network.IngressRule{})
}

func (*IptablesSuite) TestParseIngressRulesChecksComment(c *gc.C) {
	assertParseIngressRules(c, `
Chain INPUT (policy ACCEPT)
target     prot opt source               destination         
ACCEPT     tcp  --  0.0.0.0/0            0.0.0.0/0            tcp dpt:53 /* managed by lxd-bridge */
ACCEPT     tcp  --  0.0.0.0/0            0.0.0.0/0            tcp dpt:53 /* juju ingress */
ACCEPT     udp  --  0.0.0.0/0            0.0.0.0/0            udp dpt:53 /* managed by lxd-bridge */
ACCEPT     udp  --  0.0.0.0/0            0.0.0.0/0            udp dpt:67
`[1:], []network.IngressRule{{
		PortRange: network.PortRange{
			Protocol: "tcp",
			FromPort: 53,
			ToPort:   53,
		},
		SourceCIDRs: []string{"0.0.0.0/0"},
	}})
}

func (*IptablesSuite) TestParseIngressRules(c *gc.C) {
	assertParseIngressRules(c, `
Chain INPUT (policy ACCEPT)
target     prot opt source               destination         
ACCEPT     tcp  --  0.0.0.0/0            0.0.0.0/0    multiport dports 3456:3458 /* juju ingress */
ACCEPT     tcp  --  1.2.3.4/20           0.0.0.0/0    tcp dpt:12345 /* juju ingress */
ACCEPT     udp  --  1.2.3.4/20           0.0.0.0/0    udp dpt:12345 /* juju ingress */
ACCEPT     icmp --  0.0.0.0/0            0.0.0.0/0    icmptype 8 /* juju ingress */
`[1:],
		[]network.IngressRule{{
			PortRange: network.PortRange{
				Protocol: "tcp",
				FromPort: 3456,
				ToPort:   3458,
			},
			SourceCIDRs: []string{"0.0.0.0/0"},
		}, {
			PortRange: network.PortRange{
				Protocol: "tcp",
				FromPort: 12345,
				ToPort:   12345,
			},
			SourceCIDRs: []string{"1.2.3.4/20"},
		}, {
			PortRange: network.PortRange{
				Protocol: "udp",
				FromPort: 12345,
				ToPort:   12345,
			},
			SourceCIDRs: []string{"1.2.3.4/20"},
		}, {
			PortRange: network.PortRange{
				Protocol: "icmp",
				FromPort: -1,
				ToPort:   -1,
			},
			SourceCIDRs: []string{"0.0.0.0/0"},
		}},
	)
}

func assertParseIngressRules(c *gc.C, in string, expect []network.IngressRule) {
	rules, err := iptables.ParseIngressRules(strings.NewReader(in))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, jc.DeepEquals, expect)
}

type renderer interface {
	Render() string
}

func assertRender(c *gc.C, r renderer, expect string) {
	c.Assert(r.Render(), gc.Equals, expect)
}
