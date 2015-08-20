// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
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

func (s *CreateSuite) TestRunWithoutSubnetsSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`created space "myspace" with no subnets\n`,
		"", // no stdout, just stderr
		"myspace",
	)

	s.api.CheckCallNames(c, "CreateSpace", "Close")
	s.api.CheckCall(c, 0, "CreateSpace", "myspace", []string(nil), true)
}

func (s *CreateSuite) TestRunWithSubnetsSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`created space "myspace" with subnets 10.1.2.0/24, 4.3.2.0/28\n`,
		"", // no stdout, just stderr
		"myspace", "10.1.2.0/24", "4.3.2.0/28",
	)

	s.api.CheckCallNames(c, "CreateSpace", "Close")
	s.api.CheckCall(c,
		0, "CreateSpace",
		"myspace", s.Strings("10.1.2.0/24", "4.3.2.0/28"), true,
	)
}

func (s *CreateSuite) TestRunWhenSpacesAPIFails(c *gc.C) {
	s.api.SetErrors(errors.New("API error"))

	s.AssertRunFails(c,
		`cannot create space "foo": API error`,
		"foo", "10.1.2.0/24",
	)

	s.api.CheckCallNames(c, "CreateSpace", "Close")
	s.api.CheckCall(c, 0, "CreateSpace", "foo", s.Strings("10.1.2.0/24"), true)
}

func (s *CreateSuite) TestRunAPIConnectFails(c *gc.C) {
	s.command = space.NewCreateCommand(nil)
	s.AssertRunFails(c,
		"cannot connect to the API server: no environment specified",
		"myspace", "10.20.30.0/24",
	)
	// No API calls recoreded.
	s.api.CheckCallNames(c)
}
