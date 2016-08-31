// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

type LoginCommandSuite struct {
	BaseSuite
	mockAPI *mockLoginAPI
}

var _ = gc.Suite(&LoginCommandSuite{})

func (s *LoginCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockLoginAPI{}
}

func (s *LoginCommandSuite) run(c *gc.C, stdin string, args ...string) (*cmd.Context, juju.NewAPIConnectionParams, error) {
	var argsOut juju.NewAPIConnectionParams
	cmd, _ := user.NewLoginCommandForTest(func(args juju.NewAPIConnectionParams) (user.LoginAPI, user.ConnectionAPI, error) {
		argsOut = args
		// The account details are modified in place, so take a copy.
		accountDetails := *argsOut.AccountDetails
		argsOut.AccountDetails = &accountDetails
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

type mockLoginAPI struct{}

func (*mockLoginAPI) Close() error {
	return nil
}

func (*mockLoginAPI) ControllerAccess() string {
	return "superuser"
}
