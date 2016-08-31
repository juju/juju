// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

type LoginCommandSuite struct {
	BaseSuite
	mockAPI  *mockLoginAPI
	loginErr error
}

var _ = gc.Suite(&LoginCommandSuite{})

func (s *LoginCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockLoginAPI{}
	s.loginErr = nil
}

func (s *LoginCommandSuite) run(c *gc.C, stdin string, args ...string) (*cmd.Context, juju.NewAPIConnectionParams, error) {
	var argsOut juju.NewAPIConnectionParams
	cmd, _ := user.NewLoginCommandForTest(func(args juju.NewAPIConnectionParams) (user.LoginAPI, user.ConnectionAPI, error) {
		argsOut = args
		// The account details are modified in place, so take a copy.
		accountDetails := *argsOut.AccountDetails
		argsOut.AccountDetails = &accountDetails
		if s.loginErr != nil {
			err := s.loginErr
			s.loginErr = nil
			return nil, nil, err
		}
		return s.mockAPI, s.mockAPI, nil
	}, s.store)
	ctx := coretesting.Context(c)
	if stdin == "" {
		stdin = "sekrit\n"
	}
	ctx.Stdin = strings.NewReader(stdin)
	err := coretesting.InitCommand(cmd, args)
	if err != nil {
		return nil, argsOut, err
	}
	err = cmd.Run(ctx)
	return ctx, argsOut, err
}

func (s *LoginCommandSuite) TestInit(c *gc.C) {
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
		wrappedCommand, command := user.NewLoginCommandForTest(nil, s.store)
		err := coretesting.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(command.User, gc.Equals, test.user)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *LoginCommandSuite) TestLogin(c *gc.C) {
	context, args, err := s.run(c, "current-user\nsekrit\n")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(context), gc.Equals, "")
	c.Assert(coretesting.Stderr(context), gc.Equals, `
username: You are now logged in to "testing" as "current-user@local".
`[1:],
	)
	s.assertStorePassword(c, "current-user@local", "", "superuser")
	c.Assert(args.AccountDetails, jc.DeepEquals, &jujuclient.AccountDetails{
		User: "current-user@local",
	})
}

func (s *LoginCommandSuite) TestLoginNewUser(c *gc.C) {
	err := s.store.RemoveAccount("testing")
	c.Assert(err, jc.ErrorIsNil)
	context, args, err := s.run(c, "", "new-user")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(context), gc.Equals, "")
	c.Assert(coretesting.Stderr(context), gc.Equals, `
You are now logged in to "testing" as "new-user@local".
`[1:],
	)
	s.assertStorePassword(c, "new-user@local", "", "superuser")
	c.Assert(args.AccountDetails, jc.DeepEquals, &jujuclient.AccountDetails{
		User: "new-user@local",
	})
}

func (s *LoginCommandSuite) TestLoginAlreadyLoggedInSameUser(c *gc.C) {
	_, _, err := s.run(c, "", "current-user")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LoginCommandSuite) TestLoginAlreadyLoggedInDifferentUser(c *gc.C) {
	_, _, err := s.run(c, "", "other-user")
	c.Assert(err, gc.ErrorMatches, `already logged in

Run "juju logout" first before attempting to log in as a different user.
`)
}

func (s *LoginCommandSuite) TestLoginWithMacaroons(c *gc.C) {
	err := s.store.RemoveAccount("testing")
	c.Assert(err, jc.ErrorIsNil)
	context, args, err := s.run(c, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(context), gc.Equals, "")
	c.Assert(coretesting.Stderr(context), gc.Equals, `
You are now logged in to "testing" as "user@external".
`[1:],
	)
	c.Assert(args.AccountDetails, jc.DeepEquals, &jujuclient.AccountDetails{})
}

func (s *LoginCommandSuite) TestLoginWithMacaroonsNotSupported(c *gc.C) {
	err := s.store.RemoveAccount("testing")
	c.Assert(err, jc.ErrorIsNil)
	s.loginErr = &params.Error{Code: params.CodeNoCreds, Message: "barf"}
	context, _, err := s.run(c, "new-user\nsekrit\n")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(context), gc.Equals, "")
	c.Assert(coretesting.Stderr(context), gc.Equals, `
username: You are now logged in to "testing" as "new-user@local".
`[1:],
	)
}

type mockLoginAPI struct{}

func (*mockLoginAPI) Close() error {
	return nil
}

func (*mockLoginAPI) AuthTag() names.Tag {
	return names.NewUserTag("user@external")
}

func (*mockLoginAPI) ControllerAccess() string {
	return "superuser"
}
