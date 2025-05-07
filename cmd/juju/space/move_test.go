// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/juju/space"
)

type MoveSuite struct {
	BaseSpaceSuite
}

var _ = tc.Suite(&MoveSuite{})

func (s *MoveSuite) SetUpTest(c *tc.C) {
	s.BaseSpaceSuite.SetUpTest(c)
	s.newCommand = space.NewMoveCommand
}

func (s *MoveSuite) TestInit(c *tc.C) {
	for i, test := range []struct {
		about         string
		args          []string
		cidrsOptional bool

		expectName  string
		expectCIDRs []string
		expectErr   string
	}{{
		about:     "no arguments",
		expectErr: "invalid arguments specified: space name is required",
	}, {
		about:     "invalid space name - with invalid characters",
		args:      s.Strings("%inv#alid"),
		expectErr: `invalid arguments specified: "%inv#alid" is not a valid space name`,
	}, {
		about:      "valid space name with invalid CIDR",
		args:       s.Strings("space-name", "noCIDR"),
		expectName: "space-name",
		expectErr:  `invalid arguments specified: "noCIDR" is not a valid CIDR`,
	}, {
		about:         "valid space with one valid and one invalid CIDR (CIDRs required)",
		args:          s.Strings("space-name", "10.1.0.0/16", "nonsense"),
		cidrsOptional: false,
		expectName:    "space-name",
		expectCIDRs:   s.Strings("10.1.0.0/16"),
		expectErr:     `invalid arguments specified: "nonsense" is not a valid CIDR`,
	}, {
		about:         "valid space with one valid and one invalid CIDR (CIDRs optional)",
		args:          s.Strings("space-name", "10.1.0.0/16", "nonsense"),
		expectName:    "space-name",
		cidrsOptional: true,
		expectCIDRs:   s.Strings("10.1.0.0/16"),
		expectErr:     `invalid arguments specified: "nonsense" is not a valid CIDR`,
	}, {
		about:       "valid space with valid but overlapping CIDRs",
		args:        s.Strings("space-name", "10.1.0.0/16", "10.1.0.1/16"),
		expectName:  "space-name",
		expectCIDRs: s.Strings("10.1.0.0/16"),
		expectErr:   `invalid arguments specified: subnet "10.1.0.1/16" overlaps with "10.1.0.0/16"`,
	}, {
		about:       "valid space with valid but duplicated CIDRs",
		args:        s.Strings("space-name", "10.10.0.0/24", "10.10.0.0/24"),
		expectName:  "space-name",
		expectCIDRs: s.Strings("10.10.0.0/24"),
		expectErr:   `invalid arguments specified: duplicate subnet "10.10.0.0/24" specified`,
	}, {
		about:         "valid space name with no other arguments (CIDRs required)",
		args:          s.Strings("space-name"),
		cidrsOptional: false,
		expectName:    "space-name",
		expectErr:     "invalid arguments specified: CIDRs required but not provided",
		expectCIDRs:   s.Strings(),
	}, {
		about:       "all ok - CIDRs updated",
		args:        s.Strings("space-name", "10.10.0.0/24", "2001:db8::1/32"),
		expectName:  "space-name",
		expectCIDRs: s.Strings("10.10.0.0/24", "2001:db8::/32"),
	}} {
		c.Logf("test #%d: %s", i, test.about)
		command, err := s.InitCommand(c, test.args...)
		if test.expectErr != "" {
			c.Check(err, tc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
			command := command.(*space.MoveCommand)
			c.Check(command.Name, tc.Equals, test.expectName)
			c.Check(command.CIDRs.SortedValues(), tc.DeepEquals, test.expectCIDRs)
		}
	}
}

func (s *MoveSuite) TestOutput(c *tc.C) {
	assertAPICalls := func() {
		s.api.CheckCallNames(c, "SubnetsByCIDR", "MoveSubnets", "Close")
		s.api.ResetCalls()
	}
	makeArgs := func(format string, extraArgs ...string) []string {
		args := s.Strings(extraArgs...)
		if format != "" {
			args = append(args, "--format", format)
		}
		return args
	}
	assertOutput := func(args []string, expected string) {
		s.AssertRunSucceeds(c, "", expected, args...)
	}

	for i, test := range []struct {
		expected string
		format   string
		args     []string
	}{
		{
			expected: "Subnet 2001:db8::/32 moved from internal to public\n",
			args:     []string{"public", "2001:db8::/32"},
		},
		{
			expected: "Subnet 2001:db8::/32 moved from internal to public\n",
			format:   "human",
			args:     []string{"public", "2001:db8::/32"},
		},
		{
			expected: `
Space From	Space To	CIDR         
internal  	public  	2001:db8::/32
          	        	             
`[1:],
			format: "tabular",
			args:   []string{"public", "2001:db8::/32"},
		},
		{
			expected: `[{"from":"internal","to":"public","cidr":"2001:db8::/32"}]
`,
			format: "json",
			args:   []string{"public", "2001:db8::/32"},
		},
		{
			expected: `
- from: internal
  to: public
  cidr: 2001:db8::/32
`[1:],
			format: "yaml",
			args:   []string{"public", "2001:db8::/32"},
		},
	} {
		c.Logf("test #%d", i)
		assertOutput(makeArgs(test.format, test.args...), test.expected)
		assertAPICalls()
	}
}

func (s *MoveSuite) TestWithNoSubnetTags(c *tc.C) {
	s.api.SubnetsByCIDRResp = s.api.SubnetsByCIDRResp[0:0]

	expected := "error getting subnet tags for 2001:db8::/32"
	s.AssertRunFails(c, expected, []string{"public", "2001:db8::/32"}...)

	s.api.CheckCallNames(c, "SubnetsByCIDR", "Close")
}

func (s *MoveSuite) TestWhenAPIFails(c *tc.C) {
	s.api.SetErrors(errors.New("boom"))

	expected := "failed to get subnets by CIDR: boom"
	s.AssertRunFails(c, expected, []string{"public", "2001:db8::/32"}...)

	s.api.CheckCallNames(c, "SubnetsByCIDR", "Close")
}
