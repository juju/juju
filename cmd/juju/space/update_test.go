// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
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

func (s *UpdateSuite) TestRunWithSubnetsSucceeds(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "myspace", "10.1.2.0/24", "4.3.2.0/28")
	s.CheckOutputsStderr(c, stdout, stderr, err, `updated space "myspace": changed subnets to 10.1.2.0/24, 4.3.2.0/28\n`)
	s.api.CheckCall(c, 1, "UpdateSpace", "myspace", s.Strings("10.1.2.0/24", "4.3.2.0/28"))
	s.api.CheckCallNames(c, "AllSubnets", "UpdateSpace", "Close")
}

func (s *UpdateSuite) TestRunWhenSubnetsFails(c *gc.C) {
	s.api.SetErrors(errors.New("boom"))

	stdout, stderr, err := s.RunSubCommand(c, "foo", "10.1.2.0/24")
	s.CheckOutputsErr(c, stdout, stderr, err, `cannot fetch available subnets: boom`)
	s.api.CheckCallNames(c, "AllSubnets", "Close")
}

func (s *UpdateSuite) TestRunWithUnknownSubnetsFails(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "foo", "10.20.30.0/24", "2001:db8::/64")
	s.CheckOutputsErr(c, stdout, stderr, err, "unknown subnets specified: 10.20.30.0/24, 2001:db8::/64")
	s.api.CheckCallNames(c, "AllSubnets", "Close")
}

func (s *UpdateSuite) TestRunAPIConnectFails(c *gc.C) {
	// TODO(dimitern): Change this once API is implemented.
	s.command = space.NewUpdateCommand(nil)
	stdout, stderr, err := s.RunSubCommand(c, "myspace", "10.20.30.0/24")
	s.CheckOutputsErr(c, stdout, stderr, err, "cannot connect to API server: API not implemented yet!")
	// No API calls recoreded.
	s.api.CheckCallNames(c)
}
