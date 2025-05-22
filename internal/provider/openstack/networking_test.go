// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	stdtesting "testing"

	"github.com/go-goose/goose/v5/neutron"
	"github.com/go-goose/goose/v5/nova"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/testhelpers"
)

type networkingSuite struct {
	testhelpers.IsolationSuite

	base    *MockNetworkingBase
	neutron *MockNetworkingNeutron
	nova    *MockNetworkingNova
	client  *MockNetworkingAuthenticatingClient
	ecfg    *MockNetworkingEnvironConfig

	serverAZ        string
	externalNetwork string
	ip              string
	ip2             string
	ip3             string
}

func TestNetworkingSuite(t *stdtesting.T) {
	tc.Run(t, &networkingSuite{})
}

func (s *networkingSuite) SetUpTest(c *tc.C) {
	s.serverAZ = "test-me"
	s.externalNetwork = "ext-net"
	s.ip = "10.4.5.6"
	s.ip2 = "10.4.5.42"
	s.ip3 = "10.4.5.75"
}

func (s *networkingSuite) TestAllocatePublicIPConfiguredExternalNetwork(c *tc.C) {
	// Get a FIP for an instance with a configured external-network,
	// which has available FIPs. Other external networks do exist -
	// at last 1 in the same AZ as the instance. Should get the FIP
	// on the configured external-network.
	defer s.setupMocks(c).Finish()
	s.expectExternalNetwork()
	s.expectListFloatingIPsV2FromConfig()
	s.expectListExternalNetworksV2() // resolveNeutronNetwork()
	s.expectListInternalNetworksV2()
	s.expectListExternalNetworksV2() // getExternalNeutronNetworksByAZ()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fip, tc.NotNil)
	c.Assert(*fip, tc.Equals, s.ip)
}

func (s *networkingSuite) TestAllocatePublicIPUnconfiguredExternalNetwork(c *tc.C) {
	// Get a FIP for an instance with an external network in the same AZ
	// having an available FIP.  The first external network in the list
	// does not have an available FIP.  No configured external-networks.
	defer s.setupMocks(c).Finish()
	s.externalNetwork = ""
	s.expectExternalNetwork()
	s.expectListFloatingIPsV2NotFromConfig()
	s.expectListInternalNetworksV2()
	s.expectListExternalNetworksV2() // getExternalNeutronNetworksByAZ()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fip, tc.NotNil)
	c.Assert(*fip, tc.Equals, s.ip2)
}

func (s *networkingSuite) TestAllocatePublicIPUnconfiguredExternalNetworkMultiAZ(c *tc.C) {
	// Get a FIP for an instance with an external network in the same AZ
	// having an available FIP. This external network exists in multiple
	// AZ, the one we want is not first in the list. The first external
	// network in the list does not have an available FIP.  No configured
	// external-networks.
	defer s.setupMocks(c).Finish()
	s.externalNetwork = ""
	s.expectExternalNetwork()
	s.expectListFloatingIPsV2NotFromConfig()
	s.expectListInternalNetworksV2()
	s.expectListExternalNetworksV2MultiAZ() // getExternalNeutronNetworksByAZ()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fip, tc.NotNil)
	c.Assert(*fip, tc.Equals, s.ip2)
}

func (s *networkingSuite) TestAllocatePublicIPFail(c *tc.C) {
	// Find external-networks, but none have an available FIP, nor
	// are they able to create one.
	defer s.setupMocks(c).Finish()
	s.expectExternalNetwork()
	s.expectListFloatingIPsV2Empty()
	s.expectListExternalNetworksV2() // resolveNeutronNetwork()
	s.expectListInternalNetworksV2()
	s.expectListExternalNetworksV2() // getExternalNeutronNetworksByAZ()
	s.expectAllocateFloatingIPV2FailAll()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(fip, tc.IsNil)
}

func (s *networkingSuite) TestAllocatePublicIPEmtpyAZEqualEmptyString(c *tc.C) {
	// Test for lp: 1891227 fix.  An empty slice for AZ should be
	// treated as an empty string AZ.
	s.serverAZ = ""
	defer s.setupMocks(c).Finish()
	s.externalNetwork = ""
	s.expectListExternalNetworksV2() // resolveNeutronNetwork()
	s.expectExternalNetwork()
	s.neutron.EXPECT().ListNetworksV2(gomock.Any()).Return([]neutron.NetworkV2{
		{
			Id:   "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Name: "test-me",
		},
	}, nil).AnyTimes()
	s.expectListFloatingIPsV2NotFromConfig()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fip, tc.NotNil)
	c.Assert(*fip, tc.Equals, s.ip2)
}

func (s *networkingSuite) TestAllocatePublicIPNoneAvailable(c *tc.C) {
	// Get a FIP for an instance with an external network in the same AZ
	// having an available FIP.  No FIPs are available in the configured
	// external network, so allocate one.  The first network fails to
	// allocate, the 2nd succeeds.
	defer s.setupMocks(c).Finish()
	s.expectExternalNetwork()
	s.expectListFloatingIPsV2FromConfigInUse()
	s.expectListExternalNetworksV2() // resolveNeutronNetwork()
	s.expectListInternalNetworksV2() // findNetworkAZForHostAddrs()
	s.expectListExternalNetworksV2() // getExternalNeutronNetworksByAZ()
	s.expectAllocateFloatingIPV2()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fip, tc.NotNil)
	c.Assert(*fip, tc.Equals, s.ip3)
}

func (s *networkingSuite) TestAllocatePublicIPFailNoNetworkInAZ(c *tc.C) {
	// No external network in same AZ as the instance is found, no
	// external network is configured.
	defer s.setupMocks(c).Finish()
	s.externalNetwork = ""
	s.expectExternalNetwork()
	s.expectListInternalNetworksV2()        // findNetworkAZForHostAddrs()
	s.expectListExternalNetworksV2NotInAZ() // getExternalNeutronNetworksByAZ()

	fip, err := s.runAllocatePublicIP()
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(fip, tc.IsNil)
}

func (s *networkingSuite) TestNetworkInterfaces(c *tc.C) {
	defer s.expectNeutronCalls(c).Finish()
	s.externalNetwork = ""
	s.expectListSubnets()

	s.neutron.EXPECT().ListFloatingIPsV2(gomock.Any()).Return([]neutron.FloatingIPV2{
		{
			FixedIP: "10.0.0.2",
			IP:      "10.245.164.31",
		},
	}, nil)

	s.neutron.EXPECT().ListPortsV2(gomock.Any()).Return([]neutron.PortV2{
		{
			DeviceId:    "another-instance",
			DeviceOwner: "compute:nova",
		},
		{
			Id:          "nic-0",
			DeviceId:    "inst-0",
			NetworkId:   "deadbeef-0bad-400d-8000-4b1ddbeefbeef",
			DeviceOwner: "compute:nova",
			MACAddress:  "aa:bb:cc:dd:ee:ff",
			Status:      "ACTIVE",
			FixedIPs: []neutron.PortFixedIPsV2{
				{
					IPAddress: "192.168.0.2",
					SubnetID:  "sub-42",
				},
				{
					IPAddress: "10.0.0.2",
					SubnetID:  "sub-665",
				},
			},
		},
		{
			Id:          "nic-1",
			DeviceId:    "inst-0",
			NetworkId:   "deadbeef-0bad-400d-8000-4b1ddbeefbeef",
			DeviceOwner: "compute:nova",
			MACAddress:  "10:20:30:40:50:60",
			Status:      "N/A",
			FixedIPs: []neutron.PortFixedIPsV2{
				{
					IPAddress: "192.168.0.42",
					SubnetID:  "sub-42",
				},
			},
		},
	}, nil)

	nn := &NeutronNetworking{NetworkingBase: s.base}

	res, err := nn.NetworkInterfaces([]instance.Id{"inst-0"})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(res, tc.HasLen, 1)
	c.Assert(res[0], tc.HasLen, 2, tc.Commentf("expected to get 2 NICs for machine-0"))

	nic0 := res[0][0]
	c.Assert(nic0.InterfaceType, tc.Equals, network.EthernetDevice)
	c.Assert(nic0.Origin, tc.Equals, network.OriginProvider)
	c.Assert(nic0.Disabled, tc.IsFalse)
	c.Assert(nic0.MACAddress, tc.Equals, "aa:bb:cc:dd:ee:ff")
	c.Assert(nic0.Addresses, tc.DeepEquals, network.ProviderAddresses{
		network.NewMachineAddress(
			"192.168.0.2",
			network.WithCIDR("192.168.0.0/24"),
			network.WithScope(network.ScopeCloudLocal),
			network.WithConfigType(network.ConfigStatic),
		).AsProviderAddress(),
		network.NewMachineAddress(
			"10.0.0.2",
			network.WithCIDR("10.0.0.0/24"),
			network.WithScope(network.ScopeCloudLocal),
			network.WithConfigType(network.ConfigStatic),
		).AsProviderAddress(),
	})
	c.Assert(nic0.ShadowAddresses, tc.DeepEquals, network.ProviderAddresses{
		network.NewMachineAddress(
			"10.245.164.31",
			network.WithScope(network.ScopePublic),
		).AsProviderAddress(),
	})
	c.Assert(nic0.ProviderId, tc.Equals, network.Id("nic-0"))
	c.Assert(nic0.ProviderSubnetId, tc.Equals, network.Id("sub-42"), tc.Commentf("expected NIC to use the provider subnet ID for the primary NIC address"))

	nic1 := res[0][1]
	c.Assert(nic1.InterfaceType, tc.Equals, network.EthernetDevice)
	c.Assert(nic1.Origin, tc.Equals, network.OriginProvider)
	c.Assert(nic1.Disabled, tc.IsTrue, tc.Commentf("expected device to be listed as disabled"))
	c.Assert(nic1.MACAddress, tc.Equals, "10:20:30:40:50:60")
	c.Assert(nic1.Addresses, tc.DeepEquals, network.ProviderAddresses{
		network.NewMachineAddress(
			"192.168.0.42",
			network.WithCIDR("192.168.0.0/24"),
			network.WithScope(network.ScopeCloudLocal),
			network.WithConfigType(network.ConfigStatic),
		).AsProviderAddress(),
	})
	c.Assert(nic1.ProviderId, tc.Equals, network.Id("nic-1"))
	c.Assert(nic1.ProviderSubnetId, tc.Equals, network.Id("sub-42"), tc.Commentf("expected NIC to use the provider subnet ID for the primary NIC address"))
}

func (s *networkingSuite) TestNetworkInterfacesPartialMatch(c *tc.C) {
	defer s.expectNeutronCalls(c).Finish()
	s.externalNetwork = ""
	s.expectListSubnets()

	s.neutron.EXPECT().ListFloatingIPsV2(gomock.Any()).Return(nil, nil)

	s.neutron.EXPECT().ListPortsV2(gomock.Any()).Return([]neutron.PortV2{
		{
			Id:          "nic-0",
			DeviceId:    "inst-0",
			NetworkId:   "deadbeef-0bad-400d-8000-4b1ddbeefbeef",
			DeviceOwner: "compute:nova",
			MACAddress:  "aa:bb:cc:dd:ee:ff",
			Status:      "ACTIVE",
		},
	}, nil)

	nn := &NeutronNetworking{NetworkingBase: s.base}

	res, err := nn.NetworkInterfaces([]instance.Id{"inst-0", "bogus-0"})
	c.Assert(err, tc.Equals, environs.ErrPartialInstances)

	c.Assert(res, tc.HasLen, 2)
	c.Assert(res[0], tc.HasLen, 1, tc.Commentf("expected to get 1 NIC for inst-0"))
	c.Assert(res[1], tc.IsNil, tc.Commentf("expected a nil slice for non-matched machines"))
}

func (s *networkingSuite) expectNeutronCalls(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.client = NewMockNetworkingAuthenticatingClient(ctrl)
	s.client.EXPECT().TenantId().Return("TenantId").AnyTimes()

	s.neutron = NewMockNetworkingNeutron(ctrl)

	s.ecfg = NewMockNetworkingEnvironConfig(ctrl)

	s.base = NewMockNetworkingBase(ctrl)
	bExp := s.base.EXPECT()
	bExp.neutron().Return(s.neutron).AnyTimes()
	bExp.client().Return(s.client).AnyTimes()
	bExp.ecfg().Return(s.ecfg).AnyTimes()

	return ctrl
}

func (s *networkingSuite) expectListSubnets() {
	s.ecfg.EXPECT().networks().Return([]string{"int-net"})

	s.expectExternalNetwork()
	s.neutron.EXPECT().ListNetworksV2(gomock.Any()).Return([]neutron.NetworkV2{
		{
			Id:                "deadbeef-0bad-400d-8000-4b1ddbeefbeef",
			Name:              "int-net",
			AvailabilityZones: []string{s.serverAZ},
		},
	}, nil)
	s.neutron.EXPECT().ListSubnetsV2().Return([]neutron.SubnetV2{
		{
			Id:        "sub-42",
			NetworkId: "deadbeef-0bad-400d-8000-4b1ddbeefbeef",
			Cidr:      "192.168.0.0/24",
		},
		{
			Id:        "sub-665",
			NetworkId: "deadbeef-0bad-400d-8000-4b1ddbeefbeef",
			Cidr:      "10.0.0.0/24",
		},
	}, nil)
	s.neutron.EXPECT().GetNetworkV2("deadbeef-0bad-400d-8000-4b1ddbeefbeef").Return(&neutron.NetworkV2{
		AvailabilityZones: []string{"mars"},
	}, nil).AnyTimes()
}

func (s *networkingSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.neutron = NewMockNetworkingNeutron(ctrl)
	s.nova = NewMockNetworkingNova(ctrl)
	s.client = NewMockNetworkingAuthenticatingClient(ctrl)
	s.ecfg = NewMockNetworkingEnvironConfig(ctrl)

	s.base = NewMockNetworkingBase(ctrl)
	bExp := s.base.EXPECT()
	bExp.client().Return(s.client).AnyTimes()
	bExp.neutron().Return(s.neutron).AnyTimes()
	bExp.nova().Return(s.nova)
	bExp.ecfg().Return(s.ecfg)

	s.client.EXPECT().TenantId().Return("TenantId").AnyTimes()
	s.nova.EXPECT().GetServer(gomock.Any()).Return(&nova.ServerDetail{
		Addresses: map[string][]nova.IPAddress{
			"int-net": {},
		},
		AvailabilityZone: s.serverAZ,
	}, nil)

	return ctrl
}

func (s *networkingSuite) runAllocatePublicIP() (*string, error) {
	networking := &NeutronNetworking{NetworkingBase: s.base}
	return networking.AllocatePublicIP(instance.Id("32"))
}

func (s *networkingSuite) expectListFloatingIPsV2FromConfig() {
	s.neutron.EXPECT().ListFloatingIPsV2(gomock.Any()).Return([]neutron.FloatingIPV2{
		{FloatingNetworkId: "deadbeef-0bad-400d-8000-4b1ddeadbeef", IP: s.ip},
	}, nil)
}

func (s *networkingSuite) expectListFloatingIPsV2FromConfigInUse() {
	s.neutron.EXPECT().ListFloatingIPsV2(gomock.Any()).Return([]neutron.FloatingIPV2{{
		FloatingNetworkId: "deadbeef-0bad-400d-8000-4b1ddeadbeef",
		FixedIP:           "10.7.8.9",
		IP:                s.ip,
	}}, nil)
}

func (s *networkingSuite) expectListFloatingIPsV2NotFromConfig() {
	s.neutron.EXPECT().ListFloatingIPsV2(gomock.Any()).Return([]neutron.FloatingIPV2{
		{FloatingNetworkId: "deadbeef-0bad-400d-8000-4b1d0d06f00d", IP: s.ip2},
	}, nil)
}

func (s *networkingSuite) expectListFloatingIPsV2Empty() {
	s.neutron.EXPECT().ListFloatingIPsV2(gomock.Any()).Return([]neutron.FloatingIPV2{}, nil)
}

func (s *networkingSuite) expectExternalNetwork() {
	s.ecfg.EXPECT().externalNetwork().Return(s.externalNetwork)
}

func (s *networkingSuite) expectListExternalNetworksV2() {
	s.neutron.EXPECT().ListNetworksV2(gomock.Any()).Return([]neutron.NetworkV2{
		{
			Id:                "deadbeef-0bad-400d-8000-4b1ddeadbeef",
			Name:              s.externalNetwork,
			External:          true,
			AvailabilityZones: []string{s.serverAZ},
		}, {
			Name:              "do-not-pick-me",
			External:          true,
			AvailabilityZones: []string{"failme"},
		}, {
			Id:                "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Name:              "unconfigured-ext-net",
			External:          true,
			AvailabilityZones: []string{s.serverAZ},
		},
	}, nil)
}

func (s *networkingSuite) expectListInternalNetworksV2() {
	s.neutron.EXPECT().ListNetworksV2(gomock.Any()).Return([]neutron.NetworkV2{
		{
			Id:                "deadbeef-0bad-400d-8000-4b1ddbeefbeef",
			Name:              "int-net",
			AvailabilityZones: []string{s.serverAZ},
		}, {
			Name:              "internal-do-not-pick-me",
			AvailabilityZones: []string{"failme"},
		}, {
			Id:                "deadbeef-0bad-400d-8000-4b1d8273450d",
			Name:              "unconfigured-int-net",
			AvailabilityZones: []string{s.serverAZ},
		},
	}, nil)
}

func (s *networkingSuite) expectListExternalNetworksV2MultiAZ() {
	s.neutron.EXPECT().ListNetworksV2(gomock.Any()).Return([]neutron.NetworkV2{
		{
			Name:              "do-not-pick-me",
			AvailabilityZones: []string{"failme"},
		}, {
			Id:                "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Name:              "unconfigured-ext-net",
			AvailabilityZones: []string{"other", s.serverAZ},
		},
	}, nil).AnyTimes()
}

func (s *networkingSuite) expectListExternalNetworksV2NotInAZ() {
	s.neutron.EXPECT().ListNetworksV2(gomock.Any()).Return([]neutron.NetworkV2{
		{
			Name:              "do-not-pick-me",
			AvailabilityZones: []string{"failme"},
		}, {
			Id:                "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Name:              "unconfigured-ext-net",
			AvailabilityZones: []string{"other"},
		},
	}, nil).AnyTimes()
}

func (s *networkingSuite) expectAllocateFloatingIPV2() {
	s.neutron.EXPECT().AllocateFloatingIPV2("deadbeef-0bad-400d-8000-4b1ddeadbeef").Return(nil, errors.NotFoundf("fip"))
	s.neutron.EXPECT().AllocateFloatingIPV2("deadbeef-0bad-400d-8000-4b1d0d06f00d").Return(&neutron.FloatingIPV2{
		FloatingNetworkId: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		IP:                s.ip3,
	}, nil)
}

func (s *networkingSuite) expectAllocateFloatingIPV2FailAll() {
	s.neutron.EXPECT().AllocateFloatingIPV2(gomock.Any()).Return(nil, errors.NotFoundf("fip")).AnyTimes()
}
