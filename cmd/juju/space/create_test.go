// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/testing"
	gc "gopkg.in/check.v1"
)

type spaceCreateSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&spaceCreateSuite{})

// runCommand runs the api-endpoints command with the given arguments
// and returns the output and any error.
func (s *spaceCreateSuite) runCommand(c *gc.C, args ...string) (string, string, error) {
	ctx, err := testing.RunCommand(c, envcmd.Wrap(&space.CreateCommand{}), args...)
	if err != nil {
		return "", "", err
	}
	return testing.Stdout(ctx), testing.Stderr(ctx), nil
}

func (s *spaceCreateSuite) testInitFail(c *gc.C, errPat string, args ...string) {
	testing.TestInit(c, &space.CreateCommand{}, args, errPat)
}

func (s *spaceCreateSuite) testInitOK(c *gc.C, args ...string) {
	testing.TestInit(c, &space.CreateCommand{}, args, "")
}

func (s *spaceCreateSuite) TestHelp(c *gc.C) {
	cc := space.CreateCommand{}
	info := cc.Info()
	s.testSubcmdHelp(c, info, "create")
}

func (s *spaceCreateSuite) TestNoName(c *gc.C) {
	s.testInitFail(c, "No space named in command")
}

func (s *spaceCreateSuite) TestNamed(c *gc.C) {
	s.testInitOK(c, "name")
}

func (s *spaceCreateSuite) TestNamedDash(c *gc.C) {
	s.testInitOK(c, "dmz-cluster-public")
}

func (s *spaceCreateSuite) TestNamedSpace(c *gc.C) {
	s.testInitFail(c, "Space name .+ is invalid",
		"dmz cluster public")
}

func (s *spaceCreateSuite) TestNamedWithCIDR(c *gc.C) {
	s.testInitOK(c, "dmz-cluster-public",
		"1.2.3.4/24")
}

func (s *spaceCreateSuite) TestNamedWithTwoCIDR(c *gc.C) {
	s.testInitOK(c, "dmz-cluster-public",
		"1.2.3.4/24", "2.2.3.4/24")
}

func (s *spaceCreateSuite) TestNamedWithInvalidCIDR(c *gc.C) {
	s.testInitFail(c, ".+ is not a valid CIDR",
		"dmz-cluster-public", "1.3.4/24", "2.2.3.4/24")
}

func (s *spaceCreateSuite) TestNamedWithDuplicateCIDR(c *gc.C) {
	s.testInitFail(c, "Duplicate subnet in space .+",
		"dmz-cluster-public", "1.2.3.4/24", "1.2.3.4/24")
}

func (s *spaceCreateSuite) TestNamedWithDuplicateCIDRDueToMask(c *gc.C) {
	s.testInitFail(c, "Duplicate subnet in space .+",
		"dmz-cluster-public", "1.2.3.5/24", "1.2.3.4/24")
}
