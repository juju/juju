// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
)

type SwitchUserCommandSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&SwitchUserCommandSuite{})

func (s *SwitchUserCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.store = jujuclienttesting.NewMemStore()
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = &jujuclient.ControllerAccounts{
		Accounts: map[string]jujuclient.AccountDetails{
			"current-user@local": {
				User:     "current-user@local",
				Password: "old-password",
			},
			"other@local": {
				User:     "other@local",
				Password: "old-password",
			},
		},
		CurrentAccount: "current-user@local",
	}
	err := modelcmd.WriteCurrentController("testing")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SwitchUserCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd, _ := user.NewSwitchUserCommandForTest(s.store)
	return coretesting.RunCommand(c, cmd, args...)
}

func (s *SwitchUserCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		user        string
		generate    bool
		errorString string
	}{
		{
			errorString: "you must specify a user to switch to",
		}, {
			args: []string{"bob"},
			user: "bob",
		}, {
			args: []string{"bob@local"},
			user: "bob@local",
		}, {
			args:        []string{"--foobar"},
			errorString: "flag provided but not defined: --foobar",
		}, {
			args:        []string{"bob", "dobbs"},
			errorString: `unrecognized args: \["dobbs"\]`,
		},
	} {
		c.Logf("test %d", i)
		wrappedCommand, command := user.NewSwitchUserCommandForTest(s.store)
		err := coretesting.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(command.User, gc.Equals, test.user)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *SwitchUserCommandSuite) assertCurrentUser(c *gc.C, user string) {
	current, err := s.store.CurrentAccount("testing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(current, gc.Equals, user)
}

func (s *SwitchUserCommandSuite) TestSwitchUser(c *gc.C) {
	s.assertCurrentUser(c, "current-user@local")
	context, err := s.run(c, "other")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentUser(c, "other@local")
	c.Assert(coretesting.Stdout(context), gc.Equals, "")
	c.Assert(coretesting.Stderr(context), gc.Equals, "current-user@local -> other@local\n")
}

func (s *SwitchUserCommandSuite) TestSwitchUserNoChange(c *gc.C) {
	s.assertCurrentUser(c, "current-user@local")
	context, err := s.run(c, "current-user@local")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentUser(c, "current-user@local")
	c.Assert(coretesting.Stdout(context), gc.Equals, "")
	c.Assert(coretesting.Stderr(context), gc.Equals, "current-user@local (no change)\n")
}

func (s *SwitchUserCommandSuite) TestSwitchUserUnknown(c *gc.C) {
	s.assertCurrentUser(c, "current-user@local")
	context, err := s.run(c, "unknown@local")
	c.Assert(err, gc.ErrorMatches, "account testing:unknown@local not found")
	s.assertCurrentUser(c, "current-user@local")
	c.Assert(coretesting.Stdout(context), gc.Equals, "")
	c.Assert(coretesting.Stderr(context), gc.Equals, "")
}
