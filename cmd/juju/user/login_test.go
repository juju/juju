// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api"
	apibase "github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type LoginCommandSuite struct {
	BaseSuite
	apiConnection *loginMockAPI

	// apiConnectionParams is set when the mock newAPIConnection
	// implementation installed by SetUpTest is called.
	apiConnectionParams juju.NewAPIConnectionParams
}

func TestLoginCommandSuite(t *stdtesting.T) {
	tc.Run(t, &LoginCommandSuite{})
}

func (s *LoginCommandSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.apiConnection = &loginMockAPI{
		controllerTag:    names.NewControllerTag(mockControllerUUID),
		authTag:          names.NewUserTag("user@external"),
		controllerAccess: "superuser",
	}
	s.apiConnectionParams = juju.NewAPIConnectionParams{}
	s.PatchValue(user.NewAPIConnection, func(_ context.Context, p juju.NewAPIConnectionParams) (api.Connection, error) {
		// The account details are modified in place, so take a copy.
		accountDetails := *p.AccountDetails
		p.AccountDetails = &accountDetails
		s.apiConnectionParams = p
		return s.apiConnection, nil
	})
	s.PatchValue(user.ListModels, func(_ context.Context, c api.Connection, userName string) ([]apibase.UserModel, error) {
		return nil, nil
	})
	s.PatchValue(user.APIOpen, func(c *modelcmd.CommandBase, _ context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return s.apiConnection, nil
	})
	s.PatchValue(user.LoginClientStore, s.store)
}

func (s *LoginCommandSuite) TestInitError(c *tc.C) {
	for i, test := range []struct {
		args   []string
		stderr string
	}{{
		args:   []string{"--foobar"},
		stderr: `ERROR option provided but not defined: --foobar\n`,
	}, {
		args:   []string{"foobar", "extra"},
		stderr: `ERROR unrecognized args: \["extra"\]\n`,
	}} {
		c.Logf("test %d", i)
		stdout, stderr, code := runLogin(c, "", test.args...)
		c.Check(stdout, tc.Equals, "")
		c.Check(stderr, tc.Matches, test.stderr)
		c.Assert(code, tc.Equals, 2)
	}
}

func (s *LoginCommandSuite) TestLogin(c *tc.C) {
	// When we run login with a current controller,
	// it will just verify that we can log in, leave
	// every unchanged and print nothing.
	stdout, stderr, code := runLogin(c, "")
	c.Check(code, tc.Equals, 0)
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Equals, "")
	s.assertStorePassword(c, "current-user", "old-password", "superuser")
	c.Assert(s.apiConnectionParams.AccountDetails, tc.DeepEquals, &jujuclient.AccountDetails{
		User:     "current-user",
		Password: "old-password",
	})
}

func (s *LoginCommandSuite) TestLoginNewUser(c *tc.C) {
	err := s.store.RemoveAccount("testing")
	c.Assert(err, tc.ErrorIsNil)
	stdout, stderr, code := runLogin(c, "", "-u", "new-user")
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Matches, `
Welcome, new-user. You are now logged into "testing".

There are no models available(.|\n)*`[1:])
	c.Assert(code, tc.Equals, 0)
	s.assertStorePassword(c, "new-user", "", "superuser")
	c.Assert(s.apiConnectionParams.AccountDetails, tc.DeepEquals, &jujuclient.AccountDetails{
		User: "new-user",
	})
}

func (s *LoginCommandSuite) TestLoginAlreadyLoggedInSameUser(c *tc.C) {
	stdout, stderr, code := runLogin(c, "", "-u", "current-user")
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Equals, "")
	c.Assert(code, tc.Equals, 0)
}

func (s *LoginCommandSuite) TestLoginWithOneAvailableModel(c *tc.C) {
	s.PatchValue(user.ListModels, func(_ context.Context, c api.Connection, userName string) ([]apibase.UserModel, error) {
		return []apibase.UserModel{{
			Name:  "foo",
			UUID:  "some-uuid",
			Owner: "bob",
			Type:  "iaas",
		}}, nil
	})
	err := s.store.RemoveAccount("testing")
	c.Assert(err, tc.ErrorIsNil)
	stdout, stderr, code := runLogin(c, "")
	c.Assert(code, tc.Equals, 0)
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Matches, `Welcome, user@external. You are now logged into "testing".

Current model set to "bob/foo".
`)
}

func (s *LoginCommandSuite) TestLoginWithSeveralAvailableModels(c *tc.C) {
	s.PatchValue(user.ListModels, func(_ context.Context, c api.Connection, userName string) ([]apibase.UserModel, error) {
		return []apibase.UserModel{{
			Name:  "foo",
			UUID:  "some-uuid",
			Owner: "bob",
			Type:  "iaas",
		}, {
			Name:  "bar",
			UUID:  "some-uuid",
			Owner: "alice",
			Type:  "iaas",
		}}, nil
	})
	err := s.store.RemoveAccount("testing")
	c.Assert(err, tc.ErrorIsNil)
	stdout, stderr, code := runLogin(c, "")
	c.Assert(code, tc.Equals, 0)
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Matches, `Welcome, user@external. You are now logged into "testing".

There are 2 models available. Use "juju switch" to select
one of them:
  - juju switch alice/bar
  - juju switch bob/foo
`)
}

func (s *LoginCommandSuite) TestLoginWithNonExistentController(c *tc.C) {
	stdout, stderr, code := runLogin(c, "", "-c", "something")
	c.Assert(code, tc.Equals, 1)
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Matches, `ERROR controller "something" does not exist\n`)
}

func (s *LoginCommandSuite) TestLoginWithNoCurrentController(c *tc.C) {
	s.store.CurrentControllerName = ""
	stdout, stderr, code := runLogin(c, "")
	c.Assert(code, tc.Equals, 1)
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Matches, `ERROR no current controller\n`)
}

func (s *LoginCommandSuite) TestLoginAlreadyLoggedInDifferentUser(c *tc.C) {
	stdout, stderr, code := runLogin(c, "", "-u", "other-user")
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Equals, `
ERROR cannot log into controller "testing": already logged in as current-user.

Run "juju logout" first before attempting to log in as a different user.
`[1:])
	c.Assert(code, tc.Equals, 1)
}

func (s *LoginCommandSuite) TestLoginWithExistingInvalidPassword(c *tc.C) {
	call := 0
	*user.NewAPIConnection = func(ctx context.Context, p juju.NewAPIConnectionParams) (api.Connection, error) {
		call++
		switch call {
		case 1:
			// First time: try to log in with existing details.
			c.Check(p.AccountDetails.User, tc.Equals, "current-user")
			c.Check(p.AccountDetails.Password, tc.Equals, "old-password")
			return nil, errors.Unauthorizedf("cannot login with that silly old password")
		case 2:
			// Second time: try external-user auth.
			c.Check(p.AccountDetails.User, tc.Equals, "")
			c.Check(p.AccountDetails.Password, tc.Equals, "")
			return nil, params.Error{
				Code:    params.CodeNoCreds,
				Message: params.CodeNoCreds,
			}
		case 3:
			// Third time: empty password: (the real
			// NewAPIConnection would prompt for it)
			c.Check(p.AccountDetails.User, tc.Equals, "other-user")
			c.Check(p.AccountDetails.Password, tc.Equals, "")
			return s.apiConnection, nil
		default:
			c.Errorf("NewAPIConnection called too many times")
			return nil, errors.Errorf("too many calls")
		}
	}
	stdout, stderr, code := runLogin(c, "other-user\n")
	c.Check(code, tc.Equals, 0)
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Matches, `Enter username: 
Welcome, other-user. (.|\n)+`)
}

func (s *LoginCommandSuite) TestLoginWithMacaroons(c *tc.C) {
	err := s.store.RemoveAccount("testing")
	c.Assert(err, tc.ErrorIsNil)
	stdout, stderr, code := runLogin(c, "")
	c.Check(stderr, tc.Matches, `
Welcome, user@external. You are now logged into "testing".

There are no models available(.|\n)*`[1:])
	c.Check(stdout, tc.Equals, ``)
	c.Assert(code, tc.Equals, 0)
	c.Assert(s.apiConnectionParams.AccountDetails, tc.DeepEquals, &jujuclient.AccountDetails{})
}

func (s *LoginCommandSuite) TestLoginWithMacaroonsNotSupported(c *tc.C) {
	err := s.store.RemoveAccount("testing")
	c.Assert(err, tc.ErrorIsNil)
	*user.NewAPIConnection = func(ctx context.Context, p juju.NewAPIConnectionParams) (api.Connection, error) {
		if !c.Check(p.AccountDetails, tc.NotNil) {
			return nil, errors.New("no account details")
		}
		if p.AccountDetails.User == "" && p.AccountDetails.Password == "" {
			return nil, &params.Error{Code: params.CodeNoCreds, Message: "barf"}
		}
		c.Check(p.AccountDetails.User, tc.Equals, "new-user")
		return s.apiConnection, nil
	}
	stdout, stderr, code := runLogin(c, "new-user\n")
	c.Check(stdout, tc.Equals, ``)
	c.Check(stderr, tc.Matches, `
Enter username: 
Welcome, new-user. You are now logged into "testing".

There are no models available(.|\n)*`[1:])
	c.Assert(code, tc.Equals, 0)
}

func (s *LoginCommandSuite) TestLoginWithCAVerification(c *tc.C) {
	caCert := testing.CACertX509
	fingerprint, _, err := pki.Fingerprint([]byte(testing.CACert))
	c.Assert(err, tc.ErrorIsNil)

	specs := []struct {
		descr     string
		input     string
		host      string
		endpoint  string
		expRegex  string
		expCode   int
		extraArgs []string
	}{
		{
			descr:    "user trusts CA cert",
			input:    "y\n",
			host:     "myprivatecontroller:443",
			endpoint: "127.0.0.1:443",
			expRegex: `
Controller "myprivatecontroller:443" (127.0.0.1:443) presented a CA cert that could not be verified.
CA fingerprint: [` + fingerprint + `]
Trust remote controller? (y/N): 
Welcome, new-user. You are now logged into "foo".

There are no models available. You can add models with
"juju add-model", or you can ask an administrator or owner
of a model to grant access to that model with "juju grant".
`,
		},
		{
			descr:    "user does not trust CA cert",
			input:    "n\n",
			host:     "myprivatecontroller:443",
			endpoint: "127.0.0.1:443",
			expRegex: `
Controller "myprivatecontroller:443" (127.0.0.1:443) presented a CA cert that could not be verified.
CA fingerprint: [` + fingerprint + `]
Trust remote controller? (y/N): 
ERROR cannot log into "myprivatecontroller:443": controller CA not trusted
`,
			expCode: 1,
		},
		{
			descr:    "user does not trust CA cert when logging to a controller IP",
			input:    "n\n",
			host:     "127.0.0.1:443",
			endpoint: "127.0.0.1:443",
			expRegex: `
Controller "127.0.0.1:443" presented a CA cert that could not be verified.
CA fingerprint: [` + fingerprint + `]
Trust remote controller? (y/N): 
ERROR cannot log into "127.0.0.1:443": controller CA not trusted
`,
			expCode: 1,
		},
		{
			descr:     "user does not trust CA cert when logging to a controller IP without prompt",
			input:     "",
			extraArgs: []string{"--no-prompt"},
			host:      "127.0.0.1:443",
			endpoint:  "127.0.0.1:443",
			expRegex: `
Controller "127.0.0.1:443" presented a CA cert that could not be verified.
CA fingerprint: [` + fingerprint + `]
ERROR cannot log into "127.0.0.1:443": controller CA not trusted
`,
			expCode: 1,
		},
		{
			descr:     "user trusts blindly with --trust",
			input:     "",
			extraArgs: []string{"--no-prompt", "--trust"},
			host:      "127.0.0.1:443",
			endpoint:  "127.0.0.1:443",
			expRegex: `
Controller "127.0.0.1:443" presented a CA cert that could not be verified.
CA fingerprint: [` + fingerprint + `]
Welcome, new-user. You are now logged into "foo".

There are no models available. You can add models with
"juju add-model", or you can ask an administrator or owner
of a model to grant access to that model with "juju grant".
`,
			expCode: 0,
		},
		{
			descr:     "user does not trust CA cert when logging to a controller IP ignoring --trust",
			input:     "n\n",
			extraArgs: []string{"--trust"},
			host:      "127.0.0.1:443",
			endpoint:  "127.0.0.1:443",
			expRegex: `
Controller "127.0.0.1:443" presented a CA cert that could not be verified.
CA fingerprint: [` + fingerprint + `]
Ignoring --trust without --no-prompt
Trust remote controller? (y/N): 
ERROR cannot log into "127.0.0.1:443": controller CA not trusted
`,
			expCode: 1,
		},
	}

	for specIndex, spec := range specs {
		c.Logf("test %d: %s", specIndex, spec.descr)
		_ = s.store.RemoveAccount("foo")
		_ = s.store.RemoveController("foo")

		*user.APIOpen = func(c *modelcmd.CommandBase, ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
			if err := opts.VerifyCA(spec.host, spec.endpoint, caCert); err != nil {
				return nil, err
			}
			return s.apiConnection, nil
		}

		stdout, stderr, code := runLogin(c, spec.input, append([]string{spec.host, "-c", "foo", "-u", "new-user"}, spec.extraArgs...)...)
		c.Check(stdout, tc.Equals, ``)
		c.Check(stderr, tc.Equals, spec.expRegex[1:])
		c.Assert(code, tc.Equals, spec.expCode)

		// For successful login make sure that the controller CA cert
		// gets persisted in the controller store
		if code == 0 {
			ctrl, err := s.store.ControllerByName("foo")
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(ctrl.CACert, tc.Equals, testing.CACert)
		}
	}
}

func (s *LoginCommandSuite) TestLoginUsingKnownControllerEndpoint(c *tc.C) {
	var (
		existingName string
		details      jujuclient.ControllerDetails
	)
	for existingName, details = range s.store.Controllers {
		break
	}

	s.store.Controllers["controller-with-no-account"] = jujuclient.ControllerDetails{
		APIEndpoints:   []string{"1.1.1.1:12345"},
		CACert:         testing.CACert,
		ControllerUUID: testing.ControllerTag.Id(),
	}

	specs := []struct {
		descr  string
		cmd    []string
		expErr string
	}{
		{
			descr: "user provides an endpoint as the controller name and has a local account for the controller",
			cmd:   []string{details.APIEndpoints[0]},
			expErr: `
ERROR This controller has already been registered on this client as "` + existingName + `".
To login as user "current-user" run 'juju login -u current-user -c ` + existingName + `'.
`,
		},
		{
			descr: "user provides an endpoint as the controller name and does not have a local account for the controller",
			cmd:   []string{"1.1.1.1:12345"},
			expErr: `
ERROR This controller has already been registered on this client as "controller-with-no-account".
To login run 'juju login -c controller-with-no-account'.
`,
		},
		{
			descr: "user provides an endpoint and overrides the controller name",
			cmd:   []string{details.APIEndpoints[0], "-c", "some-controller-name"},
			expErr: `
ERROR This controller has already been registered on this client as "` + existingName + `".
To login as user "current-user" run 'juju login -u current-user -c ` + existingName + `'.
`,
		},
	}

	for specIndex, spec := range specs {
		c.Logf("test %d: %s (juju login %s)", specIndex, spec.descr, spec.cmd)
		stdout, stderr, code := runLogin(c, "", spec.cmd...)
		c.Check(stdout, tc.Equals, ``)
		c.Check(stderr, tc.Equals, spec.expErr[1:])
		c.Assert(code, tc.Equals, 1)
	}
}

func (s *LoginCommandSuite) TestLoginErrorSpecifyingUsernameWithOIDC(c *tc.C) {
	s.store.Controllers["oidc-controller"] = jujuclient.ControllerDetails{
		APIEndpoints:   []string{"1.1.1.1:12345"},
		CACert:         testing.CACert,
		ControllerUUID: testing.ControllerTag.Id(),
		OIDCLogin:      true,
	}
	s.store.CurrentControllerName = "oidc-controller"
	stdout, stderr, code := runLogin(c, "", "-u", "some-user")
	c.Assert(code, tc.Equals, 1)
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Matches, `ERROR cannot specify a username during login to a controller with OIDC, remove the username and try again\n`)
}

// TestLoginWithOIDCWithNoAccountverifies that login to a controller (with OIDC) known
// to the client store is successful, e.g. when running `juju logout` then `juju login`.
func (s *LoginCommandSuite) TestLoginWithOIDCWithNoAccount(c *tc.C) {
	s.store.Controllers["oidc-controller"] = jujuclient.ControllerDetails{
		APIEndpoints:   []string{"1.1.1.1:12345"},
		CACert:         testing.CACert,
		ControllerUUID: testing.ControllerTag.Id(),
		OIDCLogin:      true,
	}
	s.store.CurrentControllerName = "oidc-controller"
	var checkPatchFuncCalled bool
	s.PatchValue(user.NewAPIConnection, func(_ context.Context, p juju.NewAPIConnectionParams) (api.Connection, error) {
		sessionTokenLogin := api.NewSessionTokenLoginProvider("", nil, nil)
		c.Check(p.DialOpts.LoginProvider, tc.FitsTypeOf, sessionTokenLogin)
		c.Check(p.AccountDetails, tc.NotNil)
		if p.AccountDetails != nil {
			p.AccountDetails.SessionToken = "new-token"
		}
		checkPatchFuncCalled = true
		return s.apiConnection, nil
	})
	_, _, code := runLogin(c, "")
	c.Assert(code, tc.Equals, 0)
	c.Assert(checkPatchFuncCalled, tc.Equals, true)

	account := s.store.Accounts["oidc-controller"]
	c.Assert(account.SessionToken, tc.Equals, "new-token")
	c.Assert(account.User, tc.Equals, "user@external")
}

// TestLoginToPublicControllerWithOIDC verifies that login to a controller (with OIDC)
// that we have never seen before is successful and correctly updates the client store.
func (s *LoginCommandSuite) TestLoginToPublicControllerWithOIDC(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	sessionLoginFactory := NewMockSessionLoginFactory(ctrl)
	sessionLoginProvider := NewMockLoginProvider(ctrl)

	var checkPatchFuncCalled bool
	*user.APIOpen = func(cmd *modelcmd.CommandBase, ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		checkPatchFuncCalled = true
		_, err := opts.LoginProvider.Login(ctx, nil)
		c.Check(err, tc.ErrorIsNil)
		return s.apiConnection, nil
	}

	var tokenCallbackFunc func(string)
	sessionLoginFactory.EXPECT().NewLoginProvider("", gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ string, _ io.Writer, f func(string)) api.LoginProvider {
			tokenCallbackFunc = f
			return sessionLoginProvider
		})

	sessionLoginProvider.EXPECT().Login(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ apibase.APICaller) (*api.LoginResultParams, error) {
			tokenCallbackFunc("session-token")
			return nil, nil
		})

	_, _, code := runLoginWithFakeSessionLoginProvider(c, sessionLoginFactory, "mycontroller.com", "-c", "oidc-controller")
	c.Assert(code, tc.Equals, 0)
	c.Assert(checkPatchFuncCalled, tc.Equals, true)

	acc := s.store.Accounts["oidc-controller"]
	c.Assert(acc.SessionToken, tc.Equals, "session-token")
	c.Assert(acc.User, tc.Equals, "user@external")
}

func runLoginWithFakeSessionLoginProvider(c *tc.C, factory modelcmd.SessionLoginFactory, args ...string) (stdout, stderr string, errCode int) {
	loginCmd := user.NewLoginCommandWithSessionLoginFactory(factory)
	return run(c, "", loginCmd, args...)
}

func runLogin(c *tc.C, stdin string, args ...string) (stdout, stderr string, errCode int) {
	loginCmd := user.NewLoginCommand()
	return run(c, stdin, loginCmd, args...)
}

func run(c *tc.C, stdin string, command cmd.Command, args ...string) (stdout, stderr string, errCode int) {
	c.Logf("in LoginControllerSuite.run")
	var stdoutBuf, stderrBuf bytes.Buffer
	ctxt := &cmd.Context{
		Dir:    c.MkDir(),
		Stdin:  strings.NewReader(stdin),
		Stdout: &stdoutBuf,
		Stderr: &stderrBuf,
	}
	exitCode := cmd.Main(command, ctxt, args)
	return stdoutBuf.String(), stderrBuf.String(), exitCode
}
