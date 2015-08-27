// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/subnet"
	"github.com/juju/juju/feature"
	coretesting "github.com/juju/juju/testing"
)

type RemoveSuite struct {
	BaseSubnetSuite
}

var _ = gc.Suite(&RemoveSuite{})

func (s *RemoveSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetFeatureFlags(feature.PostNetCLIMVP)
	s.BaseSubnetSuite.SetUpTest(c)
	s.command = subnet.NewRemoveCommand(s.api)
	c.Assert(s.command, gc.NotNil)
}

func (s *RemoveSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		about      string
		args       []string
		expectCIDR string
		expectErr  string
	}{{
		about:     "no arguments",
		expectErr: "CIDR is required",
	}, {
		about:     "an invalid CIDR",
		args:      s.Strings("foo"),
		expectErr: `"foo" is not a valid CIDR`,
	}, {
		about:      "too many arguments (first is valid)",
		args:       s.Strings("10.0.0.0/8", "bar", "baz"),
		expectCIDR: "10.0.0.0/8",
		expectErr:  `unrecognized args: \["bar" "baz"\]`,
	}, {
		about:     "incorrectly specified CIDR",
		args:      s.Strings("5.4.3.2/10"),
		expectErr: `"5.4.3.2/10" is not correctly specified, expected "5.0.0.0/10"`,
	}} {
		c.Logf("test #%d: %s", i, test.about)
		// Create a new instance of the subcommand for each test, but
		// since we're not running the command no need to use
		// envcmd.Wrap().
		command := subnet.NewRemoveCommand(s.api)
		err := coretesting.InitCommand(command, test.args)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(command.CIDR, gc.Equals, test.expectCIDR)
		}

		// No API calls should be recorded at this stage.
		s.api.CheckCallNames(c)
	}
}

func (s *RemoveSuite) TestRunWithIPv4CIDRSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`marked subnet "10.20.0.0/16" for removal\n`,
		"", // empty stdout.
		"10.20.0.0/16",
	)

	s.api.CheckCallNames(c, "RemoveSubnet", "Close")
	s.api.CheckCall(c, 0, "RemoveSubnet", names.NewSubnetTag("10.20.0.0/16"))
}

func (s *RemoveSuite) TestRunWithIPv6CIDRSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`marked subnet "2001:db8::/32" for removal\n`,
		"", // empty stdout.
		"2001:db8::/32",
	)

	s.api.CheckCallNames(c, "RemoveSubnet", "Close")
	s.api.CheckCall(c, 0, "RemoveSubnet", names.NewSubnetTag("2001:db8::/32"))
}

func (s *RemoveSuite) TestRunWithNonExistingSubnetFails(c *gc.C) {
	s.api.SetErrors(errors.NotFoundf("subnet %q", "10.10.0.0/24"))

	err := s.AssertRunFails(c,
		`cannot remove subnet "10.10.0.0/24": subnet "10.10.0.0/24" not found`,
		"10.10.0.0/24",
	)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.api.CheckCallNames(c, "RemoveSubnet", "Close")
	s.api.CheckCall(c, 0, "RemoveSubnet", names.NewSubnetTag("10.10.0.0/24"))
}

func (s *RemoveSuite) TestRunWithSubnetInUseFails(c *gc.C) {
	s.api.SetErrors(errors.Errorf("subnet %q is still in use", "10.10.0.0/24"))

	s.AssertRunFails(c,
		`cannot remove subnet "10.10.0.0/24": subnet "10.10.0.0/24" is still in use`,
		"10.10.0.0/24",
	)

	s.api.CheckCallNames(c, "RemoveSubnet", "Close")
	s.api.CheckCall(c, 0, "RemoveSubnet", names.NewSubnetTag("10.10.0.0/24"))
}

func (s *RemoveSuite) TestRunAPIConnectFails(c *gc.C) {
	// TODO(dimitern): Change this once API is implemented.
	s.command = subnet.NewRemoveCommand(nil)
	s.AssertRunFails(c,
		"cannot connect to the API server: no environment specified",
		"10.10.0.0/24",
	)

	// No API calls recoreded.
	s.api.CheckCallNames(c)
}
