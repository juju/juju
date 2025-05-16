// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/rpc/params"
)

type AddSuite struct {
	BaseSpaceSuite
}

func TestAddSuite(t *stdtesting.T) { tc.Run(t, &AddSuite{}) }
func (s *AddSuite) SetUpTest(c *tc.C) {
	s.BaseSpaceSuite.SetUpTest(c)
	s.newCommand = space.NewAddCommand
}

func (s *AddSuite) TestRunWithoutSubnetsSucceeds(c *tc.C) {
	s.AssertRunSucceeds(c,
		`added space "myspace" with no subnets\n`,
		"", // no stdout, just stderr
		"myspace",
	)

	s.api.CheckCallNames(c, "AddSpace", "Close")
	s.api.CheckCall(c, 0, "AddSpace", "myspace", []string(nil), true)
}

func (s *AddSuite) TestRunWithSubnetsSucceeds(c *tc.C) {
	s.AssertRunSucceeds(c,
		`added space "myspace" with subnets 10.1.2.0/24, 4.3.2.0/28\n`,
		"", // no stdout, just stderr
		"myspace", "10.1.2.0/24", "4.3.2.0/28",
	)

	s.api.CheckCallNames(c, "AddSpace", "Close")
	s.api.CheckCall(c,
		0, "AddSpace",
		"myspace", s.Strings("10.1.2.0/24", "4.3.2.0/28"), true,
	)
}

func (s *AddSuite) TestRunWhenSpacesNotSupported(c *tc.C) {
	s.api.SetErrors(errors.NewNotSupported(nil, "spaces not supported"))

	err := s.AssertRunSpacesNotSupported(c,
		`cannot add space "foo": spaces not supported`,
		"foo", "10.1.2.0/24",
	)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)

	s.api.CheckCallNames(c, "AddSpace", "Close")
	s.api.CheckCall(c, 0, "AddSpace", "foo", s.Strings("10.1.2.0/24"), true)
}

func (s *AddSuite) TestRunWhenSpacesAPIFails(c *tc.C) {
	s.api.SetErrors(errors.New("API error"))

	s.AssertRunFails(c,
		`cannot add space "foo": API error`,
		"foo", "10.1.2.0/24",
	)

	s.api.CheckCallNames(c, "AddSpace", "Close")
	s.api.CheckCall(c, 0, "AddSpace", "foo", s.Strings("10.1.2.0/24"), true)
}

func (s *AddSuite) TestRunUnauthorizedMentionsJujuGrant(c *tc.C) {
	s.api.SetErrors(&params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	})

	s.AssertRunFailsUnauthorized(c,
		`*.juju grant.*`,
		"foo", "10.1.2.0/24",
	)
}

func (s *AddSuite) TestRunWhenSpacesBlocked(c *tc.C) {
	s.api.SetErrors(&params.Error{Code: params.CodeOperationBlocked, Message: "nope"})
	stdout, stderr, err := s.RunCommand(c, "foo", "10.1.2.0/24")
	c.Assert(err, tc.ErrorMatches, `
cannot add space "foo": nope

All operations that change model have been disabled for the current model.
To enable changes, run

    juju enable-command all

`[1:])
	c.Assert(stderr, tc.Equals, "")
	c.Assert(stdout, tc.Equals, "")

	s.api.CheckCallNames(c, "AddSpace", "Close")
	s.api.CheckCall(c, 0, "AddSpace", "foo", s.Strings("10.1.2.0/24"), true)
}
