// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	gitjujutesting "github.com/juju/testing"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/charm"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

// JujuConnSuite provides a freshly bootstrapped juju.Conn
// for each test. It also includes testing.BaseSuite.
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
	gitjujutesting.FakeHomeSuite
	testing.MgoSuite
	envtesting.ToolsFixture
	Conn         *juju.Conn
	State        *state.State
	APIConn      *juju.APIConn
	APIState     *api.State
	apiStates    []*api.State // additional api.States to close on teardown
	ConfigStore  configstore.Storage
	BackingState *state.State // The State being used by the API server
	RootDir      string       // The faked-up root directory.
	LogDir       string
	oldHome      string
	oldJujuHome  string
	environ      environs.Environ
	DummyConfig  testing.Attrs
}

const AdminSecret = "dummy-secret"

func (s *JujuConnSuite) SetUpSuite(c *gc.C) {
	s.FakeHomeSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *JujuConnSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.FakeHomeSuite.TearDownSuite(c)
}

func (s *JujuConnSuite) SetUpTest(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.setUpConn(c)
}

func (s *JujuConnSuite) TearDownTest(c *gc.C) {
	s.tearDownConn(c)
	s.ToolsFixture.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
	s.FakeHomeSuite.TearDownTest(c)
}

// Reset returns environment state to that which existed at the start of
// the test.
func (s *JujuConnSuite) Reset(c *gc.C) {
	s.tearDownConn(c)
	s.setUpConn(c)
}

func (s *JujuConnSuite) AddUser(c *gc.C, username string) *state.User {
	user, err := s.State.AddUser(username, "", "password")
	c.Assert(err, gc.IsNil)
	return user
}

func (s *JujuConnSuite) StateInfo(c *gc.C) *state.Info {
	info, _, err := s.Conn.Environ.StateInfo()
	c.Assert(err, gc.IsNil)
	info.Password = "dummy-secret"
	return info
}

func (s *JujuConnSuite) APIInfo(c *gc.C) *api.Info {
	_, apiInfo, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, gc.IsNil)
	apiInfo.Tag = "user-admin"
	apiInfo.Password = "dummy-secret"
	return apiInfo
}

// openAPIAs opens the API and ensures that the *api.State returned will be
// closed during the test teardown by using a cleanup function.
func (s *JujuConnSuite) openAPIAs(c *gc.C, tag, password, nonce string) *api.State {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, gc.IsNil)
	info.Tag = tag
	info.Password = password
	info.Nonce = nonce
	apiState, err := api.Open(info, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	c.Assert(apiState, gc.NotNil)
	s.apiStates = append(s.apiStates, apiState)
	return apiState
}

// OpenAPIAs opens the API using the given identity tag and password for
// authentication.  The returned *api.State should not be closed by the caller
// as a cleanup function has been registered to do that.
func (s *JujuConnSuite) OpenAPIAs(c *gc.C, tag, password string) *api.State {
	return s.openAPIAs(c, tag, password, "")
}

// OpenAPIAsMachine opens the API using the given machine tag, password and
// nonce for authentication. The returned *api.State should not be closed by
// the caller as a cleanup function has been registered to do that.
func (s *JujuConnSuite) OpenAPIAsMachine(c *gc.C, tag, password, nonce string) *api.State {
	return s.openAPIAs(c, tag, password, nonce)
}

// OpenAPIAsNewMachine creates a new machine entry that lives in system state,
// and then uses that to open the API. The returned *api.State should not be
// closed by the caller as a cleanup function has been registered to do that.
// The machine will run the supplied jobs; if none are given, JobHostUnits is assumed.
func (s *JujuConnSuite) OpenAPIAsNewMachine(c *gc.C, jobs ...state.MachineJob) (*api.State, *state.Machine) {
	if len(jobs) == 0 {
		jobs = []state.MachineJob{state.JobHostUnits}
	}
	machine, err := s.State.AddMachine("quantal", jobs...)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	return s.openAPIAs(c, machine.Tag(), password, "fake_nonce"), machine
}

func PreferredDefaultVersions(conf *config.Config, template version.Binary) []version.Binary {
	prefVersion := template
	prefVersion.Series = config.PreferredSeries(conf)
	defaultVersion := template
	defaultVersion.Series = testing.FakeDefaultSeries
	return []version.Binary{prefVersion, defaultVersion}
}

func (s *JujuConnSuite) setUpConn(c *gc.C) {
	if s.RootDir != "" {
		panic("JujuConnSuite.setUpConn without teardown")
	}
	s.RootDir = c.MkDir()
	s.oldHome = utils.Home()
	home := filepath.Join(s.RootDir, "/home/ubuntu")
	err := os.MkdirAll(home, 0777)
	c.Assert(err, gc.IsNil)
	utils.SetHome(home)
	s.oldJujuHome = osenv.SetJujuHome(filepath.Join(home, ".juju"))
	err = os.Mkdir(osenv.JujuHome(), 0777)
	c.Assert(err, gc.IsNil)

	err = os.MkdirAll(s.DataDir(), 0777)
	c.Assert(err, gc.IsNil)
	s.PatchEnvironment(osenv.JujuEnvEnvKey, "")

	// TODO(rog) remove these files and add them only when
	// the tests specifically need them (in cmd/juju for example)
	s.writeSampleConfig(c, osenv.JujuHomePath("environments.yaml"))

	err = ioutil.WriteFile(osenv.JujuHomePath("dummyenv-cert.pem"), []byte(testing.CACert), 0666)
	c.Assert(err, gc.IsNil)

	err = ioutil.WriteFile(osenv.JujuHomePath("dummyenv-private-key.pem"), []byte(testing.CAKey), 0600)
	c.Assert(err, gc.IsNil)

	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	s.ConfigStore = store

	ctx := testing.Context(c)
	environ, err := environs.PrepareFromName("dummyenv", ctx, s.ConfigStore)
	c.Assert(err, gc.IsNil)
	// sanity check we've got the correct environment.
	c.Assert(environ.Name(), gc.Equals, "dummyenv")
	s.PatchValue(&dummy.DataDir, s.DataDir())
	s.LogDir = c.MkDir()
	s.PatchValue(&dummy.LogDir, s.LogDir)

	versions := PreferredDefaultVersions(environ.Config(), version.Binary{Number: version.Current.Number, Series: "precise", Arch: "amd64"})
	versions = append(versions, version.Current)

	// Upload tools for both preferred and fake default series
	envtesting.MustUploadFakeToolsVersions(environ.Storage(), versions...)
	c.Assert(bootstrap.Bootstrap(ctx, environ, environs.BootstrapParams{}), gc.IsNil)

	s.BackingState = environ.(GetStater).GetStateInAPIServer()

	conn, err := juju.NewConn(environ)
	c.Assert(err, gc.IsNil)
	s.Conn = conn
	s.State = conn.State

	apiConn, err := juju.NewAPIConn(environ, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	s.APIConn = apiConn
	s.APIState = apiConn.State
	s.environ = environ
}

func (s *JujuConnSuite) writeSampleConfig(c *gc.C, path string) {
	if s.DummyConfig == nil {
		s.DummyConfig = dummy.SampleConfig()
	}
	attrs := s.DummyConfig.Merge(testing.Attrs{
		"admin-secret":  AdminSecret,
		"agent-version": version.Current.Number.String(),
	}).Delete("name")
	whole := map[string]interface{}{
		"environments": map[string]interface{}{
			"dummyenv": attrs,
		},
	}
	data, err := goyaml.Marshal(whole)
	c.Assert(err, gc.IsNil)
	s.WriteConfig(string(data))
}

type GetStater interface {
	GetStateInAPIServer() *state.State
}

func (s *JujuConnSuite) tearDownConn(c *gc.C) {
	serverAlive := testing.MgoServer.Addr() != ""

	// Bootstrap will set the admin password, and render non-authorized use
	// impossible. s.State may still hold the right password, so try to reset
	// the password so that the MgoSuite soft-resetting works. If that fails,
	// it will still work, but it will take a while since it has to kill the
	// whole database and start over.
	if err := s.State.SetAdminMongoPassword(""); err != nil && serverAlive {
		c.Logf("cannot reset admin password: %v", err)
	}
	for _, st := range s.apiStates {
		err := st.Close()
		if serverAlive {
			c.Assert(err, gc.IsNil)
		}
	}
	err := s.Conn.Close()
	if serverAlive {
		c.Assert(err, gc.IsNil)
	}
	err = s.APIConn.Close()
	if serverAlive {
		c.Assert(err, gc.IsNil)
	}
	dummy.Reset()
	s.apiStates = nil
	s.Conn = nil
	s.State = nil
	utils.SetHome(s.oldHome)
	osenv.SetJujuHome(s.oldJujuHome)
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
	path := osenv.JujuHomePath("environments.yaml")
	err := ioutil.WriteFile(path, []byte(configData), 0600)
	if err != nil {
		panic(err)
	}
}

func (s *JujuConnSuite) AddTestingCharm(c *gc.C, name string) *state.Charm {
	ch := testing.Charms.Dir(name)
	ident := fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL("local:quantal/" + ident)
	repo, err := charm.InferRepository(curl.Reference, testing.Charms.Path())
	c.Assert(err, gc.IsNil)
	sch, err := s.Conn.PutCharm(curl, repo, false)
	c.Assert(err, gc.IsNil)
	return sch
}

func (s *JujuConnSuite) AddTestingService(c *gc.C, name string, ch *state.Charm) *state.Service {
	return s.AddTestingServiceWithNetworks(c, name, ch, nil)
}

func (s *JujuConnSuite) AddTestingServiceWithNetworks(c *gc.C, name string, ch *state.Charm, networks []string) *state.Service {
	c.Assert(s.State, gc.NotNil)
	service, err := s.State.AddService(name, "user-admin", ch, networks)
	c.Assert(err, gc.IsNil)
	return service
}

func (s *JujuConnSuite) AgentConfigForTag(c *gc.C, tag string) agent.ConfigSetter {
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	config, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			DataDir:           s.DataDir(),
			Tag:               tag,
			UpgradedToVersion: version.Current.Number,
			Password:          password,
			Nonce:             "nonce",
			StateAddresses:    s.StateInfo(c).Addrs,
			APIAddresses:      s.APIInfo(c).Addrs,
			CACert:            testing.CACert,
		})
	c.Assert(err, gc.IsNil)
	return config
}
