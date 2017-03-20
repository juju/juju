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

func (s *environNetSuite) cannedZones() {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusUp, "", ""),
		google.NewZone("b-zone", google.StatusUp, "", ""),
	}
}

func (s environNetSuite) cannedSubnets() {
	s.FakeConn.Subnets = []*compute.Subnetwork{{
		Id:          1234,
		IpCidrRange: "10.0.10.0/24",
		Name:        "go-team",
		Network:     "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/go-team",
		Region:      "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1",
		SelfLink:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/go-team",
	}, {
		Id:          1235,
		IpCidrRange: "10.0.20.0/24",
		Name:        "shellac",
		Network:     "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/shellac",
		Region:      "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1",
		SelfLink:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/shellac",
	}}
}

func (s *environNetSuite) TestGettingAllSubnets(c *gc.C) {
	s.cannedZones()
	s.cannedSubnets()

	subnets, err := s.NetEnv.Subnets(instance.UnknownId, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []network.SubnetInfo{{
		ProviderId:        "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/go-team",
		CIDR:              "10.0.10.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
	}, {
		ProviderId:        "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/shellac",
		CIDR:              "10.0.20.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
	}})
}

func (s *environNetSuite) TestRestrictingToSubnets(c *gc.C) {
	s.cannedZones()
	s.cannedSubnets()

	subnets, err := s.NetEnv.Subnets(instance.UnknownId, []network.Id{
		"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/shellac",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.DeepEquals, []network.SubnetInfo{{
		ProviderId:        "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/shellac",
		CIDR:              "10.0.20.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
	}})
}

func (s *environNetSuite) TestRestrictingToSubnetsWithMissing(c *gc.C) {
	s.cannedZones()
	s.cannedSubnets()

	subnets, err := s.NetEnv.Subnets(instance.UnknownId, []network.Id{
		"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/shellac",
		"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/brunettes",
	})
	c.Assert(err, gc.ErrorMatches, `subnets \[https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/brunettes\] not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(subnets, gc.IsNil)
}

func (s *environNetSuite) TestSpecificInstance(c *gc.C) {
	s.cannedZones()
	s.cannedSubnets()
	s.FakeEnviron.Insts = []instance.Instance{s.NewInstance(c, "moana")}

	subnets, err := s.NetEnv.Subnets(instance.Id("moana"), nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []network.SubnetInfo{{
		ProviderId:        "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/go-team",
		CIDR:              "10.0.10.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
	}})
}

func (s *environNetSuite) TestSpecificInstanceAndRestrictedSubnets(c *gc.C) {
	s.cannedZones()
	s.cannedSubnets()
	s.FakeEnviron.Insts = []instance.Instance{s.NewInstance(c, "moana")}

	subnets, err := s.NetEnv.Subnets(instance.Id("moana"), []network.Id{
		"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/go-team",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, []network.SubnetInfo{{
		ProviderId:        "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/go-team",
		CIDR:              "10.0.10.0/24",
		AvailabilityZones: []string{"a-zone", "b-zone"},
	}})
}

func (s *environNetSuite) TestSpecificInstanceAndRestrictedSubnetsWithMissing(c *gc.C) {
	s.cannedZones()
	s.cannedSubnets()
	s.FakeEnviron.Insts = []instance.Instance{s.NewInstance(c, "moana")}

	subnets, err := s.NetEnv.Subnets(instance.Id("moana"), []network.Id{
		"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/go-team",
		"https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/shellac",
	})
	c.Assert(err, gc.ErrorMatches, `subnets \[https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/shellac\] not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(subnets, gc.IsNil)
}

func (s *environNetSuite) TestInterfaces(c *gc.C) {
	s.cannedZones()
	s.cannedSubnets()
	s.FakeEnviron.Insts = []instance.Instance{s.NewInstance(c, "moana")}

	infos, err := s.NetEnv.NetworkInterfaces(instance.Id("moana"))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(infos, gc.DeepEquals, []network.InterfaceInfo{{
		DeviceIndex: 0,
		CIDR:        "10.0.10.0/24",
		// XXX(xtian): not sure about this - the network interface has
		// no id in GCE, each machine has exactly one interface, so it
		// can be identified by the machine's id.
		ProviderId:        "moana",
		ProviderSubnetId:  "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/go-team",
		AvailabilityZones: []string{"a-zone", "b-zone"},
		InterfaceName:     "somenetif",
		InterfaceType:     network.EthernetInterface,
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        network.ConfigDHCP,
		Address:           network.NewScopedAddress("10.0.10.3", network.ScopeCloudLocal),
	}})
}
