// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

// LoginControllerSuite tests the functionality of the login command
// that logs into controllers rather than choosing a user name.
// Most of the tests come from cmd/juju/controller/register_test.go - eventually
// that command will be deleted.
type LoginControllerSuite struct {
	BaseSuite
	apiConnection      *loginMockAPI
	listModels         func(jujuclient.ClientStore, string, string) ([]base.UserModel, error)
	listModelsUserName string
	server             *httptest.Server
	httpHandler        http.Handler
}

var _ = gc.Suite(&LoginControllerSuite{})

func (s *LoginControllerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.httpHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	s.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.httpHandler.ServeHTTP(w, r)
	}))

	serverURL, err := url.Parse(s.server.URL)
	c.Assert(err, jc.ErrorIsNil)
	s.apiConnection = &loginMockAPI{
		controllerTag: names.NewControllerTag(mockControllerUUID),
		addr:          serverURL.Host,
	}
	s.listModelsUserName = ""
	s.PatchValue(user.ListModels, func(_ *modelcmd.ControllerCommandBase, userName string) ([]base.UserModel, error) {
		s.listModelsUserName = userName
		return nil, nil
	})
	s.PatchValue(user.APIOpen, func(c *modelcmd.JujuCommandBase, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return s.apiConnection, nil
	})
	s.PatchValue(user.LoginClientStore, s.store)
	s.PatchEnvironment("JUJU_DIRECTORY", "http://0.1.2.3/directory")
}

func (s *LoginControllerSuite) TearDownTest(c *gc.C) {
	s.server.Close()
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *LoginControllerSuite) TestRegisterPublic(c *gc.C) {
	dirSrv := serveDirectory(map[string]string{
		"bighost": "bighost.jujucharms.com:443",
	})
	defer dirSrv.Close()
	os.Setenv("JUJU_DIRECTORY", dirSrv.URL)
	s.apiConnection.authTag = names.NewUserTag("bob@external")
	s.apiConnection.controllerAccess = "login"
	stdout, stderr, code := s.run(c, "bighost")
	c.Check(stderr, gc.Equals, `
Welcome, bob@external. You are now logged into "bighost".
`[1:]+user.NoModelsMessage)
	c.Check(stdout, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	// The controller and account details should be recorded with
	// the specified controller name and user
	// name from the auth tag.

	controller, err := s.store.ControllerByName("bighost")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controller, jc.DeepEquals, &jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		APIEndpoints:   []string{"bighost.jujucharms.com:443"},
	})
	account, err := s.store.AccountDetails("bighost")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(account, jc.DeepEquals, &jujuclient.AccountDetails{
		User:            "bob@external",
		LastKnownAccess: "login",
	})
}

func (s *LoginControllerSuite) TestRegisterPublicHostname(c *gc.C) {
	s.apiConnection.authTag = names.NewUserTag("bob@external")
	s.apiConnection.controllerAccess = "login"
	stdout, stderr, code := s.run(c, "--host", "0.1.2.3")
	c.Check(stderr, gc.Equals, `
Welcome, bob@external. You are now logged into "0.1.2.3".
`[1:]+user.NoModelsMessage)
	c.Check(stdout, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	// The controller and account details should be recorded with
	// the specified controller name and user
	// name from the auth tag.
	controller, err := s.store.ControllerByName("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controller, jc.DeepEquals, &jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		APIEndpoints:   []string{"0.1.2.3:443"},
	})
	account, err := s.store.AccountDetails("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(account, jc.DeepEquals, &jujuclient.AccountDetails{
		User:            "bob@external",
		LastKnownAccess: "login",
	})
}

func (s *LoginControllerSuite) TestRegisterPublicHostnameWithPort(c *gc.C) {
	s.apiConnection.authTag = names.NewUserTag("bob@external")
	s.apiConnection.controllerAccess = "login"
	stdout, stderr, code := s.run(c, "--host", "0.1.2.3:5678")
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Equals, "error: cannot use \"0.1.2.3:5678\" as controller name - use -c flag to choose a different one\n")
	c.Check(code, gc.Equals, 1)
}

func (s *LoginControllerSuite) TestRegisterPublicHostnameWithPortAndControllerFlag(c *gc.C) {
	s.apiConnection.authTag = names.NewUserTag("bob@external")
	s.apiConnection.controllerAccess = "login"
	stdout, stderr, code := s.run(c, "-c", "foo", "--host", "0.1.2.3:5678")
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Equals, `
Welcome, bob@external. You are now logged into "foo".
`[1:]+user.NoModelsMessage)
	c.Check(stdout, gc.Equals, "")
	c.Assert(code, gc.Equals, 0)

	// The controller and account details should be recorded with
	// the specified controller name and user
	// name from the auth tag.
	controller, err := s.store.ControllerByName("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controller, jc.DeepEquals, &jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		APIEndpoints:   []string{"0.1.2.3:5678"},
	})
}

func (s *LoginControllerSuite) TestRegisterPublicAPIOpenError(c *gc.C) {
	srv := serveDirectory(map[string]string{"bighost": "https://0.1.2.3/directory"})
	defer srv.Close()
	os.Setenv("JUJU_DIRECTORY", srv.URL)
	*user.APIOpen = func(c *modelcmd.JujuCommandBase, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return nil, errors.New("open failed")
	}
	stdout, stderr, code := s.run(c, "bighost")
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Matches, `error: open failed\n`)
	c.Check(code, gc.Equals, 1)
}

func (s *LoginControllerSuite) run(c *gc.C, args ...string) (stdout, stderr string, errCode int) {
	c.Logf("in LoginControllerSuite.run")
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

	// addr is returned by Addr.
	addr string

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
