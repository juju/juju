// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"github.com/juju/cmd"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/testing"
)

type DisableUserSuite struct {
	BaseSuite
	mock mockDisableUserAPI
}

var _ = gc.Suite(&DisableUserSuite{})

type disenableCommand interface {
	cmd.Command
	username() string
}

func (s *DisableUserSuite) disableUserCommand() cmd.Command {
	return envcmd.Wrap(&user.DisableCommand{})
}

func (s *DisableUserSuite) enableUserCommand() cmd.Command {
	return envcmd.Wrap(&user.EnableCommand{})
}

func (s *DisableUserSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(user.GetDisableUserAPI, func(*user.DisableUserBase) (user.DisenableUserAPI, error) {
		return &s.mock, nil
	})
}

func (s *DisableUserSuite) testInit(c *gc.C, command user.DisenableCommand) {
	for i, test := range []struct {
		args     []string
		errMatch string
		user     string
		enable   bool
	}{
		{
			errMatch: "no username supplied",
		}, {
			args:     []string{"username", "password"},
			errMatch: `unrecognized args: \["password"\]`,
		}, {
			args: []string{"username"},
			user: "username",
		},
	} {
		c.Logf("test %d, args %v", i, test.args)
		err := testing.InitCommand(command, test.args)
		if test.errMatch == "" {
			c.Assert(err, gc.IsNil)
			c.Assert(command.Username(), gc.Equals, test.user)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *DisableUserSuite) TestInit(c *gc.C) {
	s.testInit(c, &user.EnableCommand{})
	s.testInit(c, &user.DisableCommand{})
}

func (s *DisableUserSuite) TestDisable(c *gc.C) {
	username := "testing"
	_, err := testing.RunCommand(c, s.disableUserCommand(), username)
	c.Assert(err, gc.IsNil)
	c.Assert(s.mock.disable, gc.Equals, username)
}

func (s *DisableUserSuite) TestEnable(c *gc.C) {
	username := "testing"
	_, err := testing.RunCommand(c, s.enableUserCommand(), username)
	c.Assert(err, gc.IsNil)
	c.Assert(s.mock.enable, gc.Equals, username)
}

type mockDisableUserAPI struct {
	enable  string
	disable string
}

var _ user.DisenableUserAPI = (*mockDisableUserAPI)(nil)

func (m *mockDisableUserAPI) Close() error {
	return nil
}

func (m *mockDisableUserAPI) EnableUser(username string) error {
	m.enable = username
	return nil
}

func (m *mockDisableUserAPI) DisableUser(username string) error {
	m.disable = username
	return nil
}
