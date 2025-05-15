// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/provider/gce/google"
)

type environNetSuite struct {
	gce.BaseSuite
	NetEnv environs.NetworkingEnviron
}

var _ = tc.Suite(&environNetSuite{})

func (s *environNetSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	netEnv, ok := environs.SupportsNetworking(s.Env)
	c.Assert(ok, tc.IsTrue)
	s.NetEnv = netEnv
}

func (s *environNetSuite) cannedData() {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusUp, "", ""),
		google.NewZone("b-zone", google.StatusUp, "", ""),
	}
	s.FakeConn.Networks_ = []*compute.Network{{
		Id:                    9876,
		Name:                  "go-team1",
		AutoCreateSubnetworks: true,
		SelfLink:              "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/go-team1",
		Subnetworks: []string{
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/go-team",
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-central1/subnetworks/go-team",
		},
	}, {
		Id:                    8765,
		Name:                  "albini",
		AutoCreateSubnetworks: false,
		SelfLink:              "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/albini",
		Subnetworks: []string{
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/shellac",
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-central1/subnetworks/flour",
		},
	}, {
		Id:                    4567,
		Name:                  "legacy",
		AutoCreateSubnetworks: false,
		IPv4Range:             "10.240.0.0/16",
		SelfLink:              "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/legacy",
	}}
	s.FakeConn.Subnets = []*compute.Subnetwork{{
		Id:          1234,
		IpCidrRange: "10.0.10.0/24",
		Name:        "go-team",
		Network:     "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/go-team1",
		Region:      "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1",
		SelfLink:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/go-team",
	}, {
		Id:          1235,
		IpCidrRange: "10.0.20.0/24",
		Name:        "shellac",
		Network:     "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/albini",
		Region:      "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1",
		SelfLink:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/shellac",
	}}
}

func (s *environNetSuite) TestSubnetsInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	_, err := s.NetEnv.Subnets(c.Context(), nil)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environNetSuite) TestGettingAllSubnets(c *tc.C) {
	s.cannedData()

	subnets, err := s.NetEnv.Subnets(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(subnets, tc.DeepEquals, []corenetwork.SubnetInfo{{
		ProviderId:        "go-team",
		ProviderNetworkId: "go-team1",
		CIDR:              "10.0.10.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		VLANTag:           0,
	}, {
		ProviderId:        "shellac",
		ProviderNetworkId: "albini",
		CIDR:              "10.0.20.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		VLANTag:           0,
	}, {
		ProviderId:        "legacy",
		ProviderNetworkId: "legacy",
		CIDR:              "10.240.0.0/16",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		VLANTag:           0,
	}})
}

func (s *environNetSuite) TestRestrictingToSubnets(c *tc.C) {
	s.cannedData()

	subnets, err := s.NetEnv.Subnets(c.Context(), []corenetwork.Id{"shellac"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.DeepEquals, []corenetwork.SubnetInfo{{
		ProviderId:        "shellac",
		ProviderNetworkId: "albini",
		CIDR:              "10.0.20.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		VLANTag:           0,
	}})
}

func (s *environNetSuite) TestRestrictingToSubnetsWithMissing(c *tc.C) {
	s.cannedData()

	subnets, err := s.NetEnv.Subnets(c.Context(), []corenetwork.Id{"shellac", "brunettes"})
	c.Assert(err, tc.ErrorMatches, `subnets \["brunettes"\] not found`)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(subnets, tc.IsNil)
}

func (s *environNetSuite) TestInterfaces(c *tc.C) {
	s.cannedData()
	s.FakeEnviron.Insts = []instances.Instance{s.NewInstance(c, "moana")}

	infoList, err := s.NetEnv.NetworkInterfaces(c.Context(), []instance.Id{"moana"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infoList, tc.HasLen, 1)
	infos := infoList[0]

	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:      0,
		ProviderId:       "moana/somenetif",
		ProviderSubnetId: "go-team",
		InterfaceName:    "somenetif",
		InterfaceType:    corenetwork.EthernetDevice,
		Disabled:         false,
		NoAutoStart:      false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.10.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		Origin: corenetwork.OriginProvider,
	}})
}

func (s *environNetSuite) TestNetworkInterfaceInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	s.cannedData()
	baseInst := s.NewBaseInstance(c, "moana")
	// This isn't possible in GCE at the moment, but we don't want to
	// break when it is.
	summary := &baseInst.InstanceSummary
	summary.NetworkInterfaces = append(summary.NetworkInterfaces, &compute.NetworkInterface{
		Name:       "othernetif",
		NetworkIP:  "10.0.20.3",
		Network:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/shellac",
		Subnetwork: "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/shellac",
		AccessConfigs: []*compute.AccessConfig{{
			Type:  "ONE_TO_ONE_NAT",
			Name:  "ExternalNAT",
			NatIP: "25.185.142.227",
		}},
	})
	s.FakeEnviron.Insts = []instances.Instance{s.NewInstanceFromBase(baseInst)}

	_, err := s.NetEnv.NetworkInterfaces(c.Context(), []instance.Id{"moana"})
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environNetSuite) TestInterfacesForMultipleInstances(c *tc.C) {
	s.cannedData()
	baseInst1 := s.NewBaseInstance(c, "i-1")

	// Create a second instance and patch its interface list
	baseInst2 := s.NewBaseInstance(c, "i-2")
	b2summary := &baseInst2.InstanceSummary
	b2summary.NetworkInterfaces = []*compute.NetworkInterface{
		{
			Name:       "netif-0",
			NetworkIP:  "10.0.10.42",
			Network:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/go-team",
			Subnetwork: "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/go-team",
			AccessConfigs: []*compute.AccessConfig{{
				Type:  "ONE_TO_ONE_NAT",
				Name:  "ExternalNAT",
				NatIP: "25.185.142.227",
			}},
		},
		{
			Name:       "netif-1",
			NetworkIP:  "10.0.20.42",
			Network:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/shellac",
			Subnetwork: "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/shellac",
			// No public IP
		},
	}
	s.FakeEnviron.Insts = []instances.Instance{
		s.NewInstanceFromBase(baseInst1),
		s.NewInstanceFromBase(baseInst2),
	}

	infoLists, err := s.NetEnv.NetworkInterfaces(c.Context(), []instance.Id{
		"i-1",
		"i-2",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infoLists, tc.HasLen, 2)

	// Check interfaces for first instance
	infos := infoLists[0]
	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:      0,
		ProviderId:       "i-1/somenetif",
		ProviderSubnetId: "go-team",
		InterfaceName:    "somenetif",
		InterfaceType:    corenetwork.EthernetDevice,
		Disabled:         false,
		NoAutoStart:      false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.10.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		Origin: corenetwork.OriginProvider,
	}})

	// Check interfaces for second instance
	infos = infoLists[1]
	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:      0,
		ProviderId:       "i-2/netif-0",
		ProviderSubnetId: "go-team",
		InterfaceName:    "netif-0",
		InterfaceType:    corenetwork.EthernetDevice,
		Disabled:         false,
		NoAutoStart:      false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.10.42",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		ShadowAddresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress("25.185.142.227", corenetwork.WithScope(corenetwork.ScopePublic)).AsProviderAddress(),
		},
		Origin: corenetwork.OriginProvider,
	}, {
		DeviceIndex:      1,
		ProviderId:       "i-2/netif-1",
		ProviderSubnetId: "shellac",
		InterfaceName:    "netif-1",
		InterfaceType:    corenetwork.EthernetDevice,
		Disabled:         false,
		NoAutoStart:      false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.42",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.20.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		Origin: corenetwork.OriginProvider,
	}})
}

func (s *environNetSuite) TestPartialInterfacesForMultipleInstances(c *tc.C) {
	s.cannedData()
	baseInst1 := s.NewBaseInstance(c, "i-1")
	s.FakeEnviron.Insts = []instances.Instance{s.NewInstanceFromBase(baseInst1)}

	infoLists, err := s.NetEnv.NetworkInterfaces(c.Context(), []instance.Id{"i-1", "bogus"})
	c.Assert(err, tc.Equals, environs.ErrPartialInstances)
	c.Assert(infoLists, tc.HasLen, 2)

	// Check interfaces for first instance
	infos := infoLists[0]
	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:      0,
		ProviderId:       "i-1/somenetif",
		ProviderSubnetId: "go-team",
		InterfaceName:    "somenetif",
		InterfaceType:    corenetwork.EthernetDevice,
		Disabled:         false,
		NoAutoStart:      false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.10.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		Origin: corenetwork.OriginProvider,
	}})

	// Check that the slot for the second instance is nil
	c.Assert(infoLists[1], tc.IsNil, tc.Commentf("expected slot for unknown instance to be nil"))
}

func (s *environNetSuite) TestInterfacesMulti(c *tc.C) {
	s.cannedData()
	baseInst := s.NewBaseInstance(c, "moana")
	// This isn't possible in GCE at the moment, but we don't want to
	// break when it is.
	summary := &baseInst.InstanceSummary
	summary.NetworkInterfaces = append(summary.NetworkInterfaces, &compute.NetworkInterface{
		Name:       "othernetif",
		NetworkIP:  "10.0.20.3",
		Network:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/shellac",
		Subnetwork: "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/shellac",
		AccessConfigs: []*compute.AccessConfig{{
			Type:  "ONE_TO_ONE_NAT",
			Name:  "ExternalNAT",
			NatIP: "25.185.142.227",
		}},
	})
	s.FakeEnviron.Insts = []instances.Instance{s.NewInstanceFromBase(baseInst)}

	infoList, err := s.NetEnv.NetworkInterfaces(c.Context(), []instance.Id{"moana"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infoList, tc.HasLen, 1)
	infos := infoList[0]

	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:      0,
		ProviderId:       "moana/somenetif",
		ProviderSubnetId: "go-team",
		InterfaceName:    "somenetif",
		InterfaceType:    corenetwork.EthernetDevice,
		Disabled:         false,
		NoAutoStart:      false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.10.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		Origin: corenetwork.OriginProvider,
	}, {
		DeviceIndex:      1,
		ProviderId:       "moana/othernetif",
		ProviderSubnetId: "shellac",
		InterfaceName:    "othernetif",
		InterfaceType:    corenetwork.EthernetDevice,
		Disabled:         false,
		NoAutoStart:      false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.20.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.20.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		ShadowAddresses: corenetwork.ProviderAddresses{
			corenetwork.NewMachineAddress("25.185.142.227", corenetwork.WithScope(corenetwork.ScopePublic)).AsProviderAddress(),
		},
		Origin: corenetwork.OriginProvider,
	}})
}

func (s *environNetSuite) TestInterfacesLegacy(c *tc.C) {
	s.cannedData()
	baseInst := s.NewBaseInstance(c, "moana")
	// When we're using a legacy network there'll be no subnet.
	summary := &baseInst.InstanceSummary
	summary.NetworkInterfaces = []*compute.NetworkInterface{{
		Name:       "somenetif",
		NetworkIP:  "10.240.0.2",
		Network:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/legacy",
		Subnetwork: "",
		AccessConfigs: []*compute.AccessConfig{{
			Type:  "ONE_TO_ONE_NAT",
			Name:  "ExternalNAT",
			NatIP: "25.185.142.227",
		}},
	}}
	s.FakeEnviron.Insts = []instances.Instance{s.NewInstanceFromBase(baseInst)}

	infoList, err := s.NetEnv.NetworkInterfaces(c.Context(), []instance.Id{"moana"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infoList, tc.HasLen, 1)
	infos := infoList[0]

	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:      0,
		ProviderId:       "moana/somenetif",
		ProviderSubnetId: "",
		InterfaceName:    "somenetif",
		InterfaceType:    corenetwork.EthernetDevice,
		Disabled:         false,
		NoAutoStart:      false,
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
	s.cannedData()
	baseInst := s.NewBaseInstance(c, "moana")
	// This isn't possible in GCE at the moment, but we don't want to
	// break when it is.
	summary := &baseInst.InstanceSummary
	summary.NetworkInterfaces = append(summary.NetworkInterfaces, &compute.NetworkInterface{
		Name:       "othernetif",
		NetworkIP:  "10.0.10.4",
		Network:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/go-team1",
		Subnetwork: "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/go-team",
		AccessConfigs: []*compute.AccessConfig{{
			Type:  "ONE_TO_ONE_NAT",
			Name:  "ExternalNAT",
			NatIP: "25.185.142.227",
		}},
	})
	s.FakeEnviron.Insts = []instances.Instance{s.NewInstanceFromBase(baseInst)}

	infoList, err := s.NetEnv.NetworkInterfaces(c.Context(), []instance.Id{"moana"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infoList, tc.HasLen, 1)
	infos := infoList[0]

	c.Assert(infos, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:      0,
		ProviderId:       "moana/somenetif",
		ProviderSubnetId: "go-team",
		InterfaceName:    "somenetif",
		InterfaceType:    corenetwork.EthernetDevice,
		Disabled:         false,
		NoAutoStart:      false,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"10.0.10.3",
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR("10.0.10.0/24"),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		Origin: corenetwork.OriginProvider,
	}, {
		DeviceIndex:      1,
		ProviderId:       "moana/othernetif",
		ProviderSubnetId: "go-team",
		InterfaceName:    "othernetif",
		InterfaceType:    corenetwork.EthernetDevice,
		Disabled:         false,
		NoAutoStart:      false,
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
