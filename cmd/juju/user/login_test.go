// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"errors"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

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
username: password: 
You are now logged in to "testing" as "current-user@local".
`[1:],
	)
	s.assertStorePassword(c, "current-user@local", "", "superuser")
	s.assertStoreMacaroon(c, "current-user@local", fakeLocalLoginMacaroon(names.NewUserTag("current-user@local")))
	c.Assert(args.AccountDetails, jc.DeepEquals, &jujuclient.AccountDetails{
		User:     "current-user@local",
		Password: "sekrit",
	})
}

func (s *LoginCommandSuite) TestLoginNewUser(c *gc.C) {
	err := s.store.RemoveAccount("testing")
	c.Assert(err, jc.ErrorIsNil)
	context, args, err := s.run(c, "", "new-user")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(context), gc.Equals, "")
	c.Assert(coretesting.Stderr(context), gc.Equals, `
password: 
You are now logged in to "testing" as "new-user@local".
`[1:],
	)
	s.assertStorePassword(c, "new-user@local", "", "superuser")
	s.assertStoreMacaroon(c, "new-user@local", fakeLocalLoginMacaroon(names.NewUserTag("new-user@local")))
	c.Assert(args.AccountDetails, jc.DeepEquals, &jujuclient.AccountDetails{
		User:     "new-user@local",
		Password: "sekrit",
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

func (s *LoginCommandSuite) TestLoginFail(c *gc.C) {
	s.mockAPI.SetErrors(errors.New("failed to do something"))
	_, _, err := s.run(c, "", "current-user")
	c.Assert(err, gc.ErrorMatches, "failed to create a temporary credential: failed to do something")
	s.assertStorePassword(c, "current-user@local", "old-password", "")
	s.assertStoreMacaroon(c, "current-user@local", nil)
}

type mockLoginAPI struct {
	mockChangePasswordAPI
}

func (*mockLoginAPI) ControllerAccess() string {
	return "superuser"
}
