// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
	coretesting "github.com/juju/juju/testing"
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

func (s *CreateSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		about       string
		args        []string
		expectName  string
		expectCIDRs []string
		expectErr   string
	}{{
		about:     "no arguments",
		expectErr: "space name is required",
	}, {
		about:     "invalid space name - with invalid characters",
		args:      s.Strings("%inv$alid"),
		expectErr: `"%inv\$alid" is not a valid space name`,
	}, {
		about:     "invalid space name - using underscores",
		args:      s.Strings("42_space"),
		expectErr: `"42_space" is not a valid space name`,
	}, {
		about:      "valid space name with invalid CIDR",
		args:       s.Strings("new-space", "noCIDR"),
		expectName: "new-space",
		expectErr:  `"noCIDR" is not a valid CIDR`,
	}, {
		about:       "valid space with one valid and one invalid CIDR",
		args:        s.Strings("new-space", "10.1.0.0/16", "nonsense"),
		expectName:  "new-space",
		expectCIDRs: s.Strings("10.1.0.0/16"),
		expectErr:   `"nonsense" is not a valid CIDR`,
	}, {
		about:       "valid space with valid but overlapping CIDRs",
		args:        s.Strings("new-space", "10.1.0.0/16", "10.1.0.1/16"),
		expectName:  "new-space",
		expectCIDRs: s.Strings("10.1.0.0/16"),
		expectErr:   `subnet "10.1.0.1/16" overlaps with "10.1.0.0/16"`,
	}, {
		about:       "valid space with valid but duplicated CIDRs",
		args:        s.Strings("new-space", "10.10.0.0/24", "10.10.0.0/24"),
		expectName:  "new-space",
		expectCIDRs: s.Strings("10.10.0.0/24"),
		expectErr:   `duplicate subnet "10.10.0.0/24" specified`,
	}, {
		about:       "all ok",
		args:        s.Strings("myspace", "10.10.0.0/24", "2001:db8::1/32"),
		expectName:  "myspace",
		expectCIDRs: s.Strings("10.10.0.0/24", "2001:db8::/32"),
	}} {
		c.Logf("test #%d: %s", i, test.about)
		// Create a new instance of the subcommand for each test, but
		// since we're not running the command no need to use
		// envcmd.Wrap().
		command := space.NewCreateCommand(s.api)
		err := coretesting.InitCommand(command, test.args)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(command.Name, gc.Equals, test.expectName)
		c.Check(command.CIDRs.SortedValues(), jc.DeepEquals, test.expectCIDRs)
		// No API calls should be recorded at this stage.
		s.api.CheckCallNames(c)
	}
}

func (s *CreateSuite) TestRunNoSubnetsSucceeds(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "myspace")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `created space "myspace" with no subnets\n`)
	s.api.CheckCalls(c, []testing.StubCall{{
		FuncName: "CreateSpace",
		Args:     []interface{}{"myspace", []string{}},
	}, {
		FuncName: "Close",
		Args:     nil,
	}})
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
	s.api.SetErrors(errors.AlreadyExistsf("space %q", "foo"))

	stdout, stderr, err := s.RunSubCommand(c, "foo")
	c.Assert(err, gc.ErrorMatches, `cannot create space "foo": space "foo" already exists`)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	s.api.CheckCallNames(c, "CreateSpace", "Close")
}

func (s *CreateSuite) TestRunWhenSubnetsFails(c *gc.C) {
	s.api.SetErrors(errors.New("boom"))

	stdout, stderr, err := s.RunSubCommand(c, "foo", "10.1.2.0/24")
	c.Assert(err, gc.ErrorMatches, `cannot fetch available subnets: boom`)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	s.api.CheckCallNames(c, "AllSubnets", "Close")
}

func (s *CreateSuite) TestRunWithUnknownSubnetsFails(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "foo", "10.20.30.0/24", "2001:db8::/64")
	c.Assert(err, gc.ErrorMatches, "unknown subnets specified: 10.20.30.0/24, 2001:db8::/64")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	s.api.CheckCallNames(c, "AllSubnets", "Close")
}

func (s *CreateSuite) TestRunAPIConnectFails(c *gc.C) {
	// TODO(dimitern): Change this once API is implemented.
	s.command = space.NewCreateCommand(nil)
	stdout, stderr, err := s.RunSubCommand(c, "myspace")
	c.Assert(err, gc.ErrorMatches, "cannot connect to API server: API not implemented yet!")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	// No API calls recoreded.
	s.api.CheckCallNames(c)
}
