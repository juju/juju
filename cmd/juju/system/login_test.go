// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system_test

import (
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/system"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type LoginSuite struct {
	testing.FakeJujuHomeSuite
	apiConnection *mockAPIConnection
	openError     error
	store         configstore.Storage
	username      string
	password      string
}

var _ = gc.Suite(&LoginSuite{})

func (s *LoginSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.store = configstore.NewMem()
	s.PatchValue(&configstore.Default, func() (configstore.Storage, error) {
		return s.store, nil
	})
	s.openError = nil
	s.apiConnection = &mockAPIConnection{
		serverTag: testing.EnvironmentTag,
		addr:      "192.168.2.1:1234",
	}
	s.username = "valid-user"
	s.password = "sekrit"
}

func (s *LoginSuite) apiOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	if s.openError != nil {
		return nil, s.openError
	}
	s.apiConnection.info = info
	s.apiConnection.opts = opts
	return s.apiConnection, nil
}

func (s *LoginSuite) getUserManager(conn api.Connection) (system.UserManager, error) {
	return s.apiConnection, nil
}

func (s *LoginSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := system.NewLoginCommand(s.apiOpen, s.getUserManager)
	return testing.RunCommand(c, command, args...)
}

func (s *LoginSuite) runServerFile(c *gc.C, args ...string) (*cmd.Context, error) {
	serverFilePath := filepath.Join(c.MkDir(), "server.yaml")
	content := `
addresses: ["192.168.2.1:1234", "192.168.2.2:1234"]
ca-cert: a-cert
username: ` + s.username + `
password: ` + s.password + `
`
	err := ioutil.WriteFile(serverFilePath, []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)
	allArgs := []string{"foo", "--server", serverFilePath}
	allArgs = append(allArgs, args...)
	return s.run(c, allArgs...)
}

func (s *LoginSuite) TestInit(c *gc.C) {
	loginCommand := system.NewLoginCommand(nil, nil)

	err := testing.InitCommand(loginCommand, []string{})
	c.Assert(err, gc.ErrorMatches, "no name specified")

	err = testing.InitCommand(loginCommand, []string{"foo"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(loginCommand.Name, gc.Equals, "foo")

	err = testing.InitCommand(loginCommand, []string{"foo", "bar"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *LoginSuite) TestNoSpecifiedServerFile(c *gc.C) {
	_, err := s.run(c, "foo")
	c.Assert(err, gc.ErrorMatches, "no server file specified")
}

func (s *LoginSuite) TestMissingServerFile(c *gc.C) {
	serverFilePath := filepath.Join(c.MkDir(), "server.yaml")
	_, err := s.run(c, "foo", "--server", serverFilePath)
	c.Assert(errors.Cause(err), jc.Satisfies, os.IsNotExist)
}

func (s *LoginSuite) TestBadServerFile(c *gc.C) {
	serverFilePath := filepath.Join(c.MkDir(), "server.yaml")
	err := ioutil.WriteFile(serverFilePath, []byte("&^%$#@"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, "foo", "--server", serverFilePath)
	c.Assert(err, gc.ErrorMatches, "YAML error: did not find expected alphabetic or numeric character")
}

func (s *LoginSuite) TestBadUser(c *gc.C) {
	serverFilePath := filepath.Join(c.MkDir(), "server.yaml")
	content := `
username: omg@not@valid
`
	err := ioutil.WriteFile(serverFilePath, []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, "foo", "--server", serverFilePath)
	c.Assert(err, gc.ErrorMatches, `"omg@not@valid" is not a valid username`)
}

func (s *LoginSuite) TestAPIOpenError(c *gc.C) {
	s.openError = errors.New("open failed")
	_, err := s.runServerFile(c)
	c.Assert(err, gc.ErrorMatches, `open failed`)
}

func (s *LoginSuite) TestMacaroonLogin(c *gc.C) {
	s.username, s.password = "", ""
	_, err := s.runServerFile(c)
	c.Assert(err, jc.ErrorIsNil)

	info := s.apiConnection.info
	opts := s.apiConnection.opts
	c.Assert(info, gc.NotNil)
	c.Assert(info.UseMacaroons, gc.Equals, true)
	c.Assert(info.Tag, gc.Equals, nil)
	c.Assert(info.Password, gc.Equals, "")
	c.Assert(opts.BakeryClient, gc.NotNil)
}

func (s *LoginSuite) TestOldServerNoServerUUID(c *gc.C) {
	s.apiConnection.serverTag = names.EnvironTag{}
	_, err := s.runServerFile(c)
	c.Assert(err, gc.ErrorMatches, `juju system too old to support login`)
}

func (s *LoginSuite) TestWritesConfig(c *gc.C) {
	ctx, err := s.runServerFile(c)
	c.Assert(err, jc.ErrorIsNil)

	info, err := s.store.ReadInfo("foo")
	c.Assert(err, jc.ErrorIsNil)
	creds := info.APICredentials()
	c.Assert(creds.User, gc.Equals, "valid-user")
	// Make sure that the password was changed, and that the new
	// value was not "sekrit".
	c.Assert(creds.Password, gc.Not(gc.Equals), "sekrit")
	c.Assert(creds.Password, gc.Equals, s.apiConnection.password)
	endpoint := info.APIEndpoint()
	c.Assert(endpoint.CACert, gc.Equals, "a-cert")
	c.Assert(endpoint.EnvironUUID, gc.Equals, "")
	c.Assert(endpoint.ServerUUID, gc.Equals, testing.EnvironmentTag.Id())
	c.Assert(endpoint.Addresses, jc.DeepEquals, []string{"192.168.2.1:1234"})
	c.Assert(endpoint.Hostnames, jc.DeepEquals, []string{"192.168.2.1:1234"})

	c.Assert(testing.Stderr(ctx), jc.Contains, "cached connection details as system \"foo\"\n")
	c.Assert(testing.Stderr(ctx), jc.Contains, "password updated\n")
}

func (s *LoginSuite) TestKeepPassword(c *gc.C) {
	_, err := s.runServerFile(c, "--keep-password")
	c.Assert(err, jc.ErrorIsNil)

	info, err := s.store.ReadInfo("foo")
	c.Assert(err, jc.ErrorIsNil)
	creds := info.APICredentials()
	c.Assert(creds.User, gc.Equals, "valid-user")
	c.Assert(creds.Password, gc.Equals, "sekrit")
}

func (s *LoginSuite) TestRemoteUsersKeepPassword(c *gc.C) {
	s.username = "user@remote"
	_, err := s.runServerFile(c)
	c.Assert(err, jc.ErrorIsNil)

	info, err := s.store.ReadInfo("foo")
	c.Assert(err, jc.ErrorIsNil)
	creds := info.APICredentials()
	c.Assert(creds.User, gc.Equals, "user@remote")
	c.Assert(creds.Password, gc.Equals, "sekrit")
}

func (s *LoginSuite) TestConnectsUsingServerFileInfo(c *gc.C) {
	s.username = "valid-user@local"
	_, err := s.runServerFile(c)
	c.Assert(err, jc.ErrorIsNil)

	info := s.apiConnection.info
	c.Assert(info.Addrs, jc.DeepEquals, []string{"192.168.2.1:1234", "192.168.2.2:1234"})
	c.Assert(info.CACert, gc.Equals, "a-cert")
	c.Assert(info.EnvironTag.Id(), gc.Equals, "")
	c.Assert(info.Tag.Id(), gc.Equals, "valid-user@local")
	c.Assert(info.Password, gc.Equals, "sekrit")
	c.Assert(info.Nonce, gc.Equals, "")
}

func (s *LoginSuite) TestWritesCurrentSystem(c *gc.C) {
	_, err := s.runServerFile(c)
	c.Assert(err, jc.ErrorIsNil)
	currentSystem, err := envcmd.ReadCurrentSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currentSystem, gc.Equals, "foo")
}

type mockAPIConnection struct {
	api.Connection
	info         *api.Info
	opts         api.DialOpts
	addr         string
	apiHostPorts [][]network.HostPort
	serverTag    names.EnvironTag
	username     string
	password     string
}

func (*mockAPIConnection) Close() error {
	return nil
}

func (m *mockAPIConnection) Addr() string {
	return m.addr
}

func (m *mockAPIConnection) APIHostPorts() [][]network.HostPort {
	return m.apiHostPorts
}

func (m *mockAPIConnection) ServerTag() (names.EnvironTag, error) {
	if m.serverTag.Id() == "" {
		return m.serverTag, errors.New("no server tag")
	}
	return m.serverTag, nil
}

func (m *mockAPIConnection) SetPassword(username, password string) error {
	m.username = username
	m.password = password
	return nil
}

var _ = gc.Suite(&macaroonLoginSuite{})

type macaroonLoginSuite struct {
	jujutesting.JujuConnSuite
	discharger     *bakerytest.Discharger
	checker        func(string, string) ([]checkers.Caveat, error)
	srv            *apiserver.Server
	client         api.Connection
	serverFilePath string
}

func (s *macaroonLoginSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.discharger = bakerytest.NewDischarger(nil, func(req *http.Request, cond, arg string) ([]checkers.Caveat, error) {
		return s.checker(cond, arg)
	})

	environTag := names.NewEnvironTag(s.State.EnvironUUID())

	// Make a new version of the state that doesn't object to us
	// changing the identity URL, so we can create a state server
	// that will see that.
	st, err := state.Open(environTag, s.MongoInfo(c), mongo.DefaultDialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	err = st.UpdateEnvironConfig(map[string]interface{}{
		config.IdentityURL: s.discharger.Location(),
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.client, s.srv = s.newClientAndServer(c)

	s.Factory.MakeUser(c, &factory.UserParams{
		Name: "test",
	})

	var serverDetails envcmd.ServerFile
	serverDetails.Addresses = []string{s.srv.Addr().String()}
	serverDetails.CACert = coretesting.CACert
	content, err := goyaml.Marshal(serverDetails)
	c.Assert(err, jc.ErrorIsNil)

	s.serverFilePath = filepath.Join(c.MkDir(), "server.yaml")

	err = ioutil.WriteFile(s.serverFilePath, content, 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *macaroonLoginSuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *macaroonLoginSuite) TestsSuccessfulLogin(c *gc.C) {
	s.checker = func(cond, arg string) ([]checkers.Caveat, error) {
		if cond == "is-authenticated-user" {
			return []checkers.Caveat{checkers.DeclaredCaveat("username", "test")}, nil
		}
		return nil, errors.New("unknown caveat")
	}

	allArgs := []string{"foo", "--server", s.serverFilePath}

	command := system.NewLoginCommand(nil, nil)
	_, err := testing.RunCommand(c, command, allArgs...)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *macaroonLoginSuite) TestsFailToObtainDischargeLogin(c *gc.C) {
	s.checker = func(cond, arg string) ([]checkers.Caveat, error) {
		return nil, errors.New("unknown caveat")
	}

	allArgs := []string{"foo", "--server", s.serverFilePath}
	command := system.NewLoginCommand(nil, nil)
	_, err := testing.RunCommand(c, command, allArgs...)
	c.Assert(err, gc.ErrorMatches, ".*third party refused discharge: cannot discharge: unknown caveat")
}

func (s *macaroonLoginSuite) TestsUnknownUserLogin(c *gc.C) {
	s.checker = func(cond, arg string) ([]checkers.Caveat, error) {
		if cond == "is-authenticated-user" {
			return []checkers.Caveat{checkers.DeclaredCaveat("username", "testUnknown")}, nil
		}
		return nil, errors.New("unknown caveat")
	}

	allArgs := []string{"foo", "--server", s.serverFilePath}

	command := system.NewLoginCommand(nil, nil)
	_, err := testing.RunCommand(c, command, allArgs...)
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password")
}

// newClientAndServer returns a new running API server.
func (s *macaroonLoginSuite) newClientAndServer(c *gc.C) (api.Connection, *apiserver.Server) {
	listener, err := net.Listen("tcp", "localhost:0")
	c.Assert(err, jc.ErrorIsNil)
	srv, err := apiserver.NewServer(s.State, listener, apiserver.ServerConfig{
		Cert: []byte(coretesting.ServerCert),
		Key:  []byte(coretesting.ServerKey),
		Tag:  names.NewMachineTag("0"),
	})
	c.Assert(err, jc.ErrorIsNil)

	client, err := api.Open(&api.Info{
		Addrs:  []string{srv.Addr().String()},
		CACert: coretesting.CACert,
	}, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)

	return client, srv
}
