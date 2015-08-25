// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/system"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type LoginSuite struct {
	testing.FakeJujuHomeSuite
	apiConnection *mockAPIConnection
	openError     error
	store         configstore.Storage
	username      string
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
}

func (s *LoginSuite) apiOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	if s.openError != nil {
		return nil, s.openError
	}
	s.apiConnection.info = info
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
password: sekrit
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
