// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"math/rand"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type cmdSpaceSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdSpaceSuite) AddSubnets(c *gc.C, infos []state.SubnetInfo) []*state.Subnet {
	results := make([]*state.Subnet, len(infos))
	for i, info := range infos {
		subnet, err := s.State.AddSubnet(info)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(subnet.CIDR(), gc.Equals, info.CIDR)
		results[i] = subnet
	}
	return results
}

func (s *cmdSpaceSuite) MakeSubnetInfos(c *gc.C, space string, cidrTemplate string, count int) (
	infos []state.SubnetInfo,
	ids []string,
) {
	infos = make([]state.SubnetInfo, count)
	ids = make([]string, count)
	for i := range infos {
		ids[i] = fmt.Sprintf(cidrTemplate, i)
		infos[i] = state.SubnetInfo{
			// ProviderId it needs to be unique in state.
			ProviderId:       network.Id(fmt.Sprintf("sub-%d", rand.Int())),
			CIDR:             ids[i],
			SpaceName:        space,
			AvailabilityZone: "zone1",
		}
	}
	return infos, ids
}

func (s *cmdSpaceSuite) AddSpace(c *gc.C, name string, ids []string, public bool) *state.Space {
	space, err := s.State.AddSpace(name, "", ids, public)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(space.Name(), gc.Equals, name)
	subnets, err := space.Subnets()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, len(ids))
	return space
}

const expectedSuccess = ""

func (s *cmdSpaceSuite) Run(c *gc.C, args ...string) (string, string, error) {
	cmdArgs := append([]string{"space"}, args...)
	context, err := runJujuCommand(c, cmdArgs...)
	stdout, stderr := "", ""
	if context != nil {
		stdout = testing.Stdout(context)
		stderr = testing.Stderr(context)
	}
	return stdout, stderr, err
}

func (s *cmdSpaceSuite) RunCreate(c *gc.C, expectedError string, args ...string) {
	cmdArgs := append([]string{"create"}, args...)
	_, stderr, err := s.Run(c, cmdArgs...)
	if expectedError != "" {
		c.Assert(err, gc.NotNil)
		c.Assert(stderr, jc.Contains, expectedError)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *cmdSpaceSuite) RunList(c *gc.C, expectedError string, args ...string) {
	cmdArgs := append([]string{"list"}, args...)
	_, stderr, err := s.Run(c, cmdArgs...)
	if expectedError != "" {
		c.Assert(err, gc.NotNil)
		c.Assert(stderr, jc.Contains, expectedError)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *cmdSpaceSuite) AssertOutput(c *gc.C, context *cmd.Context, expectedOut, expectedErr string) {
	c.Assert(testing.Stdout(context), gc.Equals, expectedOut)
	c.Assert(testing.Stderr(context), gc.Equals, expectedErr)
}

func (s *cmdSpaceSuite) TestSpaceCreateNotSupported(c *gc.C) {
	isEnabled := dummy.SetSupportsSpaces(false)
	defer dummy.SetSupportsSpaces(isEnabled)

	expectedError := "cannot create space \"foo\": spaces not supported"
	s.RunCreate(c, expectedError, "foo")
}

func (s *cmdSpaceSuite) TestSpaceCreateNoName(c *gc.C) {
	expectedError := "invalid arguments specified: space name is required"
	s.RunCreate(c, expectedError)
}

func (s *cmdSpaceSuite) TestSpaceCreateInvalidName(c *gc.C) {
	expectedError := `invalid arguments specified: " f o o " is not a valid space name`
	s.RunCreate(c, expectedError, " f o o ")
}

func (s *cmdSpaceSuite) TestSpaceCreateWithInvalidSubnets(c *gc.C) {
	expectedError := `invalid arguments specified: "nonsense" is not a valid CIDR`
	s.RunCreate(c, expectedError, "myspace", "nonsense", "10.20.0.0/16")
}

func (s *cmdSpaceSuite) TestSpaceCreateWithUnknownSubnet(c *gc.C) {
	expectedError := `cannot create space "foo": adding space "foo": subnet "10.10.0.0/16" not found`
	s.RunCreate(c, expectedError, "foo", "10.10.0.0/16")
}

func (s *cmdSpaceSuite) TestSpaceCreateAlreadyExistingName(c *gc.C) {
	s.AddSpace(c, "foo", nil, true)

	expectedError := `cannot create space "foo": adding space "foo": space "foo" already exists`
	s.RunCreate(c, expectedError, "foo")
}

func (s *cmdSpaceSuite) TestSpaceCreateNoSubnets(c *gc.C) {
	stdout, stderr, err := s.Run(c, "create", "myspace")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, jc.Contains, "created space \"myspace\" with no subnets\n")

	space, err := s.State.Space("myspace")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(space.Name(), gc.Equals, "myspace")
	subnets, err := space.Subnets()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, 0)
}

func (s *cmdSpaceSuite) TestSpaceCreateWithSubnets(c *gc.C) {
	infos, _ := s.MakeSubnetInfos(c, "", "10.1%d.0.0/16", 2)
	s.AddSubnets(c, infos)

	_, stderr, err := s.Run(
		c,
		"create", "myspace", "10.10.0.0/16", "10.11.0.0/16",
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stderr, jc.Contains,
		"created space \"myspace\" with subnets 10.10.0.0/16, 10.11.0.0/16\n",
	)

	space, err := s.State.Space("myspace")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(space.Name(), gc.Equals, "myspace")
	subnets, err := space.Subnets()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, 2)
	c.Assert(subnets[0].SpaceName(), gc.Equals, "myspace")
	c.Assert(subnets[1].SpaceName(), gc.Equals, "myspace")
}

func (s *cmdSpaceSuite) TestSpaceListNoResults(c *gc.C) {
	_, stderr, err := s.Run(c, "list")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stderr, jc.Contains,
		"no spaces to display\n",
	)
}

func (s *cmdSpaceSuite) TestSpaceListOneResultNoSubnets(c *gc.C) {
	s.AddSpace(c, "myspace", nil, true)

	expectedOutput := "{\"spaces\":{\"myspace\":{}}}\n"
	stdout, _, err := s.Run(c, "list", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, jc.Contains, expectedOutput)
}

func (s *cmdSpaceSuite) TestSpaceListMoreResults(c *gc.C) {
	infos1, ids1 := s.MakeSubnetInfos(c, "space1", "10.10.%d.0/24", 3)
	s.AddSubnets(c, infos1)
	s.AddSpace(c, "space1", ids1, true)

	infos2, ids2 := s.MakeSubnetInfos(c, "space2", "10.20.%d.0/24", 1)
	s.AddSubnets(c, infos2)
	s.AddSpace(c, "space2", ids2, false)

	stdout, stderr, err := s.Run(c, "list", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stderr, gc.Equals, "")

	// We dont' check the output in detail, just a few things - the
	// rest it tested separately.
	c.Assert(stdout, jc.Contains, "spaces:")
	c.Assert(stdout, jc.Contains, "space1:")
	c.Assert(stdout, jc.Contains, "10.10.2.0/24:")
	c.Assert(stdout, jc.Contains, "space2:")
	c.Assert(stdout, jc.Contains, "10.20.0.0/24:")
	c.Assert(stdout, jc.Contains, "zones:")
	c.Assert(stdout, jc.Contains, "zone1")
}

func (s *cmdSpaceSuite) TestSpaceListNotSupported(c *gc.C) {
	isEnabled := dummy.SetSupportsSpaces(false)
	defer dummy.SetSupportsSpaces(isEnabled)

	expectedError := "cannot list spaces: spaces not supported"
	s.RunList(c, expectedError)
}
