// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	"github.com/juju/cmd"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/testing"
)

type unshareSuite struct {
	fakeEnvSuite
}

var _ = gc.Suite(&unshareSuite{})

func (s *unshareSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command, _ := environment.NewUnshareCommand(s.fake)
	return testing.RunCommand(c, command, args...)
}

func (s *unshareSuite) TestInit(c *gc.C) {
	wrappedCommand, unshareCmd := environment.NewUnshareCommand(s.fake)
	err := testing.InitCommand(wrappedCommand, []string{})
	c.Assert(err, gc.ErrorMatches, "no users specified")

	err = testing.InitCommand(wrappedCommand, []string{"not valid/0"})
	c.Assert(err, gc.ErrorMatches, `invalid username: "not valid/0"`)

	err = testing.InitCommand(wrappedCommand, []string{"bob@local", "sam"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(unshareCmd.Users[0], gc.Equals, names.NewUserTag("bob@local"))
	c.Assert(unshareCmd.Users[1], gc.Equals, names.NewUserTag("sam"))
}

func (s *unshareSuite) TestPassesValues(c *gc.C) {
	sam := names.NewUserTag("sam")
	ralph := names.NewUserTag("ralph")

	_, err := s.run(c, "sam", "ralph")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.removeUsers, jc.DeepEquals, []names.UserTag{sam, ralph})
}

func (s *unshareSuite) TestBlockUnShare(c *gc.C) {
	s.fake.err = &params.Error{Code: params.CodeOperationBlocked}
	_, err := s.run(c, "sam")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Check(c.GetTestLog(), jc.Contains, "To unblock changes")
}
