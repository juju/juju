// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/azure"
	"github.com/juju/juju/internal/provider/azure/internal/azuretesting"
)

func (s *environSuite) TestSubnetsSuccessOld(c *tc.C) {
	env := s.openEnviron(c)

	// We wait for common resource creation, then query subnets
	// in the default virtual network created for every model.
	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: []*armnetwork.Subnet{
				{
					ID: new("provider-sub-id"),
					Properties: &armnetwork.SubnetPropertiesFormat{
						AddressPrefix: new("10.0.0.0/24"),
					},
				},
				{
					// Result without an address prefix is ignored.
					Properties: &armnetwork.SubnetPropertiesFormat{},
				},
			},
		}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, tc.IsTrue)

	subs, err := netEnv.Subnets(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(subs, tc.HasLen, 1)
	c.Check(subs[0].ProviderId, tc.Equals, corenetwork.Id("provider-sub-id"))
	c.Check(subs[0].CIDR, tc.Equals, "10.0.0.0/24")
}

func (s *environSuite) TestSubnetsSuccessNew(c *tc.C) {
	env := s.openEnviron(c)

	// We wait for common resource creation, then query subnets
	// in the default virtual network created for every model.
	// This now tests the case where a subnet has both IPv4 and IPv6 prefixes.
	// The new behavior is to return two SubnetInfo rows (one per family).
	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: []*armnetwork.Subnet{
				{
					ID: new("provider-sub-id"),
					Properties: &armnetwork.SubnetPropertiesFormat{
						AddressPrefixes: []*string{
							new("fd00:27e8:ed0b::/64"),
							new("10.0.0.0/24"),
						},
					},
				},
				{
					// Result without an address prefix is ignored.
					Properties: &armnetwork.SubnetPropertiesFormat{},
				},
			},
		}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, tc.IsTrue)

	subs, err := netEnv.Subnets(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)

	// With AddressPrefixes containing both IPv4 and IPv6, we get two entries.
	c.Assert(subs, tc.HasLen, 2)

	// Sort by CIDR to ensure predictable order.
	if subs[0].CIDR > subs[1].CIDR {
		subs[0], subs[1] = subs[1], subs[0]
	}

	// IPv4 entry: bare ProviderId
	c.Check(subs[0].ProviderId, tc.Equals, corenetwork.Id("provider-sub-id"))
	c.Check(subs[0].CIDR, tc.Equals, "10.0.0.0/24")

	// IPv6 entry: :ipv6 suffix
	c.Check(subs[1].ProviderId, tc.Equals, corenetwork.Id("provider-sub-id:ipv6"))
	c.Check(subs[1].CIDR, tc.Equals, "fd00:27e8:ed0b::/64")
}

func (s *environSuite) TestNetworkInterfacesSuccess(c *tc.C) {
	env := s.openEnviron(c)

	// We wait for common resource creation, then query subnets
	// in the default virtual network created for every model.
	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: []*armnetwork.Subnet{
				{
					ID: new("subnet-42"),
					Properties: &armnetwork.SubnetPropertiesFormat{
						AddressPrefix: new("10.0.0.0/24"),
					},
				},
				{
					ID: new("subnet-665"),
					Properties: &armnetwork.SubnetPropertiesFormat{
						AddressPrefix: new("172.0.0.0/24"),
					},
				},
			},
		}),
		makeSender(".*/networkInterfaces", armnetwork.InterfaceListResult{
			Value: []*armnetwork.Interface{
				{
					ID: new("az-nic-0"),
					Properties: &armnetwork.InterfacePropertiesFormat{
						Primary:    new(true),
						MacAddress: new("AA-BB-CC-DD-EE-FF"), // azure reports MACs in this format; they are normalized internally
						IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
							{
								Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
									PrivateIPAddress:          new("10.0.0.42"),
									PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
									Subnet: &armnetwork.Subnet{
										ID: new("subnet-42"), // should match one of values returned by the Subnets() call
									},
									Primary: new(false),
								},
							},
							{
								Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
									PrivateIPAddress:          new("172.0.0.99"),
									PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodStatic),
									Subnet: &armnetwork.Subnet{
										ID: new("subnet-665"), // should match one of values returned by the Subnets() call
									},
									// This is the primary address for the NIC and should appear first in the mapped interface.
									Primary: new(true),
									PublicIPAddress: &armnetwork.PublicIPAddress{
										ID: new("bogus"), // should be ignored
									},
								},
							},
						},
					},
					Tags: map[string]*string{
						"juju-machine-name": new("machine-0"),
					},
				},
				{
					ID: new("az-nic-1"),
					Properties: &armnetwork.InterfacePropertiesFormat{
						Primary:    new(true),
						MacAddress: new("BA-D0-C0-FF-EE-42"), // azure reports MACs in this format; they are normalized internally
						IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
							{
								Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
									Subnet: &armnetwork.Subnet{
										ID: new("subnet-42"), // should match one of values returned by the Subnets() call
									},
									PublicIPAddress: &armnetwork.PublicIPAddress{
										ID: new("az-ip-1"),
									},
									Primary: new(true),
								},
							},
						},
					},
					Tags: map[string]*string{
						"juju-machine-name": new("machine-0"),
					},
				},
			},
		}),
		makeSender(".*/publicIPAddresses", armnetwork.PublicIPAddressListResult{
			Value: []*armnetwork.PublicIPAddress{
				{
					ID: new("az-ip-0"),
					Properties: &armnetwork.PublicIPAddressPropertiesFormat{
						PublicIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodStatic),
						IPAddress:                new("20.30.40.50"),
					},
				},
				{
					ID: new("az-ip-1"),
					Properties: &armnetwork.PublicIPAddressPropertiesFormat{
						PublicIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
						IPAddress:                new("1.2.3.4"),
					},
				},
			},
		}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, tc.IsTrue)

	res, err := netEnv.NetworkInterfaces(c.Context(), []instance.Id{"machine-0"})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(res, tc.HasLen, 1)
	c.Assert(res[0], tc.HasLen, 2, tc.Commentf("expected to get 2 NICs for machine-0"))

	nic0 := res[0][0]
	c.Assert(nic0.InterfaceType, tc.Equals, corenetwork.EthernetDevice)
	c.Assert(nic0.Origin, tc.Equals, corenetwork.OriginProvider)
	c.Assert(nic0.MACAddress, tc.Equals, "aa:bb:cc:dd:ee:ff")
	c.Assert(nic0.Addresses, tc.DeepEquals, corenetwork.ProviderAddresses{
		// The primary IP address for the NIC should appear first.
		corenetwork.NewMachineAddress(
			"172.0.0.99",
			corenetwork.WithCIDR("172.0.0.0/24"),
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithConfigType(corenetwork.ConfigStatic),
		).AsProviderAddress(
			corenetwork.WithProviderSubnetID("subnet-665"),
		),
		corenetwork.NewMachineAddress(
			"10.0.0.42",
			corenetwork.WithCIDR("10.0.0.0/24"),
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress(
			corenetwork.WithProviderSubnetID("subnet-42"),
		),
	})
	c.Assert(nic0.ShadowAddresses, tc.HasLen, 0)
	c.Assert(nic0.ProviderId, tc.Equals, corenetwork.Id("az-nic-0"))

	nic1 := res[0][1]
	c.Assert(nic1.InterfaceType, tc.Equals, corenetwork.EthernetDevice)
	c.Assert(nic1.Origin, tc.Equals, corenetwork.OriginProvider)
	c.Assert(nic1.MACAddress, tc.Equals, "ba:d0:c0:ff:ee:42")
	c.Assert(nic1.Addresses, tc.HasLen, 0)
	c.Assert(nic1.ShadowAddresses, tc.DeepEquals, corenetwork.ProviderAddresses{
		corenetwork.NewMachineAddress(
			"1.2.3.4",
			corenetwork.WithScope(corenetwork.ScopePublic),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress(),
	}, tc.Commentf("expected public address to be also included in the shadow addresses list"))
	c.Assert(nic1.ProviderId, tc.Equals, corenetwork.Id("az-nic-1"))
	c.Assert(nic1.ConfigType, tc.Equals, corenetwork.ConfigDHCP, tc.Commentf("expected NIC to use the config type for the primary NIC address"))
}

func (s *environSuite) TestNetworkInterfacesPartialMatch(c *tc.C) {
	env := s.openEnviron(c)

	// We wait for common resource creation, then query subnets
	// in the default virtual network created for every model.
	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: []*armnetwork.Subnet{},
		}),
		makeSender(".*/networkInterfaces", armnetwork.InterfaceListResult{
			Value: []*armnetwork.Interface{
				{
					ID: new("az-nic-0"),
					Properties: &armnetwork.InterfacePropertiesFormat{
						Primary:    new(true),
						MacAddress: new("AA-BB-CC-DD-EE-FF"), // azure reports MACs in this format; they are normalized internally
					},
					Tags: map[string]*string{
						"juju-machine-name": new("machine-0"),
					},
				},
			},
		}),
		makeSender(".*/publicIPAddresses", armnetwork.PublicIPAddressListResult{}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, tc.IsTrue)

	res, err := netEnv.NetworkInterfaces(c.Context(), []instance.Id{"machine-0", "bogus-0"})
	c.Assert(err, tc.Equals, environs.ErrPartialInstances)

	c.Assert(res, tc.HasLen, 2)
	c.Assert(res[0], tc.HasLen, 1, tc.Commentf("expected to get 1 NIC for machine-0"))
	c.Assert(res[1], tc.IsNil, tc.Commentf("expected a nil slice for non-matched machines"))
}

func (s *environSuite) TestNetworkTemplateResourcesDualStack(c *tc.C) {
	// networkTemplateResources should always produce a dual-stack VNet
	// and dual-stack subnets, regardless of any constraint.
	resources, deps := azure.NetworkTemplateResources("westus", s.envTags, []int{17070}, nil)

	// First resource: NSG.
	c.Assert(resources, tc.HasLen, 2)
	c.Check(resources[0].Name, tc.Equals, azure.SecurityGroupName)

	// Second resource: VNet.
	vnet := resources[1]
	c.Check(vnet.Name, tc.Equals, azure.InternalNetworkName)

	vnetProps, ok := vnet.Properties.(*armnetwork.VirtualNetworkPropertiesFormat)
	c.Assert(ok, tc.IsTrue)
	c.Assert(vnetProps.AddressSpace, tc.NotNil)

	// VNet address space must include both IPv4 prefixes and the IPv6 ULA prefix.
	addrPrefixes := vnetProps.AddressSpace.AddressPrefixes
	c.Assert(addrPrefixes, tc.HasLen, 3)
	c.Check(toValue(addrPrefixes[0]), tc.Equals, azure.InternalSubnetPrefix)
	c.Check(toValue(addrPrefixes[1]), tc.Equals, azure.VnetIPv6Prefix)
	c.Check(toValue(addrPrefixes[2]), tc.Equals, azure.ControllerSubnetPrefix)

	// Subnets: internal + controller (since apiPorts is non-empty).
	c.Assert(vnetProps.Subnets, tc.HasLen, 2)

	internalSubnet := vnetProps.Subnets[0]
	c.Assert(toValue(internalSubnet.Name), tc.Equals, azure.InternalSubnetName)
	c.Assert(internalSubnet.Properties.AddressPrefixes, tc.HasLen, 2)
	c.Check(toValue(internalSubnet.Properties.AddressPrefixes[0]), tc.Equals, azure.InternalSubnetPrefix)
	c.Check(toValue(internalSubnet.Properties.AddressPrefixes[1]), tc.Equals, azure.InternalSubnetIPv6Prefix)
	// AddressPrefix (singular) should be nil when using AddressPrefixes.
	c.Check(internalSubnet.Properties.AddressPrefix, tc.IsNil)

	controllerSubnet := vnetProps.Subnets[1]
	c.Assert(toValue(controllerSubnet.Name), tc.Equals, azure.ControllerSubnetName)
	c.Assert(controllerSubnet.Properties.AddressPrefixes, tc.HasLen, 2)
	c.Check(toValue(controllerSubnet.Properties.AddressPrefixes[0]), tc.Equals, azure.ControllerSubnetPrefix)
	c.Check(toValue(controllerSubnet.Properties.AddressPrefixes[1]), tc.Equals, azure.ControllerSubnetIPv6Prefix)

	// NSG ID is returned as a dependency.
	c.Assert(deps, tc.HasLen, 1)
}

func (s *environSuite) TestNetworkTemplateResourcesNoControllerSubnet(c *tc.C) {
	// When no API ports are given, no controller subnet should be created,
	// but the VNet and internal subnet should still be dual-stack.
	resources, _ := azure.NetworkTemplateResources("westus", s.envTags, nil, nil)

	c.Assert(resources, tc.HasLen, 2)
	vnet := resources[1]

	vnetProps, ok := vnet.Properties.(*armnetwork.VirtualNetworkPropertiesFormat)
	c.Assert(ok, tc.IsTrue)

	// VNet: internal IPv4 + IPv6 ULA only (no controller prefix).
	addrPrefixes := vnetProps.AddressSpace.AddressPrefixes
	c.Assert(addrPrefixes, tc.HasLen, 2)
	c.Check(toValue(addrPrefixes[0]), tc.Equals, azure.InternalSubnetPrefix)
	c.Check(toValue(addrPrefixes[1]), tc.Equals, azure.VnetIPv6Prefix)

	// Only one subnet (internal, no controller).
	c.Assert(vnetProps.Subnets, tc.HasLen, 1)
	c.Assert(vnetProps.Subnets[0].Properties.AddressPrefixes, tc.HasLen, 2)
	c.Check(toValue(vnetProps.Subnets[0].Properties.AddressPrefixes[1]), tc.Equals, azure.InternalSubnetIPv6Prefix)
}

// TestSubnetsDualStackSubnet tests that allSubnets() emits two SubnetInfo rows
// (one per IP family) for a dual-stack subnet, with the IPv6 one suffixed :ipv6.
func (s *environSuite) TestSubnetsDualStackSubnet(c *tc.C) {
	env := s.openEnviron(c)

	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: []*armnetwork.Subnet{
				{
					ID: new("provider-dual-stack-subnet"),
					Properties: &armnetwork.SubnetPropertiesFormat{
						AddressPrefixes: []*string{
							new("192.168.0.0/20"),
							new("fd00::/64"),
						},
					},
				},
			},
		}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, tc.IsTrue)

	subs, err := netEnv.Subnets(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)

	// Should have two entries: one IPv4, one IPv6.
	c.Assert(subs, tc.HasLen, 2)

	// Sort by CIDR to ensure predictable order.
	if subs[0].CIDR > subs[1].CIDR {
		subs[0], subs[1] = subs[1], subs[0]
	}

	// IPv4 entry: bare ProviderId.
	c.Check(subs[0].ProviderId, tc.Equals, corenetwork.Id("provider-dual-stack-subnet"))
	c.Check(subs[0].CIDR, tc.Equals, "192.168.0.0/20")

	// IPv6 entry: :ipv6 suffix.
	c.Check(subs[1].ProviderId, tc.Equals, corenetwork.Id("provider-dual-stack-subnet:ipv6"))
	c.Check(subs[1].CIDR, tc.Equals, "fd00::/64")
}

// TestSubnetsIPv4OnlySubnet tests regression: IPv4-only subnets still return
// one entry with bare ProviderId.
func (s *environSuite) TestSubnetsIPv4OnlySubnet(c *tc.C) {
	env := s.openEnviron(c)

	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: []*armnetwork.Subnet{
				{
					ID: new("provider-ipv4-only-subnet"),
					Properties: &armnetwork.SubnetPropertiesFormat{
						AddressPrefix: new("10.0.0.0/24"),
					},
				},
			},
		}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, tc.IsTrue)

	subs, err := netEnv.Subnets(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)

	// Should have one entry (IPv4 only).
	c.Assert(subs, tc.HasLen, 1)
	c.Check(subs[0].ProviderId, tc.Equals, corenetwork.Id("provider-ipv4-only-subnet"))
	c.Check(subs[0].CIDR, tc.Equals, "10.0.0.0/24")
}

// TestNetworkInterfacesDualStackNIC tests that a NIC with both IPv4 and IPv6
// IP configurations gets tagged with the correct per-family CIDR and ProviderSubnetID.
func (s *environSuite) TestNetworkInterfacesDualStackNIC(c *tc.C) {
	env := s.openEnviron(c)

	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: []*armnetwork.Subnet{
				{
					ID: new("dual-stack-subnet"),
					Properties: &armnetwork.SubnetPropertiesFormat{
						AddressPrefixes: []*string{
							new("192.168.0.0/20"),
							new("fd00::/64"),
						},
					},
				},
			},
		}),
		makeSender(".*/networkInterfaces", armnetwork.InterfaceListResult{
			Value: []*armnetwork.Interface{
				{
					ID: new("dual-stack-nic"),
					Properties: &armnetwork.InterfacePropertiesFormat{
						Primary: new(true),
						IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
							{
								Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
									PrivateIPAddress:          new("192.168.0.10"),
									PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
									PrivateIPAddressVersion:   to.Ptr(armnetwork.IPVersionIPv4),
									Subnet: &armnetwork.Subnet{
										ID: new("dual-stack-subnet"),
									},
									Primary: new(false),
								},
							},
							{
								Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
									PrivateIPAddress:          new("fd00::10"),
									PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
									PrivateIPAddressVersion:   to.Ptr(armnetwork.IPVersionIPv6),
									Subnet: &armnetwork.Subnet{
										ID: new("dual-stack-subnet"),
									},
									Primary: new(true),
								},
							},
						},
					},
					Tags: map[string]*string{
						"juju-machine-name": new("machine-0"),
					},
				},
			},
		}),
		makeSender(".*/publicIPAddresses", armnetwork.PublicIPAddressListResult{}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, tc.IsTrue)

	res, err := netEnv.NetworkInterfaces(c.Context(), []instance.Id{"machine-0"})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(res, tc.HasLen, 1)
	c.Assert(res[0], tc.HasLen, 1)

	nic := res[0][0]
	// IPv6 address should be primary (appear first).
	c.Assert(nic.Addresses, tc.HasLen, 2)

	// Check IPv6 address: should have :ipv6 suffix and correct CIDR.
	ipv6Addr := nic.Addresses[0]
	c.Check(ipv6Addr.Value, tc.Equals, "fd00::10")
	c.Check(ipv6Addr.CIDR, tc.Equals, "fd00::/64")
	c.Check(ipv6Addr.ProviderSubnetID, tc.Equals, corenetwork.Id("dual-stack-subnet:ipv6"))

	// Check IPv4 address: bare ProviderSubnetID.
	ipv4Addr := nic.Addresses[1]
	c.Check(ipv4Addr.Value, tc.Equals, "192.168.0.10")
	c.Check(ipv4Addr.CIDR, tc.Equals, "192.168.0.0/20")
	c.Check(ipv4Addr.ProviderSubnetID, tc.Equals, corenetwork.Id("dual-stack-subnet"))
}

// TestNetworkInterfacesDualStackNICLegacyModel tests backward compatibility:
// when allSubnets() returns only bare-ID IPv4 entries (simulating a legacy
// model with IPv4-only subnets), an IPv6 IP configuration gets no CIDR tag.
// This is a documented fallback behavior.
func (s *environSuite) TestNetworkInterfacesDualStackNICLegacyModel(c *tc.C) {
	env := s.openEnviron(c)

	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		// Legacy model: only bare IPv4 subnet, no :ipv6 entry.
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: []*armnetwork.Subnet{
				{
					ID: new("legacy-subnet"),
					Properties: &armnetwork.SubnetPropertiesFormat{
						AddressPrefix: new("10.0.0.0/24"),
					},
				},
			},
		}),
		makeSender(".*/networkInterfaces", armnetwork.InterfaceListResult{
			Value: []*armnetwork.Interface{
				{
					ID: new("legacy-nic"),
					Properties: &armnetwork.InterfacePropertiesFormat{
						Primary: new(true),
						IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
							{
								Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
									PrivateIPAddress:          new("10.0.0.10"),
									PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
									PrivateIPAddressVersion:   to.Ptr(armnetwork.IPVersionIPv4),
									Subnet: &armnetwork.Subnet{
										ID: new("legacy-subnet"),
									},
									Primary: new(false),
								},
							},
							{
								Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
									PrivateIPAddress:          new("fd00::10"),
									PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
									PrivateIPAddressVersion:   to.Ptr(armnetwork.IPVersionIPv6),
									Subnet: &armnetwork.Subnet{
										ID: new("legacy-subnet"),
									},
									Primary: new(true),
								},
							},
						},
					},
					Tags: map[string]*string{
						"juju-machine-name": new("machine-0"),
					},
				},
			},
		}),
		makeSender(".*/publicIPAddresses", armnetwork.PublicIPAddressListResult{}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, tc.IsTrue)

	res, err := netEnv.NetworkInterfaces(c.Context(), []instance.Id{"machine-0"})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(res, tc.HasLen, 1)
	c.Assert(res[0], tc.HasLen, 1)

	nic := res[0][0]
	c.Assert(nic.Addresses, tc.HasLen, 2)

	// IPv6 address: lookup for :ipv6 suffixed ID misses, so no CIDR tag.
	ipv6Addr := nic.Addresses[0]
	c.Check(ipv6Addr.Value, tc.Equals, "fd00::10")
	c.Check(ipv6Addr.CIDR, tc.Equals, "") // No CIDR tag (legacy fallback).
	c.Check(ipv6Addr.ProviderSubnetID, tc.Equals, corenetwork.Id("legacy-subnet:ipv6"))

	// IPv4 address: tagged as expected.
	ipv4Addr := nic.Addresses[1]
	c.Check(ipv4Addr.Value, tc.Equals, "10.0.0.10")
	c.Check(ipv4Addr.CIDR, tc.Equals, "10.0.0.0/24")
	c.Check(ipv4Addr.ProviderSubnetID, tc.Equals, corenetwork.Id("legacy-subnet"))
}

// TestStripIPFamilySuffix tests the stripIPFamilySuffix helper.
func (s *environSuite) TestStripIPFamilySuffix(c *tc.C) {
	testCases := []struct {
		input    string
		expected string
	}{
		{
			input:    "/subscriptions/abc123/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/vnet/subnets/subnet:ipv6",
			expected: "/subscriptions/abc123/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/vnet/subnets/subnet",
		},
		{
			input:    "/subscriptions/abc123/stuff:ipv4",
			expected: "/subscriptions/abc123/stuff",
		},
		{
			input:    "/subscriptions/abc123/stuff:other",
			expected: "/subscriptions/abc123/stuff:other", // :other is not a valid suffix, left alone.
		},
		{
			input:    "plain-id",
			expected: "plain-id", // No colon, unchanged.
		},
		{
			input:    "",
			expected: "", // Empty string, unchanged.
		},
	}

	for _, test := range testCases {
		got := azure.StripIPFamilySuffix(test.input)
		c.Check(got, tc.Equals, test.expected, tc.Commentf("stripIPFamilySuffix(%q)", test.input))
	}
}

// TestSubnetProviderIDForFamily tests the subnetProviderIDForFamily helper.
func (s *environSuite) TestSubnetProviderIDForFamily(c *tc.C) {
	testCases := []struct {
		azureID  string
		isIPv6   bool
		expected corenetwork.Id
	}{
		{
			azureID:  "/subscriptions/abc/providers/Microsoft.Network/virtualNetworks/vnet/subnets/subnet",
			isIPv6:   false,
			expected: corenetwork.Id("/subscriptions/abc/providers/Microsoft.Network/virtualNetworks/vnet/subnets/subnet"),
		},
		{
			azureID:  "/subscriptions/abc/providers/Microsoft.Network/virtualNetworks/vnet/subnets/subnet",
			isIPv6:   true,
			expected: corenetwork.Id("/subscriptions/abc/providers/Microsoft.Network/virtualNetworks/vnet/subnets/subnet:ipv6"),
		},
	}

	for _, test := range testCases {
		got := azure.SubnetProviderIDForFamily(test.azureID, test.isIPv6)
		c.Check(got, tc.Equals, test.expected, tc.Commentf("SubnetProviderIDForFamily(%q, %v)", test.azureID, test.isIPv6))
	}
}

// TestDeduplicateSubnets tests the deduplicateSubnets helper.
// It verifies that :ipv6-suffixed IDs produce wantIPv6=true,
// bare IDs produce wantIPv6=false, and duplicates are collapsed
// with the OR of their wantIPv6 flags.
func (s *environSuite) TestDeduplicateSubnets(c *tc.C) {
	type sel struct {
		id       string
		wantIPv6 bool
	}
	testCases := []struct {
		input    []corenetwork.Id
		expected []sel
	}{
		{
			input: []corenetwork.Id{
				"subnet-1",
				"subnet-1:ipv6",
				"subnet-2",
			},
			expected: []sel{
				{"subnet-1", true},
				{"subnet-2", false},
			},
		},
		{
			input: []corenetwork.Id{
				"subnet-1:ipv6",
				"subnet-1",
			},
			expected: []sel{
				{"subnet-1", true},
			},
		},
		{
			input: []corenetwork.Id{
				"subnet-1",
				"subnet-2",
			},
			expected: []sel{
				{"subnet-1", false},
				{"subnet-2", false},
			},
		},
		{
			input:    []corenetwork.Id{},
			expected: []sel{},
		},
		{
			input: []corenetwork.Id{
				"subnet-1:ipv6",
				"subnet-2:ipv6",
			},
			expected: []sel{
				{"subnet-1", true},
				{"subnet-2", true},
			},
		},
	}

	for _, test := range testCases {
		got := azure.DeduplicateSubnets(test.input)
		want := make([]azure.SubnetSelection, len(test.expected))
		for i, s := range test.expected {
			want[i] = azure.SubnetSelection{ID: corenetwork.Id(s.id), WantIPv6: s.wantIPv6}
		}
		c.Check(got, tc.DeepEquals, want, tc.Commentf("DeduplicateSubnets(%v)", test.input))
	}
}

func (s *environSuite) TestFindSubnetByIDSuccess(c *tc.C) {
	env := s.openEnviron(c)
	subnets := []*armnetwork.Subnet{{
		ID:   new("/path/to/subnet1"),
		Name: new("subnet1"),
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefix: new("192.168.0.0/20"),
		},
	}, {
		ID:   new("/path/to/subnet2"),
		Name: new("subnet2"),
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefixes: []*string{new("10.0.0.0/24"), new("fd00::/64")},
		},
	}}
	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: subnets,
		}),
	}

	res, err := azure.FindSubnetByID(c.Context(), env, "/path/to/subnet2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)
	c.Assert(toValue(res.Name), tc.Equals, "subnet2")
}

func (s *environSuite) TestFindSubnetByIDSuccessSuffix(c *tc.C) {
	env := s.openEnviron(c)
	subnets := []*armnetwork.Subnet{{
		ID:   new("/path/to/subnet1"),
		Name: new("subnet1"),
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefixes: []*string{new("192.168.0.0/20"), new("fd00::/64")},
		},
	}}
	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: subnets,
		}),
	}

	// Look up using a Juju-suffixed ID, should still resolve to subnet1.
	res, err := azure.FindSubnetByID(c.Context(), env, "/path/to/subnet1:ipv6")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.NotNil)
	c.Assert(toValue(res.Name), tc.Equals, "subnet1")
}

func (s *environSuite) TestFindSubnetByIDNotFound(c *tc.C) {
	env := s.openEnviron(c)
	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: []*armnetwork.Subnet{},
		}),
	}

	res, err := azure.FindSubnetByID(c.Context(), env, "/path/to/subnet1")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(res, tc.IsNil)
}
