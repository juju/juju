// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
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

func (s *SpacesSuite) SetUpSuite(c *gc.C) {
	s.ConnSuite.SetUpSuite(c)
}

func (s *SpacesSuite) TearDownSuite(c *gc.C) {
	s.ConnSuite.TearDownSuite(c)
}

func (s *SpacesSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
}

func (s *SpacesSuite) TearDownTest(c *gc.C) {
	s.ConnSuite.TearDownTest(c)
}

func (s *SpacesSuite) AddSubnets(c *gc.C, CIDRs []string) {
	for _, cidr := range CIDRs {
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

		subnetInfo := state.SubnetInfo{
			ProviderId:        "ProviderId",
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

func (s *SpacesSuite) TestAddSpace(c *gc.C) {
	name := "MySpace"
	subnets := []string{"1.1.1.0/24"}
	isPublic := false

	s.AddSubnets(c, subnets)

	assertSpace := func(space *state.Space) {
		c.Assert(space.Doc().Name, gc.Equals, name)
		c.Assert(space.Doc().Subnets, jc.DeepEquals, subnets)
		c.Assert(space.Doc().IsPublic, gc.Equals, isPublic)

		c.Assert(space.Life(), gc.Equals, state.Alive)
		c.Assert(strings.HasSuffix(space.ID(), name), jc.IsTrue)
		c.Assert(space.String(), gc.Equals, name)
		c.Assert(space.GoString(), gc.Equals, name)
	}

	space, err := s.State.AddSpace(name, subnets, isPublic)
	c.Assert(err, jc.ErrorIsNil)
	assertSpace(space)

	// We should get the same space back from the database
	id := space.ID()
	space, err = s.State.Space(name)
	c.Assert(err, jc.ErrorIsNil)
	assertSpace(space)
	c.Assert(id, gc.Equals, space.ID())
}
