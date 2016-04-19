// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
)

type AddSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&AddSuite{})

func (s *AddSuite) SetUpTest(c *gc.C) {
	s.BaseSpaceSuite.SetUpTest(c)
	s.command = space.NewAddCommandForTest(s.api)
	c.Assert(s.command, gc.NotNil)
}

func (s *AddSuite) TestRunWithoutSubnetsSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`addd space "myspace" with no subnets\n`,
		"", // no stdout, just stderr
		"myspace",
	)

	s.api.CheckCallNames(c, "AddSpace", "Close")
	s.api.CheckCall(c, 0, "AddSpace", "myspace", []string(nil), true)
}

func (s *AddSuite) TestRunWithSubnetsSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`addd space "myspace" with subnets 10.1.2.0/24, 4.3.2.0/28\n`,
		"", // no stdout, just stderr
		"myspace", "10.1.2.0/24", "4.3.2.0/28",
	)

	s.api.CheckCallNames(c, "AddSpace", "Close")
	s.api.CheckCall(c,
		0, "AddSpace",
		"myspace", s.Strings("10.1.2.0/24", "4.3.2.0/28"), true,
	)
}

func (s *AddSuite) TestRunWhenSpacesNotSupported(c *gc.C) {
	s.api.SetErrors(errors.NewNotSupported(nil, "spaces not supported"))

	err := s.AssertRunSpacesNotSupported(c,
		`cannot add space "foo": spaces not supported`,
		"foo", "10.1.2.0/24",
	)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)

	s.api.CheckCallNames(c, "AddSpace", "Close")
	s.api.CheckCall(c, 0, "AddSpace", "foo", s.Strings("10.1.2.0/24"), true)
}

func (s *AddSuite) TestRunWhenSpacesAPIFails(c *gc.C) {
	s.api.SetErrors(errors.New("API error"))

	s.AssertRunFails(c,
		`cannot add space "foo": API error`,
		"foo", "10.1.2.0/24",
	)

	s.api.CheckCallNames(c, "AddSpace", "Close")
	s.api.CheckCall(c, 0, "AddSpace", "foo", s.Strings("10.1.2.0/24"), true)
}
