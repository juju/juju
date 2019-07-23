// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet_test

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/subnet"
	"github.com/juju/juju/core/network"
)

type AddSuite struct {
	BaseSubnetSuite
}

var _ = gc.Suite(&AddSuite{})

func (s *AddSuite) SetUpTest(c *gc.C) {
	s.BaseSubnetSuite.SetUpTest(c)
	s.newCommand = subnet.NewAddCommand
}

func (s *AddSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		about            string
		args             []string
		expectCIDR       string
		expectRawCIDR    string
		expectProviderId string
		expectSpace      string
		expectZones      []string
		expectErr        string
	}{{
		about:     "no arguments",
		expectErr: "either CIDR or provider ID is required",
	}, {
		about:     "single argument - invalid CIDR: space name is required",
		args:      s.Strings("anything"),
		expectErr: "space name is required",
	}, {
		about:     "single argument - valid CIDR: space name is required",
		args:      s.Strings("10.0.0.0/8"),
		expectErr: "space name is required",
	}, {
		about:     "single argument - incorrect CIDR: space name is required",
		args:      s.Strings("10.10.0.0/8"),
		expectErr: "space name is required",
	}, {
		about:            "two arguments: an invalid CIDR is assumed to mean ProviderId",
		args:             s.Strings("foo", "bar"),
		expectProviderId: "foo",
		expectSpace:      "bar",
	}, {
		about:         "two arguments: an incorrectly specified CIDR is fixed",
		args:          s.Strings("10.10.0.0/8", "bar"),
		expectCIDR:    "10.0.0.0/8",
		expectRawCIDR: "10.10.0.0/8",
		expectSpace:   "bar",
	}, {
		about:         "more arguments parsed as zones",
		args:          s.Strings("10.0.0.0/8", "new-space", "zone1", "zone2"),
		expectCIDR:    "10.0.0.0/8",
		expectRawCIDR: "10.0.0.0/8",
		expectSpace:   "new-space",
		expectZones:   s.Strings("zone1", "zone2"),
	}, {
		about:         "CIDR and invalid space name, one zone",
		args:          s.Strings("10.10.0.0/24", "%inv$alid", "zone"),
		expectCIDR:    "10.10.0.0/24",
		expectRawCIDR: "10.10.0.0/24",
		expectErr:     `"%inv\$alid" is not a valid space name`,
	}, {
		about:         "incorrect CIDR and invalid space name, no zones",
		args:          s.Strings("10.10.0.0/8", "%inv$alid"),
		expectCIDR:    "10.0.0.0/8",
		expectRawCIDR: "10.10.0.0/8",
		expectErr:     `"%inv\$alid" is not a valid space name`,
	}, {
		about:            "ProviderId and invalid space name, two zones",
		args:             s.Strings("foo", "%inv$alid", "zone1", "zone2"),
		expectProviderId: "foo",
		expectErr:        `"%inv\$alid" is not a valid space name`,
	}} {
		c.Logf("test #%d: %s", i, test.about)
		command, err := s.InitCommand(c, test.args...)
		if test.expectErr != "" {
			prefixedErr := "invalid arguments specified: " + test.expectErr
			c.Check(err, gc.ErrorMatches, prefixedErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
			command := command.(*subnet.AddCommand)
			c.Check(command.CIDR.Id(), gc.Equals, test.expectCIDR)
			c.Check(command.RawCIDR, gc.Equals, test.expectRawCIDR)
			c.Check(command.ProviderId, gc.Equals, test.expectProviderId)
			c.Check(command.Space.Id(), gc.Equals, test.expectSpace)
			c.Check(command.Zones, jc.DeepEquals, test.expectZones)
		}
		// No API calls should be recorded at this stage.
		s.api.CheckCallNames(c)
	}
}

func (s *AddSuite) TestRunWithIPv4CIDRSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`added subnet with CIDR "10.20.0.0/24" in space "myspace"\n`,
		"", // empty stdout.
		"10.20.0.0/24", "myspace",
	)

	s.api.CheckCallNames(c, "AddSubnet", "Close")
	s.api.CheckCall(c, 0, "AddSubnet",
		names.NewSubnetTag("10.20.0.0/24"),
		network.Id(""),
		names.NewSpaceTag("myspace"),
		[]string(nil),
	)
}

func (s *AddSuite) TestRunWithIncorrectlyGivenCIDRSucceedsWithWarning(c *gc.C) {
	expectStderr := strings.Join([]string{
		"(.|\n)*",
		"WARNING: using CIDR \"10.0.0.0/8\" instead of ",
		"the incorrectly specified \"10.10.0.0/8\".\n",
		"added subnet with CIDR \"10.0.0.0/8\" in space \"myspace\"\n",
	}, "")

	s.AssertRunSucceeds(c,
		expectStderr,
		"", // empty stdout.
		"10.10.0.0/8", "myspace",
	)

	s.api.CheckCallNames(c, "AddSubnet", "Close")
	s.api.CheckCall(c, 0, "AddSubnet",
		names.NewSubnetTag("10.0.0.0/8"),
		network.Id(""),
		names.NewSpaceTag("myspace"),
		[]string(nil),
	)
}

func (s *AddSuite) TestRunWithProviderIdSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`added subnet with ProviderId "foo" in space "myspace"\n`,
		"", // empty stdout.
		"foo", "myspace", "zone1", "zone2",
	)

	s.api.CheckCallNames(c, "AddSubnet", "Close")
	s.api.CheckCall(c, 0, "AddSubnet",
		names.SubnetTag{},
		network.Id("foo"),
		names.NewSpaceTag("myspace"),
		s.Strings("zone1", "zone2"),
	)
}

func (s *AddSuite) TestRunWithIPv6CIDRSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`added subnet with CIDR "2001:db8::/32" in space "hyperspace"\n`,
		"", // empty stdout.
		"2001:db8::/32", "hyperspace",
	)

	s.api.CheckCallNames(c, "AddSubnet", "Close")
	s.api.CheckCall(c, 0, "AddSubnet",
		names.NewSubnetTag("2001:db8::/32"),
		network.Id(""),
		names.NewSpaceTag("hyperspace"),
		[]string(nil),
	)
}

func (s *AddSuite) TestRunWithExistingSubnetFails(c *gc.C) {
	s.api.SetErrors(errors.AlreadyExistsf("subnet %q", "10.10.0.0/24"))

	err := s.AssertRunFails(c,
		`cannot add subnet: subnet "10.10.0.0/24" already exists`,
		"10.10.0.0/24", "space",
	)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)

	s.api.CheckCallNames(c, "AddSubnet", "Close")
	s.api.CheckCall(c, 0, "AddSubnet",
		names.NewSubnetTag("10.10.0.0/24"),
		network.Id(""),
		names.NewSpaceTag("space"),
		[]string(nil),
	)
}

func (s *AddSuite) TestRunWithNonExistingSpaceFails(c *gc.C) {
	s.api.SetErrors(errors.NotFoundf("space %q", "space"))

	err := s.AssertRunFails(c,
		`cannot add subnet: space "space" not found`,
		"10.10.0.0/24", "space", "zone1", "zone2",
	)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.api.CheckCallNames(c, "AddSubnet", "Close")
	s.api.CheckCall(c, 0, "AddSubnet",
		names.NewSubnetTag("10.10.0.0/24"),
		network.Id(""),
		names.NewSpaceTag("space"),
		s.Strings("zone1", "zone2"),
	)
}

func (s *AddSuite) TestRunUnauthorizedMentionsJujuGrant(c *gc.C) {
	s.api.SetErrors(&params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	})
	_, stderr, _ := s.RunCommand(c, "10.10.0.0/24", "myspace")
	c.Assert(strings.Replace(stderr, "\n", " ", -1), gc.Matches, `.*juju grant.*`)
}

func (s *AddSuite) TestRunWithAmbiguousCIDRDisplaysError(c *gc.C) {
	apiError := errors.New(`multiple subnets with CIDR "10.10.0.0/24" <snip>`)
	s.api.SetErrors(apiError)

	s.AssertRunSucceeds(c,
		fmt.Sprintf("ERROR: %v.\n", apiError),
		"",
		"10.10.0.0/24", "space", "zone1", "zone2",
	)

	s.api.CheckCallNames(c, "AddSubnet", "Close")
	s.api.CheckCall(c, 0, "AddSubnet",
		names.NewSubnetTag("10.10.0.0/24"),
		network.Id(""),
		names.NewSpaceTag("space"),
		s.Strings("zone1", "zone2"),
	)
}
