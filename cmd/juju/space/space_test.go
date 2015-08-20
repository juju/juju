// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/feature"
	coretesting "github.com/juju/juju/testing"
)

var mvpSubcommandNames = []string{
	"create",
	"list",
	"help",
}

var postMVPSubcommandNames = []string{
	"remove",
	"update",
	"rename",
}

type SpaceCommandSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&SpaceCommandSuite{})

func (s *SpaceCommandSuite) TestHelpSubcommandsMVP(c *gc.C) {
	s.BaseSuite.SetFeatureFlags()
	s.BaseSpaceSuite.SetUpTest(c) // looks evil, but works fine

	ctx, err := coretesting.RunCommand(c, s.superCmd, "--help")
	c.Assert(err, jc.ErrorIsNil)

	namesFound := coretesting.ExtractCommandsFromHelpOutput(ctx)
	c.Assert(namesFound, jc.SameContents, mvpSubcommandNames)
}

func (s *SpaceCommandSuite) TestHelpSubcommandsPostMVP(c *gc.C) {
	s.BaseSuite.SetFeatureFlags(feature.PostNetCLIMVP)
	s.BaseSpaceSuite.SetUpTest(c) // looks evil, but works fine

	ctx, err := coretesting.RunCommand(c, s.superCmd, "--help")
	c.Assert(err, jc.ErrorIsNil)

	namesFound := coretesting.ExtractCommandsFromHelpOutput(ctx)
	allSubcommandNames := append(mvpSubcommandNames, postMVPSubcommandNames...)
	c.Assert(namesFound, jc.SameContents, allSubcommandNames)
}

func (s *SpaceCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		about         string
		args          []string
		cidrsOptional bool

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
		about:         "valid space with one valid and one invalid CIDR (CIDRs required)",
		args:          s.Strings("space-name", "10.1.0.0/16", "nonsense"),
		cidrsOptional: false,
		expectName:    "space-name",
		expectCIDRs:   s.Strings("10.1.0.0/16"),
		expectErr:     `"nonsense" is not a valid CIDR`,
	}, {
		about:         "valid space with one valid and one invalid CIDR (CIDRs optional)",
		args:          s.Strings("space-name", "10.1.0.0/16", "nonsense"),
		expectName:    "space-name",
		cidrsOptional: true,
		expectCIDRs:   s.Strings("10.1.0.0/16"),
		expectErr:     `"nonsense" is not a valid CIDR`,
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
		about:         "valid space name with no other arguments (CIDRs required)",
		args:          s.Strings("space-name"),
		cidrsOptional: false,
		expectName:    "space-name",
		expectErr:     "CIDRs required but not provided",
		expectCIDRs:   s.Strings(),
	}, {
		about:         "valid space name with no other arguments (CIDRs optional)",
		args:          s.Strings("space-name"),
		cidrsOptional: true,
		expectName:    "space-name",
		expectCIDRs:   s.Strings(),
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
		name, CIDRs, err := space.ParseNameAndCIDRs(test.args, test.cidrsOptional)
		if test.expectErr != "" {
			prefixedErr := "invalid arguments specified: " + test.expectErr
			c.Check(err, gc.ErrorMatches, prefixedErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(name, gc.Equals, test.expectName)
		c.Check(CIDRs.SortedValues(), jc.DeepEquals, test.expectCIDRs)
	}
}
