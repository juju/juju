// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/azure/internal/azuretesting"
)

func (s *environSuite) TestSubnetsSuccessOld(c *gc.C) {
	env := s.openEnviron(c)

	// We wait for common resource creation, then query subnets
	// in the default virtual network created for every model.
	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: []*armnetwork.Subnet{
				{
					ID: to.Ptr("provider-sub-id"),
					Properties: &armnetwork.SubnetPropertiesFormat{
						AddressPrefix: to.Ptr("10.0.0.0/24"),
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
	c.Assert(ok, jc.IsTrue)

	subs, err := netEnv.Subnets(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subs, gc.HasLen, 1)
	c.Check(subs[0].ProviderId, gc.Equals, corenetwork.Id("provider-sub-id"))
	c.Check(subs[0].CIDR, gc.Equals, "10.0.0.0/24")
}

func (s *environSuite) TestSubnetsSuccessNew(c *gc.C) {
	env := s.openEnviron(c)

	// We wait for common resource creation, then query subnets
	// in the default virtual network created for every model.
	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: []*armnetwork.Subnet{
				{
					ID: to.Ptr("provider-sub-id"),
					Properties: &armnetwork.SubnetPropertiesFormat{
						AddressPrefixes: []*string{
							to.Ptr("fd00:27e8:ed0b::/64"),
							to.Ptr("10.0.0.0/24"),
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
	c.Assert(ok, jc.IsTrue)

	subs, err := netEnv.Subnets(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subs, gc.HasLen, 1)
	c.Check(subs[0].ProviderId, gc.Equals, corenetwork.Id("provider-sub-id"))
	c.Check(subs[0].CIDR, gc.Equals, "10.0.0.0/24")
}

func (s *environSuite) TestNetworkInterfacesSuccess(c *gc.C) {
	env := s.openEnviron(c)

	// We wait for common resource creation, then query subnets
	// in the default virtual network created for every model.
	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", armnetwork.SubnetListResult{
			Value: []*armnetwork.Subnet{
				{
					ID: to.Ptr("subnet-42"),
					Properties: &armnetwork.SubnetPropertiesFormat{
						AddressPrefix: to.Ptr("10.0.0.0/24"),
					},
				},
				{
					ID: to.Ptr("subnet-665"),
					Properties: &armnetwork.SubnetPropertiesFormat{
						AddressPrefix: to.Ptr("172.0.0.0/24"),
					},
				},
			},
		}),
		makeSender(".*/networkInterfaces", armnetwork.InterfaceListResult{
			Value: []*armnetwork.Interface{
				{
					ID: to.Ptr("az-nic-0"),
					Properties: &armnetwork.InterfacePropertiesFormat{
						Primary:    to.Ptr(true),
						MacAddress: to.Ptr("AA-BB-CC-DD-EE-FF"), // azure reports MACs in this format; they are normalized internally
						IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
							{
								Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
									PrivateIPAddress:          to.Ptr("10.0.0.42"),
									PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
									Subnet: &armnetwork.Subnet{
										ID: to.Ptr("subnet-42"), // should match one of values returned by the Subnets() call
									},
									Primary: to.Ptr(false),
								},
							},
							{
								Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
									PrivateIPAddress:          to.Ptr("172.0.0.99"),
									PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodStatic),
									Subnet: &armnetwork.Subnet{
										ID: to.Ptr("subnet-665"), // should match one of values returned by the Subnets() call
									},
									// This is the primary address for the NIC and should appear first in the mapped interface.
									Primary: to.Ptr(true),
									PublicIPAddress: &armnetwork.PublicIPAddress{
										ID: to.Ptr("bogus"), // should be ignored
									},
								},
							},
						},
					},
					Tags: map[string]*string{
						"juju-machine-name": to.Ptr("machine-0"),
					},
				},
				{
					ID: to.Ptr("az-nic-1"),
					Properties: &armnetwork.InterfacePropertiesFormat{
						Primary:    to.Ptr(true),
						MacAddress: to.Ptr("BA-D0-C0-FF-EE-42"), // azure reports MACs in this format; they are normalized internally
						IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
							{
								Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
									Subnet: &armnetwork.Subnet{
										ID: to.Ptr("subnet-42"), // should match one of values returned by the Subnets() call
									},
									PublicIPAddress: &armnetwork.PublicIPAddress{
										ID: to.Ptr("az-ip-1"),
									},
									Primary: to.Ptr(true),
								},
							},
						},
					},
					Tags: map[string]*string{
						"juju-machine-name": to.Ptr("machine-0"),
					},
				},
			},
		}),
		makeSender(".*/publicIPAddresses", armnetwork.PublicIPAddressListResult{
			Value: []*armnetwork.PublicIPAddress{
				{
					ID: to.Ptr("az-ip-0"),
					Properties: &armnetwork.PublicIPAddressPropertiesFormat{
						PublicIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodStatic),
						IPAddress:                to.Ptr("20.30.40.50"),
					},
				},
				{
					ID: to.Ptr("az-ip-1"),
					Properties: &armnetwork.PublicIPAddressPropertiesFormat{
						PublicIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
						IPAddress:                to.Ptr("1.2.3.4"),
					},
				},
			},
		}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, jc.IsTrue)

	res, err := netEnv.NetworkInterfaces(context.Background(), []instance.Id{"machine-0"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0], gc.HasLen, 2, gc.Commentf("expected to get 2 NICs for machine-0"))

	nic0 := res[0][0]
	c.Assert(nic0.InterfaceType, gc.Equals, corenetwork.EthernetDevice)
	c.Assert(nic0.Origin, gc.Equals, corenetwork.OriginProvider)
	c.Assert(nic0.MACAddress, gc.Equals, "aa:bb:cc:dd:ee:ff")
	c.Assert(nic0.Addresses, gc.DeepEquals, corenetwork.ProviderAddresses{
		// The primary IP address for the NIC should appear first.
		corenetwork.NewMachineAddress(
			"172.0.0.99",
			corenetwork.WithCIDR("172.0.0.0/24"),
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithConfigType(corenetwork.ConfigStatic),
		).AsProviderAddress(),
		corenetwork.NewMachineAddress(
			"10.0.0.42",
			corenetwork.WithCIDR("10.0.0.0/24"),
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress(),
	})
	c.Assert(nic0.ShadowAddresses, gc.HasLen, 0)
	c.Assert(nic0.ProviderId, gc.Equals, corenetwork.Id("az-nic-0"))
	c.Assert(nic0.ProviderSubnetId, gc.Equals, corenetwork.Id("subnet-665"), gc.Commentf("expected NIC to use the provider subnet ID for the primary NIC address"))
	c.Assert(nic0.ConfigType, gc.Equals, corenetwork.ConfigStatic, gc.Commentf("expected NIC to use the config type for the primary NIC address"))

	nic1 := res[0][1]
	c.Assert(nic1.InterfaceType, gc.Equals, corenetwork.EthernetDevice)
	c.Assert(nic1.Origin, gc.Equals, corenetwork.OriginProvider)
	c.Assert(nic1.MACAddress, gc.Equals, "ba:d0:c0:ff:ee:42")
	c.Assert(nic1.Addresses, gc.HasLen, 0)
	c.Assert(nic1.ShadowAddresses, gc.DeepEquals, corenetwork.ProviderAddresses{
		corenetwork.NewMachineAddress(
			"1.2.3.4",
			corenetwork.WithScope(corenetwork.ScopePublic),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress(),
	}, gc.Commentf("expected public address to be also included in the shadow addresses list"))
	c.Assert(nic1.ProviderId, gc.Equals, corenetwork.Id("az-nic-1"))
	c.Assert(nic1.ConfigType, gc.Equals, corenetwork.ConfigDHCP, gc.Commentf("expected NIC to use the config type for the primary NIC address"))
}

func (s *environSuite) TestNetworkInterfacesPartialMatch(c *gc.C) {
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
					ID: to.Ptr("az-nic-0"),
					Properties: &armnetwork.InterfacePropertiesFormat{
						Primary:    to.Ptr(true),
						MacAddress: to.Ptr("AA-BB-CC-DD-EE-FF"), // azure reports MACs in this format; they are normalized internally
					},
					Tags: map[string]*string{
						"juju-machine-name": to.Ptr("machine-0"),
					},
				},
			},
		}),
		makeSender(".*/publicIPAddresses", armnetwork.PublicIPAddressListResult{}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, jc.IsTrue)

	res, err := netEnv.NetworkInterfaces(context.Background(), []instance.Id{"machine-0", "bogus-0"})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)

	c.Assert(res, gc.HasLen, 2)
	c.Assert(res[0], gc.HasLen, 1, gc.Commentf("expected to get 1 NIC for machine-0"))
	c.Assert(res[1], gc.IsNil, gc.Commentf("expected a nil slice for non-matched machines"))
}
