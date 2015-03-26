// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type spaceCreateSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&spaceCreateSuite{})

// runCommand runs the api-endpoints command with the given arguments
// and returns the output and any error.
func (s *spaceCreateSuite) runCommand(c *gc.C, args ...string) (string, string, error) {
	ctx, err := testing.RunCommand(c, &space.CreateCommand{}, args...)
	if err != nil {
		return "", "", err
	}
	return testing.Stdout(ctx), testing.Stderr(ctx), nil
}

func (s *spaceCreateSuite) TestHelp(c *gc.C) {
	cc := space.CreateCommand{}
	info := cc.Info()
	s.testSubcmdHelp(c, info, "create")
}

func (s *spaceCreateSuite) TestNoName(c *gc.C) {
	_, _, err := s.runCommand(c)
	c.Assert(err, gc.ErrorMatches, "No space named in command")
}

func (s *spaceCreateSuite) TestNamed(c *gc.C) {
	_, _, err := s.runCommand(c, "name")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceCreateSuite) TestNamedDash(c *gc.C) {
	_, _, err := s.runCommand(c, "dmz-cluster-public")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceCreateSuite) TestNamedSpace(c *gc.C) {
	_, _, err := s.runCommand(c, "dmz cluster public")
	c.Assert(err, gc.ErrorMatches, "Space name .+ is invalid")
}

func (s *spaceCreateSuite) TestNamedWithCIDR(c *gc.C) {
	_, _, err := s.runCommand(c, "dmz-cluster-public", "1.2.3.4/24")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceCreateSuite) TestNamedWithTwoCIDR(c *gc.C) {
	_, _, err := s.runCommand(c, "dmz-cluster-public", "1.2.3.4/24", "2.2.3.4/24")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceCreateSuite) TestNamedWithInvalidCIDR(c *gc.C) {
	_, _, err := s.runCommand(c, "dmz-cluster-public", "1.3.4/24", "2.2.3.4/24")
	c.Assert(err, gc.ErrorMatches, ".+ is not a valid CIDR")
}

func (s *spaceCreateSuite) TestNamedWithDuplicateCIDR(c *gc.C) {
	_, _, err := s.runCommand(c, "dmz-cluster-public", "1.2.3.4/24", "1.2.3.4/24")
	c.Assert(err, gc.ErrorMatches, "Duplicate subnet in space .+")
}

func (s *spaceCreateSuite) TestNamedWithDuplicateCIDRDueToMask(c *gc.C) {
	_, _, err := s.runCommand(c, "dmz-cluster-public", "1.2.3.5/24", "1.2.3.4/24")
	c.Assert(err, gc.ErrorMatches, "Duplicate subnet in space .+")
}
