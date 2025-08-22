// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/gce"
)

type environNetSuite struct {
	gce.BaseSuite

	zones     []*compute.Zone
	instances []*compute.Instance
	networks  []*compute.Network
	subnets   []*compute.Subnetwork
}

var _ = gc.Suite(&environNetSuite{})

func (s *environNetSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.zones = []*compute.Zone{{
		Name:   "home-zone",
		Status: "UP",
	}, {
		Name:   "away-zone",
		Status: "UP",
	}}
	s.instances = []*compute.Instance{{
		Name: "inst-0",
		Zone: "home-zone",
		NetworkInterfaces: []*compute.NetworkInterface{{
			Name:       "netif-0",
			NetworkIP:  "10.0.20.3",
			Subnetwork: "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network2",
		}},
	}, {
		Name: "inst-1",
		Zone: "away-zone",
		NetworkInterfaces: []*compute.NetworkInterface{{
			Name:       "netif-0",
			NetworkIP:  "10.0.10.42",
			Subnetwork: "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network1",
		}},
	}}
	s.networks = []*compute.Network{{
		Name:     "default",
		SelfLink: "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/default",
		Subnetworks: []string{
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network1",
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network2",
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network3",
		},
	}, {
		Name:     "another",
		SelfLink: "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/another",
		Subnetworks: []string{
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network4",
		},
	}, {
		Name:      "legacy",
		IPv4Range: "10.240.0.0/16",
		SelfLink:  "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/legacy",
	}}
	s.subnets = []*compute.Subnetwork{{
		Name:        "sub-network1",
		IpCidrRange: "10.0.10.0/24",
		SelfLink:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network1",
		Network:     "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/default",
	}, {
		Name:        "sub-network2",
		IpCidrRange: "10.0.20.0/24",
		SelfLink:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network2",
		Network:     "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/another",
	}}
}

func (s *environNetSuite) TestSubnetsInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(nil, gce.InvalidCredentialError)

	_, err := env.Subnets(s.CallCtx, instance.UnknownId, nil)
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environNetSuite) TestGettingAllSubnets(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	subnets, err := env.Subnets(s.CallCtx, instance.UnknownId, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []corenetwork.SubnetInfo{{
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

func (s *environNetSuite) TestSuperSubnets(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)

	subnets, err := env.SuperSubnets(s.CallCtx)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []string{
		"10.0.10.0/24",
		"10.0.20.0/24",
		"10.240.0.0/16",
	})
}

func (s *environNetSuite) TestRestrictingToSubnets(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	subnets, err := env.Subnets(s.CallCtx, instance.UnknownId, []corenetwork.Id{
		"sub-network1",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.DeepEquals, []corenetwork.SubnetInfo{{
		ProviderId:        "sub-network1",
		ProviderNetworkId: "default",
		CIDR:              "10.0.10.0/24",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		VLANTag:           0,
	}})
}

func (s *environNetSuite) TestRestrictingToSubnetsWithMissing(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	subnets, err := env.Subnets(s.CallCtx, instance.UnknownId, []corenetwork.Id{"sub-network1", "sub-network4"})
	c.Assert(err, gc.ErrorMatches, `subnets \["sub-network4"\] not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(subnets, gc.IsNil)
}

func (s *environNetSuite) TestSpecificInstance(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil).Times(2)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	subnets, err := env.Subnets(s.CallCtx, "inst-0", []corenetwork.Id{"sub-network2"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []corenetwork.SubnetInfo{{
		ProviderId:        "sub-network2",
		ProviderNetworkId: "another",
		CIDR:              "10.0.20.0/24",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		VLANTag:           0,
	}})
}

func (s *environNetSuite) TestSpecificInstanceAndRestrictedSubnets(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil).Times(2)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	subnets, err := env.Subnets(s.CallCtx, "inst-0", []corenetwork.Id{"sub-network2"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []corenetwork.SubnetInfo{{
		ProviderId:        "sub-network2",
		ProviderNetworkId: "another",
		CIDR:              "10.0.20.0/24",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		VLANTag:           0,
	}})
}

func (s *environNetSuite) TestSpecificInstanceAndRestrictedSubnetsWithMissing(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil).Times(2)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	subnets, err := env.Subnets(s.CallCtx, "inst-0", []corenetwork.Id{"sub-network1", "sub-network2"})
	c.Assert(err, gc.ErrorMatches, `subnets \["sub-network1"\] not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(subnets, gc.IsNil)
}

func (s *environNetSuite) TestInterfaces(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	infoList, err := env.NetworkInterfaces(s.CallCtx, []instance.Id{"inst-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(infoList, gc.HasLen, 1)
	infos := infoList[0]

	c.Assert(infos, gc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:       0,
		ProviderId:        "inst-0/netif-0",
		ProviderSubnetId:  "sub-network2",
		ProviderNetworkId: "another",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		InterfaceName:     "netif-0",
		InterfaceType:     corenetwork.EthernetDevice,
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.20.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		Origin: corenetwork.OriginProvider,
	}})
}

func (s *environNetSuite) TestNetworkInterfaceInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(nil, gce.InvalidCredentialError)

	_, err := env.NetworkInterfaces(s.CallCtx, []instance.Id{"inst-0"})
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environNetSuite) TestInterfacesForMultipleInstances(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	s.instances[1].NetworkInterfaces = append(s.instances[1].NetworkInterfaces, &compute.NetworkInterface{
		Name:       "netif-1",
		NetworkIP:  "10.0.20.44",
		Network:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/default",
		Subnetwork: "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network1",
		AccessConfigs: []*compute.AccessConfig{{
			Type:  "ONE_TO_ONE_NAT",
			Name:  "ExternalNAT",
			NatIP: "25.185.142.227",
		}},
	})

	infoLists, err := env.NetworkInterfaces(s.CallCtx, []instance.Id{"inst-0", "inst-1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(infoLists, gc.HasLen, 2)

	// Check interfaces for first instance
	infos := infoLists[0]
	c.Assert(infos, jc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:       0,
		ProviderId:        "inst-0/netif-0",
		ProviderSubnetId:  "sub-network2",
		ProviderNetworkId: "another",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		InterfaceName:     "netif-0",
		InterfaceType:     corenetwork.EthernetDevice,
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.20.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		Origin: corenetwork.OriginProvider,
	}})

	// Check interfaces for second instance
	infos = infoLists[1]
	c.Assert(infos, jc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:       0,
		ProviderId:        "inst-1/netif-0",
		ProviderSubnetId:  "sub-network1",
		ProviderNetworkId: "default",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		InterfaceName:     "netif-0",
		InterfaceType:     corenetwork.EthernetDevice,
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.10.42",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		Origin: corenetwork.OriginProvider,
	}, {
		DeviceIndex:       1,
		ProviderId:        "inst-1/netif-1",
		ProviderSubnetId:  "sub-network1",
		ProviderNetworkId: "default",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		InterfaceName:     "netif-1",
		InterfaceType:     corenetwork.EthernetDevice,
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.44",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		ShadowAddresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress("25.185.142.227", corenetwork.WithScope(corenetwork.ScopePublic)).AsProviderAddress(),
		},
		Origin: corenetwork.OriginProvider,
	}})
}

func (s *environNetSuite) TestPartialInterfacesForMultipleInstances(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	infoLists, err := env.NetworkInterfaces(s.CallCtx, []instance.Id{"inst-0", "bogus"})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(infoLists, gc.HasLen, 2)

	// Check interfaces for first instance
	infos := infoLists[0]
	c.Assert(infos, gc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:       0,
		ProviderId:        "inst-0/netif-0",
		ProviderSubnetId:  "sub-network2",
		ProviderNetworkId: "another",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		InterfaceName:     "netif-0",
		InterfaceType:     corenetwork.EthernetDevice,
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.20.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		Origin: corenetwork.OriginProvider,
	}})

	// Check that the slot for the second instance is nil
	c.Assert(infoLists[1], gc.IsNil, gc.Commentf("expected slot for unknown instance to be nil"))
}

func (s *environNetSuite) TestInterfacesMulti(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.instances[0].NetworkInterfaces = append(s.instances[0].NetworkInterfaces, &compute.NetworkInterface{
		Name:       "othernetif",
		NetworkIP:  "10.0.10.4",
		Network:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/default",
		Subnetwork: "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network1",
		AccessConfigs: []*compute.AccessConfig{{
			Type:  "ONE_TO_ONE_NAT",
			Name:  "ExternalNAT",
			NatIP: "25.185.142.227",
		}},
	})

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	infoList, err := env.NetworkInterfaces(s.CallCtx, []instance.Id{"inst-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(infoList, gc.HasLen, 1)
	infos := infoList[0]

	c.Assert(infos, gc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:       0,
		ProviderId:        "inst-0/netif-0",
		ProviderSubnetId:  "sub-network2",
		ProviderNetworkId: "another",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		InterfaceName:     "netif-0",
		InterfaceType:     corenetwork.EthernetDevice,
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.20.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		Origin: corenetwork.OriginProvider,
	}, {
		DeviceIndex:       1,
		ProviderId:        "inst-0/othernetif",
		ProviderSubnetId:  "sub-network1",
		ProviderNetworkId: "default",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		InterfaceName:     "othernetif",
		InterfaceType:     corenetwork.EthernetDevice,
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.10.4",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		ShadowAddresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress("25.185.142.227", corenetwork.WithScope(corenetwork.ScopePublic)).AsProviderAddress(),
		},
		Origin: corenetwork.OriginProvider,
	}})
}

func (s *environNetSuite) TestInterfacesLegacy(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService) // When we're using a legacy network there'll be no subnet.

	s.instances[0].NetworkInterfaces = []*compute.NetworkInterface{{
		Name:      "somenetif",
		NetworkIP: "10.240.0.2",
		Network:   "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/legacy",
		AccessConfigs: []*compute.AccessConfig{{
			Type:  "ONE_TO_ONE_NAT",
			Name:  "ExternalNAT",
			NatIP: "25.185.142.227",
		}},
	}}

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)

	infoList, err := env.NetworkInterfaces(s.CallCtx, []instance.Id{"inst-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(infoList, gc.HasLen, 1)
	infos := infoList[0]

	c.Assert(infos, gc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:       0,
		ProviderId:        "inst-0/somenetif",
		ProviderSubnetId:  "",
		ProviderNetworkId: "legacy",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		InterfaceName:     "somenetif",
		InterfaceType:     corenetwork.EthernetDevice,
		Disabled:          false,
		NoAutoStart:       false,
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

func (s *environNetSuite) TestInterfacesSameSubnetwork(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.instances[0].NetworkInterfaces = append(s.instances[0].NetworkInterfaces, &compute.NetworkInterface{
		Name:       "othernetif",
		NetworkIP:  "10.0.10.4",
		Network:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/default",
		Subnetwork: "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-east1/subnetworks/sub-network1",
		AccessConfigs: []*compute.AccessConfig{{
			Type:  "ONE_TO_ONE_NAT",
			Name:  "ExternalNAT",
			NatIP: "25.185.142.227",
		}},
	})

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return(s.zones, nil)
	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env), "PENDING", "STAGING", "RUNNING").
		Return(s.instances, nil)
	s.MockService.EXPECT().Networks(gomock.Any()).Return(s.networks, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1").Return(s.subnets, nil)

	infoList, err := env.NetworkInterfaces(s.CallCtx, []instance.Id{"inst-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(infoList, gc.HasLen, 1)
	infos := infoList[0]

	c.Assert(infos, gc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:       0,
		ProviderId:        "inst-0/netif-0",
		ProviderSubnetId:  "sub-network2",
		ProviderNetworkId: "another",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		InterfaceName:     "netif-0",
		InterfaceType:     corenetwork.EthernetDevice,
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.20.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		Origin: corenetwork.OriginProvider,
	}, {
		DeviceIndex:       1,
		ProviderId:        "inst-0/othernetif",
		ProviderSubnetId:  "sub-network1",
		ProviderNetworkId: "default",
		AvailabilityZones: []string{"home-zone", "away-zone"},
		InterfaceName:     "othernetif",
		InterfaceType:     corenetwork.EthernetDevice,
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.10.4",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		ShadowAddresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress("25.185.142.227", corenetwork.WithScope(corenetwork.ScopePublic)).AsProviderAddress(),
		},
		Origin: corenetwork.OriginProvider,
	}})
}
