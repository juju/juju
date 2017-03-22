// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	// "github.com/juju/version"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	// "github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	// "github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
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
		Id:   9876,
		Name: "go-team1",
		AutoCreateSubnetworks: true,
		SelfLink:              "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/go-team1",
		Subnetworks: []string{
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/go-team",
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-central1/subnetworks/go-team",
		},
	}, {
		Id:   8765,
		Name: "albini",
		AutoCreateSubnetworks: false,
		SelfLink:              "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/albini",
		Subnetworks: []string{
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/shellac",
			"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/us-central1/subnetworks/flour",
		},
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

func (s *environNetSuite) TestGettingAllSubnets(c *gc.C) {
	s.cannedData()

	subnets, err := s.NetEnv.Subnets(instance.UnknownId, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []network.SubnetInfo{{
		ProviderId:        "go-team",
		ProviderNetworkId: "go-team1",
		CIDR:              "10.0.10.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		VLANTag:           0,
		SpaceProviderId:   "",
	}, {
		ProviderId:        "shellac",
		ProviderNetworkId: "albini",
		CIDR:              "10.0.20.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		VLANTag:           0,
		SpaceProviderId:   "",
	}})
}

func (s *environNetSuite) TestRestrictingToSubnets(c *gc.C) {
	s.cannedData()

	subnets, err := s.NetEnv.Subnets(instance.UnknownId, []network.Id{
		"shellac",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.DeepEquals, []network.SubnetInfo{{
		ProviderId:        "shellac",
		ProviderNetworkId: "albini",
		CIDR:              "10.0.20.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		VLANTag:           0,
		SpaceProviderId:   "",
	}})
}

func (s *environNetSuite) TestRestrictingToSubnetsWithMissing(c *gc.C) {
	s.cannedData()

	subnets, err := s.NetEnv.Subnets(instance.UnknownId, []network.Id{"shellac", "brunettes"})
	c.Assert(err, gc.ErrorMatches, `subnets \[brunettes\] not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(subnets, gc.IsNil)
}

func (s *environNetSuite) TestSpecificInstance(c *gc.C) {
	s.cannedData()
	s.FakeEnviron.Insts = []instance.Instance{s.NewInstance(c, "moana")}

	subnets, err := s.NetEnv.Subnets(instance.Id("moana"), nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []network.SubnetInfo{{
		ProviderId:        "go-team",
		ProviderNetworkId: "go-team1",
		CIDR:              "10.0.10.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		VLANTag:           0,
		SpaceProviderId:   "",
	}})
}

func (s *environNetSuite) TestSpecificInstanceAndRestrictedSubnets(c *gc.C) {
	s.cannedData()
	s.FakeEnviron.Insts = []instance.Instance{s.NewInstance(c, "moana")}

	subnets, err := s.NetEnv.Subnets(instance.Id("moana"), []network.Id{"go-team"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []network.SubnetInfo{{
		ProviderId:        "go-team",
		ProviderNetworkId: "go-team1",
		CIDR:              "10.0.10.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		VLANTag:           0,
		SpaceProviderId:   "",
	}})
}

func (s *environNetSuite) TestSpecificInstanceAndRestrictedSubnetsWithMissing(c *gc.C) {
	s.cannedData()
	s.FakeEnviron.Insts = []instance.Instance{s.NewInstance(c, "moana")}

	subnets, err := s.NetEnv.Subnets(instance.Id("moana"), []network.Id{"go-team", "shellac"})
	c.Assert(err, gc.ErrorMatches, `subnets \[shellac\] not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(subnets, gc.IsNil)
}

func (s *environNetSuite) TestInterfaces(c *gc.C) {
	s.cannedData()
	s.FakeEnviron.Insts = []instance.Instance{s.NewInstance(c, "moana")}

	infos, err := s.NetEnv.NetworkInterfaces(instance.Id("moana"))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(infos, gc.DeepEquals, []network.InterfaceInfo{{
		DeviceIndex:       0,
		CIDR:              "10.0.10.0/24",
		ProviderId:        "moana/0",
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
	s.FakeEnviron.Insts = []instance.Instance{s.NewInstanceFromBase(baseInst)}

	infos, err := s.NetEnv.NetworkInterfaces(instance.Id("moana"))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(infos, gc.DeepEquals, []network.InterfaceInfo{{
		DeviceIndex:       0,
		CIDR:              "10.0.10.0/24",
		ProviderId:        "moana/0",
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
		ProviderId:        "moana/1",
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
