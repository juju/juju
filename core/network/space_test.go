// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
)

type spaceSuite struct {
	testhelpers.IsolationSuite

	spaces network.SpaceInfos
}

func TestSpaceSuite(t *testing.T) {
	tc.Run(t, &spaceSuite{})
}

func (s *spaceSuite) SetUpTest(c *tc.C) {
	s.spaces = network.SpaceInfos{
		{ID: "1", Name: "space1", Subnets: []network.SubnetInfo{{ID: "11", CIDR: "10.0.0.0/24"}}},
		{ID: "2", Name: "space2", Subnets: []network.SubnetInfo{{ID: "12", CIDR: "10.0.1.0/24"}}},
		{ID: "3", Name: "space3", Subnets: []network.SubnetInfo{{ID: "13", CIDR: "10.0.2.0/24"}}},
	}
}

func (s *spaceSuite) TestString(c *tc.C) {
	result := s.spaces.String()
	c.Assert(result, tc.Equals, `"space1", "space2", "space3"`)
}

func (s *spaceSuite) TestGetByName(c *tc.C) {
	c.Assert(s.spaces.GetByName("space1"), tc.NotNil)
	c.Assert(s.spaces.GetByName("space666"), tc.IsNil)
}

func (s *spaceSuite) TestGetByID(c *tc.C) {
	c.Assert(s.spaces.GetByID("1"), tc.NotNil)
	c.Assert(s.spaces.GetByID("999"), tc.IsNil)
}

func (s *spaceSuite) TestContainsName(c *tc.C) {
	c.Assert(s.spaces.ContainsName("space3"), tc.IsTrue)
	c.Assert(s.spaces.ContainsName("space666"), tc.IsFalse)
}

func (s *spaceSuite) TestMinus(c *tc.C) {
	infos := network.SpaceInfos{
		{ID: "2", Name: "space2"},
		{ID: "3", Name: "space3"},
	}
	result := s.spaces.Minus(infos)
	c.Assert(result, tc.DeepEquals, network.SpaceInfos{s.spaces[0]})
}

func (s *spaceSuite) TestMinusNoDiff(c *tc.C) {
	infos := network.SpaceInfos{
		{ID: "1", Name: "space1"},
		{ID: "2", Name: "space2"},
		{ID: "3", Name: "space3"},
	}
	result := s.spaces.Minus(infos)
	c.Assert(result, tc.DeepEquals, network.SpaceInfos{})
}

func (s *spaceSuite) TestInferSpaceFromAddress(c *tc.C) {
	queries := map[string]network.SpaceName{
		"10.0.0.42": "space1",
		"10.0.1.1":  "space2",
		"10.0.2.99": "space3",
	}

	for addr, expSpaceName := range queries {
		si, err := s.spaces.InferSpaceFromAddress(addr)
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("infer space for address %q", addr))
		c.Assert(si.Name, tc.Equals, expSpaceName, tc.Commentf("infer space for address %q", addr))
	}

	// Check that CIDR collisions are detected
	s.spaces = append(
		s.spaces,
		network.SpaceInfo{ID: "-3", Name: "inverse", Subnets: []network.SubnetInfo{{CIDR: "10.0.2.0/24"}}},
	)

	_, err := s.spaces.InferSpaceFromAddress("10.0.2.255")
	c.Assert(err, tc.ErrorMatches, ".*address matches the same CIDR in multiple spaces")

	// Check for no-match-found
	_, err = s.spaces.InferSpaceFromAddress("99.99.99.99")
	c.Assert(err, tc.ErrorMatches, ".*unable to infer space for address.*")
}

func (s *spaceSuite) TestInferSpaceFromCIDRAndSubnetID(c *tc.C) {
	infos := network.SpaceInfos{
		{ID: "1", Name: "space1", Subnets: []network.SubnetInfo{{CIDR: "10.0.0.0/24", ProviderId: "1"}}},
		{ID: "2", Name: "space2", Subnets: []network.SubnetInfo{{CIDR: "10.0.1.0/24", ProviderId: "2"}}},
	}

	si, err := infos.InferSpaceFromCIDRAndSubnetID("10.0.0.0/24", "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(si.Name, tc.Equals, network.SpaceName("space1"))

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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(si.Name, tc.Equals, network.SpaceName("space2"))

	si, err = infos.InferSpaceFromCIDRAndSubnetID("10.0.1.0/24", "3")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(si.Name, tc.Equals, network.SpaceName("inverse"))

	// Check for no-match-found
	_, err = infos.InferSpaceFromCIDRAndSubnetID("10.0.1.0/24", "42")
	c.Assert(err, tc.ErrorMatches, ".*unable to infer space.*")
}

func (s *spaceSuite) TestAllSubnetInfos(c *tc.C) {
	subnets, err := s.spaces.AllSubnetInfos()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(subnets, tc.DeepEquals, network.SubnetInfos{
		{ID: "11", CIDR: "10.0.0.0/24"},
		{ID: "12", CIDR: "10.0.1.0/24"},
		{ID: "13", CIDR: "10.0.2.0/24"},
	})
}

func (s *spaceSuite) TestMoveSubnets(c *tc.C) {
	_, err := s.spaces.MoveSubnets(network.MakeIDSet("11", "12"), "space4")
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)

	_, err = s.spaces.MoveSubnets(network.MakeIDSet("666"), "space3")
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)

	spaces, err := s.spaces.MoveSubnets(network.MakeIDSet("11", "12"), "space3")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spaces, tc.DeepEquals, network.SpaceInfos{
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
	c.Assert(s.spaces, tc.DeepEquals, network.SpaceInfos{
		{ID: "1", Name: "space1", Subnets: []network.SubnetInfo{{ID: "11", CIDR: "10.0.0.0/24"}}},
		{ID: "2", Name: "space2", Subnets: []network.SubnetInfo{{ID: "12", CIDR: "10.0.1.0/24"}}},
		{ID: "3", Name: "space3", Subnets: []network.SubnetInfo{{ID: "13", CIDR: "10.0.2.0/24"}}},
	})
}

func (s *spaceSuite) TestSubnetCIDRsBySpaceID(c *tc.C) {
	res := s.spaces.SubnetCIDRsBySpaceID()
	c.Assert(res, tc.DeepEquals, map[string][]string{
		"1": {"10.0.0.0/24"},
		"2": {"10.0.1.0/24"},
		"3": {"10.0.2.0/24"},
	})
}

func (s *spaceSuite) TestConvertSpaceName(c *tc.C) {
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
		c.Check(result, tc.Equals, test.expected)
	}
}

// This test guarantees that the AlphaSpaceId is a crafted, well-known v5 UUID
// using a Juju namespace and a fixed string ("juju.network.space.alpha").
func (s *spaceSuite) TestAlphaSpaceID(c *tc.C) {
	// Juju UUID namespace that we (should) use for all Juju well-known UUIDs.
	jujuUUIDNamespace := "96bb15e6-8b85-448b-9fce-ede1a1700e64"
	namespaceUUID, err := uuid.Parse(jujuUUIDNamespace)
	c.Assert(err, tc.ErrorIsNil)

	alphaSpaceUUID := uuid.NewSHA1(namespaceUUID, []byte("juju.network.space.alpha"))
	c.Assert(alphaSpaceUUID.String(), tc.Equals, network.AlphaSpaceId)
}

func (s *spaceSuite) TestAlphaSpaceName(c *tc.C) {
	c.Assert(network.AlphaSpaceName, tc.Equals, "alpha")
}
