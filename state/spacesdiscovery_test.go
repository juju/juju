// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type networkLessEnviron struct {
	environs.Environ
}

type networkedEnviron struct {
	environs.NetworkingEnviron

	stub           *testing.Stub
	spaceDiscovery bool
	spaces         []network.SpaceInfo
	subnets        []network.SubnetInfo

	callCtxUsed context.ProviderCallContext
}

type SpacesDiscoverySuite struct {
	ConnSuite

	environ     networkedEnviron
	usedEnviron environs.Environ
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
		ProviderId:        "2-INFAN-10-100-30-0-24",
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
		if len(subnetInfo.AvailabilityZones) > 0 {
			c.Check(subnetInfo.AvailabilityZones[0], gc.Equals, subnet.AvailabilityZone())
		} else {
			c.Check(subnet.AvailabilityZone(), gc.Equals, "")
		}
		c.Check(subnetInfo.ProviderId, gc.Equals, subnet.ProviderId())
		c.Check(subnetInfo.ProviderNetworkId, gc.Equals, subnet.ProviderNetworkId())
		c.Check(subnetInfo.VLANTag, gc.Equals, subnet.VLANTag())
	}
}

func checkSpacesEqual(c *gc.C, spaces []*state.Space, spaceInfos []network.SpaceInfo) {
	// Filter out the default space for comparisons.
	filtered := spaces[:0]
	for _, s := range spaces {
		if s.Name() != environs.DefaultSpaceName {
			filtered = append(filtered, s)
		}
	}

	c.Assert(len(spaceInfos), gc.Equals, len(filtered))
	for i, spaceInfo := range spaceInfos {
		space := filtered[i]
		c.Check(spaceInfo.Name, gc.Equals, space.Name())
		c.Check(spaceInfo.ProviderId, gc.Equals, space.ProviderId())
		subnets, err := space.Subnets()
		c.Assert(err, jc.ErrorIsNil)
		checkSubnetsEqual(c, subnets, spaceInfo.Subnets)
	}
}

func (e *networkedEnviron) Subnets(ctx context.ProviderCallContext, inst instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	e.stub.AddCall("Subnets", ctx, inst, subnetIds)
	e.callCtxUsed = ctx
	return e.subnets, e.stub.NextErr()
}
func (e *networkedEnviron) Spaces(ctx context.ProviderCallContext) ([]network.SpaceInfo, error) {
	e.stub.AddCall("Spaces", ctx)
	e.callCtxUsed = ctx
	return e.spaces, e.stub.NextErr()
}

func (e *networkedEnviron) SupportsSpaceDiscovery(ctx context.ProviderCallContext) (bool, error) {
	e.stub.AddCall("SupportsSpaceDiscovery", ctx)
	e.callCtxUsed = ctx
	return e.spaceDiscovery, e.stub.NextErr()
}

func (s *SpacesDiscoverySuite) TestReloadSpacesNetworklessEnviron(c *gc.C) {
	err := s.State.ReloadSpaces(networkLessEnviron{})
	c.Check(err, gc.ErrorMatches, "spaces discovery in a non-networking environ not supported")
}

func (s *SpacesDiscoverySuite) TestReloadSpacesSupportsSpaceDiscoveryBroken(c *gc.C) {
	s.environ = networkedEnviron{
		stub: &testing.Stub{},
	}
	s.environ.stub.SetErrors(errors.New("SupportsSpaceDiscovery is broken"))
	s.usedEnviron = &s.environ
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Check(err, gc.ErrorMatches, "SupportsSpaceDiscovery is broken")
}

func (s *SpacesDiscoverySuite) TestReloadSpacesSubnetsOnly(c *gc.C) {
	s.environ = networkedEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: false,
		subnets:        twoSubnets,
	}
	s.usedEnviron = &s.environ
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Check(err, jc.ErrorIsNil)
	s.environ.stub.CheckCallNames(c, "SupportsSpaceDiscovery", "Subnets")
	s.environ.stub.CheckCall(c, 1, "Subnets", s.environ.callCtxUsed, instance.UnknownId, []network.Id{})

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)
	checkSubnetsEqual(c, subnets, twoSubnets)
}

func (s *SpacesDiscoverySuite) TestReloadSpacesSubnetsOnlySubnetsBroken(c *gc.C) {
	s.environ = networkedEnviron{
		stub: &testing.Stub{},
	}
	s.environ.stub.SetErrors(nil, errors.New("Subnets is broken"))
	s.usedEnviron = &s.environ
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Check(err, gc.ErrorMatches, "Subnets is broken")
	s.environ.stub.CheckCallNames(c, "SupportsSpaceDiscovery", "Subnets")
	s.environ.stub.CheckCall(c, 1, "Subnets", s.environ.callCtxUsed, instance.UnknownId, []network.Id{})
}

// TODO(wpk) 2017-05-24 this test will have to be rewritten when we support removing spaces/subnets in discovery.
func (s *SpacesDiscoverySuite) TestReloadSpacesSubnetsOnlyAddsSubnets(c *gc.C) {
	s.environ = networkedEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: false,
		subnets:        twoSubnets,
	}
	s.usedEnviron = &s.environ
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	s.environ.subnets = anotherTwoSubnets
	err = s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)
	checkSubnetsEqual(c, subnets, fourSubnets)
}

// TODO(wpk) 2017-05-24 this test will have to be enabled only when we we support removing spaces/subnets in discovery.
func (s *SpacesDiscoverySuite) TestReloadSpacesSubnetsOnlyReplacesSubnets(c *gc.C) {
	c.Skip("Removing subnets not supported")
	s.environ = networkedEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: false,
		subnets:        twoSubnets,
	}
	s.usedEnviron = &s.environ
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	s.environ.subnets = anotherTwoSubnets
	err = s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)
	checkSubnetsEqual(c, subnets, anotherTwoSubnets)
}

func (s *SpacesDiscoverySuite) TestReloadSpacesSubnetsOnlyIdempotent(c *gc.C) {
	s.environ = networkedEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: false,
		subnets:        twoSubnets,
	}
	s.usedEnviron = &s.environ
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	subnets1, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	subnets2, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnets1, gc.DeepEquals, subnets2)
}

func (s *SpacesDiscoverySuite) TestReloadSpacesSpacesBroken(c *gc.C) {
	s.environ = networkedEnviron{
		spaceDiscovery: true,
		stub:           &testing.Stub{},
	}
	s.environ.stub.SetErrors(nil, errors.New("Spaces is broken"))
	s.usedEnviron = &s.environ
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Check(err, gc.ErrorMatches, "Spaces is broken")
	s.environ.stub.CheckCallNames(c, "SupportsSpaceDiscovery", "Spaces")
}

func (s *SpacesDiscoverySuite) TestReloadSpaces(c *gc.C) {
	s.environ = networkedEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: true,
		spaces:         spaceOne,
	}
	s.usedEnviron = &s.environ
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Check(err, jc.ErrorIsNil)
	s.environ.stub.CheckCallNames(c, "SupportsSpaceDiscovery", "Spaces")

	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	checkSpacesEqual(c, spaces, spaceOne)
}

// TODO(wpk) 2017-05-24 this test will have to be rewritten when we support removing spaces/subnets in discovery.
func (s *SpacesDiscoverySuite) TestReloadSpacesAddsSpaces(c *gc.C) {
	s.environ = networkedEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: true,
		spaces:         spaceOne,
	}
	s.usedEnviron = &s.environ
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	s.environ.spaces = spaceTwo
	err = s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	checkSpacesEqual(c, spaces, twoSpaces)
}

// TODO(wpk) 2017-05-24 this test will have to be enabled only when we we support removing spaces/subnets in discovery.
func (s *SpacesDiscoverySuite) TestReloadSpacesReplacesSpaces(c *gc.C) {
	c.Skip("Removing spaces not supported")
	s.environ = networkedEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: true,
		spaces:         spaceOne,
	}
	s.usedEnviron = &s.environ
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	s.environ.spaces = spaceTwo
	err = s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	checkSpacesEqual(c, spaces, spaceTwo)
}

func (s *SpacesDiscoverySuite) TestReloadSpacesIdempotent(c *gc.C) {
	s.environ = networkedEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: true,
		spaces:         twoSpaces,
	}
	s.usedEnviron = &s.environ
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	spaces1, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	spaces2, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces1, gc.DeepEquals, spaces2)
}

func (s *SpacesDiscoverySuite) TestReloadSubnetsWithFAN(c *gc.C) {
	s.environ = networkedEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: false,
		subnets:        twoSubnets,
	}
	s.usedEnviron = &s.environ

	s.Model.UpdateModelConfig(map[string]interface{}{"fan-config": "10.100.0.0/16=253.0.0.0/8"}, nil)
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)

	checkSubnetsEqual(c, subnets, twoSubnetsAfterFAN)
}

func (s *SpacesDiscoverySuite) TestReloadSubnetsIgnoredWithFAN(c *gc.C) {
	s.environ = networkedEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: false,
		subnets:        twoSubnetsAndIgnored,
	}
	s.usedEnviron = &s.environ

	// This is just a test configuration. This configuration may be
	// considered invalid in the future. Here we show that this
	// configuration is ignored.
	s.Model.UpdateModelConfig(map[string]interface{}{"fan-config": "fe80:dead:beef::/48=fe80:dead:beef::/24"}, nil)
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)

	checkSubnetsEqual(c, subnets, twoSubnets)
}

func (s *SpacesDiscoverySuite) TestReloadSpacesWithFAN(c *gc.C) {
	s.environ = networkedEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: true,
		spaces:         spaceOne,
	}
	s.usedEnviron = &s.environ

	s.Model.UpdateModelConfig(map[string]interface{}{"fan-config": "10.100.0.0/16=253.0.0.0/8"}, nil)
	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	checkSpacesEqual(c, spaces, spaceOneAfterFAN)
}

func (s *SpacesDiscoverySuite) TestReloadSubnetsIgnored(c *gc.C) {
	s.environ = networkedEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: false,
		subnets:        twoSubnetsAndIgnored,
	}
	s.usedEnviron = &s.environ

	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	subnets, err := s.State.AllSubnets()
	c.Assert(err, jc.ErrorIsNil)

	checkSubnetsEqual(c, subnets, twoSubnets)
}

func (s *SpacesDiscoverySuite) TestReloadSpacesIgnored(c *gc.C) {
	s.environ = networkedEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: true,
		spaces:         spaceOneAndIgnored,
	}
	s.usedEnviron = &s.environ

	err := s.State.ReloadSpaces(s.usedEnviron)
	c.Assert(err, jc.ErrorIsNil)

	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	checkSpacesEqual(c, spaces, spaceOne)
}
