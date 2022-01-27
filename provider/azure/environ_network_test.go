// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-08-01/network"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
)

func (s *environSuite) TestSubnetsInstanceIDError(c *gc.C) {
	env := s.openEnviron(c)

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, jc.IsTrue)

	_, err := netEnv.Subnets(s.callCtx, "some-ID", nil)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *environSuite) TestSubnetsSuccess(c *gc.C) {
	env := s.openEnviron(c)

	// We wait for common resource creation, then query subnets
	// in the default virtual network created for every model.
	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", network.SubnetListResult{
			Value: &[]network.Subnet{
				{
					ID: to.StringPtr("provider-sub-id"),
					SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
						AddressPrefix: to.StringPtr("10.0.0.0/24"),
					},
				},
				{
					// Result without an address prefix is ignored.
					SubnetPropertiesFormat: &network.SubnetPropertiesFormat{},
				},
			},
		}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, jc.IsTrue)

	subs, err := netEnv.Subnets(s.callCtx, instance.UnknownId, nil)
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
		makeSender("/virtualNetworks/juju-internal-network/subnets", network.SubnetListResult{
			Value: &[]network.Subnet{
				{
					ID: to.StringPtr("subnet-42"),
					SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
						AddressPrefix: to.StringPtr("10.0.0.0/24"),
					},
				},
				{
					ID: to.StringPtr("subnet-665"),
					SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
						AddressPrefix: to.StringPtr("172.0.0.0/24"),
					},
				},
			},
		}),
		makeSender(".*/networkInterfaces", network.InterfaceListResult{
			Value: &[]network.Interface{
				{
					ID: to.StringPtr("az-nic-0"),
					InterfacePropertiesFormat: &network.InterfacePropertiesFormat{
						Primary:    to.BoolPtr(true),
						MacAddress: to.StringPtr("AA-BB-CC-DD-EE-FF"), // azure reports MACs in this format; they are normalized internally
						IPConfigurations: &[]network.InterfaceIPConfiguration{
							{
								InterfaceIPConfigurationPropertiesFormat: &network.InterfaceIPConfigurationPropertiesFormat{
									PrivateIPAddress:          to.StringPtr("10.0.0.42"),
									PrivateIPAllocationMethod: network.Dynamic,
									Subnet: &network.Subnet{
										ID: to.StringPtr("subnet-42"), // should match one of values returned by the Subnets() call
									},
									Primary: to.BoolPtr(false),
								},
							},
							{
								InterfaceIPConfigurationPropertiesFormat: &network.InterfaceIPConfigurationPropertiesFormat{
									PrivateIPAddress:          to.StringPtr("172.0.0.99"),
									PrivateIPAllocationMethod: network.Static,
									Subnet: &network.Subnet{
										ID: to.StringPtr("subnet-665"), // should match one of values returned by the Subnets() call
									},
									// This is the primary address for the NIC and should appear first in the mapped interface.
									Primary: to.BoolPtr(true),
									PublicIPAddress: &network.PublicIPAddress{
										ID: to.StringPtr("bogus"), // should be ignored
									},
								},
							},
						},
					},
					Tags: map[string]*string{
						"juju-machine-name": to.StringPtr("machine-0"),
					},
				},
				{
					ID: to.StringPtr("az-nic-1"),
					InterfacePropertiesFormat: &network.InterfacePropertiesFormat{
						Primary:    to.BoolPtr(true),
						MacAddress: to.StringPtr("BA-D0-C0-FF-EE-42"), // azure reports MACs in this format; they are normalized internally
						IPConfigurations: &[]network.InterfaceIPConfiguration{
							{
								InterfaceIPConfigurationPropertiesFormat: &network.InterfaceIPConfigurationPropertiesFormat{
									Subnet: &network.Subnet{
										ID: to.StringPtr("subnet-42"), // should match one of values returned by the Subnets() call
									},
									PublicIPAddress: &network.PublicIPAddress{
										ID: to.StringPtr("az-ip-1"),
									},
									Primary: to.BoolPtr(true),
								},
							},
						},
					},
					Tags: map[string]*string{
						"juju-machine-name": to.StringPtr("machine-0"),
					},
				},
			},
		}),
		makeSender(".*/publicIPAddresses", network.PublicIPAddressListResult{
			Value: &[]network.PublicIPAddress{
				{
					ID: to.StringPtr("az-ip-0"),
					PublicIPAddressPropertiesFormat: &network.PublicIPAddressPropertiesFormat{
						PublicIPAllocationMethod: network.Static,
						IPAddress:                to.StringPtr("20.30.40.50"),
					},
				},
				{
					ID: to.StringPtr("az-ip-1"),
					PublicIPAddressPropertiesFormat: &network.PublicIPAddressPropertiesFormat{
						PublicIPAllocationMethod: network.Dynamic,
						IPAddress:                to.StringPtr("1.2.3.4"),
					},
				},
			},
		}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, jc.IsTrue)

	res, err := netEnv.NetworkInterfaces(s.callCtx, []instance.Id{"machine-0"})
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
		makeSender("/virtualNetworks/juju-internal-network/subnets", network.SubnetListResult{
			Value: &[]network.Subnet{},
		}),
		makeSender(".*/networkInterfaces", network.InterfaceListResult{
			Value: &[]network.Interface{
				{
					ID: to.StringPtr("az-nic-0"),
					InterfacePropertiesFormat: &network.InterfacePropertiesFormat{
						Primary:    to.BoolPtr(true),
						MacAddress: to.StringPtr("AA-BB-CC-DD-EE-FF"), // azure reports MACs in this format; they are normalized internally
					},
					Tags: map[string]*string{
						"juju-machine-name": to.StringPtr("machine-0"),
					},
				},
			},
		}),
		makeSender(".*/publicIPAddresses", network.PublicIPAddressListResult{}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, jc.IsTrue)

	res, err := netEnv.NetworkInterfaces(s.callCtx, []instance.Id{"machine-0", "bogus-0"})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)

	c.Assert(res, gc.HasLen, 2)
	c.Assert(res[0], gc.HasLen, 1, gc.Commentf("expected to get 1 NIC for machine-0"))
	c.Assert(res[1], gc.IsNil, gc.Commentf("expected a nil slice for non-matched machines"))
}
