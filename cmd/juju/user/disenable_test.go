// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/testing"
)

type DisableUserSuite struct {
	BaseSuite
	mock *mockDisenableUserAPI
}

var _ = gc.Suite(&DisableUserSuite{})

func (s *DisableUserSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mock = &mockDisenableUserAPI{}
}

type disenableCommand interface {
	cmd.Command
	Username() string
}

func (s *DisableUserSuite) testInit(c *gc.C, command disenableCommand) {
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
			c.Assert(err, jc.ErrorIsNil)
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
	disableCommand := envcmd.WrapSystem(user.NewDisableCommand(s.mock))
	_, err := testing.RunCommand(c, disableCommand, username)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mock.disable, gc.Equals, username)
}

func (s *DisableUserSuite) TestEnable(c *gc.C) {
	username := "testing"
	enableCommand := envcmd.WrapSystem(user.NewEnableCommand(s.mock))
	_, err := testing.RunCommand(c, enableCommand, username)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mock.enable, gc.Equals, username)
}

type mockDisenableUserAPI struct {
	enable  string
	disable string
}

var _ user.DisenableUserAPI = (*mockDisenableUserAPI)(nil)

func (m *mockDisenableUserAPI) Close() error {
	return nil
}

func (m *mockDisenableUserAPI) EnableUser(username string) error {
	m.enable = username
	return nil
}

func (m *mockDisenableUserAPI) DisableUser(username string) error {
	m.disable = username
	return nil
}
