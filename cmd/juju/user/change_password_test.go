// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

type ChangePasswordCommandSuite struct {
	BaseSuite
	mockAPI *mockChangePasswordAPI
	store   jujuclient.ClientStore
}

var _ = gc.Suite(&ChangePasswordCommandSuite{})

func (s *ChangePasswordCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockChangePasswordAPI{}
	s.store = s.BaseSuite.store
}

func (s *ChangePasswordCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, *juju.NewAPIConnectionParams, error) {
	var argsOut juju.NewAPIConnectionParams
	newAPIConnection := func(args juju.NewAPIConnectionParams) (api.Connection, error) {
		argsOut = args
		return mockAPIConnection{}, nil
	}
	changePasswordCommand, _ := user.NewChangePasswordCommandForTest(
		newAPIConnection, s.mockAPI, s.store,
	)
	ctx := coretesting.Context(c)
	ctx.Stdin = strings.NewReader("sekrit\nsekrit\n")
	err := coretesting.InitCommand(changePasswordCommand, args)
	if err != nil {
		return ctx, nil, err
	}
	return ctx, &argsOut, changePasswordCommand.Run(ctx)
}

func (s *ChangePasswordCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		user        string
		errorString string
	}{
		{
		// no args is fine
		}, {
			args: []string{"foobar"},
			user: "foobar",
		}, {
			args:        []string{"--foobar"},
			errorString: "flag provided but not defined: --foobar",
		}, {
			args:        []string{"foobar", "extra"},
			errorString: `unrecognized args: \["extra"\]`,
		},
	} {
		c.Logf("test %d", i)
		wrappedCommand, command := user.NewChangePasswordCommandForTest(nil, nil, s.store)
		err := coretesting.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(command.User, gc.Equals, test.user)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *ChangePasswordCommandSuite) assertAPICalls(c *gc.C, user, pass string) {
	s.mockAPI.CheckCall(c, 0, "SetPassword", user, pass)
}

func (s *ChangePasswordCommandSuite) TestChangePassword(c *gc.C) {
	context, args, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAPICalls(c, "current-user@local", "sekrit")
	c.Assert(coretesting.Stdout(context), gc.Equals, "")
	c.Assert(coretesting.Stderr(context), gc.Equals, `
new password: 
type new password again: 
Your password has been updated.
`[1:])
	// The command should have logged in without a password to get a macaroon.
	c.Assert(args.AccountDetails, jc.DeepEquals, &jujuclient.AccountDetails{
		User: "current-user@local",
	})
}

func (s *ChangePasswordCommandSuite) TestChangePasswordFail(c *gc.C) {
	s.mockAPI.SetErrors(errors.New("failed to do something"))
	_, _, err := s.run(c)
	c.Assert(err, gc.ErrorMatches, "failed to do something")
	s.assertAPICalls(c, "current-user@local", "sekrit")
}

func (s *ChangePasswordCommandSuite) TestChangeOthersPassword(c *gc.C) {
	// The checks for user existence and admin rights are tested
	// at the apiserver level.
	_, _, err := s.run(c, "other")
	c.Assert(err, jc.ErrorIsNil)
	s.assertAPICalls(c, "other@local", "sekrit")
}

type mockChangePasswordAPI struct {
	testing.Stub
}

func (m *mockChangePasswordAPI) SetPassword(username, password string) error {
	m.MethodCall(m, "SetPassword", username, password)
	return m.NextErr()
}

func (*mockChangePasswordAPI) Close() error {
	return nil
}

type mockAPIConnection struct {
	api.Connection
}

func (mockAPIConnection) Close() error {
	return nil
}
