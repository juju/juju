// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
)

type defaultRouteSuite struct {
	testhelpers.IsolationSuite
}

func TestDefaultRouteSuite(t *testing.T) {
	tc.Run(t, &defaultRouteSuite{})
}

// TestMarkDefaultRouteSourceGatewayInSubnet checks that only the primary
// address whose subnet contains the gateway is marked, and not another
// primary address in a different subnet on the same device.
func (s *defaultRouteSuite) TestMarkDefaultRouteSourceGatewayInSubnet(c *tc.C) {
	// Arrange
	addrs := []network.DeviceAddress{
		defaultRouteDeviceAddress("192.168.0.5", "192.168.0.0/24", "eth0"),
		defaultRouteDeviceAddress("10.1.0.123", "10.1.0.0/24", "eth0"),
	}

	// Act
	result := network.MarkDefaultRouteSource(addrs)

	// Assert
	c.Check(addressValues(result), tc.DeepEquals, []string{
		"192.168.0.5", "10.1.0.123",
	})
	c.Check(markedValues(result), tc.DeepEquals, []string{"10.1.0.123"})
}

// TestMarkDefaultRouteSourceSkipsSecondary checks that a secondary address in
// the gateway's subnet is not marked, but the primary address is.
func (s *defaultRouteSuite) TestMarkDefaultRouteSourceSkipsSecondary(c *tc.C) {
	// Arrange
	secondary := defaultRouteDeviceAddress("10.1.0.100", "10.1.0.0/24", "eth0")
	secondary.IsSecondary = true
	addrs := []network.DeviceAddress{
		secondary,
		defaultRouteDeviceAddress("10.1.0.123", "10.1.0.0/24", "eth0"),
	}

	// Act
	result := network.MarkDefaultRouteSource(addrs)

	// Assert
	c.Check(markedValues(result), tc.DeepEquals, []string{"10.1.0.123"})
}

// TestMarkDefaultRouteSourceOnlySecondary checks that when the only address
// on the default route device is secondary, nothing is marked.
func (s *defaultRouteSuite) TestMarkDefaultRouteSourceOnlySecondary(c *tc.C) {
	// Arrange
	secondary := defaultRouteDeviceAddress("10.1.0.100", "10.1.0.0/24", "eth0")
	secondary.IsSecondary = true

	// Act
	result := network.MarkDefaultRouteSource(
		[]network.DeviceAddress{secondary})

	// Assert
	c.Check(markedValues(result), tc.HasLen, 0)
}

// TestMarkDefaultRouteSourceSkipsSecondaryOption checks that an address
// created as secondary with WithSecondary is not marked. The secondary state
// belongs to the address itself, so it must be honoured however the address
// was built.
func (s *defaultRouteSuite) TestMarkDefaultRouteSourceSkipsSecondaryOption(c *tc.C) {
	// Arrange
	secondary := network.DeviceAddress{
		SpaceAddress: network.NewSpaceAddress("10.1.0.100",
			network.WithCIDR("10.1.0.0/24"),
			network.WithScope(network.ScopeCloudLocal),
			network.WithSecondary(true),
		),
		DeviceUUID:             "eth0",
		IsDefaultGatewayDevice: true,
		GatewayAddress:         "10.1.0.1",
	}

	// Act
	result := network.MarkDefaultRouteSource(
		[]network.DeviceAddress{secondary})

	// Assert
	c.Check(markedValues(result), tc.HasLen, 0)
}

// TestMarkDefaultRouteSourceSkipsIPv6 checks that an IPv6 address on the
// default route device is not marked. There is no gateway, so every
// candidate is marked; only the IPv6 exclusion keeps the IPv6 address out.
func (s *defaultRouteSuite) TestMarkDefaultRouteSourceSkipsIPv6(c *tc.C) {
	// Arrange
	ipv6 := defaultRouteDeviceAddress("fd00::123", "fd00::/64", "eth0")
	ipv6.Type = network.IPv6Address
	ipv6.GatewayAddress = ""
	ipv4 := defaultRouteDeviceAddress("10.1.0.123", "10.1.0.0/24", "eth0")
	ipv4.GatewayAddress = ""
	addrs := []network.DeviceAddress{ipv6, ipv4}

	// Act
	result := network.MarkDefaultRouteSource(addrs)

	// Assert
	c.Check(markedValues(result), tc.DeepEquals, []string{"10.1.0.123"})
}

// TestMarkDefaultRouteSourceOnlyIPv6 checks that nothing is marked when the
// default route device has only IPv6 addresses, even though the fallback for
// a missing gateway would otherwise mark all candidates.
func (s *defaultRouteSuite) TestMarkDefaultRouteSourceOnlyIPv6(c *tc.C) {
	// Arrange
	ipv6 := defaultRouteDeviceAddress("fd00::123", "fd00::/64", "eth0")
	ipv6.Type = network.IPv6Address
	ipv6.GatewayAddress = ""

	// Act
	result := network.MarkDefaultRouteSource(
		[]network.DeviceAddress{ipv6})

	// Assert
	c.Check(markedValues(result), tc.HasLen, 0)
}

// TestMarkDefaultRouteSourceIgnoresOtherDevices checks that addresses on
// devices that do not carry the default route are never marked, including
// when they are in the gateway's subnet.
func (s *defaultRouteSuite) TestMarkDefaultRouteSourceIgnoresOtherDevices(c *tc.C) {
	// Arrange
	other := defaultRouteDeviceAddress("10.1.0.200", "10.1.0.0/24", "eth1")
	other.IsDefaultGatewayDevice = false
	other.GatewayAddress = ""
	addrs := []network.DeviceAddress{
		other,
		defaultRouteDeviceAddress("10.1.0.123", "10.1.0.0/24", "eth0"),
	}

	// Act
	result := network.MarkDefaultRouteSource(addrs)

	// Assert
	c.Check(markedValues(result), tc.DeepEquals, []string{"10.1.0.123"})
}

// TestMarkDefaultRouteSourceGatewayFallback checks that when the gateway is
// missing, junk, IPv6 or off-link, all primary IPv4 addresses on the default
// route device are marked.
func (s *defaultRouteSuite) TestMarkDefaultRouteSourceGatewayFallback(c *tc.C) {
	tests := []struct {
		about   string
		gateway string
		cidr    string
	}{
		{about: "empty gateway", gateway: "", cidr: "10.1.0.0/24"},
		{about: "nil gateway", gateway: "<nil>", cidr: "10.1.0.0/24"},
		{about: "IPv6 gateway", gateway: "fe80::1", cidr: "10.1.0.0/24"},
		{about: "off-link gateway", gateway: "10.1.0.1", cidr: "10.1.0.123/32"},
	}

	for _, test := range tests {
		// Arrange
		first := defaultRouteDeviceAddress("10.1.0.123", test.cidr, "eth0")
		first.GatewayAddress = test.gateway
		second := defaultRouteDeviceAddress("192.168.0.5", "192.168.0.0/24", "eth0")
		second.GatewayAddress = test.gateway
		// Another device never takes part.
		other := defaultRouteDeviceAddress("10.2.0.123", "10.2.0.0/24", "eth1")
		other.IsDefaultGatewayDevice = false

		// Act
		result := network.MarkDefaultRouteSource(
			[]network.DeviceAddress{first, second, other})

		// Assert
		c.Check(markedValues(result), tc.DeepEquals,
			[]string{"10.1.0.123", "192.168.0.5"}, tc.Commentf("%s", test.about))
	}
}

// TestMarkDefaultRouteSourceUnparsableCIDR checks that an address with an
// unparsable CIDR does not match the gateway, and that all primary addresses
// are marked when there is no other match.
func (s *defaultRouteSuite) TestMarkDefaultRouteSourceUnparsableCIDR(c *tc.C) {
	// Arrange
	noMatch := defaultRouteDeviceAddress("10.1.0.123", "<nil>", "eth0")
	match := defaultRouteDeviceAddress("10.1.0.124", "10.1.0.0/24", "eth0")

	// Act
	withMatch := network.MarkDefaultRouteSource(
		[]network.DeviceAddress{noMatch, match})
	withoutMatch := network.MarkDefaultRouteSource(
		[]network.DeviceAddress{noMatch})

	// Assert
	c.Check(markedValues(withMatch), tc.DeepEquals, []string{"10.1.0.124"})
	c.Check(markedValues(withoutMatch), tc.DeepEquals, []string{"10.1.0.123"})
}

// TestMarkDefaultRouteSourceMultipleDevices checks that the addresses are
// grouped by device, and that each device uses its own gateway. Two devices
// carry a default route, which can happen when the provider reports it per
// device.
func (s *defaultRouteSuite) TestMarkDefaultRouteSourceMultipleDevices(c *tc.C) {
	// Arrange
	eth0Gateway := defaultRouteDeviceAddress("10.1.0.5", "10.1.0.0/24", "eth0")
	eth0Other := defaultRouteDeviceAddress("192.168.0.5", "192.168.0.0/24", "eth0")
	eth1Gateway := defaultRouteDeviceAddress("10.2.0.5", "10.2.0.0/24", "eth1")
	eth1Gateway.GatewayAddress = "10.2.0.1"
	eth1Other := defaultRouteDeviceAddress("172.16.0.5", "172.16.0.0/24", "eth1")
	eth1Other.GatewayAddress = "10.2.0.1"
	addrs := []network.DeviceAddress{
		eth0Other, eth1Other, eth0Gateway, eth1Gateway,
	}

	// Act
	result := network.MarkDefaultRouteSource(addrs)

	// Assert
	c.Check(markedValues(result), tc.DeepEquals, []string{
		"10.1.0.5", "10.2.0.5",
	})
}

// TestMarkDefaultRouteSourceOverlappingPrefixes checks that every primary
// address whose subnet contains the gateway is marked, including when the
// subnets overlap.
func (s *defaultRouteSuite) TestMarkDefaultRouteSourceOverlappingPrefixes(c *tc.C) {
	// Arrange
	wide := defaultRouteDeviceAddress("10.0.0.5", "10.0.0.0/8", "eth0")
	narrow := defaultRouteDeviceAddress("10.1.0.5", "10.1.0.0/24", "eth0")
	other := defaultRouteDeviceAddress("192.168.0.5", "192.168.0.0/24", "eth0")
	addrs := []network.DeviceAddress{wide, other, narrow}

	// Act
	result := network.MarkDefaultRouteSource(addrs)

	// Assert
	c.Check(markedValues(result), tc.DeepEquals, []string{
		"10.0.0.5", "10.1.0.5",
	})
}

// TestMarkDefaultRouteSourceNoDefaultRouteDevice checks that nothing is
// marked when no device carries the default route, and that the output has
// the same order and values as the input.
func (s *defaultRouteSuite) TestMarkDefaultRouteSourceNoDefaultRouteDevice(c *tc.C) {
	// Arrange
	first := defaultRouteDeviceAddress("10.1.0.123", "10.1.0.0/24", "eth0")
	first.IsDefaultGatewayDevice = false
	first.GatewayAddress = ""
	second := defaultRouteDeviceAddress("10.0.0.123", "10.0.0.0/24", "eth1")
	second.IsDefaultGatewayDevice = false
	second.GatewayAddress = ""
	addrs := []network.DeviceAddress{first, second}

	// Act
	result := network.MarkDefaultRouteSource(addrs)

	// Assert
	c.Check(result, tc.DeepEquals, network.SpaceAddresses{
		first.SpaceAddress, second.SpaceAddress,
	})
}

// TestMarkDefaultRouteSourceNoAddresses checks that no addresses results in
// an empty result.
func (s *defaultRouteSuite) TestMarkDefaultRouteSourceNoAddresses(c *tc.C) {
	// Act
	result := network.MarkDefaultRouteSource(nil)

	// Assert
	c.Check(result, tc.HasLen, 0)
}

// defaultRouteDeviceAddress returns a cloud-local IPv4 device address on
// the input device. The device carries the default route, via the gateway
// 10.1.0.1.
func defaultRouteDeviceAddress(
	value, cidr, deviceUUID string,
) network.DeviceAddress {
	return network.DeviceAddress{
		SpaceAddress: network.SpaceAddress{
			SpaceID: network.AlphaSpaceId,
			Origin:  network.OriginMachine,
			MachineAddress: network.MachineAddress{
				Value:      value,
				CIDR:       cidr,
				ConfigType: network.ConfigDHCP,
				Type:       network.IPv4Address,
				Scope:      network.ScopeCloudLocal,
			},
		},
		DeviceUUID:             deviceUUID,
		IsDefaultGatewayDevice: true,
		GatewayAddress:         "10.1.0.1",
	}
}

// addressValues returns the values of all of the addresses, in order.
func addressValues(addrs network.SpaceAddresses) []string {
	values := make([]string, len(addrs))
	for i, addr := range addrs {
		values[i] = addr.Value
	}
	return values
}

// markedValues returns the values of the addresses that are marked as the
// default route source, in order.
func markedValues(addrs network.SpaceAddresses) []string {
	var values []string
	for _, addr := range addrs {
		if addr.IsDefaultRouteSource {
			values = append(values, addr.Value)
		}
	}
	return values
}
