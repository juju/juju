// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

func (s *LoginCommandSuite) TestLoginFromDirectory(c *tc.C) {
	dirSrv := serveDirectory(map[string]string{
		"bighost": "bighost.jujucharms.com:443",
	})
	defer dirSrv.Close()
	os.Setenv("JUJU_DIRECTORY", dirSrv.URL)
	s.apiConnection.authTag = names.NewUserTag("bob@external")
	s.apiConnection.controllerAccess = "login"
	stdout, stderr, code := s.run(c, "bighost")
	c.Check(stderr, tc.Equals, `
Welcome, bob@external. You are now logged into "bighost".
`[1:]+user.NoModelsMessage)
	c.Check(stdout, tc.Equals, "")
	c.Assert(code, tc.Equals, 0)

	// The controller and account details should be recorded with
	// the specified controller name and user
	// name from the auth tag.

	controller, err := s.store.ControllerByName("bighost")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controller, tc.DeepEquals, &jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		APIEndpoints:   []string{"bighost.jujucharms.com:443"},
	})
	account, err := s.store.AccountDetails("bighost")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(account, tc.DeepEquals, &jujuclient.AccountDetails{
		User:            "bob@external",
		LastKnownAccess: "login",
	})

	// Test that we can run the same command again and it works.
	stdout, stderr, code = s.run(c, "bighost")
	c.Check(code, tc.Equals, 0)
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Equals, "")
}

func (s *LoginCommandSuite) TestLoginPublicDNSName(c *tc.C) {
	s.apiConnection.authTag = names.NewUserTag("bob@external")
	s.apiConnection.controllerAccess = "login"
	stdout, stderr, code := s.run(c, "0.1.2.3")
	c.Check(stderr, tc.Equals, `
Welcome, bob@external. You are now logged into "0.1.2.3".
`[1:]+user.NoModelsMessage)
	c.Check(stdout, tc.Equals, "")
	c.Assert(code, tc.Equals, 0)

	// The controller and account details should be recorded with
	// the specified controller name and user
	// name from the auth tag.
	controller, err := s.store.ControllerByName("0.1.2.3")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controller, tc.DeepEquals, &jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		APIEndpoints:   []string{"0.1.2.3:443"},
	})
	account, err := s.store.AccountDetails("0.1.2.3")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(account, tc.DeepEquals, &jujuclient.AccountDetails{
		User:            "bob@external",
		LastKnownAccess: "login",
	})
}

func (s *LoginCommandSuite) TestLoginWithControllerOnPathSegment(c *tc.C) {
	s.apiConnection.authTag = names.NewUserTag("bob@external")
	s.apiConnection.controllerAccess = "login"
	stdout, stderr, code := s.run(c, "-c", "foo", "mycontroller.com:443/bar")
	c.Check(stderr, tc.Equals, `
Welcome, bob@external. You are now logged into "foo".
`[1:]+user.NoModelsMessage)
	c.Check(stdout, tc.Equals, "")
	c.Assert(code, tc.Equals, 0)

	// The controller and account details should be recorded with
	// the specified controller name and user
	// name from the auth tag.
	controller, err := s.store.ControllerByName("foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controller, tc.DeepEquals, &jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		APIEndpoints:   []string{"mycontroller.com:443/bar"},
	})
	account, err := s.store.AccountDetails("foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(account, tc.DeepEquals, &jujuclient.AccountDetails{
		User:            "bob@external",
		LastKnownAccess: "login",
	})
}

func (s *LoginCommandSuite) TestRegisterPublicDNSNameWithPort(c *tc.C) {
	s.apiConnection.authTag = names.NewUserTag("bob@external")
	s.apiConnection.controllerAccess = "login"
	stdout, stderr, code := s.run(c, "0.1.2.3:5678")
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Equals, "ERROR cannot use \"0.1.2.3:5678\" as a controller name - use -c option to choose a different one\n")
	c.Check(code, tc.Equals, 1)
}

func (s *LoginCommandSuite) TestRegisterPublicDNSNameWithPortAndControllerFlag(c *tc.C) {
	s.apiConnection.authTag = names.NewUserTag("bob@external")
	s.apiConnection.controllerAccess = "login"
	stdout, stderr, code := s.run(c, "-c", "foo", "0.1.2.3:5678")
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Equals, `
Welcome, bob@external. You are now logged into "foo".
`[1:]+user.NoModelsMessage)
	c.Check(stdout, tc.Equals, "")
	c.Assert(code, tc.Equals, 0)

	// The controller and account details should be recorded with
	// the specified controller name and user
	// name from the auth tag.
	controller, err := s.store.ControllerByName("foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controller, tc.DeepEquals, &jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		APIEndpoints:   []string{"0.1.2.3:5678"},
	})
}

func (s *LoginCommandSuite) TestRegisterPublicAPIOpenError(c *tc.C) {
	srv := serveDirectory(map[string]string{"bighost": "https://0.1.2.3/directory"})
	defer srv.Close()
	os.Setenv("JUJU_DIRECTORY", srv.URL)
	*user.APIOpen = func(c *modelcmd.CommandBase, ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return nil, errors.New("open failed")
	}
	stdout, stderr, code := s.run(c, "bighost")
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Matches, `ERROR cannot log into "bighost": open failed\n`)
	c.Check(code, tc.Equals, 1)
}

func (s *LoginCommandSuite) TestRegisterPublicControllerMismatch(c *tc.C) {
	srv := serveDirectory(map[string]string{"bighost": "https://0.1.2.3/directory"})
	defer srv.Close()
	os.Setenv("JUJU_DIRECTORY", srv.URL)
	s.store.Controllers["other"] = jujuclient.ControllerDetails{
		APIEndpoints:   []string{"0.1.2.3:123"},
		CACert:         testing.CACert,
		ControllerUUID: "00000000-1111-2222-3333-444444444444",
	}
	stdout, stderr, code := s.run(c, "-c", "other", "bighost")
	c.Check(stdout, tc.Equals, "")
	c.Check(stderr, tc.Matches, `
ERROR controller at "bighost" does not match existing controller.
Please choose a different controller name with the -c option, or
use "juju unregister other" to remove the existing controller\.
`[1:])
	c.Check(code, tc.Equals, 1)
}

func (s *LoginCommandSuite) run(c *tc.C, args ...string) (stdout, stderr string, errCode int) {
	var stdoutBuf, stderrBuf bytes.Buffer
	ctxt := &cmd.Context{
		Dir:    c.MkDir(),
		Stdin:  strings.NewReader(""),
		Stdout: &stdoutBuf,
		Stderr: &stderrBuf,
	}
	exitCode := cmd.Main(user.NewLoginCommand(), ctxt, args)
	return stdoutBuf.String(), stderrBuf.String(), exitCode
}

// loginMockAPIConnection implements just enough of the api.Connection interface
// to satisfy the methods used by the login command.
type loginMockAPI struct {
	// This will be nil - it's just there to satisfy the api.Connection
	// interface methods not explicitly defined by loginMockAPIConnection.
	api.Connection

	// controllerTag is returned by ControllerTag.
	controllerTag names.ControllerTag

	// authTag is returned by AuthTag.
	authTag names.Tag

	// controllerAccess is returned by ControllerAccess.
	controllerAccess string
}

func (*loginMockAPI) Close() error {
	return nil
}

func (m *loginMockAPI) ControllerTag() names.ControllerTag {
	return m.controllerTag
}

func (m *loginMockAPI) AuthTag() names.Tag {
	return m.authTag
}

func (m *loginMockAPI) ControllerAccess() string {
	return m.controllerAccess
}

const mockControllerUUID = "df136476-12e9-11e4-8a70-b2227cce2b54"

func serveDirectory(dir map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		name := strings.TrimPrefix(req.URL.Path, "/v1/controller/")
		if name == req.URL.Path || dir[name] == "" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"host":%q}`, dir[name])
	}))
}
