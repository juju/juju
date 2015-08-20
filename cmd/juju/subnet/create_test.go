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

type CreateSuite struct {
	BaseSubnetSuite
}

var _ = gc.Suite(&CreateSuite{})

func (s *CreateSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetFeatureFlags(feature.PostNetCLIMVP)
	s.BaseSubnetSuite.SetUpTest(c)
	s.command = subnet.NewCreateCommand(s.api)
	c.Assert(s.command, gc.NotNil)
}

func (s *CreateSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		about         string
		args          []string
		expectCIDR    string
		expectSpace   string
		expectZones   []string
		expectPublic  bool
		expectPrivate bool
		expectErr     string
	}{{
		about:         "no arguments",
		expectErr:     "CIDR is required",
		expectPrivate: true,
	}, {
		about:         "only a subnet argument (invalid)",
		args:          s.Strings("foo"),
		expectPrivate: true,
		expectErr:     "space name is required",
	}, {
		about:         "no zone arguments (both CIDR and space are invalid)",
		args:          s.Strings("foo", "%invalid"),
		expectPrivate: true,
		expectErr:     "at least one zone is required",
	}, {
		about:         "invalid CIDR",
		args:          s.Strings("foo", "space", "zone"),
		expectPrivate: true,
		expectErr:     `"foo" is not a valid CIDR`,
	}, {
		about:         "incorrectly specified CIDR",
		args:          s.Strings("5.4.3.2/10", "space", "zone"),
		expectPrivate: true,
		expectErr:     `"5.4.3.2/10" is not correctly specified, expected "5.0.0.0/10"`,
	}, {
		about:         "invalid space name",
		args:          s.Strings("10.10.0.0/24", "%inv$alid", "zone"),
		expectCIDR:    "10.10.0.0/24",
		expectPrivate: true,
		expectErr:     `"%inv\$alid" is not a valid space name`,
	}, {
		about:         "duplicate zones specified",
		args:          s.Strings("10.10.0.0/24", "myspace", "zone1", "zone2", "zone1"),
		expectCIDR:    "10.10.0.0/24",
		expectSpace:   "myspace",
		expectZones:   s.Strings("zone1", "zone2"),
		expectPrivate: true,
		expectErr:     `duplicate zone "zone1" specified`,
	}, {
		about:         "both --public and --private specified",
		args:          s.Strings("10.1.0.0/16", "new-space", "zone", "--public", "--private"),
		expectCIDR:    "10.1.0.0/16",
		expectSpace:   "new-space",
		expectZones:   s.Strings("zone"),
		expectErr:     `cannot specify both --public and --private`,
		expectPublic:  true,
		expectPrivate: true,
	}, {
		about:         "--public specified",
		args:          s.Strings("10.1.0.0/16", "new-space", "zone", "--public"),
		expectCIDR:    "10.1.0.0/16",
		expectSpace:   "new-space",
		expectZones:   s.Strings("zone"),
		expectPublic:  true,
		expectPrivate: false,
		expectErr:     "",
	}, {
		about:         "--private explicitly specified",
		args:          s.Strings("10.1.0.0/16", "new-space", "zone", "--private"),
		expectCIDR:    "10.1.0.0/16",
		expectSpace:   "new-space",
		expectZones:   s.Strings("zone"),
		expectPublic:  false,
		expectPrivate: true,
		expectErr:     "",
	}, {
		about:         "--private specified out of order",
		args:          s.Strings("2001:db8::/32", "--private", "space", "zone"),
		expectCIDR:    "2001:db8::/32",
		expectSpace:   "space",
		expectZones:   s.Strings("zone"),
		expectPublic:  false,
		expectPrivate: true,
		expectErr:     "",
	}, {
		about:         "--public specified twice",
		args:          s.Strings("--public", "2001:db8::/32", "--public", "space", "zone"),
		expectCIDR:    "2001:db8::/32",
		expectSpace:   "space",
		expectZones:   s.Strings("zone"),
		expectPublic:  true,
		expectPrivate: false,
		expectErr:     "",
	}} {
		c.Logf("test #%d: %s", i, test.about)
		// Create a new instance of the subcommand for each test, but
		// since we're not running the command no need to use
		// envcmd.Wrap().
		command := subnet.NewCreateCommand(s.api)
		err := coretesting.InitCommand(command, test.args)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(command.CIDR.Id(), gc.Equals, test.expectCIDR)
			c.Check(command.Space.Id(), gc.Equals, test.expectSpace)
			c.Check(command.Zones.SortedValues(), jc.DeepEquals, test.expectZones)
			c.Check(command.IsPublic, gc.Equals, test.expectPublic)
			c.Check(command.IsPrivate, gc.Equals, test.expectPrivate)
		}
		// No API calls should be recorded at this stage.
		s.api.CheckCallNames(c)
	}
}

func (s *CreateSuite) TestRunOneZoneSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`created a private subnet "10.20.0.0/24" in space "myspace" with zones zone1\n`,
		"", // empty stdout.
		"10.20.0.0/24", "myspace", "zone1",
	)

	s.api.CheckCallNames(c, "AllZones", "CreateSubnet", "Close")
	s.api.CheckCall(c, 1, "CreateSubnet",
		names.NewSubnetTag("10.20.0.0/24"), names.NewSpaceTag("myspace"), s.Strings("zone1"), false,
	)
}

func (s *CreateSuite) TestRunWithPublicAndIPv6CIDRSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`created a public subnet "2001:db8::/32" in space "space" with zones zone1\n`,
		"", // empty stdout.
		"2001:db8::/32", "space", "zone1", "--public",
	)

	s.api.CheckCallNames(c, "AllZones", "CreateSubnet", "Close")
	s.api.CheckCall(c, 1, "CreateSubnet",
		names.NewSubnetTag("2001:db8::/32"), names.NewSpaceTag("space"), s.Strings("zone1"), true,
	)
}

func (s *CreateSuite) TestRunWithMultipleZonesSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		// The list of zones is sorted both when displayed and passed
		// to CreateSubnet.
		`created a private subnet "10.20.0.0/24" in space "foo" with zones zone1, zone2\n`,
		"",                                      // empty stdout.
		"10.20.0.0/24", "foo", "zone2", "zone1", // unsorted zones
	)

	s.api.CheckCallNames(c, "AllZones", "CreateSubnet", "Close")
	s.api.CheckCall(c, 1, "CreateSubnet",
		names.NewSubnetTag("10.20.0.0/24"), names.NewSpaceTag("foo"), s.Strings("zone1", "zone2"), false,
	)
}

func (s *CreateSuite) TestRunWithAllZonesErrorFails(c *gc.C) {
	s.api.SetErrors(errors.New("boom"))

	s.AssertRunFails(c,
		`cannot fetch availability zones: boom`,
		"10.10.0.0/24", "space", "zone1",
	)
	s.api.CheckCallNames(c, "AllZones", "Close")
}

func (s *CreateSuite) TestRunWithExistingSubnetFails(c *gc.C) {
	s.api.SetErrors(nil, errors.AlreadyExistsf("subnet %q", "10.10.0.0/24"))

	err := s.AssertRunFails(c,
		`cannot create subnet "10.10.0.0/24": subnet "10.10.0.0/24" already exists`,
		"10.10.0.0/24", "space", "zone1",
	)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)

	s.api.CheckCallNames(c, "AllZones", "CreateSubnet", "Close")
	s.api.CheckCall(c, 1, "CreateSubnet",
		names.NewSubnetTag("10.10.0.0/24"), names.NewSpaceTag("space"), s.Strings("zone1"), false,
	)
}

func (s *CreateSuite) TestRunWithNonExistingSpaceFails(c *gc.C) {
	s.api.SetErrors(nil, errors.NotFoundf("space %q", "space"))

	err := s.AssertRunFails(c,
		`cannot create subnet "10.10.0.0/24": space "space" not found`,
		"10.10.0.0/24", "space", "zone1",
	)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.api.CheckCallNames(c, "AllZones", "CreateSubnet", "Close")
	s.api.CheckCall(c, 1, "CreateSubnet",
		names.NewSubnetTag("10.10.0.0/24"), names.NewSpaceTag("space"), s.Strings("zone1"), false,
	)
}

func (s *CreateSuite) TestRunWithUnknownZonesFails(c *gc.C) {
	s.AssertRunFails(c,
		// The list of unknown zones is sorted.
		"unknown zones specified: foo, no-zone",
		"10.30.30.0/24", "space", "no-zone", "zone1", "foo",
	)

	s.api.CheckCallNames(c, "AllZones", "Close")
}

func (s *CreateSuite) TestRunAPIConnectFails(c *gc.C) {
	// TODO(dimitern): Change this once API is implemented.
	s.command = subnet.NewCreateCommand(nil)
	s.AssertRunFails(c,
		"cannot connect to the API server: no environment specified",
		"10.10.0.0/24", "space", "zone1",
	)

	// No API calls recoreded.
	s.api.CheckCallNames(c)
}
