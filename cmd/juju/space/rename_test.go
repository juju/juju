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

type RenameSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&RenameSuite{})

func (s *RenameSuite) SetUpTest(c *gc.C) {
	s.BaseSpaceSuite.SetUpTest(c)
	s.command = space.NewRenameCommand(s.api)
	c.Assert(s.command, gc.NotNil)
}

func (s *RenameSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		about         string
		args          []string
		expectName    string
		expectNewName string
		expectErr     string
	}{{
		about:     "no arguments",
		expectErr: "old-name is required",
	}, {
		about:     "No new name",
		args:      s.Strings("a-space"),
		expectErr: "new-name is required",
	}, {
		about:     "invalid space name - with invalid characters",
		args:      s.Strings("%inv$alid", "new-name"),
		expectErr: `"%inv\$alid" is not a valid space name`,
	}, {
		about:     "invalid space name - using underscores",
		args:      s.Strings("42_space", "new-name"),
		expectErr: `"42_space" is not a valid space name`,
	}, {
		about:     "valid space name with invalid new name",
		args:      s.Strings("a-space", "inv#alid"),
		expectErr: `"inv#alid" is not a valid space name`,
	}, {
		about:     "valid space name with CIDR as new name",
		args:      s.Strings("a-space", "1.2.3.4/24"),
		expectErr: `"1.2.3.4/24" is not a valid space name`,
	}, {
		about:         "more than two arguments",
		args:          s.Strings("a-space", "another-space", "rubbish"),
		expectErr:     `unrecognized args: \["rubbish"\]`,
		expectName:    "a-space",
		expectNewName: "another-space",
	}, {
		about:         "all ok",
		args:          s.Strings("a-space", "another-space"),
		expectName:    "a-space",
		expectNewName: "another-space",
	}} {
		c.Logf("test #%d: %s", i, test.about)
		// Create a new instance of the subcommand for each test, but
		// since we're not running the command no need to use
		// envcmd.Wrap().
		command := space.NewRenameCommand(s.api) // surely can use s.command??
		err := coretesting.InitCommand(command, test.args)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(command.Name, gc.Equals, test.expectName)
		c.Check(command.NewName, gc.Equals, test.expectNewName)
		// No API calls should be recorded at this stage.
		s.api.CheckCallNames(c)
	}
}

func (s *RenameSuite) TestRunWithSubnetsSucceeds(c *gc.C) {
	stdout, stderr, err := s.RunSubCommand(c, "a-space", "another-space")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `renamed space "a-space" to "another-space"\n`)
	s.api.CheckCalls(c, []testing.StubCall{{
		FuncName: "RenameSpace",
		Args:     []interface{}{"a-space", "another-space"},
	}, {
		FuncName: "Close",
		Args:     nil,
	}})
}

func (s *RenameSuite) TestRunWhenRenameFails(c *gc.C) {
	s.api.SetErrors(errors.New("boom"))

	stdout, stderr, err := s.RunSubCommand(c, "foo", "bar")
	c.Assert(err, gc.ErrorMatches, `cannot rename space "foo": boom`)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	s.api.CheckCallNames(c, "RenameSpace", "Close")
}

func (s *RenameSuite) TestRunAPIConnectFails(c *gc.C) {
	// TODO(dimitern): Change this once API is implemented.
	s.command = space.NewRenameCommand(nil)
	stdout, stderr, err := s.RunSubCommand(c, "myspace", "mynewspace")
	c.Assert(err, gc.ErrorMatches, "cannot connect to API server: API not implemented yet!")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	// No API calls recoreded.
	s.api.CheckCallNames(c)
}
