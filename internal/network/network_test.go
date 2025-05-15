// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"context"
	"net"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/testing"
)

type InterfaceInfoSuite struct{}

var _ = tc.Suite(&InterfaceInfoSuite{})

type RouteSuite struct{}

var _ = tc.Suite(&RouteSuite{})

func checkRouteIsValid(c *tc.C, r corenetwork.Route) {
	c.Check(r.Validate(), tc.ErrorIsNil)
}

func checkRouteErrEquals(c *tc.C, r corenetwork.Route, errString string) {
	err := r.Validate()
	c.Assert(err, tc.NotNil)
	c.Check(err.Error(), tc.Equals, errString)
}

func (s *RouteSuite) TestValidIPv4(c *tc.C) {
	checkRouteIsValid(c, corenetwork.Route{
		DestinationCIDR: "0.1.2.3/24",
		GatewayIP:       "0.1.2.1",
		Metric:          20,
	})
}

func (s *RouteSuite) TestValidIPv6(c *tc.C) {
	checkRouteIsValid(c, corenetwork.Route{
		DestinationCIDR: "2001:db8:a0b:12f0::1/64",
		GatewayIP:       "2001:db8:a0b:12f0::1",
		Metric:          10,
	})
}

func (s *RouteSuite) TestInvalidMixedIP(c *tc.C) {
	checkRouteErrEquals(c, corenetwork.Route{
		DestinationCIDR: "0.1.2.3/24",
		GatewayIP:       "2001:db8::1",
		Metric:          10,
	}, "DestinationCIDR is IPv4 (0.1.2.3/24) but GatewayIP is IPv6 (2001:db8::1)")
	checkRouteErrEquals(c, corenetwork.Route{
		DestinationCIDR: "2001:db8::1/64",
		GatewayIP:       "0.1.2.1",
		Metric:          10,
	}, "DestinationCIDR is IPv6 (2001:db8::1/64) but GatewayIP is IPv4 (0.1.2.1)")
}

func (s *RouteSuite) TestInvalidNotCIDR(c *tc.C) {
	checkRouteErrEquals(c, corenetwork.Route{
		DestinationCIDR: "0.1.2.3",
		GatewayIP:       "0.1.2.1",
		Metric:          10,
	}, "DestinationCIDR not valid: invalid CIDR address: 0.1.2.3")
	checkRouteErrEquals(c, corenetwork.Route{
		DestinationCIDR: "2001:db8::2",
		GatewayIP:       "2001:db8::1",
		Metric:          10,
	}, "DestinationCIDR not valid: invalid CIDR address: 2001:db8::2")
}

func (s *RouteSuite) TestInvalidNotIP(c *tc.C) {
	checkRouteErrEquals(c, corenetwork.Route{
		DestinationCIDR: "0.1.2.3/24",
		GatewayIP:       "0.1.2.1/16",
		Metric:          10,
	}, `GatewayIP is not a valid IP address: "0.1.2.1/16"`)
	checkRouteErrEquals(c, corenetwork.Route{
		DestinationCIDR: "0.1.2.3/24",
		GatewayIP:       "",
		Metric:          10,
	}, `GatewayIP is not a valid IP address: ""`)
	checkRouteErrEquals(c, corenetwork.Route{
		DestinationCIDR: "2001:db8::2/64",
		GatewayIP:       "",
		Metric:          10,
	}, `GatewayIP is not a valid IP address: ""`)
}

func (s *RouteSuite) TestInvalidMetric(c *tc.C) {
	checkRouteErrEquals(c, corenetwork.Route{
		DestinationCIDR: "0.1.2.3/24",
		GatewayIP:       "0.1.2.1",
		Metric:          -1,
	}, `Metric is negative: -1`)
}

type NetworkSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&NetworkSuite{})

func (s *NetworkSuite) TestFilterBridgeAddresses(c *tc.C) {
	s.PatchValue(&network.AddressesForInterfaceName, func(name string) ([]string, error) {
		if name == network.DefaultLXDBridge {
			return []string{
				"10.0.4.1",
				"10.0.5.1/24",
			}, nil
		}
		c.Fatalf("unknown bridge name: %q", name)
		return nil, nil
	})

	inputAddresses := corenetwork.NewMachineAddresses([]string{
		"127.0.0.1",
		"2001:db8::1",
		"10.0.0.1",
		"10.0.4.1",      // filtered (directly from LXD bridge)
		"10.0.5.10",     // filtered (from LXD bridge, 10.0.5.1/24)
		"10.0.6.10",     // unfiltered
		"192.168.122.1", // filtered (from virbr0 bridge, 192.168.122.1)
		"192.168.123.42",
		"localhost",    // unfiltered because it isn't an IP address
		"252.16.134.1", // unfiltered Class E reserved address, used by Fan.
	}).AsProviderAddresses()
	filteredAddresses := corenetwork.NewMachineAddresses([]string{
		"127.0.0.1",
		"2001:db8::1",
		"10.0.0.1",
		"10.0.6.10",
		"192.168.122.1",
		"192.168.123.42",
		"localhost",
		"252.16.134.1",
	}).AsProviderAddresses()
	c.Assert(network.FilterBridgeAddresses(context.Background(), inputAddresses), tc.DeepEquals, filteredAddresses)
}

func checkQuoteSpaceSet(c *tc.C, expected string, spaces ...string) {
	spaceSet := set.NewStrings(spaces...)
	c.Check(network.QuoteSpaceSet(spaceSet), tc.Equals, expected)
}

func (s *NetworkSuite) TestQuoteSpaceSet(c *tc.C) {
	// Only the 'empty string' space
	checkQuoteSpaceSet(c, `""`, "")
	// No spaces
	checkQuoteSpaceSet(c, `<none>`)
	// One space
	checkQuoteSpaceSet(c, `"a"`, "a")
	// Two spaces are sorted
	checkQuoteSpaceSet(c, `"a", "b"`, "a", "b")
	checkQuoteSpaceSet(c, `"a", "b"`, "b", "a")
	// Mixed
	checkQuoteSpaceSet(c, `"", "b"`, "b", "")
}

type CIDRSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&CIDRSuite{})

func (s *CIDRSuite) TestSubnetInAnyRange(c *tc.C) {
	type test struct {
		cidrs    []string
		subnet   string
		included bool
	}

	tests := []*test{
		{
			cidrs: []string{
				"192.168.8.0/21",
				"192.168.20.0/24",
			},
			subnet:   "192.168.8.0/24",
			included: true,
		}, {
			cidrs: []string{
				"192.168.8.0/21",
				"192.168.20.0/24",
			},
			subnet:   "192.168.12.128/26",
			included: true,
		}, {
			cidrs: []string{
				"192.168.8.0/21",
				"192.168.20.0/24",
			},
			subnet:   "192.168.20.128/27",
			included: true,
		}, {
			cidrs: []string{
				"192.168.8.0/21",
				"192.168.20.0/24",
			},
			subnet:   "192.168.15.255/32",
			included: true,
		}, {
			cidrs: []string{
				"192.168.8.0/21",
				"192.168.20.0/24",
			},
			subnet:   "192.168.16.64/26",
			included: false,
		}, {
			cidrs: []string{
				"2620:0:2d0:200:0:0:0:10/116",
				"2630:0:2d0:200:0:0:0:10/120",
			},
			subnet:   "2620:0:2d0:200:0:0:0:0/124",
			included: true,
		}, {
			cidrs: []string{
				"2620:0:2d0:200:0:0:0:10/116",
				"2630:0:2d0:200:0:0:0:10/120",
			},
			subnet:   "2620:0:2d0:200:0:0:0:10/128",
			included: true,
		}, {
			cidrs: []string{
				"2620:0:2d0:200:0:0:0:10/116",
				"2630:0:2d0:200:0:0:0:10/120",
			},
			subnet:   "2620:0:2d0:200:0:0:20:10/120",
			included: false,
		},
	}

	for i, t := range tests {
		c.Logf("test %d: %v in %v?", i, t.subnet, t.cidrs)
		cidrs := make([]*net.IPNet, len(t.cidrs))
		for i, cidrStr := range t.cidrs {
			_, cidr, err := net.ParseCIDR(cidrStr)
			c.Assert(err, tc.ErrorIsNil)
			cidrs[i] = cidr
		}
		_, subnet, err := net.ParseCIDR(t.subnet)
		c.Assert(err, tc.ErrorIsNil)
		result := network.SubnetInAnyRange(cidrs, subnet)
		c.Assert(result, tc.Equals, t.included)
	}
}
