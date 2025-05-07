// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

type DisableUserSuite struct {
	BaseSuite
	mock *mockDisenableUserAPI
}

var _ = tc.Suite(&DisableUserSuite{})

func (s *DisableUserSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mock = &mockDisenableUserAPI{}
}

func (s *DisableUserSuite) testInit(c *tc.C, wrappedCommand cmd.Command, command *user.DisenableUserBase) {
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
		err := cmdtesting.InitCommand(wrappedCommand, test.args)
		if test.errMatch == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(command.User, tc.Equals, test.user)
		} else {
			c.Assert(err, tc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *DisableUserSuite) TestInit(c *tc.C) {
	wrappedCommand, command := user.NewEnableCommandForTest(nil, s.store)
	s.testInit(c, wrappedCommand, command)
	wrappedCommand, command = user.NewDisableCommandForTest(nil, s.store)
	s.testInit(c, wrappedCommand, command)
}

func (s *DisableUserSuite) TestDisable(c *tc.C) {
	username := "testing"
	disableCommand, _ := user.NewDisableCommandForTest(s.mock, s.store)
	_, err := cmdtesting.RunCommand(c, disableCommand, username)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mock.disable, tc.Equals, username)
}

func (s *DisableUserSuite) TestEnable(c *tc.C) {
	username := "testing"
	enableCommand, _ := user.NewEnableCommandForTest(s.mock, s.store)
	_, err := cmdtesting.RunCommand(c, enableCommand, username)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mock.enable, tc.Equals, username)
}

type mockDisenableUserAPI struct {
	enable  string
	disable string
}

func (m *mockDisenableUserAPI) Close() error {
	return nil
}

func (m *mockDisenableUserAPI) EnableUser(ctx context.Context, username string) error {
	m.enable = username
	return nil
}

func (m *mockDisenableUserAPI) DisableUser(ctx context.Context, username string) error {
	m.disable = username
	return nil
}
