// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"context"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
)

type ChangePasswordCommandSuite struct {
	BaseSuite
	mockAPI *mockChangePasswordAPI
	store   jujuclient.ClientStore
}

var _ = tc.Suite(&ChangePasswordCommandSuite{})

func (s *ChangePasswordCommandSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockChangePasswordAPI{}
	s.store = s.BaseSuite.store
}

func (s *ChangePasswordCommandSuite) run(c *tc.C, stdin string, args ...string) (*cmd.Context, *juju.NewAPIConnectionParams, error) {
	var argsOut juju.NewAPIConnectionParams
	newAPIConnection := func(ctx context.Context, args juju.NewAPIConnectionParams) (api.Connection, error) {
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

func (s *ChangePasswordCommandSuite) TestInit(c *tc.C) {
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
			c.Check(command.User, tc.Equals, test.user)
			c.Check(command.Reset, tc.Equals, test.reset)
		} else {
			c.Check(err, tc.ErrorMatches, test.errorString)
		}
	}
}

func (s *ChangePasswordCommandSuite) assertAPICalls(c *tc.C, user, pass string) {
	s.mockAPI.CheckCall(c, 0, "SetPassword", user, pass)
}

func (s *ChangePasswordCommandSuite) TestChangePassword(c *tc.C) {
	context, args, err := s.run(c, "sekrit\nsekrit\n")
	c.Assert(err, tc.ErrorIsNil)
	s.assertAPICalls(c, "current-user", "sekrit")
	c.Assert(cmdtesting.Stdout(context), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, `
new password: 
type new password again: 
Your password has been changed.
`[1:])
	// The command should have logged in without a password to get a macaroon.
	c.Assert(args.AccountDetails, tc.DeepEquals, &jujuclient.AccountDetails{
		User: "current-user",
	})
}

func (s *ChangePasswordCommandSuite) TestChangePasswordNoPrompt(c *tc.C) {
	context, args, err := s.run(c, "sneaky-password\n", "--no-prompt")
	c.Assert(err, tc.ErrorIsNil)
	s.assertAPICalls(c, "current-user", "sneaky-password")
	c.Assert(cmdtesting.Stdout(context), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, `
reading password from stdin...
Your password has been changed.
`[1:])
	// The command should have logged in without a password to get a macaroon.
	c.Assert(args.AccountDetails, tc.DeepEquals, &jujuclient.AccountDetails{
		User: "current-user",
	})
}

func (s *ChangePasswordCommandSuite) TestChangePasswordFail(c *tc.C) {
	s.mockAPI.SetErrors(errors.New("failed to do something"))
	_, _, err := s.run(c, "sekrit\nsekrit\n")
	c.Assert(err, tc.ErrorMatches, "failed to do something")
	s.assertAPICalls(c, "current-user", "sekrit")
}

func (s *ChangePasswordCommandSuite) TestChangeOthersPassword(c *tc.C) {
	// The checks for user existence and admin rights are tested
	// at the apiserver level.
	_, _, err := s.run(c, "sekrit\nsekrit\n", "other")
	c.Assert(err, tc.ErrorIsNil)
	s.assertAPICalls(c, "other", "sekrit")
}

func (s *ChangePasswordCommandSuite) TestResetSelfPasswordFail(c *tc.C) {
	context, _, err := s.run(c, "", "--reset")
	s.assertResetSelfPasswordFail(c, context, err)
}

func (s *ChangePasswordCommandSuite) TestResetSelfPasswordSpecifyYourselfFail(c *tc.C) {
	context, _, err := s.run(c, "", "--reset", "current-user")
	s.assertResetSelfPasswordFail(c, context, err)
}

func (s *ChangePasswordCommandSuite) TestResetPasswordFail(c *tc.C) {
	s.mockAPI.SetErrors(errors.New("failed to do something"))
	context, _, err := s.run(c, "", "--reset", "other")
	c.Assert(err, tc.ErrorMatches, "failed to do something")
	s.mockAPI.CheckCalls(c, []testing.StubCall{
		{"ResetPassword", []interface{}{"other"}},
	})
	// TODO (anastasiamac 2017-08-17)
	// should probably warn user that something did not go well enough
	c.Assert(cmdtesting.Stdout(context), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
}

func (s *ChangePasswordCommandSuite) TestResetOthersPassword(c *tc.C) {
	// The checks for user existence and admin rights are tested
	// at the apiserver level.
	s.mockAPI.key = []byte("no cats or dragons")
	context, _, err := s.run(c, "", "other", "--reset")
	c.Assert(err, tc.ErrorIsNil)
	s.mockAPI.CheckCalls(c, []testing.StubCall{
		{"ResetPassword", []interface{}{"other"}},
	})
	c.Assert(cmdtesting.Stdout(context), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), tc.Matches, `
Password for "other" has been reset.
Ask the user to run:
     juju register (.+)
`[1:])
}

func (s *ChangePasswordCommandSuite) assertResetSelfPasswordFail(c *tc.C, context *cmd.Context, err error) {
	c.Assert(err, tc.ErrorIsNil)
	s.mockAPI.CheckCalls(c, nil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), tc.Matches, `
You cannot reset your own password.
If you want to change it, please call `[1:]+"`juju change-user-password`"+` without --reset option.
`)
}

type mockChangePasswordAPI struct {
	testing.Stub
	key []byte
}

func (m *mockChangePasswordAPI) SetPassword(ctx context.Context, username, password string) error {
	m.MethodCall(m, "SetPassword", username, password)
	return m.NextErr()
}

func (m *mockChangePasswordAPI) ResetPassword(ctx context.Context, username string) ([]byte, error) {
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
