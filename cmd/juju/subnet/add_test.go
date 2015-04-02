// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/subnet"
	coretesting "github.com/juju/juju/testing"
)

type AddSuite struct {
	BaseSubnetSuite
}

var _ = gc.Suite(&AddSuite{})

func (s *AddSuite) SetUpTest(c *gc.C) {
	s.BaseSubnetSuite.SetUpTest(c)
	s.command = subnet.NewAddCommand(s.api)
	c.Assert(s.command, gc.NotNil)
}

func (s *AddSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		about       string
		args        []string
		expectCIDR  string
		expectSpace string
		expectErr   string
	}{{
		about:     "no arguments",
		expectErr: "CIDR is required",
	}, {
		about:     "only a subnet argument (invalid)",
		args:      s.Strings("foo"),
		expectErr: "space name is required",
	}, {
		about:       "too many arguments (first two valid)",
		args:        s.Strings("10.0.0.0/8", "new-space", "bar", "baz"),
		expectCIDR:  "10.0.0.0/8",
		expectSpace: "new-space",
		expectErr:   `unrecognized args: \["bar" "baz"\]`,
	}, {
		about:     "invalid CIDR",
		args:      s.Strings("foo", "space"),
		expectErr: `"foo" is not a valid CIDR`,
	}, {
		about:     "incorrectly specified CIDR",
		args:      s.Strings("5.4.3.2/10", "space"),
		expectErr: `"5.4.3.2/10" is not correctly specified, expected "5.0.0.0/10"`,
	}, {
		about:      "invalid space name",
		args:       s.Strings("10.10.0.0/24", "%inv$alid", "zone"),
		expectCIDR: "10.10.0.0/24",
		expectErr:  `"%inv\$alid" is not a valid space name`,
	}} {
		c.Logf("test #%d: %s", i, test.about)
		// Create a new instance of the subcommand for each test, but
		// since we're not running the command no need to use
		// envcmd.Wrap().
		command := subnet.NewAddCommand(s.api)
		err := coretesting.InitCommand(command, test.args)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(command.CIDR, gc.Equals, test.expectCIDR)
		c.Check(command.SpaceName, gc.Equals, test.expectSpace)

		// No API calls should be recorded at this stage.
		s.api.CheckCallNames(c)
	}
}

func (s *AddSuite) TestRunWithIPv4CIDRSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`added subnet "10.20.0.0/24" in space "myspace"\n`,
		"10.20.0.0/24", "myspace",
	)

	s.api.CheckCallNames(c, "AddSubnet", "Close")
	s.api.CheckCall(c, 0, "AddSubnet",
		"10.20.0.0/24", "myspace",
	)
}

func (s *AddSuite) TestRunWithIPv6CIDRSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`added subnet "2001:db8::/32" in space "hyperspace"\n`,
		"2001:db8::/32", "hyperspace",
	)

	s.api.CheckCallNames(c, "AddSubnet", "Close")
	s.api.CheckCall(c, 0, "AddSubnet",
		"2001:db8::/32", "hyperspace",
	)
}

func (s *AddSuite) TestRunWithExistingSubnetFails(c *gc.C) {
	s.api.SetErrors(errors.AlreadyExistsf("subnet %q", "10.10.0.0/24"))

	err := s.AssertRunFails(c,
		`cannot add subnet "10.10.0.0/24": subnet "10.10.0.0/24" already exists`,
		"10.10.0.0/24", "space",
	)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)

	s.api.CheckCallNames(c, "AddSubnet", "Close")
	s.api.CheckCall(c, 0, "AddSubnet",
		"10.10.0.0/24", "space",
	)
}

func (s *AddSuite) TestRunWithNonExistingSpaceFails(c *gc.C) {
	s.api.SetErrors(errors.NotFoundf("space %q", "space"))

	err := s.AssertRunFails(c,
		`cannot add subnet "10.10.0.0/24": space "space" not found`,
		"10.10.0.0/24", "space",
	)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.api.CheckCallNames(c, "AddSubnet", "Close")
	s.api.CheckCall(c, 0, "AddSubnet",
		"10.10.0.0/24", "space",
	)
}

func (s *AddSuite) TestRunAPIConnectFails(c *gc.C) {
	// TODO(dimitern): Change this once API is implemented.
	s.command = subnet.NewAddCommand(nil)
	s.AssertRunFails(c,
		"cannot connect to API server: API not implemented yet!",
		"10.10.0.0/24", "space",
	)

	// No API calls recoreded.
	s.api.CheckCallNames(c)
}
