// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
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

func (s *ChangePasswordCommandSuite) run(c *gc.C, stdin string, args ...string) (*cmd.Context, *juju.NewAPIConnectionParams, error) {
	var argsOut juju.NewAPIConnectionParams
	newAPIConnection := func(args juju.NewAPIConnectionParams) (api.Connection, error) {
		argsOut = args
		return mockAPIConnection{}, nil
	}
	changePasswordCommand, _ := user.NewChangePasswordCommandForTest(
		newAPIConnection, s.mockAPI, s.store,
	)
	ctx := cmdtesting.Context(c)
	ctx.Stdin = strings.NewReader(stdin)
	err := cmdtesting.InitCommand(changePasswordCommand, args)
	if err != nil {
		return ctx, nil, err
	}
	return ctx, &argsOut, changePasswordCommand.Run(ctx)
}

func (s *ChangePasswordCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		user        string
		reset       bool
		errorString string
	}{
		{
			// no args is fine
		}, {
			args: []string{"foobar"},
			user: "foobar",
		}, {
			args:        []string{"--foobar"},
			errorString: "option provided but not defined: --foobar",
		}, {
			args:  []string{"--reset"},
			reset: true,
		}, {
			args:        []string{"foobar", "extra"},
			errorString: `unrecognized args: \["extra"\]`,
		},
	} {
		c.Logf("test %d", i)
		wrappedCommand, command := user.NewChangePasswordCommandForTest(nil, nil, s.store)
		err := cmdtesting.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(command.User, gc.Equals, test.user)
			c.Check(command.Reset, gc.Equals, test.reset)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *ChangePasswordCommandSuite) assertAPICalls(c *gc.C, user, pass string) {
	s.mockAPI.CheckCall(c, 0, "SetPassword", user, pass)
}

func (s *ChangePasswordCommandSuite) TestChangePassword(c *gc.C) {
	context, args, err := s.run(c, "sekrit\nsekrit\n")
	c.Assert(err, jc.ErrorIsNil)
	s.assertAPICalls(c, "current-user", "sekrit")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, `
new password: 
type new password again: 
Your password has been changed.
`[1:])
	// The command should have logged in without a password to get a macaroon.
	c.Assert(args.AccountDetails, jc.DeepEquals, &jujuclient.AccountDetails{
		User: "current-user",
	})
}

func (s *ChangePasswordCommandSuite) TestChangePasswordNoPrompt(c *gc.C) {
	context, args, err := s.run(c, "sneaky-password\n", "--no-prompt")
	c.Assert(err, jc.ErrorIsNil)
	s.assertAPICalls(c, "current-user", "sneaky-password")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, `
reading password from stdin...
Your password has been changed.
`[1:])
	// The command should have logged in without a password to get a macaroon.
	c.Assert(args.AccountDetails, jc.DeepEquals, &jujuclient.AccountDetails{
		User: "current-user",
	})
}

func (s *ChangePasswordCommandSuite) TestChangePasswordFail(c *gc.C) {
	s.mockAPI.SetErrors(errors.New("failed to do something"))
	_, _, err := s.run(c, "sekrit\nsekrit\n")
	c.Assert(err, gc.ErrorMatches, "failed to do something")
	s.assertAPICalls(c, "current-user", "sekrit")
}

func (s *ChangePasswordCommandSuite) TestChangeOthersPassword(c *gc.C) {
	// The checks for user existence and admin rights are tested
	// at the apiserver level.
	_, _, err := s.run(c, "sekrit\nsekrit\n", "other")
	c.Assert(err, jc.ErrorIsNil)
	s.assertAPICalls(c, "other", "sekrit")
}

func (s *ChangePasswordCommandSuite) TestResetSelfPasswordFail(c *gc.C) {
	context, _, err := s.run(c, "", "--reset")
	s.assertResetSelfPasswordFail(c, context, err)
}

func (s *ChangePasswordCommandSuite) TestResetSelfPasswordSpecifyYourselfFail(c *gc.C) {
	context, _, err := s.run(c, "", "--reset", "current-user")
	s.assertResetSelfPasswordFail(c, context, err)
}

func (s *ChangePasswordCommandSuite) TestResetPasswordFail(c *gc.C) {
	s.mockAPI.SetErrors(errors.New("failed to do something"))
	context, _, err := s.run(c, "", "--reset", "other")
	c.Assert(err, gc.ErrorMatches, "failed to do something")
	s.mockAPI.CheckCalls(c, []testing.StubCall{
		{"ResetPassword", []interface{}{"other"}},
	})
	// TODO (anastasiamac 2017-08-17)
	// should probably warn user that something did not go well enough
	c.Assert(cmdtesting.Stdout(context), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
}

func (s *ChangePasswordCommandSuite) TestResetOthersPassword(c *gc.C) {
	// The checks for user existence and admin rights are tested
	// at the apiserver level.
	s.mockAPI.key = []byte("no cats or dragons")
	context, _, err := s.run(c, "", "other", "--reset")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCalls(c, []testing.StubCall{
		{"ResetPassword", []interface{}{"other"}},
	})
	c.Assert(cmdtesting.Stdout(context), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), gc.Matches, `
Password for "other" has been reset.
Ask the user to run:
     juju register (.+)
`[1:])
}

func (s *ChangePasswordCommandSuite) assertResetSelfPasswordFail(c *gc.C, context *cmd.Context, err error) {
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCalls(c, nil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), gc.Matches, `
You cannot reset your own password.
If you want to change it, please call `[1:]+"`juju change-user-password`"+` without --reset option.
`)
}

type mockChangePasswordAPI struct {
	testing.Stub
	key []byte
}

func (m *mockChangePasswordAPI) SetPassword(username, password string) error {
	m.MethodCall(m, "SetPassword", username, password)
	return m.NextErr()
}

func (m *mockChangePasswordAPI) ResetPassword(username string) ([]byte, error) {
	m.MethodCall(m, "ResetPassword", username)
	return m.key, m.NextErr()
}

func (m *mockChangePasswordAPI) Close() error {
	m.MethodCall(m, "Close")
	return nil
}

type mockAPIConnection struct {
	api.Connection
}

func (mockAPIConnection) Close() error {
	return nil
}
