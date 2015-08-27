// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/state"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type SpacesSuite struct {
	ConnSuite
}

var _ = gc.Suite(&SpacesSuite{})

func (s *SpacesSuite) addSubnets(c *gc.C, CIDRs []string) {
	for i, cidr := range CIDRs {
		ip, ipNet, err := net.ParseCIDR(cidr)
		c.Assert(err, jc.ErrorIsNil)

		// Generate the high IP address from the CIDR
		// First create a copy of the low IP address
		highIp := ip

		// By default we always get 16 bytes for each IP address. We want to
		// reduce this to 4 if we were provided an IPv4 address.
		if ip.To4() != nil {
			highIp = ip.To4()
			ip = ip.To4()
		}

		// To generate a high IP address we bitwise not each byte of the subnet
		// mask and OR it to the low IP address.
		for i, b := range ipNet.Mask {
			if i < len(ip) {
				highIp[i] |= ^b
			}
		}

		providerId := fmt.Sprintf("ProviderId%d", i)
		subnetInfo := state.SubnetInfo{
			ProviderId:        providerId,
			CIDR:              cidr,
			VLANTag:           79,
			AllocatableIPLow:  ip.String(),
			AllocatableIPHigh: highIp.String(),
			AvailabilityZone:  "AvailabilityZone",
		}
		_, err = s.State.AddSubnet(subnetInfo)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *SpacesSuite) assertNoSpace(c *gc.C, name string) {
	_, err := s.State.Space(name)
	c.Assert(err, gc.ErrorMatches, "space \""+name+"\" not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func assertSpace(c *gc.C, space *state.Space, name string, subnets []string, isPublic bool) {
	c.Assert(space.Name(), gc.Equals, name)
	actualSubnets, err := space.Subnets()
	c.Assert(err, jc.ErrorIsNil)
	actualSubnetIds := make([]string, len(actualSubnets))
	for i, subnet := range actualSubnets {
		actualSubnetIds[i] = subnet.CIDR()
	}
	c.Assert(actualSubnetIds, jc.SameContents, subnets)
	c.Assert(state.SpaceDoc(space).IsPublic, gc.Equals, isPublic)

	c.Assert(space.Life(), gc.Equals, state.Alive)
	c.Assert(strings.HasSuffix(space.ID(), name), jc.IsTrue)
	c.Assert(space.String(), gc.Equals, name)
}

func (s *SpacesSuite) TestAddSpace(c *gc.C) {
	name := "my-space"
	subnets := []string{"1.1.1.0/24"}
	isPublic := false
	s.addSubnets(c, subnets)

	space, err := s.State.AddSpace(name, subnets, isPublic)
	c.Assert(err, jc.ErrorIsNil)
	assertSpace(c, space, name, subnets, isPublic)

	// We should get the same space back from the database
	id := space.ID()
	space, err = s.State.Space(name)
	c.Assert(err, jc.ErrorIsNil)
	assertSpace(c, space, name, subnets, isPublic)
	c.Assert(id, gc.Equals, space.ID())
}

func (s *SpacesSuite) TestAddSpaceManySubnets(c *gc.C) {
	name := "my-space"
	subnets := []string{"1.1.1.0/24", "2.1.1.0/24", "3.1.1.0/24", "4.1.1.0/24", "5.1.1.0/24"}
	isPublic := false
	s.addSubnets(c, subnets)

	space, err := s.State.AddSpace(name, subnets, isPublic)
	c.Assert(err, jc.ErrorIsNil)
	assertSpace(c, space, name, subnets, isPublic)

	// We should get the same space back from the database
	id := space.ID()
	space, err = s.State.Space(name)
	c.Assert(err, jc.ErrorIsNil)
	assertSpace(c, space, name, subnets, isPublic)
	c.Assert(id, gc.Equals, space.ID())
}

func (s *SpacesSuite) TestAddSpaceSubnetsDoNotExist(c *gc.C) {
	name := "my-space"
	subnets := []string{"1.1.1.0/24"}
	isPublic := false

	_, err := s.State.AddSpace(name, subnets, isPublic)
	c.Assert(err, gc.ErrorMatches, "adding space \"my-space\": subnet \"1.1.1.0/24\" not found")
	s.assertNoSpace(c, name)
}

func (s *SpacesSuite) TestAddSpaceDuplicateSpace(c *gc.C) {
	name := "my-space"
	subnets := []string{"1.1.1.0/24"}
	isPublic := false
	s.addSubnets(c, subnets)

	space, err := s.State.AddSpace(name, subnets, isPublic)
	c.Assert(err, jc.ErrorIsNil)
	assertSpace(c, space, name, subnets, isPublic)

	// We should get the same space back from the database
	id := space.ID()
	space, err = s.State.Space(name)
	c.Assert(err, jc.ErrorIsNil)
	assertSpace(c, space, name, subnets, isPublic)
	c.Assert(id, gc.Equals, space.ID())

	// Trying to add the same space again should fail
	space, err = s.State.AddSpace(name, subnets, isPublic)
	c.Assert(err, gc.ErrorMatches, "adding space \"my-space\": space \"my-space\" already exists")

	// The space should still be there
	space, err = s.State.Space(name)
	c.Assert(err, jc.ErrorIsNil)
	assertSpace(c, space, name, subnets, isPublic)
	c.Assert(id, gc.Equals, space.ID())
}

func (s *SpacesSuite) TestAddSpaceInvalidName(c *gc.C) {
	name := "-"
	subnets := []string{"1.1.1.0/24"}
	isPublic := false
	s.addSubnets(c, subnets)

	_, err := s.State.AddSpace(name, subnets, isPublic)
	c.Assert(err, gc.ErrorMatches, "adding space \"-\": invalid space name")
	s.assertNoSpace(c, name)
}

func (s *SpacesSuite) TestAddSpaceEmptyName(c *gc.C) {
	name := ""
	subnets := []string{"1.1.1.0/24"}
	isPublic := false
	s.addSubnets(c, subnets)

	_, err := s.State.AddSpace(name, subnets, isPublic)
	c.Assert(err, gc.ErrorMatches, "adding space \"\": invalid space name")
	s.assertNoSpace(c, name)
}

func (s *SpacesSuite) TestSpaceSubnets(c *gc.C) {
	name := "my-space"
	subnets := []string{"1.1.1.0/24", "2.1.1.0/24", "3.1.1.0/24", "4.1.1.0/24", "5.1.1.0/24"}
	isPublic := false
	s.addSubnets(c, subnets)

	space, err := s.State.AddSpace(name, subnets, isPublic)
	c.Assert(err, jc.ErrorIsNil)

	expected := []*state.Subnet{}
	for _, cidr := range subnets {
		subnet, err := s.State.Subnet(cidr)
		c.Assert(err, jc.ErrorIsNil)
		expected = append(expected, subnet)
	}
	actual, err := space.Subnets()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, jc.DeepEquals, expected)
}

func (s *SpacesSuite) TestAllSpaces(c *gc.C) {
	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spaces, jc.DeepEquals, []*state.Space{})

	subnets := []string{"1.1.1.0/24", "2.1.1.0/24", "3.1.1.0/24"}
	isPublic := false
	s.addSubnets(c, subnets)

	first, err := s.State.AddSpace("first", []string{"1.1.1.0/24"}, isPublic)
	c.Assert(err, jc.ErrorIsNil)
	second, err := s.State.AddSpace("second", []string{"2.1.1.0/24"}, isPublic)
	c.Assert(err, jc.ErrorIsNil)
	third, err := s.State.AddSpace("third", []string{"3.1.1.0/24"}, isPublic)
	c.Assert(err, jc.ErrorIsNil)

	actual, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, jc.SameContents, []*state.Space{first, second, third})
}
