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
	apiConnection *loginMockAPI

	// apiConnectionParams is set when the mock newAPIConnection
	// implementation installed by SetUpTest is called.
	apiConnectionParams juju.NewAPIConnectionParams
}

var _ = gc.Suite(&LoginCommandSuite{})

func (s *LoginCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.apiConnection = &loginMockAPI{
		controllerTag:    names.NewControllerTag(mockControllerUUID),
		authTag:          names.NewUserTag("user@external"),
		controllerAccess: "superuser",
	}
	s.apiConnectionParams = juju.NewAPIConnectionParams{}
	s.PatchValue(user.NewAPIConnection, func(p juju.NewAPIConnectionParams) (api.Connection, error) {
		// The account details are modified in place, so take a copy.
		accountDetails := *p.AccountDetails
		p.AccountDetails = &accountDetails
		s.apiConnectionParams = p
		return s.apiConnection, nil
	})
	s.PatchValue(user.ListModels, func(c api.Connection, userName string) ([]apibase.UserModel, error) {
		return nil, nil
	})
	s.PatchValue(user.APIOpen, func(c *modelcmd.CommandBase, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return s.apiConnection, nil
	})
	s.PatchValue(user.LoginClientStore, s.store)
}

func (s *LoginCommandSuite) TestInitError(c *gc.C) {
	for i, test := range []struct {
		args   []string
		stderr string
	}{{
		args:   []string{"--foobar"},
		stderr: `ERROR flag provided but not defined: --foobar\n`,
	}, {
		args:   []string{"foobar", "extra"},
		stderr: `ERROR unrecognized args: \["extra"\]\n`,
	}} {
		c.Logf("test %d", i)
		stdout, stderr, code := runLogin(c, "", test.args...)
		c.Check(stdout, gc.Equals, "")
		c.Check(stderr, gc.Matches, test.stderr)
		c.Assert(code, gc.Equals, 2)
	}
}

func (s *LoginCommandSuite) TestLogin(c *gc.C) {
	// When we run login with a current controller,
	// it will just verify that we can log in, leave
	// every unchanged and print nothing.
	stdout, stderr, code := runLogin(c, "")
	c.Check(code, gc.Equals, 0)
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Equals, "")
	s.assertStorePassword(c, "current-user", "old-password", "superuser")
	c.Assert(s.apiConnectionParams.AccountDetails, jc.DeepEquals, &jujuclient.AccountDetails{
		User:     "current-user",
		Password: "old-password",
	})
}

func (s *LoginCommandSuite) TestLoginNewUser(c *gc.C) {
	err := s.store.RemoveAccount("testing")
	c.Assert(err, jc.ErrorIsNil)
	stdout, stderr, code := runLogin(c, "", "-u", "new-user")
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Matches, `
Welcome, new-user. You are now logged into "testing".

There are no models available(.|\n)*`[1:])
	c.Assert(code, gc.Equals, 0)
	s.assertStorePassword(c, "new-user", "", "superuser")
	c.Assert(s.apiConnectionParams.AccountDetails, jc.DeepEquals, &jujuclient.AccountDetails{
		User: "new-user",
	})
}

func (s *LoginCommandSuite) TestLoginAlreadyLoggedInSameUser(c *gc.C) {
	stdout, stderr, code := runLogin(c, "", "-u", "current-user")
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)
}

func (s *LoginCommandSuite) TestLoginWithOneAvailableModel(c *gc.C) {
	s.PatchValue(user.ListModels, func(c api.Connection, userName string) ([]apibase.UserModel, error) {
		return []apibase.UserModel{{
			Name:  "foo",
			UUID:  "some-uuid",
			Owner: "bob",
		}}, nil
	})
	err := s.store.RemoveAccount("testing")
	c.Assert(err, jc.ErrorIsNil)
	stdout, stderr, code := runLogin(c, "")
	c.Assert(code, gc.Equals, 0)
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Matches, `Welcome, user@external. You are now logged into "testing".

Current model set to "bob/foo".
`)
}

func (s *LoginCommandSuite) TestLoginWithSeveralAvailableModels(c *gc.C) {
	s.PatchValue(user.ListModels, func(c api.Connection, userName string) ([]apibase.UserModel, error) {
		return []apibase.UserModel{{
			Name:  "foo",
			UUID:  "some-uuid",
			Owner: "bob",
		}, {
			Name:  "bar",
			UUID:  "some-uuid",
			Owner: "alice",
		}}, nil
	})
	err := s.store.RemoveAccount("testing")
	c.Assert(err, jc.ErrorIsNil)
	stdout, stderr, code := runLogin(c, "")
	c.Assert(code, gc.Equals, 0)
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Matches, `Welcome, user@external. You are now logged into "testing".

There are 2 models available. Use "juju switch" to select
one of them:
  - juju switch alice/bar
  - juju switch bob/foo
`)
}

func (s *LoginCommandSuite) TestLoginWithNonExistentController(c *gc.C) {
	stdout, stderr, code := runLogin(c, "", "-c", "something")
	c.Assert(code, gc.Equals, 1)
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Matches, `ERROR controller "something" does not exist\n`)
}

func (s *LoginCommandSuite) TestLoginWithNoCurrentController(c *gc.C) {
	s.store.CurrentControllerName = ""
	stdout, stderr, code := runLogin(c, "")
	c.Assert(code, gc.Equals, 1)
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Matches, `ERROR no current controller\n`)
}

func (s *LoginCommandSuite) TestLoginAlreadyLoggedInDifferentUser(c *gc.C) {
	stdout, stderr, code := runLogin(c, "", "-u", "other-user")
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Equals, `
ERROR cannot log into controller "testing": already logged in as current-user.

Run "juju logout" first before attempting to log in as a different user.
`[1:])
	c.Assert(code, gc.Equals, 1)
}

func (s *LoginCommandSuite) TestLoginWithExistingInvalidPassword(c *gc.C) {
	call := 0
	*user.NewAPIConnection = func(p juju.NewAPIConnectionParams) (api.Connection, error) {
		call++
		switch call {
		case 1:
			// First time: try to log in with existing details.
			c.Check(p.AccountDetails.User, gc.Equals, "current-user")
			c.Check(p.AccountDetails.Password, gc.Equals, "old-password")
			return nil, errors.Unauthorizedf("cannot login with that silly old password")
		case 2:
			// Second time: try external-user auth.
			c.Check(p.AccountDetails.User, gc.Equals, "")
			c.Check(p.AccountDetails.Password, gc.Equals, "")
			return nil, params.Error{
				Code:    params.CodeNoCreds,
				Message: params.CodeNoCreds,
			}
		case 3:
			// Third time: empty password: (the real
			// NewAPIConnection would prompt for it)
			c.Check(p.AccountDetails.User, gc.Equals, "other-user")
			c.Check(p.AccountDetails.Password, gc.Equals, "")
			return s.apiConnection, nil
		default:
			c.Errorf("NewAPIConnection called too many times")
			return nil, errors.Errorf("too many calls")
		}
	}
	stdout, stderr, code := runLogin(c, "other-user\n")
	c.Check(code, gc.Equals, 0)
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Matches, `username: Welcome, other-user. (.|\n)+`)
}

func (s *LoginCommandSuite) TestLoginWithMacaroons(c *gc.C) {
	err := s.store.RemoveAccount("testing")
	c.Assert(err, jc.ErrorIsNil)
	stdout, stderr, code := runLogin(c, "")
	c.Check(stderr, gc.Matches, `
Welcome, user@external. You are now logged into "testing".

There are no models available(.|\n)*`[1:])
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
		return s.apiConnection, nil
	}
	stdout, stderr, code := runLogin(c, "new-user\n")
	c.Check(stdout, gc.Equals, ``)
	c.Check(stderr, gc.Matches, `
username: Welcome, new-user. You are now logged into "testing".

There are no models available(.|\n)*`[1:])
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
