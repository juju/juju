// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"math/rand"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
)

type cmdSpaceSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdSpaceSuite) AddSubnets(c *gc.C, infos []network.SubnetInfo) []*state.Subnet {
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
	infos []network.SubnetInfo,
) {
	infos = make([]network.SubnetInfo, count)
	for i := range infos {
		infos[i] = network.SubnetInfo{
			// ProviderId it needs to be unique in state.
			ProviderId:        network.Id(fmt.Sprintf("sub-%d", rand.Int())),
			CIDR:              fmt.Sprintf(cidrTemplate, i),
			SpaceID:           space,
			AvailabilityZones: []string{"zone1"},
		}
	}
	return infos
}

func (s *cmdSpaceSuite) AddSpace(c *gc.C, name string, ids []string, public bool) *state.Space {
	space, err := s.State.AddSpace(name, "", ids, public)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(space.Name(), gc.Equals, name)
	spaceInfo, err := space.NetworkSpace()
	c.Assert(err, jc.ErrorIsNil)
	subnets := spaceInfo.Subnets
	c.Assert(subnets, gc.HasLen, len(ids))
	return space
}

const expectedSuccess = ""

func (s *cmdSpaceSuite) Run(c *gc.C, args ...string) (string, string, error) {
	context, err := runCommand(c, args...)
	stdout, stderr := "", ""
	if context != nil {
		stdout = cmdtesting.Stdout(context)
		stderr = cmdtesting.Stderr(context)
	}
	return stdout, stderr, err
}

func (s *cmdSpaceSuite) RunAdd(c *gc.C, expectedError string, args ...string) {
	cmdArgs := append([]string{"add-space"}, args...)
	_, stderr, err := s.Run(c, cmdArgs...)
	if expectedError != "" {
		c.Assert(err, gc.NotNil)
		c.Assert(stderr, jc.Contains, expectedError)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *cmdSpaceSuite) RunList(c *gc.C, expectedError string, args ...string) {
	cmdArgs := append([]string{"spaces"}, args...)
	_, stderr, err := s.Run(c, cmdArgs...)
	if expectedError != "" {
		c.Assert(err, gc.NotNil)
		c.Assert(stderr, jc.Contains, expectedError)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *cmdSpaceSuite) AssertOutput(c *gc.C, context *cmd.Context, expectedOut, expectedErr string) {
	c.Assert(cmdtesting.Stdout(context), gc.Equals, expectedOut)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, expectedErr)
}

func (s *cmdSpaceSuite) TestSpaceCreateNotSupported(c *gc.C) {
	isEnabled := dummy.SetSupportsSpaces(false)
	defer dummy.SetSupportsSpaces(isEnabled)

	expectedError := "cannot add space \"foo\": spaces not supported"
	s.RunAdd(c, expectedError, "foo")
}

func (s *cmdSpaceSuite) TestSpaceCreateNoName(c *gc.C) {
	expectedError := "invalid arguments specified: space name is required"
	s.RunAdd(c, expectedError)
}

func (s *cmdSpaceSuite) TestSpaceCreateInvalidName(c *gc.C) {
	expectedError := `invalid arguments specified: " f o o " is not a valid space name`
	s.RunAdd(c, expectedError, " f o o ")
}

func (s *cmdSpaceSuite) TestSpaceCreateWithInvalidSubnets(c *gc.C) {
	expectedError := `invalid arguments specified: "nonsense" is not a valid CIDR`
	s.RunAdd(c, expectedError, "myspace", "nonsense", "10.20.0.0/16")
}

func (s *cmdSpaceSuite) TestSpaceCreateWithUnknownSubnet(c *gc.C) {
	expectedError := `cannot add space "foo": subnet "10.10.0.0/16" not found`
	s.RunAdd(c, expectedError, "foo", "10.10.0.0/16")
}

func (s *cmdSpaceSuite) TestSpaceCreateAlreadyExistingName(c *gc.C) {
	s.AddSpace(c, "foo", nil, true)

	expectedError := `cannot add space "foo": adding space "foo": space "foo" already exists`
	s.RunAdd(c, expectedError, "foo")
}

func (s *cmdSpaceSuite) TestSpaceCreateNoSubnets(c *gc.C) {
	stdout, stderr, err := s.Run(c, "add-space", "myspace")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, jc.Contains, "added space \"myspace\" with no subnets\n")

	space, err := s.State.SpaceByName("myspace")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(space.Name(), gc.Equals, "myspace")
	spaceInfo, err := space.NetworkSpace()
	c.Assert(err, jc.ErrorIsNil)
	subnets := spaceInfo.Subnets
	c.Assert(subnets, gc.HasLen, 0)
}

func (s *cmdSpaceSuite) TestSpaceCreateWithSubnets(c *gc.C) {
	infos := s.MakeSubnetInfos(c, "", "10.1%d.0.0/16", 2)
	s.AddSubnets(c, infos)

	_, stderr, err := s.Run(
		c,
		"add-space", "myspace", "10.10.0.0/16", "10.11.0.0/16",
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stderr, jc.Contains,
		"added space \"myspace\" with subnets 10.10.0.0/16, 10.11.0.0/16\n",
	)

	space, err := s.State.SpaceByName("myspace")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(space.Name(), gc.Equals, "myspace")
	spaceInfo, err := space.NetworkSpace()
	c.Assert(err, jc.ErrorIsNil)
	subnets := spaceInfo.Subnets
	c.Assert(subnets, gc.HasLen, 2)
	c.Assert(subnets[0].SpaceName, gc.Equals, "myspace")
	c.Assert(subnets[1].SpaceName, gc.Equals, "myspace")
}

func (s *cmdSpaceSuite) TestSpaceListDefaultOnly(c *gc.C) {
	stdout, _, err := s.Run(c, "spaces")
	c.Assert(err, jc.ErrorIsNil)

	expected := `
Name   Space ID  Subnets
alpha  0                
                        
`[1:]

	c.Assert(stdout, gc.Equals, expected)
}

func (s *cmdSpaceSuite) TestSpaceListOneResultNoSubnets(c *gc.C) {
	s.AddSpace(c, "myspace", nil, true)

	// The default space is listed in addition to the one we added.
	expectedOutput := "{\"spaces\":[{\"id\":\"0\",\"name\":\"alpha\",\"subnets\":{}},{\"id\":\"1\",\"name\":\"myspace\",\"subnets\":{}}]}\n"
	stdout, _, err := s.Run(c, "spaces", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, jc.Contains, expectedOutput)
}

func (s *cmdSpaceSuite) TestSpaceListMoreResults(c *gc.C) {
	space1 := s.AddSpace(c, "space1", nil, true)
	infos1 := s.MakeSubnetInfos(c, space1.Id(), "10.10.%d.0/24", 3)
	s.AddSubnets(c, infos1)

	space2 := s.AddSpace(c, "space2", nil, false)
	infos2 := s.MakeSubnetInfos(c, space2.Id(), "10.20.%d.0/24", 1)
	s.AddSubnets(c, infos2)

	stdout, stderr, err := s.Run(c, "spaces", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stderr, gc.Equals, "")

	// We dont' check the output in detail, just a few things - the
	// rest it tested separately.
	c.Assert(stdout, jc.Contains, "spaces:")
	c.Assert(stdout, jc.Contains, "id: \"0\"")
	c.Assert(stdout, jc.Contains, "id: \"1\"")
	c.Assert(stdout, jc.Contains, "name: space1")
	c.Assert(stdout, jc.Contains, "10.10.2.0/24:")
	c.Assert(stdout, jc.Contains, "name: space2")
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
