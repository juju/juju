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

type UpdateSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&UpdateSuite{})

func (s *UpdateSuite) SetUpTest(c *gc.C) {
	s.BaseSpaceSuite.SetUpTest(c)
	s.command = space.NewUpdateCommand(s.api)
	c.Assert(s.command, gc.NotNil)
}

func (s *UpdateSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		about         string
		args          []string
		expectName    string
		expectNewName string
		expectCIDRs   []string
		expectErr     string
	}{{
		about:     "no arguments",
		expectErr: "space name is required",
	}, {
		about:      "invalid space name - with invalid characters",
		args:       s.Strings("%inv#alid"),
		expectName: "%inv#alid",
		expectErr:  `"%inv#alid" is not a valid space name`,
	}, {
		about:         "rename to invalid name",
		args:          s.Strings("space-name", "--rename", "#invalid"),
		expectName:    "space-name",
		expectNewName: "#invalid",
		expectErr:     `"#invalid" is not a valid space name`,
	}, {
		about:      "valid space name with invalid CIDR",
		args:       s.Strings("space-name", "noCIDR"),
		expectName: "space-name",
		expectErr:  `"noCIDR" is not a valid CIDR`,
	}, {
		about:       "valid space with one valid and one invalid CIDR",
		args:        s.Strings("space-name", "10.1.0.0/16", "nonsense"),
		expectName:  "space-name",
		expectCIDRs: s.Strings("10.1.0.0/16"),
		expectErr:   `"nonsense" is not a valid CIDR`,
	}, {
		about:       "valid space with valid but overlapping CIDRs",
		args:        s.Strings("space-name", "10.1.0.0/16", "10.1.0.1/16"),
		expectName:  "space-name",
		expectCIDRs: s.Strings("10.1.0.0/16"),
		expectErr:   `subnet "10.1.0.1/16" overlaps with "10.1.0.0/16"`,
	}, {
		about:       "valid space with valid but duplicated CIDRs",
		args:        s.Strings("space-name", "10.10.0.0/24", "10.10.0.0/24"),
		expectName:  "space-name",
		expectCIDRs: s.Strings("10.10.0.0/24"),
		expectErr:   `duplicate subnet "10.10.0.0/24" specified`,
	}, {
		about:      "valid space name with no other arguments",
		args:       s.Strings("space-name"),
		expectName: "space-name",
		expectErr:  "new name or updated CIDRs required",
	}, {
		about:       "all ok - CIDRs updated",
		args:        s.Strings("space-name", "10.10.0.0/24", "2001:db8::1/32"),
		expectName:  "space-name",
		expectCIDRs: s.Strings("10.10.0.0/24", "2001:db8::/32"),
	}, {
		about:         "all ok - name updated",
		args:          s.Strings("space-name", "--rename", "new-name"),
		expectName:    "space-name",
		expectNewName: "new-name",
	}, {
		about:         "all ok - name and CIDRs updated",
		args:          s.Strings("space-name", "--rename", "new-name", "10.10.0.0/24", "2001:db8::1/32"),
		expectName:    "space-name",
		expectNewName: "new-name",
		expectCIDRs:   s.Strings("10.10.0.0/24", "2001:db8::/32"),
	}} {
		c.Logf("test #%d: %s", i, test.about)
		// Create a new instance of the subcommand for each test, but
		// since we're not running the command no need to use
		// envcmd.Wrap().
		command := space.NewUpdateCommand(s.api)
		err := coretesting.InitCommand(command, test.args)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(command.Name, gc.Equals, test.expectName)
		c.Check(command.NewName, gc.Equals, test.expectNewName)
		c.Check(command.CIDRs.SortedValues(), jc.DeepEquals, test.expectCIDRs)
		// No API calls should be recorded at this stage.
		s.api.CheckCallNames(c)
	}
}

func (s *UpdateSuite) TestRunWithSubnetsSucceeds(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "myspace", "10.1.2.0/24", "4.3.2.0/28")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `updated space "myspace": no rename, changed subnets to 10.1.2.0/24, 4.3.2.0/28\n`)
	s.api.CheckCalls(c, []testing.StubCall{{
		FuncName: "AllSubnets",
		Args:     nil,
	}, {
		FuncName: "UpdateSpace",
		Args:     []interface{}{"myspace", "", s.Strings("10.1.2.0/24", "4.3.2.0/28")},
	}, {
		FuncName: "Close",
		Args:     nil,
	}})
}

func (s *UpdateSuite) TestRunWithRenameSucceeds(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "myspace", "--rename", "facebook")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `updated space "myspace": renamed to facebook, deleted all subnets\n`)
	s.api.CheckCalls(c, []testing.StubCall{{
		FuncName: "UpdateSpace",
		Args:     []interface{}{"myspace", "facebook", []string{}},
	}, {
		FuncName: "Close",
		Args:     nil,
	}})
}

func (s *UpdateSuite) TestRunWithRenameAndSubnetsSucceeds(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "myspace", "--rename", "facebook", "10.1.2.0/24", "4.3.2.0/28")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `updated space "myspace": renamed to facebook, changed subnets to 10.1.2.0/24, 4.3.2.0/28\n`)
	s.api.CheckCalls(c, []testing.StubCall{{
		FuncName: "AllSubnets",
		Args:     nil,
	}, {
		FuncName: "UpdateSpace",
		Args:     []interface{}{"myspace", "facebook", s.Strings("10.1.2.0/24", "4.3.2.0/28")},
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
