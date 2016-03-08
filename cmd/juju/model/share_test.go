// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/cmd"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/testing"
)

type shareSuite struct {
	fakeEnvSuite
}

var _ = gc.Suite(&shareSuite{})

func (s *shareSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command, _ := model.NewShareCommandForTest(s.fake)
	return testing.RunCommand(c, command, args...)
}

func (s *shareSuite) TestInit(c *gc.C) {
	wrappedCmd, shareCmd := model.NewShareCommandForTest(s.fake)
	err := testing.InitCommand(wrappedCmd, []string{})
	c.Assert(err, gc.ErrorMatches, "no users specified")

	err = testing.InitCommand(wrappedCmd, []string{"bob@local", "sam"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(shareCmd.Users[0], gc.Equals, names.NewUserTag("bob@local"))
	c.Assert(shareCmd.Users[1], gc.Equals, names.NewUserTag("sam"))

	err = testing.InitCommand(wrappedCmd, []string{"not valid/0"})
	c.Assert(err, gc.ErrorMatches, `invalid username: "not valid/0"`)
}

func (s *shareSuite) TestPassesValues(c *gc.C) {
	sam := names.NewUserTag("sam")
	ralph := names.NewUserTag("ralph")

	_, err := s.run(c, "sam", "ralph")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.addUsers, jc.DeepEquals, []names.UserTag{sam, ralph})
}

func (s *shareSuite) TestBlockShare(c *gc.C) {
	s.fake.err = &params.Error{Code: params.CodeOperationBlocked}
	_, err := s.run(c, "sam")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Check(c.GetTestLog(), jc.Contains, "To unblock changes")
}
