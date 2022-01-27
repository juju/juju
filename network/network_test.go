// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"io/ioutil"
	"net"
	"path/filepath"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type InterfaceInfoSuite struct {
	info corenetwork.InterfaceInfos
}

var _ = gc.Suite(&InterfaceInfoSuite{})

type RouteSuite struct{}

var _ = gc.Suite(&RouteSuite{})

func checkRouteIsValid(c *gc.C, r corenetwork.Route) {
	c.Check(r.Validate(), jc.ErrorIsNil)
}

func checkRouteErrEquals(c *gc.C, r corenetwork.Route, errString string) {
	err := r.Validate()
	c.Assert(err, gc.NotNil)
	c.Check(err.Error(), gc.Equals, errString)
}

func (s *RouteSuite) TestValidIPv4(c *gc.C) {
	checkRouteIsValid(c, corenetwork.Route{
		DestinationCIDR: "0.1.2.3/24",
		GatewayIP:       "0.1.2.1",
		Metric:          20,
	})
}

func (s *RouteSuite) TestValidIPv6(c *gc.C) {
	checkRouteIsValid(c, corenetwork.Route{
		DestinationCIDR: "2001:db8:a0b:12f0::1/64",
		GatewayIP:       "2001:db8:a0b:12f0::1",
		Metric:          10,
	})
}

func (s *RouteSuite) TestInvalidMixedIP(c *gc.C) {
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

func (s *RouteSuite) TestInvalidNotCIDR(c *gc.C) {
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

func (s *RouteSuite) TestInvalidNotIP(c *gc.C) {
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

func (s *RouteSuite) TestInvalidMetric(c *gc.C) {
	checkRouteErrEquals(c, corenetwork.Route{
		DestinationCIDR: "0.1.2.3/24",
		GatewayIP:       "0.1.2.1",
		Metric:          -1,
	}, `Metric is negative: -1`)
}

type NetworkSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&NetworkSuite{})

func (s *NetworkSuite) TestFilterBridgeAddresses(c *gc.C) {
	lxcFakeNetConfig := filepath.Join(c.MkDir(), "lxc-net")
	// We create an LXC bridge named "foobar", and then put 10.0.3.1,
	// 10.0.3.4 and 10.0.3.5/24 on that bridge.
	// We also put 10.0.4.1 and 10.0.5.1/24 onto whatever bridge LXD is
	// configured to use.
	// And 192.168.122.1 on virbr0
	netConf := []byte(`
  # comments ignored
LXC_BR= ignored
LXC_ADDR = "fooo"
 LXC_BRIDGE = " foobar " # detected, spaces stripped
anything else ignored
LXC_BRIDGE="ignored"`[1:])
	err := ioutil.WriteFile(lxcFakeNetConfig, netConf, 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&network.InterfaceByNameAddrs, func(name string) ([]net.Addr, error) {
		if name == "foobar" {
			return []net.Addr{
				&net.IPAddr{IP: net.IPv4(10, 0, 3, 1)},
				&net.IPAddr{IP: net.IPv4(10, 0, 3, 4)},
				// Try a CIDR 10.0.3.5/24 as well.
				&net.IPNet{IP: net.IPv4(10, 0, 3, 5), Mask: net.IPv4Mask(255, 255, 255, 0)},
			}, nil
		} else if name == network.DefaultLXDBridge {
			return []net.Addr{
				&net.IPAddr{IP: net.IPv4(10, 0, 4, 1)},
				// Try a CIDR 10.0.5.1/24 as well.
				&net.IPNet{IP: net.IPv4(10, 0, 5, 1), Mask: net.IPv4Mask(255, 255, 255, 0)},
			}, nil
		} else if name == network.DefaultKVMBridge {
			return []net.Addr{
				&net.IPAddr{IP: net.IPv4(192, 168, 122, 1)},
			}, nil
		}
		c.Fatalf("unknown bridge name: %q", name)
		return nil, nil
	})
	s.PatchValue(&network.LXCNetDefaultConfig, lxcFakeNetConfig)

	inputAddresses := corenetwork.NewMachineAddresses([]string{
		"127.0.0.1",
		"2001:db8::1",
		"10.0.0.1",
		"10.0.3.1",      // filtered (directly as IP)
		"10.0.3.3",      // filtered (by the 10.0.3.5/24 CIDR)
		"10.0.3.5",      // filtered (directly)
		"10.0.3.4",      // filtered (directly)
		"10.0.4.1",      // filtered (directly from LXD bridge)
		"10.0.5.10",     // filtered (from LXD bridge, 10.0.5.1/24)
		"10.0.6.10",     // unfiltered
		"192.168.122.1", // filtered (from virbr0 bridge, 192.168.122.1)
		"192.168.123.42",
		"localhost", // unfiltered because it isn't an IP address
	}).AsProviderAddresses()
	filteredAddresses := corenetwork.NewMachineAddresses([]string{
		"127.0.0.1",
		"2001:db8::1",
		"10.0.0.1",
		"10.0.6.10",
		"192.168.123.42",
		"localhost",
	}).AsProviderAddresses()
	c.Assert(network.FilterBridgeAddresses(inputAddresses), jc.DeepEquals, filteredAddresses)
}

func checkQuoteSpaceSet(c *gc.C, expected string, spaces ...string) {
	spaceSet := set.NewStrings(spaces...)
	c.Check(network.QuoteSpaceSet(spaceSet), gc.Equals, expected)
}

func (s *NetworkSuite) TestQuoteSpaceSet(c *gc.C) {
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

var _ = gc.Suite(&CIDRSuite{})

func (s *CIDRSuite) TestSubnetInAnyRange(c *gc.C) {
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
			c.Assert(err, jc.ErrorIsNil)
			cidrs[i] = cidr
		}
		_, subnet, err := net.ParseCIDR(t.subnet)
		c.Assert(err, jc.ErrorIsNil)
		result := network.SubnetInAnyRange(cidrs, subnet)
		c.Assert(result, gc.Equals, t.included)
	}
}
