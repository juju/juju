// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/gce"
)

type environNetSuite struct {
	gce.BaseSuite

	zones     []*computepb.Zone
	instances []*computepb.Instance
	networks  []*computepb.Network
	subnets   []*computepb.Subnetwork
}

func TestEnvironNetSuite(t *testing.T) {
	tc.Run(t, &environNetSuite{})
}

func (s *environNetSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.zones = []*computepb.Zone{{
		Name:   ptr("home-zone"),
		Status: ptr("UP"),
	}, {
		Name:   ptr("away-zone"),
		Status: ptr("UP"),
	}}
	s.instances = []*computepb.Instance{{
		Name: ptr("inst-0"),
		Zone: ptr("home-zone"),
		NetworkInterfaces: []*computepb.NetworkInterface{{
			Name:       ptr("netif-0"),
			NetworkIP:  ptr("10.0.20.3"),
			Subnetwork: ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network2"),
		}},
	}, {
		Name: ptr("inst-1"),
		Zone: ptr("away-zone"),
		NetworkInterfaces: []*computepb.NetworkInterface{{
			Name:       ptr("netif-0"),
			NetworkIP:  ptr("10.0.10.42"),
			Subnetwork: ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network1"),
		}},
	}}
	s.networks = []*computepb.Network{{
		Name:     ptr("default"),
		SelfLink: ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/default"),
		Subnetworks: []string{
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network1",
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network2",
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network3",
		},
	}, {
		Name:     ptr("another"),
		SelfLink: ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/another"),
		Subnetworks: []string{
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network4",
		},
	}, {
		Name:      ptr("legacy"),
		IPv4Range: ptr("10.240.0.0/16"),
		SelfLink:  ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/legacy"),
	}}
	s.subnets = []*computepb.Subnetwork{{
		Name:        ptr("sub-network1"),
		IpCidrRange: ptr("10.0.10.0/24"),
		SelfLink:    ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network1"),
		Network:     ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/default"),
	}, {
		Name:        ptr("sub-network2"),
		IpCidrRange: ptr("10.0.20.0/24"),
		SelfLink:    ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network2"),
		Network:     ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/another"),
	}}
}

func (s *environNetSuite) TestSubnetsInvalidCredentialError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(nil, gce.InvalidCredentialError)

	_, err := env.Subnets(c.Context(), nil)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environNetSuite) TestGettingAllSubnets(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	subnets, err := env.Subnets(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(subnets, tc.DeepEquals, []corenetwork.SubnetInfo{{
		ProviderId:        "sub-network1",
		ProviderNetworkId: "default",
		CIDR:              "10.0.10.0/24",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		VLANTag:           0,
	}, {
		ProviderId:        "sub-network2",
		ProviderNetworkId: "another",
		CIDR:              "10.0.20.0/24",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		VLANTag:           0,
	}, {
		ProviderId:        "legacy",
		ProviderNetworkId: "legacy",
		CIDR:              "10.240.0.0/16",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		VLANTag:           0,
	}})
}

func (s *environNetSuite) TestRestrictingToSubnets(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	subnets, err := env.Subnets(c.Context(), []corenetwork.Id{
		"sub-network1",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.DeepEquals, []corenetwork.SubnetInfo{{
		ProviderId:        "sub-network1",
		ProviderNetworkId: "default",
		CIDR:              "10.0.10.0/24",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		VLANTag:           0,
	}})
}

func (s *environNetSuite) TestRestrictingToSubnetsWithMissing(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	subnets, err := env.Subnets(c.Context(), []corenetwork.Id{"sub-network1", "sub-network4"})
	c.Assert(err, tc.ErrorMatches, `subnets \["sub-network4"\] not found`)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(subnets, tc.IsNil)
}

func (s *environNetSuite) TestInterfaces(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	infoList, err := env.NetworkInterfaces(c.Context(), []instance.Id{"inst-0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infoList, tc.HasLen, 1)
	infos := infoList[0]

	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:   0,
		ProviderId:    "inst-0/netif-0",
		InterfaceName: "netif-0",
		InterfaceType: corenetwork.EthernetDevice,
		Disabled:      false,
		NoAutoStart:   false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.20.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress(
			corenetwork.WithProviderSubnetID("sub-network2"),
		)},
		Origin: corenetwork.OriginProvider,
	}})
}

func (s *environNetSuite) TestNetworkInterfaceInvalidCredentialError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(nil, gce.InvalidCredentialError)

	_, err := env.NetworkInterfaces(c.Context(), []instance.Id{"inst-0"})
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environNetSuite) TestInterfacesForMultipleInstances(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	s.instances[1].NetworkInterfaces = append(s.instances[1].NetworkInterfaces, &computepb.NetworkInterface{
		Name:       ptr("netif-1"),
		NetworkIP:  ptr("10.0.20.44"),
		Network:    ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/default"),
		Subnetwork: ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network1"),
		AccessConfigs: []*computepb.AccessConfig{{
			Type:  ptr("ONE_TO_ONE_NAT"),
			Name:  ptr("ExternalNAT"),
			NatIP: ptr("25.185.142.227"),
		}},
	})

	infoLists, err := env.NetworkInterfaces(c.Context(), []instance.Id{"inst-0", "inst-1"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infoLists, tc.HasLen, 2)

	// Check interfaces for first instance
	infos := infoLists[0]
	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:   0,
		ProviderId:    "inst-0/netif-0",
		InterfaceName: "netif-0",
		InterfaceType: corenetwork.EthernetDevice,
		Disabled:      false,
		NoAutoStart:   false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.20.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress(
			corenetwork.WithProviderSubnetID("sub-network2"),
		)},
		Origin: corenetwork.OriginProvider,
	}})

	// Check interfaces for second instance
	infos = infoLists[1]
	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:   0,
		ProviderId:    "inst-1/netif-0",
		InterfaceName: "netif-0",
		InterfaceType: corenetwork.EthernetDevice,
		Disabled:      false,
		NoAutoStart:   false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.10.42",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress(
			corenetwork.WithProviderSubnetID("sub-network1"),
		)},
		Origin: corenetwork.OriginProvider,
	}, {
		DeviceIndex:   1,
		ProviderId:    "inst-1/netif-1",
		InterfaceName: "netif-1",
		InterfaceType: corenetwork.EthernetDevice,
		Disabled:      false,
		NoAutoStart:   false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.44",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress(
			corenetwork.WithProviderSubnetID("sub-network1"),
		)},
		ShadowAddresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"25.185.142.227",
			corenetwork.WithScope(corenetwork.ScopePublic),
		).AsProviderAddress()},
		Origin: corenetwork.OriginProvider,
	}})
}

func (s *environNetSuite) TestPartialInterfacesForMultipleInstances(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	infoLists, err := env.NetworkInterfaces(c.Context(), []instance.Id{"inst-0", "bogus"})
	c.Assert(err, tc.Equals, environs.ErrPartialInstances)
	c.Assert(infoLists, tc.HasLen, 2)

	// Check interfaces for first instance
	infos := infoLists[0]
	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:   0,
		ProviderId:    "inst-0/netif-0",
		InterfaceName: "netif-0",
		InterfaceType: corenetwork.EthernetDevice,
		Disabled:      false,
		NoAutoStart:   false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.20.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress(
			corenetwork.WithProviderSubnetID("sub-network2"),
		)},
		Origin: corenetwork.OriginProvider,
	}})

	// Check that the slot for the second instance is nil
	c.Assert(infoLists[1], tc.IsNil, tc.Commentf("expected slot for unknown instance to be nil"))
}

func (s *environNetSuite) TestInterfacesMulti(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.instances[0].NetworkInterfaces = append(s.instances[0].NetworkInterfaces, &computepb.NetworkInterface{
		Name:       ptr("othernetif"),
		NetworkIP:  ptr("10.0.10.4"),
		Network:    ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/default"),
		Subnetwork: ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network1"),
		AccessConfigs: []*computepb.AccessConfig{{
			Type:  ptr("ONE_TO_ONE_NAT"),
			Name:  ptr("ExternalNAT"),
			NatIP: ptr("25.185.142.227"),
		}},
	})

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	infoList, err := env.NetworkInterfaces(c.Context(), []instance.Id{"inst-0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infoList, tc.HasLen, 1)
	infos := infoList[0]

	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:   0,
		ProviderId:    "inst-0/netif-0",
		InterfaceName: "netif-0",
		InterfaceType: corenetwork.EthernetDevice,
		Disabled:      false,
		NoAutoStart:   false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.20.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress(
			corenetwork.WithProviderSubnetID("sub-network2"),
		)},
		Origin: corenetwork.OriginProvider,
	}, {
		DeviceIndex:   1,
		ProviderId:    "inst-0/othernetif",
		InterfaceName: "othernetif",
		InterfaceType: corenetwork.EthernetDevice,
		Disabled:      false,
		NoAutoStart:   false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.10.4",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress(
			corenetwork.WithProviderSubnetID("sub-network1"),
		)},
		ShadowAddresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress("25.185.142.227", corenetwork.WithScope(corenetwork.ScopePublic)).AsProviderAddress(),
		},
		Origin: corenetwork.OriginProvider,
	}})
}

func (s *environNetSuite) TestInterfacesLegacy(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService) // When we're using a legacy network there'll be no subnet.

	s.instances[0].NetworkInterfaces = []*computepb.NetworkInterface{{
		Name:      ptr("somenetif"),
		NetworkIP: ptr("10.240.0.2"),
		Network:   ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/legacy"),
		AccessConfigs: []*computepb.AccessConfig{{
			Type:  ptr("ONE_TO_ONE_NAT"),
			Name:  ptr("ExternalNAT"),
			NatIP: ptr("25.185.142.227"),
		}},
	}}

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)

	infoList, err := env.NetworkInterfaces(c.Context(), []instance.Id{"inst-0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infoList, tc.HasLen, 1)
	infos := infoList[0]

	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:   0,
		ProviderId:    "inst-0/somenetif",
		InterfaceName: "somenetif",
		InterfaceType: corenetwork.EthernetDevice,
		Disabled:      false,
		NoAutoStart:   false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.240.0.2",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.240.0.0/16"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		ShadowAddresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress("25.185.142.227", corenetwork.WithScope(corenetwork.ScopePublic)).AsProviderAddress(),
		},
		Origin: corenetwork.OriginProvider,
	}})
}

func (s *environNetSuite) TestInterfacesSameSubnetwork(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.instances[0].NetworkInterfaces = append(s.instances[0].NetworkInterfaces, &computepb.NetworkInterface{
		Name:       ptr("othernetif"),
		NetworkIP:  ptr("10.0.10.4"),
		Network:    ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/default"),
		Subnetwork: ptr("https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network1"),
		AccessConfigs: []*computepb.AccessConfig{{
			Type:  ptr("ONE_TO_ONE_NAT"),
			Name:  ptr("ExternalNAT"),
			NatIP: ptr("25.185.142.227"),
		}},
	})

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	infoList, err := env.NetworkInterfaces(c.Context(), []instance.Id{"inst-0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infoList, tc.HasLen, 1)
	infos := infoList[0]

	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:   0,
		ProviderId:    "inst-0/netif-0",
		InterfaceName: "netif-0",
		InterfaceType: corenetwork.EthernetDevice,
		Disabled:      false,
		NoAutoStart:   false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.20.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress(
			corenetwork.WithProviderSubnetID("sub-network2"),
		)},
		Origin: corenetwork.OriginProvider,
	}, {
		DeviceIndex:   1,
		ProviderId:    "inst-0/othernetif",
		InterfaceName: "othernetif",
		InterfaceType: corenetwork.EthernetDevice,
		Disabled:      false,
		NoAutoStart:   false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.10.4",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress(
			corenetwork.WithProviderSubnetID("sub-network1"),
		)},
		ShadowAddresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress("25.185.142.227", corenetwork.WithScope(corenetwork.ScopePublic)).AsProviderAddress(),
		},
		Origin: corenetwork.OriginProvider,
	}})
}
