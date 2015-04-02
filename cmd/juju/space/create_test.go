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

type CreateSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&CreateSuite{})

func (s *CreateSuite) SetUpTest(c *gc.C) {
	s.BaseSpaceSuite.SetUpTest(c)
	s.command = space.NewCreateCommand(s.api)
	c.Assert(s.command, gc.NotNil)
}

func (s *CreateSuite) TestRunNoSubnetsFails(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "myspace")
	c.Assert(err, gc.ErrorMatches, "CIDRs required but not provided")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, "")
	s.api.CheckCallNames(c)
}

func (s *CreateSuite) TestRunWithSubnetsSucceeds(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "myspace", "10.1.2.0/24", "4.3.2.0/28")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `created space "myspace" with subnets 10.1.2.0/24, 4.3.2.0/28\n`)
	s.api.CheckCalls(c, []testing.StubCall{{
		FuncName: "AllSubnets",
		Args:     nil,
	}, {
		FuncName: "CreateSpace",
		Args:     []interface{}{"myspace", s.Strings("10.1.2.0/24", "4.3.2.0/28")},
	}, {
		FuncName: "Close",
		Args:     nil,
	}})
}

func (s *CreateSuite) TestRunWithExistingSpaceFails(c *gc.C) {
	s.api.SetErrors(nil, errors.AlreadyExistsf("space %q", "foo"))

	stdout, stderr, err := s.RunSubCommand(c, "foo", "10.1.2.0/24")
	s.CheckOutputsErr(c, stdout, stderr, err, `cannot create space "foo": space "foo" already exists`)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
	s.api.CheckCallNames(c, "AllSubnets", "CreateSpace", "Close")
}

func (s *CreateSuite) TestRunWhenSubnetsFails(c *gc.C) {
	s.api.SetErrors(errors.New("boom"))

	stdout, stderr, err := s.RunSubCommand(c, "foo", "10.1.2.0/24")
	s.api.CheckCallNames(c, "AllSubnets", "Close")
	s.CheckOutputsErr(c, stdout, stderr, err, `cannot create space "foo": cannot fetch available subnets: boom`)
	s.api.CheckCallNames(c, "AllSubnets", "Close")
}

func (s *CreateSuite) TestRunWithUnknownSubnetsFails(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "foo", "10.20.30.0/24", "2001:db8::/64")
	s.CheckOutputsErr(c, stdout, stderr, err, `cannot create space "foo": unknown subnets specified: 10.20.30.0/24, 2001:db8::/64`)
	s.api.CheckCallNames(c, "AllSubnets", "Close")
}

func (s *CreateSuite) TestRunAPIConnectFails(c *gc.C) {
	// TODO(dimitern): Change this once API is implemented.
	s.command = space.NewCreateCommand(nil)
	stdout, stderr, err := s.RunSubCommand(c, "myspace", "10.20.30.0/24")
	s.CheckOutputsErr(c, stdout, stderr, err, "cannot connect to API server: API not implemented yet!")
	// No API calls recoreded.
	s.api.CheckCallNames(c)
}
