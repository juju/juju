// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

type SpacesDiscoverySuite struct {
	ConnSuite
}

var _ = gc.Suite(&SpacesDiscoverySuite{})

var twoSubnets = []network.SubnetInfo{
	{
		ProviderId:        "1",
		AvailabilityZones: []string{"1", "2"},
		CIDR:              "10.0.0.1/24",
	},
	{
		ProviderId:        "2",
		AvailabilityZones: []string{"3", "4"},
		CIDR:              "10.100.30.1/24",
	},
}

var twoSubnetsAndIgnored = []network.SubnetInfo{
	{
		ProviderId:        "1",
		AvailabilityZones: []string{"1", "2"},
		CIDR:              "10.0.0.1/24",
	},
	{
		ProviderId:        "2",
		AvailabilityZones: []string{"3", "4"},
		CIDR:              "10.100.30.1/24",
	},
	// Interface-local multicast:
	{
		ProviderId:        "foo",
		AvailabilityZones: []string{"bar", "baz"},
		CIDR:              "ff51:dead:beef::/48",
	},
	// Link-local multicast:
	{
		ProviderId:        "moo",
		AvailabilityZones: []string{"bar", "baz"},
		CIDR:              "ff32:dead:beef::/48",
	},
	// IPv6 link-local unicast:
	{
		ProviderId:        "baa",
		AvailabilityZones: []string{"bar", "baz"},
		CIDR:              "fe80:dead:beef::/48",
	},
	// IPv4 link-local unicast:
	{
		ProviderId:        "maa",
		AvailabilityZones: []string{"bar", "baz"},
		CIDR:              "169.254.13.0/24",
	},
}

var anotherTwoSubnets = []network.SubnetInfo{
	{
		ProviderId:        "3",
		AvailabilityZones: []string{"5", "6"},
		CIDR:              "10.101.0.1/24",
	},
	{
		ProviderId:        "4",
		AvailabilityZones: []string{"7", "8"},
		CIDR:              "10.105.0.1/24",
	},
}

var fourSubnets = []network.SubnetInfo{
	{
		ProviderId:        "1",
		AvailabilityZones: []string{"1", "2"},
		CIDR:              "10.0.0.1/24",
	},
	{
		ProviderId:        "2",
		AvailabilityZones: []string{"3", "4"},
		CIDR:              "10.100.30.1/24",
	},
	{
		ProviderId:        "3",
		AvailabilityZones: []string{"5", "6"},
		CIDR:              "10.101.0.1/24",
	},
	{
		ProviderId:        "4",
		AvailabilityZones: []string{"7", "8"},
		CIDR:              "10.105.0.1/24",
	},
}

var spaceOne = []network.SpaceInfo{
	{
		Name:       "space1",
		ProviderId: "1",
		Subnets:    twoSubnets,
	},
}
var spaceOneAndIgnored = []network.SpaceInfo{
	{
		Name:       "space1",
		ProviderId: "1",
		Subnets:    twoSubnetsAndIgnored,
	},
}
var spaceTwo = []network.SpaceInfo{
	{
		Name:       "space2",
		ProviderId: "2",
		Subnets:    anotherTwoSubnets,
	},
}

var twoSpaces = []network.SpaceInfo{spaceOne[0], spaceTwo[0]}

var twoSubnetsAfterFAN = []network.SubnetInfo{
	{
		ProviderId:        "1",
		AvailabilityZones: []string{"1", "2"},
		CIDR:              "10.0.0.1/24",
	},
	{
		ProviderId:        "2",
		AvailabilityZones: []string{"3", "4"},
		CIDR:              "10.100.30.1/24",
	},
	{
		ProviderId:        corenetwork.Id(fmt.Sprintf("2-%s-10-100-30-0-24", corenetwork.InFan)),
		AvailabilityZones: []string{"3", "4"},
		CIDR:              "253.30.0.0/16",
	},
}

var spaceOneAfterFAN = []network.SpaceInfo{
	{
		Name:       "space1",
		ProviderId: "1",
		Subnets:    twoSubnetsAfterFAN,
	},
}

func checkSubnetsEqual(c *gc.C, subnets []*state.Subnet, subnetInfos []network.SubnetInfo) {
	c.Assert(len(subnetInfos), gc.Equals, len(subnets))
	for i, subnetInfo := range subnetInfos {
		subnet := subnets[i]
		c.Check(subnetInfo.CIDR, gc.Equals, subnet.CIDR())
		c.Check(subnetInfo.AvailabilityZones, gc.DeepEquals, subnet.AvailabilityZones())
		c.Check(subnetInfo.ProviderId, gc.Equals, subnet.ProviderId())
		c.Check(subnetInfo.ProviderNetworkId, gc.Equals, subnet.ProviderNetworkId())
		c.Check(subnetInfo.VLANTag, gc.Equals, subnet.VLANTag())
	}
}

func checkSpacesEqual(c *gc.C, spaces []*state.Space, spaceInfos []network.SpaceInfo) {
	// Filter out the default space for comparisons.
	filtered := spaces[:0]
	for _, s := range spaces {
		if s.Name() != network.AlphaSpaceName {
			filtered = append(filtered, s)
		}
	}

	c.Assert(len(spaceInfos), gc.Equals, len(filtered))
	for i, spaceInfo := range spaceInfos {
		space := filtered[i]
		c.Check(string(spaceInfo.Name), gc.Equals, space.Name())
		c.Check(spaceInfo.ProviderId, gc.Equals, space.ProviderId())
		subnets, err := space.Subnets()
		c.Assert(err, jc.ErrorIsNil)
		checkSubnetsEqual(c, subnets, spaceInfo.Subnets)
	}
}

func (s *SpacesDiscoverySuite) TestSaveSubnetsFromProvider(c *gc.C) {
	err := s.State.SaveSubnetsFromProvider(twoSubnets, "")
	c.Check(err, jc.ErrorIsNil)

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)
	checkSubnetsEqual(c, subnets, twoSubnets)
}

// TODO(wpk) 2017-05-24 this test will have to be rewritten when we support removing spaces/subnets in discovery.
func (s *SpacesDiscoverySuite) TestSaveSubnetsFromProviderOnlyAddsSubnets(c *gc.C) {
	err := s.State.SaveSubnetsFromProvider(twoSubnets, "")
	c.Check(err, jc.ErrorIsNil)

	err = s.State.SaveSubnetsFromProvider(anotherTwoSubnets, "")
	c.Check(err, jc.ErrorIsNil)

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)
	checkSubnetsEqual(c, subnets, fourSubnets)
}

func (s *SpacesDiscoverySuite) TestSaveSubnetsFromProviderUpdatesSubnets(c *gc.C) {
	err := s.State.SaveSubnetsFromProvider(twoSubnets, "")
	c.Check(err, jc.ErrorIsNil)

	err = s.State.SaveSpacesFromProvider(spaceOne)
	c.Assert(err, jc.ErrorIsNil)

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)
	twoSubnetsWithSpace := twoSubnets
	twoSubnetsWithSpace[0].ProviderSpaceId = spaceOne[0].ProviderId
	twoSubnetsWithSpace[1].ProviderSpaceId = spaceOne[0].ProviderId
	checkSubnetsEqual(c, subnets, twoSubnetsWithSpace)
}

func (s *SpacesDiscoverySuite) TestSaveSubnetsFromProviderOnlyIdempotent(c *gc.C) {
	err := s.State.SaveSubnetsFromProvider(twoSubnets, "")
	c.Check(err, jc.ErrorIsNil)

	subnets1, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SaveSubnetsFromProvider(twoSubnets, "")
	c.Check(err, jc.ErrorIsNil)

	subnets2, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnets1, jc.DeepEquals, subnets2)
}

func (s *SpacesDiscoverySuite) TestSaveSubnetsFromProviderWithFAN(c *gc.C) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{"fan-config": "10.100.0.0/16=253.0.0.0/8"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SaveSubnetsFromProvider(twoSubnets, "")
	c.Check(err, jc.ErrorIsNil)

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)

	checkSubnetsEqual(c, subnets, twoSubnetsAfterFAN)
}

func (s *SpacesDiscoverySuite) TestSaveSubnetsFromProviderIgnoredWithFAN(c *gc.C) {
	// This is just a test configuration. This configuration may be
	// considered invalid in the future. Here we show that this
	// configuration is ignored.
	err := s.Model.UpdateModelConfig(
		map[string]interface{}{"fan-config": "fe80:dead:beef::/48=fe80:dead:beef::/24"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SaveSubnetsFromProvider(twoSubnetsAndIgnored, "")
	c.Check(err, jc.ErrorIsNil)

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)

	checkSubnetsEqual(c, subnets, twoSubnets)
}

func (s *SpacesDiscoverySuite) TestSaveSubnetsFromProviderIgnored(c *gc.C) {
	err := s.State.SaveSubnetsFromProvider(twoSubnetsAndIgnored, "")
	c.Check(err, jc.ErrorIsNil)

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)

	checkSubnetsEqual(c, subnets, twoSubnets)
}

func (s *SpacesDiscoverySuite) TestSaveSpacesFromProvider(c *gc.C) {
	err := s.State.SaveSpacesFromProvider(spaceOne)
	c.Check(err, jc.ErrorIsNil)

	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	checkSpacesEqual(c, spaces, spaceOne)
}

// TODO(wpk) 2017-05-24 this test will have to be rewritten when we support removing spaces/subnets in discovery.
func (s *SpacesDiscoverySuite) TestSaveSpacesFromProviderAddsSpaces(c *gc.C) {
	err := s.State.SaveSpacesFromProvider(spaceOne)
	c.Check(err, jc.ErrorIsNil)

	err = s.State.SaveSpacesFromProvider(spaceTwo)
	c.Check(err, jc.ErrorIsNil)

	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	checkSpacesEqual(c, spaces, twoSpaces)
}

func (s *SpacesDiscoverySuite) TestSaveSpacesFromProviderIdempotent(c *gc.C) {
	err := s.State.SaveSpacesFromProvider(twoSpaces)
	c.Check(err, jc.ErrorIsNil)

	spaces1, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SaveSpacesFromProvider(twoSpaces)
	c.Check(err, jc.ErrorIsNil)

	spaces2, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces1, gc.DeepEquals, spaces2)
}

func (s *SpacesDiscoverySuite) TestSaveSpacesFromProviderWithFAN(c *gc.C) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{"fan-config": "10.100.0.0/16=253.0.0.0/8"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SaveSpacesFromProvider(spaceOne)
	c.Check(err, jc.ErrorIsNil)

	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	checkSpacesEqual(c, spaces, spaceOneAfterFAN)
}

func (s *SpacesDiscoverySuite) TestReloadSpacesIgnored(c *gc.C) {
	err := s.State.SaveSpacesFromProvider(spaceOneAndIgnored)
	c.Check(err, jc.ErrorIsNil)

	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	checkSpacesEqual(c, spaces, spaceOne)
}
