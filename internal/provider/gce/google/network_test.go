// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"sort"

	"github.com/juju/tc"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/provider/gce/google"
)

type networkSuite struct {
	google.BaseSuite
}

var _ = tc.Suite(&networkSuite{})

func (s *networkSuite) TestNetworkSpecPath(c *tc.C) {
	spec := google.NetworkSpec{
		Name: "spam",
	}
	path := spec.Path()

	c.Check(path, tc.Equals, "global/networks/spam")
}

func (s *networkSuite) TestNetworkSpecNewInterface(c *tc.C) {
	spec := google.NetworkSpec{
		Name: "spam",
	}
	netIF := google.NewNetInterface(spec, "eggs", true)

	c.Check(netIF, tc.DeepEquals, &compute.NetworkInterface{
		Network: "global/networks/spam",
		AccessConfigs: []*compute.AccessConfig{{
			Name: "eggs",
			Type: google.NetworkAccessOneToOneNAT,
		}},
	})
}

type ByIPProtocol []*compute.FirewallAllowed

func (s ByIPProtocol) Len() int {
	return len(s)
}
func (s ByIPProtocol) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s ByIPProtocol) Less(i, j int) bool {
	return s[i].IPProtocol < s[j].IPProtocol
}

func (s *networkSuite) TestFirewallSpec(c *tc.C) {
	ports := map[string][]network.PortRange{
		"tcp":  {{FromPort: 80, ToPort: 81}, {FromPort: 8888, ToPort: 8888}},
		"udp":  {{FromPort: 1234, ToPort: 1234}},
		"icmp": {{FromPort: -1, ToPort: -1}},
	}
	fw := google.FirewallSpec("spam", "target", []string{"192.168.1.0/24", "10.0.0.0/24"}, ports)

	allowed := []*compute.FirewallAllowed{{
		IPProtocol: "icmp",
		Ports:      []string{},
	}, {
		IPProtocol: "tcp",
		Ports:      []string{"80-81", "8888"},
	}, {
		IPProtocol: "udp",
		Ports:      []string{"1234"},
	}}
	sort.Sort(ByIPProtocol(fw.Allowed))
	for i := range fw.Allowed {
		sort.Strings(fw.Allowed[i].Ports)
	}
	c.Check(fw, tc.DeepEquals, &compute.Firewall{
		Name:         "spam",
		TargetTags:   []string{"target"},
		SourceRanges: []string{"192.168.1.0/24", "10.0.0.0/24"},
		Allowed:      allowed,
	})
}

func (s *networkSuite) TestExtractAddresses(c *tc.C) {
	addresses := google.ExtractAddresses(&s.NetworkInterface)

	c.Check(addresses, tc.DeepEquals, []network.ProviderAddress{
		network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
	})
}

func (s *networkSuite) TestExtractAddressesExternal(c *tc.C) {
	s.NetworkInterface.NetworkIP = ""
	s.NetworkInterface.AccessConfigs[0].NatIP = "8.8.8.8"
	addresses := google.ExtractAddresses(&s.NetworkInterface)

	c.Check(addresses, tc.DeepEquals, []network.ProviderAddress{
		network.NewMachineAddress("8.8.8.8", network.WithScope(network.ScopePublic)).AsProviderAddress(),
	})
}

func (s *networkSuite) TestExtractAddressesEmpty(c *tc.C) {
	s.NetworkInterface.AccessConfigs = nil
	s.NetworkInterface.NetworkIP = ""
	addresses := google.ExtractAddresses(&s.NetworkInterface)

	c.Check(addresses, tc.HasLen, 0)
}
