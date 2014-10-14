// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/cmd"
	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/testing"
)

type DisableUserSuite struct {
	testing.FakeJujuHomeSuite
	mock mockDisableUserAPI
}

var _ = gc.Suite(&DisableUserSuite{})

func (s *DisableUserSuite) disableUserCommand() cmd.Command {
	return envcmd.Wrap(&DisableUserCommand{})
}

func (s *DisableUserSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.PatchValue(&getDisableUserAPI, func(*DisableUserCommand) (disableUserAPI, error) {
		return &s.mock, nil
	})
}

func (s *DisableUserSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args     []string
		errMatch string
		user     string
		enable   bool
	}{
		{
			errMatch: "no username supplied",
		}, {
			args:     []string{"not a user"},
			errMatch: `"not a user" is not a valid username`,
		}, {
			args:     []string{"username", "password"},
			errMatch: `unrecognized args: \["password"\]`,
		}, {
			args: []string{"username"},
			user: "username",
		}, {
			args:   []string{"--enable", "username"},
			user:   "username",
			enable: true,
		},
	} {
		c.Logf("test %d, args %v", i, test.args)
		disableCmd := &DisableUserCommand{}
		err := testing.InitCommand(disableCmd, test.args)
		if test.errMatch == "" {
			c.Assert(err, gc.IsNil)
			c.Assert(disableCmd.user, gc.Equals, test.user)
			c.Assert(disableCmd.enable, gc.Equals, test.enable)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *DisableUserSuite) TestDisable(c *gc.C) {
	tag := names.NewUserTag("testing")
	_, err := testing.RunCommand(c, s.disableUserCommand(), tag.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(s.mock.deactivate, gc.Equals, tag)
}

func (s *DisableUserSuite) TestEnable(c *gc.C) {
	tag := names.NewUserTag("testing")
	_, err := testing.RunCommand(c, s.disableUserCommand(), "--enable", tag.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(s.mock.activate, gc.Equals, tag)
}

type mockDisableUserAPI struct {
	activate   names.UserTag
	deactivate names.UserTag
}

func (m *mockDisableUserAPI) Close() error {
	return nil
}

func (m *mockDisableUserAPI) ActivateUser(tag names.UserTag) error {
	m.activate = tag
	return nil
}

func (m *mockDisableUserAPI) DeactivateUser(tag names.UserTag) error {
	m.deactivate = tag
	return nil
}
