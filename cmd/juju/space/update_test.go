// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
)

type UpdateSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&UpdateSuite{})

func (s *UpdateSuite) SetUpTest(c *gc.C) {
	s.BaseSpaceSuite.SetUpTest(c)
	s.command = space.NewUpdateCommand(s.api)
	c.Assert(s.command, gc.NotNil)
}

func (s *UpdateSuite) TestRunWithSubnetsSucceeds(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "myspace", "10.1.2.0/24", "4.3.2.0/28")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `updated space "myspace": changed subnets to 10.1.2.0/24, 4.3.2.0/28\n`)
	s.api.CheckCalls(c, []testing.StubCall{{
		FuncName: "AllSubnets",
		Args:     nil,
	}, {
		FuncName: "UpdateSpace",
		Args:     []interface{}{"myspace", s.Strings("10.1.2.0/24", "4.3.2.0/28")},
	}, {
		FuncName: "Close",
		Args:     nil,
	}})
}

func (s *UpdateSuite) TestRunWhenSubnetsFails(c *gc.C) {
	s.api.SetErrors(errors.New("boom"))

	stdout, stderr, err := s.RunSubCommand(c, "foo", "10.1.2.0/24")
	c.Assert(err, gc.ErrorMatches, `cannot fetch available subnets: boom`)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	s.api.CheckCallNames(c, "AllSubnets", "Close")
}

func (s *UpdateSuite) TestRunWithUnknownSubnetsFails(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "foo", "10.20.30.0/24", "2001:db8::/64")
	c.Assert(err, gc.ErrorMatches, "unknown subnets specified: 10.20.30.0/24, 2001:db8::/64")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	s.api.CheckCallNames(c, "AllSubnets", "Close")
}

func (s *UpdateSuite) TestRunAPIConnectFails(c *gc.C) {
	// TODO(dimitern): Change this once API is implemented.
	s.command = space.NewUpdateCommand(nil)
	stdout, stderr, err := s.RunSubCommand(c, "myspace", "10.20.30.0/24")
	c.Assert(err, gc.ErrorMatches, "cannot connect to API server: API not implemented yet!")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	// No API calls recoreded.
	s.api.CheckCallNames(c)
}
