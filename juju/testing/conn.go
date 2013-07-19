// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"os"
	"path/filepath"
)

// JujuConnSuite provides a freshly bootstrapped juju.Conn
// for each test. It also includes testing.LoggingSuite.
//
// It also sets up RootDir to point to a directory hierarchy
// mirroring the intended juju directory structure, including
// the following:
//     RootDir/home/ubuntu/.juju/environments.yaml
//         The dummy environments.yaml file, holding
//         a default environment named "dummyenv"
//         which uses the "dummy" environment type.
//     RootDir/var/lib/juju
//         An empty directory returned as DataDir - the
//         root of the juju data storage space.
// $HOME is set to point to RootDir/home/ubuntu.
type JujuConnSuite struct {
	// TODO: JujuConnSuite should not be concerned both with JUJU_HOME and with
	// /var/lib/juju: the use cases are completely non-overlapping, and any tests that
	// really do need both to exist ought to be embedding distinct fixtures for the
	// distinct environments.
	testing.LoggingSuite
	testing.MgoSuite
	Conn         *juju.Conn
	State        *state.State
	APIConn      *juju.APIConn
	APIState     *api.State
	BackingState *state.State // The State being used by the API server
	RootDir      string       // The faked-up root directory.
	oldHome      string
	oldJujuHome  string
	environ      environs.Environ
}

// FakeStateInfo holds information about no state - it will always
// give an error when connected to.  The machine id gives the machine id
// of the machine to be started.
func FakeStateInfo(machineId string) *state.Info {
	return &state.Info{
		Addrs:    []string{"0.1.2.3:1234"},
		Tag:      state.MachineTag(machineId),
		Password: "unimportant",
		CACert:   []byte(testing.CACert),
	}
}

// FakeAPIInfo holds information about no state - it will always
// give an error when connected to.  The machine id gives the machine id
// of the machine to be started.
func FakeAPIInfo(machineId string) *api.Info {
	return &api.Info{
		Addrs:    []string{"0.1.2.3:1234"},
		Tag:      state.MachineTag(machineId),
		Password: "unimportant",
		CACert:   []byte(testing.CACert),
	}
}

// StartInstance is a test helper function that starts an instance on the
// environment using the current series and invalid info states.
func StartInstance(c *C, env environs.Environ, machineId string) (instance.Instance, *instance.HardwareCharacteristics) {
	return StartInstanceWithConstraints(c, env, machineId, constraints.Value{})
}

// StartInstanceWithConstraints is a test helper function that starts an instance on the
// environment with the specified constraints, using the current series and invalid info states.
func StartInstanceWithConstraints(c *C, env environs.Environ, machineId string,
	cons constraints.Value) (instance.Instance, *instance.HardwareCharacteristics) {
	series := config.DefaultSeries
	inst, metadata, err := env.StartInstance(
		machineId,
		"fake_nonce",
		series,
		cons,
		FakeStateInfo(machineId),
		FakeAPIInfo(machineId),
	)
	c.Assert(err, IsNil)
	return inst, metadata
}

const AdminSecret = "dummy-secret"

var envConfig = `
environments:
    dummyenv:
        type: dummy
        state-server: true
        authorized-keys: 'i-am-a-key'
        admin-secret: ` + AdminSecret + `
        agent-version: %s
`

func (s *JujuConnSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *JujuConnSuite) TearDownSuite(c *C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *JujuConnSuite) SetUpTest(c *C) {
	s.oldJujuHome = config.SetJujuHome(c.MkDir())
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.setUpConn(c)
}

func (s *JujuConnSuite) TearDownTest(c *C) {
	s.tearDownConn(c)
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
	config.SetJujuHome(s.oldJujuHome)
}

// Reset returns environment state to that which existed at the start of
// the test.
func (s *JujuConnSuite) Reset(c *C) {
	s.tearDownConn(c)
	s.setUpConn(c)
}

func (s *JujuConnSuite) StateInfo(c *C) *state.Info {
	info, _, err := s.Conn.Environ.StateInfo()
	c.Assert(err, IsNil)
	info.Password = "dummy-secret"
	return info
}

func (s *JujuConnSuite) APIInfo(c *C) *api.Info {
	_, apiInfo, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, IsNil)
	apiInfo.Tag = "user-admin"
	apiInfo.Password = "dummy-secret"
	return apiInfo
}

func (s *JujuConnSuite) openAPIAs(c *C, tag, password, machineNonce string) *api.State {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, IsNil)
	info.Tag = tag
	info.Password = password
	info.MachineNonce = machineNonce
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	return st
}

// OpenAPIAs opens the API using the given identity tag
// and password for authentication.
func (s *JujuConnSuite) OpenAPIAs(c *C, tag, password string) *api.State {
	return s.openAPIAs(c, tag, password, "")
}

// OpenAPIAsMachine opens the API using the given machine tag,
// password and nonce for authentication.
func (s *JujuConnSuite) OpenAPIAsMachine(c *C, tag, password, nonce string) *api.State {
	return s.openAPIAs(c, tag, password, nonce)
}

func (s *JujuConnSuite) setUpConn(c *C) {
	if s.RootDir != "" {
		panic("JujuConnSuite.setUpConn without teardown")
	}
	s.RootDir = c.MkDir()
	s.oldHome = os.Getenv("HOME")
	home := filepath.Join(s.RootDir, "/home/ubuntu")
	err := os.MkdirAll(home, 0777)
	c.Assert(err, IsNil)
	os.Setenv("HOME", home)

	dataDir := filepath.Join(s.RootDir, "/var/lib/juju")
	err = os.MkdirAll(dataDir, 0777)
	c.Assert(err, IsNil)

	yaml := []byte(fmt.Sprintf(envConfig, version.Current.Number))
	err = ioutil.WriteFile(config.JujuHomePath("environments.yaml"), yaml, 0600)
	c.Assert(err, IsNil)

	err = ioutil.WriteFile(config.JujuHomePath("dummyenv-cert.pem"), []byte(testing.CACert), 0666)
	c.Assert(err, IsNil)

	err = ioutil.WriteFile(config.JujuHomePath("dummyenv-private-key.pem"), []byte(testing.CAKey), 0600)
	c.Assert(err, IsNil)

	environ, err := environs.NewFromName("dummyenv")
	c.Assert(err, IsNil)
	// sanity check we've got the correct environment.
	c.Assert(environ.Name(), Equals, "dummyenv")
	c.Assert(environs.Bootstrap(environ, constraints.Value{}), IsNil)

	s.BackingState = environ.(GetStater).GetStateInAPIServer()

	conn, err := juju.NewConn(environ)
	c.Assert(err, IsNil)
	s.Conn = conn
	s.State = conn.State

	apiConn, err := juju.NewAPIConn(environ, api.DialOpts{})
	c.Assert(err, IsNil)
	s.APIConn = apiConn
	s.APIState = apiConn.State
	s.environ = environ
}

type GetStater interface {
	GetStateInAPIServer() *state.State
}

func (s *JujuConnSuite) tearDownConn(c *C) {
	// Bootstrap will set the admin password, and render non-authorized use
	// impossible. s.State may still hold the right password, so try to reset
	// the password so that the MgoSuite soft-resetting works. If that fails,
	// it will still work, but it will take a while since it has to kill the
	// whole database and start over.
	if err := s.State.SetAdminMongoPassword(""); err != nil {
		c.Logf("cannot reset admin password: %v", err)
	}
	c.Assert(s.Conn.Close(), IsNil)
	c.Assert(s.APIConn.Close(), IsNil)
	dummy.Reset()
	s.Conn = nil
	s.State = nil
	os.Setenv("HOME", s.oldHome)
	s.oldHome = ""
	s.RootDir = ""
}

func (s *JujuConnSuite) DataDir() string {
	if s.RootDir == "" {
		panic("DataDir called out of test context")
	}
	return filepath.Join(s.RootDir, "/var/lib/juju")
}

// WriteConfig writes a juju config file to the "home" directory.
func (s *JujuConnSuite) WriteConfig(configData string) {
	if s.RootDir == "" {
		panic("SetUpTest has not been called; will not overwrite $JUJU_HOME/environments.yaml")
	}
	path := config.JujuHomePath("environments.yaml")
	err := ioutil.WriteFile(path, []byte(configData), 0600)
	if err != nil {
		panic(err)
	}
}

func (s *JujuConnSuite) AddTestingCharm(c *C, name string) *state.Charm {
	ch := testing.Charms.Dir(name)
	ident := fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL("local:series/" + ident)
	repo, err := charm.InferRepository(curl, testing.Charms.Path)
	c.Assert(err, IsNil)
	sch, err := s.Conn.PutCharm(curl, repo, false)
	c.Assert(err, IsNil)
	return sch
}
