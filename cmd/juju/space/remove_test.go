// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
	coretesting "github.com/juju/juju/testing"
)

type RemoveSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&RemoveSuite{})

func (s *RemoveSuite) SetUpTest(c *gc.C) {
	s.BaseSpaceSuite.SetUpTest(c)
	s.command = space.NewRemoveCommand(s.api)
	c.Assert(s.command, gc.NotNil)
}

func (s *RemoveSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		about      string
		args       []string
		expectName string
		expectErr  string
	}{{
		about:     "no arguments",
		expectErr: "space name is required",
	}, {
		about:     "invalid space name - with invalid characters",
		args:      s.Strings("%inv$alid"),
		expectErr: `"%inv\$alid" is not a valid space name`,
	}, {
		about:     "invalid space name - using underscores",
		args:      s.Strings("42_space"),
		expectErr: `"42_space" is not a valid space name`,
	}, {
		about:      "multiple space names aren't allowed",
		args:       s.Strings("a-space", "another-space"),
		expectErr:  `unrecognized args: \["another-space"\]`,
		expectName: "a-space",
	}, {
		about:      "delete a valid space name",
		args:       s.Strings("myspace"),
		expectName: "myspace",
	}} {
		c.Logf("test #%d: %s", i, test.about)
		// Create a new instance of the subcommand for each test, but
		// since we're not running the command no need to use
		// envcmd.Wrap().
		command := space.NewRemoveCommand(s.api)
		err := coretesting.InitCommand(command, test.args)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(command.Name, gc.Equals, test.expectName)
		// No API calls should be recorded at this stage.
		s.api.CheckCallNames(c)
	}
}

func (s *RemoveSuite) TestRunValidSpaceSucceeds(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "myspace")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `removed space "myspace"\n`)
	s.api.CheckCalls(c, []testing.StubCall{{
		FuncName: "RemoveSpace",
		Args:     []interface{}{"myspace"},
	}, {
		FuncName: "Close",
		Args:     nil,
	}})
}

func (s *RemoveSuite) TestRunWithNonExistentSpaceFails(c *gc.C) {
	s.api.SetErrors(errors.NotFoundf("space %q", "foo"))

	stdout, stderr, err := s.RunSubCommand(c, "foo")
	c.Assert(err, gc.ErrorMatches, `cannot remove space "foo": space "foo" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	s.api.CheckCallNames(c, "RemoveSpace", "Close")
}

func (s *RemoveSuite) TestRunAPIConnectFails(c *gc.C) {
	// TODO(dimitern): Change this once API is implemented.
	s.command = space.NewCreateCommand(nil)
	stdout, stderr, err := s.RunSubCommand(c, "myspace")
	c.Assert(err, gc.ErrorMatches, "cannot connect to API server: API not implemented yet!")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	// No API calls recoreded.
	s.api.CheckCallNames(c)
}
