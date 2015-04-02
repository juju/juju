// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
	coretesting "github.com/juju/juju/testing"
)

var subcommandNames = []string{
	"create",
	"remove",
	"update",
	"rename",
	"help",
}

type SpaceCommandSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&SpaceCommandSuite{})

func (s *SpaceCommandSuite) TestHelpSubcommands(c *gc.C) {
	ctx, err := coretesting.RunCommand(c, s.superCmd, "--help")
	c.Assert(err, jc.ErrorIsNil)

	namesFound := coretesting.ExtractCommandsFromHelpOutput(ctx)
	c.Assert(namesFound, jc.SameContents, subcommandNames)
}

func (s *UpdateSuite) TestInit(c *gc.C) {
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
		args:      s.Strings("%inv#alid"),
		expectErr: `"%inv#alid" is not a valid space name`,
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
		expectErr:  "CIDRs required but not provided",
	}, {
		about:       "all ok - CIDRs updated",
		args:        s.Strings("space-name", "10.10.0.0/24", "2001:db8::1/32"),
		expectName:  "space-name",
		expectCIDRs: s.Strings("10.10.0.0/24", "2001:db8::/32"),
	}} {
		c.Logf("test #%d: %s", i, test.about)
		// Create a new instance of the subcommand for each test, but
		// since we're not running the command no need to use
		// envcmd.Wrap().
		command := space.SpaceCommandBase{}
		err := command.ParseNameAndCIDRs(test.args)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(command.Name, gc.Equals, test.expectName)
		c.Check(command.CIDRs.SortedValues(), jc.DeepEquals, test.expectCIDRs)
	}
}
