// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
)

type spaceSuite struct {
	testing.IsolationSuite

	spaces network.SpaceInfos
}

var _ = gc.Suite(&spaceSuite{})

func (s *spaceSuite) SetUpTest(c *gc.C) {
	s.spaces = network.SpaceInfos{
		{ID: "1", Name: "space1", Subnets: []network.SubnetInfo{{ID: "11", CIDR: "10.0.0.0/24"}}},
		{ID: "2", Name: "space2", Subnets: []network.SubnetInfo{{ID: "12", CIDR: "10.0.1.0/24"}}},
		{ID: "3", Name: "space3", Subnets: []network.SubnetInfo{{ID: "13", CIDR: "10.0.2.0/24"}}},
	}
}

func (s *spaceSuite) TestString(c *gc.C) {
	result := s.spaces.String()
	c.Assert(result, gc.Equals, `"space1", "space2", "space3"`)
}

func (s *spaceSuite) TestGetByName(c *gc.C) {
	c.Assert(s.spaces.GetByName("space1"), gc.NotNil)
	c.Assert(s.spaces.GetByName("space666"), gc.IsNil)
}

func (s *spaceSuite) TestGetByID(c *gc.C) {
	c.Assert(s.spaces.GetByID("1"), gc.NotNil)
	c.Assert(s.spaces.GetByID("999"), gc.IsNil)
}

func (s *spaceSuite) TestContainsName(c *gc.C) {
	c.Assert(s.spaces.ContainsName("space3"), jc.IsTrue)
	c.Assert(s.spaces.ContainsName("space666"), jc.IsFalse)
}

func (s *spaceSuite) TestMinus(c *gc.C) {
	infos := network.SpaceInfos{
		{ID: "2", Name: "space2"},
		{ID: "3", Name: "space3"},
	}
	result := s.spaces.Minus(infos)
	c.Assert(result, gc.DeepEquals, network.SpaceInfos{s.spaces[0]})
}

func (s *spaceSuite) TestMinusNoDiff(c *gc.C) {
	infos := network.SpaceInfos{
		{ID: "1", Name: "space1"},
		{ID: "2", Name: "space2"},
		{ID: "3", Name: "space3"},
	}
	result := s.spaces.Minus(infos)
	c.Assert(result, gc.DeepEquals, network.SpaceInfos{})
}

func (s *spaceSuite) TestInferSpaceFromAddress(c *gc.C) {
	queries := map[string]network.SpaceName{
		"10.0.0.42": "space1",
		"10.0.1.1":  "space2",
		"10.0.2.99": "space3",
	}

	for addr, expSpaceName := range queries {
		si, err := s.spaces.InferSpaceFromAddress(addr)
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("infer space for address %q", addr))
		c.Assert(si.Name, gc.Equals, expSpaceName, gc.Commentf("infer space for address %q", addr))
	}

	// Check that CIDR collisions are detected
	s.spaces = append(
		s.spaces,
		network.SpaceInfo{ID: "-3", Name: "inverse", Subnets: []network.SubnetInfo{{CIDR: "10.0.2.0/24"}}},
	)

	_, err := s.spaces.InferSpaceFromAddress("10.0.2.255")
	c.Assert(err, gc.ErrorMatches, ".*address matches the same CIDR in multiple spaces")

	// Check for no-match-found
	_, err = s.spaces.InferSpaceFromAddress("99.99.99.99")
	c.Assert(err, gc.ErrorMatches, ".*unable to infer space for address.*")
}

func (s *spaceSuite) TestInferSpaceFromCIDRAndSubnetID(c *gc.C) {
	infos := network.SpaceInfos{
		{ID: "1", Name: "space1", Subnets: []network.SubnetInfo{{CIDR: "10.0.0.0/24", ProviderId: "1"}}},
		{ID: "2", Name: "space2", Subnets: []network.SubnetInfo{{CIDR: "10.0.1.0/24", ProviderId: "2"}}},
	}

	si, err := infos.InferSpaceFromCIDRAndSubnetID("10.0.0.0/24", "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Name, gc.Equals, network.SpaceName("space1"))

	// Check for same CIDR/different provider
	infos = append(
		infos,
		network.SpaceInfo{
			ID:      "-2",
			Name:    "inverse",
			Subnets: []network.SubnetInfo{{CIDR: "10.0.1.0/24", ProviderId: "3"}},
		},
	)

	si, err = infos.InferSpaceFromCIDRAndSubnetID("10.0.1.0/24", "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Name, gc.Equals, network.SpaceName("space2"))

	si, err = infos.InferSpaceFromCIDRAndSubnetID("10.0.1.0/24", "3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Name, gc.Equals, network.SpaceName("inverse"))

	// Check for no-match-found
	_, err = infos.InferSpaceFromCIDRAndSubnetID("10.0.1.0/24", "42")
	c.Assert(err, gc.ErrorMatches, ".*unable to infer space.*")
}

func (s *spaceSuite) TestAllSubnetInfos(c *gc.C) {
	subnets, err := s.spaces.AllSubnetInfos()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, gc.DeepEquals, network.SubnetInfos{
		{ID: "11", CIDR: "10.0.0.0/24"},
		{ID: "12", CIDR: "10.0.1.0/24"},
		{ID: "13", CIDR: "10.0.2.0/24"},
	})
}

func (s *spaceSuite) TestMoveSubnets(c *gc.C) {
	_, err := s.spaces.MoveSubnets(network.MakeIDSet("11", "12"), "space4")
	c.Check(err, jc.ErrorIs, coreerrors.NotFound)

	_, err = s.spaces.MoveSubnets(network.MakeIDSet("666"), "space3")
	c.Check(err, jc.ErrorIs, coreerrors.NotFound)

	spaces, err := s.spaces.MoveSubnets(network.MakeIDSet("11", "12"), "space3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spaces, gc.DeepEquals, network.SpaceInfos{
		{ID: "1", Name: "space1", Subnets: nil},
		{ID: "2", Name: "space2", Subnets: nil},
		{
			ID:   "3",
			Name: "space3",
			Subnets: network.SubnetInfos{
				{ID: "13", CIDR: "10.0.2.0/24"},
				{ID: "11", CIDR: "10.0.0.0/24", SpaceID: "3", SpaceName: "space3"},
				{ID: "12", CIDR: "10.0.1.0/24", SpaceID: "3", SpaceName: "space3"},
			},
		},
	})

	// Ensure the original was not mutated.
	c.Assert(s.spaces, gc.DeepEquals, network.SpaceInfos{
		{ID: "1", Name: "space1", Subnets: []network.SubnetInfo{{ID: "11", CIDR: "10.0.0.0/24"}}},
		{ID: "2", Name: "space2", Subnets: []network.SubnetInfo{{ID: "12", CIDR: "10.0.1.0/24"}}},
		{ID: "3", Name: "space3", Subnets: []network.SubnetInfo{{ID: "13", CIDR: "10.0.2.0/24"}}},
	})
}

func (s *spaceSuite) TestSubnetCIDRsBySpaceID(c *gc.C) {
	res := s.spaces.SubnetCIDRsBySpaceID()
	c.Assert(res, gc.DeepEquals, map[string][]string{
		"1": {"10.0.0.0/24"},
		"2": {"10.0.1.0/24"},
		"3": {"10.0.2.0/24"},
	})
}

func (s *spaceSuite) TestConvertSpaceName(c *gc.C) {
	empty := set.Strings{}
	nameTests := []struct {
		name     string
		existing set.Strings
		expected string
	}{
		{"foo", empty, "foo"},
		{"foo1", empty, "foo1"},
		{"Foo Thing", empty, "foo-thing"},
		{"foo^9*//++!!!!", empty, "foo9"},
		{"--Foo", empty, "foo"},
		{"---^^&*()!", empty, "empty"},
		{" ", empty, "empty"},
		{"", empty, "empty"},
		{"foo\u2318", empty, "foo"},
		{"foo--", empty, "foo"},
		{"-foo--foo----bar-", empty, "foo-foo-bar"},
		{"foo-", set.NewStrings("foo", "bar", "baz"), "foo-2"},
		{"foo", set.NewStrings("foo", "foo-2"), "foo-3"},
		{"---", set.NewStrings("empty"), "empty-2"},
	}
	for _, test := range nameTests {
		result := network.ConvertSpaceName(test.name, test.existing)
		c.Check(result, gc.Equals, test.expected)
	}
}
