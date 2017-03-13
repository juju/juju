// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"bytes"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	apibase "github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
)

type LoginCommandSuite struct {
	BaseSuite
	mockAPI *loginMockAPI

	// apiConnectionParams is set when the mock newAPIConnection
	// implementation installed by SetUpTest is called.
	apiConnectionParams juju.NewAPIConnectionParams
}

var _ = gc.Suite(&LoginCommandSuite{})

func (s *LoginCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &loginMockAPI{
		authTag:          names.NewUserTag("user@external"),
		controllerAccess: "superuser",
	}
	s.apiConnectionParams = juju.NewAPIConnectionParams{}
	s.PatchValue(user.NewAPIConnection, func(p juju.NewAPIConnectionParams) (api.Connection, error) {
		// The account details are modified in place, so take a copy.
		accountDetails := *p.AccountDetails
		p.AccountDetails = &accountDetails
		s.apiConnectionParams = p
		return s.mockAPI, nil
	})
	s.PatchValue(user.ListModels, func(_ *modelcmd.ControllerCommandBase, userName string) ([]apibase.UserModel, error) {
		return nil, errors.New("unexpected call to ListModels")
	})
	s.PatchValue(user.APIOpen, func(c *modelcmd.JujuCommandBase, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return nil, errors.New("unexpected call to apiOpen")
	})
	s.PatchValue(user.LoginClientStore, s.store)
}

func (s *LoginCommandSuite) TestInitError(c *gc.C) {
	for i, test := range []struct {
		args   []string
		stderr string
	}{{
		args:   []string{"--foobar"},
		stderr: `error: flag provided but not defined: --foobar\n`,
	}, {
		args:   []string{"foobar", "extra"},
		stderr: `error: unrecognized args: \["extra"\]\n`,
	}} {
		c.Logf("test %d", i)
		stdout, stderr, code := runLogin(c, "", test.args...)
		c.Check(stdout, gc.Equals, "")
		c.Check(stderr, gc.Matches, test.stderr)
		c.Assert(code, gc.Equals, 2)
	}
}

func (s *LoginCommandSuite) TestLogin(c *gc.C) {
	stdout, stderr, code := runLogin(c, "current-user\nsekrit\n", "-u")
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Equals, `
username: You are now logged in to "testing" as "current-user".
`[1:])
	c.Assert(code, gc.Equals, 0)
	s.assertStorePassword(c, "current-user", "", "superuser")
	c.Assert(s.apiConnectionParams.AccountDetails, jc.DeepEquals, &jujuclient.AccountDetails{
		User: "current-user",
	})
}

func (s *LoginCommandSuite) TestLoginNewUser(c *gc.C) {
	err := s.store.RemoveAccount("testing")
	c.Assert(err, jc.ErrorIsNil)
	stdout, stderr, code := runLogin(c, "sekrit\n", "new-user", "-u")
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Equals, `
You are now logged in to "testing" as "new-user".
`[1:])
	c.Assert(code, gc.Equals, 0)
	s.assertStorePassword(c, "new-user", "", "superuser")
	c.Assert(s.apiConnectionParams.AccountDetails, jc.DeepEquals, &jujuclient.AccountDetails{
		User: "new-user",
	})
}

func (s *LoginCommandSuite) TestLoginAlreadyLoggedInSameUser(c *gc.C) {
	stdout, stderr, code := runLogin(c, "", "current-user", "-u")
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Equals, `You are now logged in to "testing" as "current-user".
`)
	c.Assert(code, gc.Equals, 0)
}

func (s *LoginCommandSuite) TestLoginAlreadyLoggedInDifferentUser(c *gc.C) {
	stdout, stderr, code := runLogin(c, "sekrit\n", "-u", "other-user")
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Equals, `
error: already logged in

Run "juju logout" first before attempting to log in as a different user.
`[1:])
	c.Assert(code, gc.Equals, 1)
}

func (s *LoginCommandSuite) TestLoginWithMacaroons(c *gc.C) {
	err := s.store.RemoveAccount("testing")
	c.Assert(err, jc.ErrorIsNil)
	stdout, stderr, code := runLogin(c, "", "-u")
	c.Check(stderr, gc.Equals, `
You are now logged in to "testing" as "user@external".
`[1:])
	c.Check(stdout, gc.Equals, ``)
	c.Assert(code, gc.Equals, 0)
	c.Assert(s.apiConnectionParams.AccountDetails, jc.DeepEquals, &jujuclient.AccountDetails{})
}

func (s *LoginCommandSuite) TestLoginWithMacaroonsNotSupported(c *gc.C) {
	err := s.store.RemoveAccount("testing")
	c.Assert(err, jc.ErrorIsNil)
	*user.NewAPIConnection = func(p juju.NewAPIConnectionParams) (api.Connection, error) {
		if !c.Check(p.AccountDetails, gc.NotNil) {
			return nil, errors.New("no account details")
		}
		if p.AccountDetails.User == "" && p.AccountDetails.Password == "" {
			return nil, &params.Error{Code: params.CodeNoCreds, Message: "barf"}
		}
		c.Check(p.AccountDetails.User, gc.Equals, "new-user")
		return s.mockAPI, nil
	}
	stdout, stderr, code := runLogin(c, "new-user\nsekrit\n", "-u")
	c.Check(stdout, gc.Equals, ``)
	c.Check(stderr, gc.Equals, `
username: You are now logged in to "testing" as "new-user".
`[1:])
	c.Assert(code, gc.Equals, 0)
}

func runLogin(c *gc.C, stdin string, args ...string) (stdout, stderr string, errCode int) {
	c.Logf("in LoginControllerSuite.run")
	var stdoutBuf, stderrBuf bytes.Buffer
	ctxt := &cmd.Context{
		Dir:    c.MkDir(),
		Stdin:  strings.NewReader(stdin),
		Stdout: &stdoutBuf,
		Stderr: &stderrBuf,
	}
	exitCode := cmd.Main(user.NewLoginCommand(), ctxt, args)
	return stdoutBuf.String(), stderrBuf.String(), exitCode
}
