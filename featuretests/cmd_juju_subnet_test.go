// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	cmdsubnet "github.com/juju/juju/cmd/juju/subnet"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type cmdSubnetSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdSubnetSuite) AddSubnet(c *gc.C, info state.SubnetInfo) *state.Subnet {
	subnet, err := s.State.AddSubnet(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.CIDR(), gc.Equals, info.CIDR)
	return subnet
}

func (s *cmdSubnetSuite) AddSpace(c *gc.C, name string, ids []string, public bool) *state.Space {
	space, err := s.State.AddSpace(name, ids, public)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(space.Name(), gc.Equals, name)
	subnets, err := space.Subnets()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, len(ids))
	return space
}

func (s *cmdSubnetSuite) Run(c *gc.C, command cmd.Command, expectedError string, args ...string) *cmd.Context {
	context, err := testing.RunCommand(c, command, args...)
	if expectedError != "" {
		c.Assert(err, gc.ErrorMatches, expectedError)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
	return context
}

func (s *cmdSubnetSuite) RunSuper(c *gc.C, expectedError string, args ...string) *cmd.Context {
	return s.Run(c, cmdsubnet.NewSuperCommand(), expectedError, args...)
}

func (s *cmdSubnetSuite) RunAdd(c *gc.C, expectedError string, args ...string) *cmd.Context {
	// To capture subcommand errors, we must *NOT* to run it through
	// the supercommand, otherwise there error is logged and
	// swallowed!
	addCommand := envcmd.Wrap(&cmdsubnet.AddCommand{})
	return s.Run(c, addCommand, expectedError, args...)
}

func (s *cmdSubnetSuite) AssertOutput(c *gc.C, context *cmd.Context, expectedOut, expectedErr string) {
	c.Assert(testing.Stdout(context), gc.Equals, expectedOut)
	c.Assert(testing.Stderr(context), gc.Equals, expectedErr)
}

func (s *cmdSubnetSuite) TestSubnetAddNoArguments(c *gc.C) {
	expectedError := "invalid arguments specified: either CIDR or provider ID is required"
	s.RunSuper(c, expectedError, "add")
}

func (s *cmdSubnetSuite) TestSubnetAddInvalidCIDRTakenAsProviderId(c *gc.C) {
	expectedError := "invalid arguments specified: space name is required"
	s.RunSuper(c, expectedError, "add", "subnet-xyz")
}

func (s *cmdSubnetSuite) TestSubnetAddCIDRAndInvalidSpaceName(c *gc.C) {
	expectedError := `invalid arguments specified: " f o o " is not a valid space name`
	s.RunSuper(c, expectedError, "add", "10.0.0.0/8", " f o o ")
}

func (s *cmdSubnetSuite) TestSubnetAddAlreadyExistingCIDR(c *gc.C) {
	s.AddSpace(c, "foo", nil, true)
	s.AddSubnet(c, state.SubnetInfo{CIDR: "0.10.0.0/24"})

	expectedError := `cannot add subnet: adding subnet "0.10.0.0/24": subnet "0.10.0.0/24" already exists`
	s.RunAdd(c, expectedError, "0.10.0.0/24", "foo")
}

func (s *cmdSubnetSuite) TestSubnetAddValidCIDRUnknownByTheProvider(c *gc.C) {
	expectedError := `cannot add subnet: subnet with CIDR "10.0.0.0/8" not found`
	s.RunAdd(c, expectedError, "10.0.0.0/8", "myspace")
}

func (s *cmdSubnetSuite) TestSubnetAddWithoutAnySpaces(c *gc.C) {
	expectedError := `cannot add subnet: no spaces defined`
	s.RunAdd(c, expectedError, "0.10.0.0/24", "whatever")
}

func (s *cmdSubnetSuite) TestSubnetAddWithUnknownSpace(c *gc.C) {
	s.AddSpace(c, "yourspace", nil, true)

	expectedError := `cannot add subnet: space "myspace" not found`
	s.RunAdd(c, expectedError, "0.10.0.0/24", "myspace")
}

func (s *cmdSubnetSuite) TestSubnetAddWithoutZonesWhenProviderHasZones(c *gc.C) {
	s.AddSpace(c, "myspace", nil, true)

	context := s.RunSuper(c, expectedSuccess, "add", "0.10.0.0/24", "myspace")
	s.AssertOutput(c, context,
		"", // no stdout output
		"added subnet with CIDR \"0.10.0.0/24\" in space \"myspace\"\n",
	)

	subnet, err := s.State.Subnet("0.10.0.0/24")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.CIDR(), gc.Equals, "0.10.0.0/24")
	c.Assert(subnet.SpaceName(), gc.Equals, "myspace")
	c.Assert(subnet.ProviderId(), gc.Equals, "dummy-private")
	c.Assert(subnet.AvailabilityZone(), gc.Equals, "zone1")
}

func (s *cmdSubnetSuite) TestSubnetAddWithUnavailableZones(c *gc.C) {
	s.AddSpace(c, "myspace", nil, true)

	expectedError := `cannot add subnet: Zones contain unavailable zones: "zone2"`
	s.RunAdd(c, expectedError, "dummy-private", "myspace", "zone1", "zone2")
}

func (s *cmdSubnetSuite) TestSubnetAddWithZonesWithNoProviderZones(c *gc.C) {
	s.AddSpace(c, "myspace", nil, true)

	context := s.RunSuper(c, expectedSuccess, "add", "dummy-public", "myspace", "zone1")
	s.AssertOutput(c, context,
		"", // no stdout output
		"added subnet with ProviderId \"dummy-public\" in space \"myspace\"\n",
	)

	subnet, err := s.State.Subnet("0.20.0.0/24")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnet.CIDR(), gc.Equals, "0.20.0.0/24")
	c.Assert(subnet.SpaceName(), gc.Equals, "myspace")
	c.Assert(subnet.ProviderId(), gc.Equals, "dummy-public")
	c.Assert(subnet.AvailabilityZone(), gc.Equals, "zone1")
}

func (s *cmdSubnetSuite) TestSubnetListNoResults(c *gc.C) {
	context := s.RunSuper(c, expectedSuccess, "list")
	s.AssertOutput(c, context,
		"", // no stdout output
		"no subnets to display\n",
	)
}

func (s *cmdSubnetSuite) TestSubnetListResultsWithFilters(c *gc.C) {
	//	s.AddSpace(c, "myspace", nil, true)
	s.AddSubnet(c, state.SubnetInfo{
		CIDR: "10.0.0.0/8",
	})
	s.AddSubnet(c, state.SubnetInfo{
		CIDR:             "10.10.0.0/16",
		AvailabilityZone: "zone1",
	})
	s.AddSpace(c, "myspace", []string{"10.10.0.0/16"}, true)

	context := s.RunSuper(c,
		expectedSuccess,
		"list", "--zone", "zone1", "--space", "myspace",
	)
	c.Assert(testing.Stderr(context), gc.Equals, "") // no stderr expected
	stdout := testing.Stdout(context)
	c.Assert(stdout, jc.Contains, "subnets:")
	c.Assert(stdout, jc.Contains, "10.10.0.0/16:")
	c.Assert(stdout, jc.Contains, "space: myspace")
	c.Assert(stdout, jc.Contains, "zones:")
	c.Assert(stdout, jc.Contains, "- zone1")
	c.Assert(stdout, gc.Not(jc.Contains), "10.0.0.0/8:")
}
