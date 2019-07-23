// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/provider/gce/google"
)

type environNetSuite struct {
	gce.BaseSuite
	NetEnv environs.NetworkingEnviron
}

var _ = gc.Suite(&environNetSuite{})

func (s *environNetSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	netEnv, ok := environs.SupportsNetworking(s.Env)
	c.Assert(ok, jc.IsTrue)
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

func (s *environNetSuite) TestSubnetsInvalidCredentialError(c *gc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	_, err := s.NetEnv.Subnets(s.CallCtx, instance.UnknownId, nil)
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environNetSuite) TestGettingAllSubnets(c *gc.C) {
	s.cannedData()

	subnets, err := s.NetEnv.Subnets(s.CallCtx, instance.UnknownId, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []corenetwork.SubnetInfo{{
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

func (s *environNetSuite) TestSuperSubnets(c *gc.C) {
	s.cannedData()

	subnets, err := s.NetEnv.SuperSubnets(s.CallCtx)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []string{
		"10.0.10.0/24",
		"10.0.20.0/24",
		"10.240.0.0/16",
	})
}

func (s *environNetSuite) TestRestrictingToSubnets(c *gc.C) {
	s.cannedData()

	subnets, err := s.NetEnv.Subnets(s.CallCtx, instance.UnknownId, []corenetwork.Id{
		"shellac",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.DeepEquals, []corenetwork.SubnetInfo{{
		ProviderId:        "shellac",
		ProviderNetworkId: "albini",
		CIDR:              "10.0.20.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		VLANTag:           0,
	}})
}

func (s *environNetSuite) TestRestrictingToSubnetsWithMissing(c *gc.C) {
	s.cannedData()

	subnets, err := s.NetEnv.Subnets(s.CallCtx, instance.UnknownId, []corenetwork.Id{"shellac", "brunettes"})
	c.Assert(err, gc.ErrorMatches, `subnets \["brunettes"\] not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(subnets, gc.IsNil)
}

func (s *environNetSuite) TestSpecificInstance(c *gc.C) {
	s.cannedData()
	s.FakeEnviron.Insts = []instances.Instance{s.NewInstance(c, "moana")}

	subnets, err := s.NetEnv.Subnets(s.CallCtx, instance.Id("moana"), nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []corenetwork.SubnetInfo{{
		ProviderId:        "go-team",
		ProviderNetworkId: "go-team1",
		CIDR:              "10.0.10.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		VLANTag:           0,
	}})
}

func (s *environNetSuite) TestSpecificInstanceAndRestrictedSubnets(c *gc.C) {
	s.cannedData()
	s.FakeEnviron.Insts = []instances.Instance{s.NewInstance(c, "moana")}

	subnets, err := s.NetEnv.Subnets(s.CallCtx, instance.Id("moana"), []corenetwork.Id{"go-team"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []corenetwork.SubnetInfo{{
		ProviderId:        "go-team",
		ProviderNetworkId: "go-team1",
		CIDR:              "10.0.10.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		VLANTag:           0,
	}})
}

func (s *environNetSuite) TestSpecificInstanceAndRestrictedSubnetsWithMissing(c *gc.C) {
	s.cannedData()
	s.FakeEnviron.Insts = []instances.Instance{s.NewInstance(c, "moana")}

	subnets, err := s.NetEnv.Subnets(s.CallCtx, instance.Id("moana"), []corenetwork.Id{"go-team", "shellac"})
	c.Assert(err, gc.ErrorMatches, `subnets \["shellac"\] not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(subnets, gc.IsNil)
}

func (s *environNetSuite) TestInterfaces(c *gc.C) {
	s.cannedData()
	s.FakeEnviron.Insts = []instances.Instance{s.NewInstance(c, "moana")}

	infos, err := s.NetEnv.NetworkInterfaces(s.CallCtx, instance.Id("moana"))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(infos, gc.DeepEquals, []network.InterfaceInfo{{
		DeviceIndex:       0,
		CIDR:              "10.0.10.0/24",
		ProviderId:        "moana/somenetif",
		ProviderSubnetId:  "go-team",
		ProviderNetworkId: "go-team1",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		InterfaceName:     "somenetif",
		InterfaceType:     network.EthernetInterface,
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        network.ConfigDHCP,
		Address:           network.NewScopedAddress("10.0.10.3", network.ScopeCloudLocal),
	}})
}

func (s *environNetSuite) TestNetworkInterfaceInvalidCredentialError(c *gc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
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

	_, err := s.NetEnv.NetworkInterfaces(s.CallCtx, instance.Id("moana"))
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environNetSuite) TestInterfacesMulti(c *gc.C) {
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

	infos, err := s.NetEnv.NetworkInterfaces(s.CallCtx, instance.Id("moana"))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(infos, gc.DeepEquals, []network.InterfaceInfo{{
		DeviceIndex:       0,
		CIDR:              "10.0.10.0/24",
		ProviderId:        "moana/somenetif",
		ProviderSubnetId:  "go-team",
		ProviderNetworkId: "go-team1",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		InterfaceName:     "somenetif",
		InterfaceType:     network.EthernetInterface,
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        network.ConfigDHCP,
		Address:           network.NewScopedAddress("10.0.10.3", network.ScopeCloudLocal),
	}, {
		DeviceIndex:       1,
		CIDR:              "10.0.20.0/24",
		ProviderId:        "moana/othernetif",
		ProviderSubnetId:  "shellac",
		ProviderNetworkId: "albini",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		InterfaceName:     "othernetif",
		InterfaceType:     network.EthernetInterface,
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        network.ConfigDHCP,
		Address:           network.NewScopedAddress("10.0.20.3", network.ScopeCloudLocal),
	}})
}

func (s *environNetSuite) TestInterfacesLegacy(c *gc.C) {
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

	infos, err := s.NetEnv.NetworkInterfaces(s.CallCtx, instance.Id("moana"))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(infos, gc.DeepEquals, []network.InterfaceInfo{{
		DeviceIndex:       0,
		CIDR:              "10.240.0.0/16",
		ProviderId:        "moana/somenetif",
		ProviderSubnetId:  "",
		ProviderNetworkId: "legacy",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		InterfaceName:     "somenetif",
		InterfaceType:     network.EthernetInterface,
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        network.ConfigDHCP,
		Address:           network.NewScopedAddress("10.240.0.2", network.ScopeCloudLocal),
	}})
}

func (s *environNetSuite) TestInterfacesSameSubnetwork(c *gc.C) {
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

	infos, err := s.NetEnv.NetworkInterfaces(s.CallCtx, instance.Id("moana"))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(infos, gc.DeepEquals, []network.InterfaceInfo{{
		DeviceIndex:       0,
		CIDR:              "10.0.10.0/24",
		ProviderId:        "moana/somenetif",
		ProviderSubnetId:  "go-team",
		ProviderNetworkId: "go-team1",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		InterfaceName:     "somenetif",
		InterfaceType:     network.EthernetInterface,
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        network.ConfigDHCP,
		Address:           network.NewScopedAddress("10.0.10.3", network.ScopeCloudLocal),
	}, {
		DeviceIndex:       1,
		CIDR:              "10.0.10.0/24",
		ProviderId:        "moana/othernetif",
		ProviderSubnetId:  "go-team",
		ProviderNetworkId: "go-team1",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		InterfaceName:     "othernetif",
		InterfaceType:     network.EthernetInterface,
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        network.ConfigDHCP,
		Address:           network.NewScopedAddress("10.0.10.4", network.ScopeCloudLocal),
	}})
}
