// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/juju/state"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type SpacesSuite struct {
	ConnSuite
}

var _ = gc.Suite(&SpacesSuite{})

func (s *SpacesSuite) AddSubnets(c *gc.C, CIDRs []string) {
	for i, cidr := range CIDRs {
		ip, ipNet, err := net.ParseCIDR(cidr)

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

		c.Assert(err, jc.ErrorIsNil)
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
}

func assertSpace(c *gc.C, space *state.Space, name string, subnets []string, isPublic bool) {
	c.Assert(space.Doc().Name, gc.Equals, name)
	c.Assert(space.Doc().Subnets, jc.DeepEquals, subnets)
	c.Assert(space.Doc().IsPublic, gc.Equals, isPublic)

	c.Assert(space.Life(), gc.Equals, state.Alive)
	c.Assert(strings.HasSuffix(space.ID(), name), jc.IsTrue)
	c.Assert(space.String(), gc.Equals, name)
	c.Assert(space.GoString(), gc.Equals, name)
}

func (s *SpacesSuite) TestAddSpace(c *gc.C) {
	name := "my-space"
	subnets := []string{"1.1.1.0/24"}
	isPublic := false
	s.AddSubnets(c, subnets)

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
	s.AddSubnets(c, subnets)

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
	c.Assert(err, gc.ErrorMatches, "cannot add space \"my-space\": subnet \"1.1.1.0/24\" not found")
	s.assertNoSpace(c, name)
}

func (s *SpacesSuite) TestAddSpaceDuplicateSpace(c *gc.C) {
	name := "my-space"
	subnets := []string{"1.1.1.0/24"}
	isPublic := false
	s.AddSubnets(c, subnets)

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
	c.Assert(err, gc.ErrorMatches, "cannot add space \"my-space\": space \"my-space\" already exists")

	// The space should still be there
	space, err = s.State.Space(name)
	c.Assert(err, jc.ErrorIsNil)
	assertSpace(c, space, name, subnets, isPublic)
	c.Assert(id, gc.Equals, space.ID())
}

func (s *SpacesSuite) TestAddSpaceNoSubnets(c *gc.C) {
	name := "my-space"
	subnets := []string{}
	isPublic := false
	s.AddSubnets(c, subnets)

	_, err := s.State.AddSpace(name, subnets, isPublic)
	c.Assert(err, gc.ErrorMatches, "cannot add space \"my-space\": at least one subnet required")
	s.assertNoSpace(c, name)
}

func (s *SpacesSuite) TestAddSpaceInvalidName(c *gc.C) {
	name := "-"
	subnets := []string{"1.1.1.0/24"}
	isPublic := false
	s.AddSubnets(c, subnets)

	_, err := s.State.AddSpace(name, subnets, isPublic)
	c.Assert(err, gc.ErrorMatches, "cannot add space \"-\": invalid space name")
	s.assertNoSpace(c, name)
}

func (s *SpacesSuite) TestAddSpaceEmptyName(c *gc.C) {
	name := ""
	subnets := []string{"1.1.1.0/24"}
	isPublic := false
	s.AddSubnets(c, subnets)

	_, err := s.State.AddSpace(name, subnets, isPublic)
	c.Assert(err, gc.ErrorMatches, "cannot add space \"\": invalid space name")
	s.assertNoSpace(c, name)
}

func (s *SpacesSuite) TestAddSpaceInvalidCIDR(c *gc.C) {
	name := "my-space"
	subnets := []string{"1.1.1.256/24"}
	isPublic := false

	_, err := s.State.AddSpace(name, subnets, isPublic)
	c.Assert(err, gc.ErrorMatches, "cannot add space \"my-space\": invalid CIDR address: 1.1.1.256/24")
	s.assertNoSpace(c, name)
}
