// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type cmdSubnetSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdSubnetSuite) AddSubnet(c *gc.C, info network.SubnetInfo) *state.Subnet {
	subnet, err := s.State.AddSubnet(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.CIDR(), gc.Equals, info.CIDR)
	return subnet
}

func (s *cmdSubnetSuite) AddSpace(c *gc.C, name string, ids []string, public bool) *state.Space {
	space, err := s.State.AddSpace(name, "", ids, public)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(space.Name(), gc.Equals, name)
	spaceInfo, err := space.NetworkSpace()
	c.Assert(err, jc.ErrorIsNil)
	subnets := spaceInfo.Subnets
	c.Assert(subnets, gc.HasLen, len(ids))
	return space
}

func (s *cmdSubnetSuite) Run(c *gc.C, expectedError string, args ...string) *cmd.Context {
	context, err := runCommand(c, args...)
	if expectedError != "" {
		c.Assert(err, gc.ErrorMatches, expectedError)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
	return context
}

func (s *cmdSubnetSuite) AssertOutput(c *gc.C, context *cmd.Context, expectedOut, expectedErr string) {
	c.Assert(cmdtesting.Stdout(context), gc.Equals, expectedOut)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, expectedErr)
}

func (s *cmdSubnetSuite) TestSubnetListNoResults(c *gc.C) {
	context := s.Run(c, expectedSuccess, "subnets")
	s.AssertOutput(c, context,
		"", // no stdout output
		"No subnets to display.\n",
	)
}

func (s *cmdSubnetSuite) TestSubnetListResultsWithFilters(c *gc.C) {
	space := s.AddSpace(c, "myspace", nil, true)
	s.AddSubnet(c, network.SubnetInfo{
		CIDR: "10.0.0.0/8",
	})
	s.AddSubnet(c, network.SubnetInfo{
		CIDR:              "10.10.0.0/16",
		AvailabilityZones: []string{"zone1"},
		SpaceID:           space.Id(),
	})

	context := s.Run(c,
		expectedSuccess,
		"subnets", "--zone", "zone1", "--space", "myspace",
	)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "") // no stderr expected
	stdout := cmdtesting.Stdout(context)
	c.Assert(stdout, jc.Contains, "subnets:")
	c.Assert(stdout, jc.Contains, "10.10.0.0/16:")
	c.Assert(stdout, jc.Contains, "space: myspace")
	c.Assert(stdout, jc.Contains, "zones:")
	c.Assert(stdout, jc.Contains, "- zone1")
	c.Assert(stdout, gc.Not(jc.Contains), "10.0.0.0/8:")
}
