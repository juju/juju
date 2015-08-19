// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/charmrepo"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	statestorage "github.com/juju/juju/state/storage"
	"github.com/juju/juju/state/toolstorage"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
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
	gitjujutesting.MgoSuite
	testing.FakeJujuHomeSuite
	envtesting.ToolsFixture

	DefaultToolsStorageDir string
	DefaultToolsStorage    storage.Storage

	State        *state.State
	Environ      environs.Environ
	APIState     api.Connection
	apiStates    []api.Connection // additional api.Connections to close on teardown
	ConfigStore  configstore.Storage
	BackingState *state.State // The State being used by the API server
	RootDir      string       // The faked-up root directory.
	LogDir       string
	oldHome      string
	oldJujuHome  string
	DummyConfig  testing.Attrs
	Factory      *factory.Factory
}

const AdminSecret = "dummy-secret"

func (s *JujuConnSuite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.FakeJujuHomeSuite.SetUpSuite(c)
}

func (s *JujuConnSuite) TearDownSuite(c *gc.C) {
	s.FakeJujuHomeSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *JujuConnSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(&configstore.DefaultAdminUsername, dummy.AdminUserTag().Name())
	s.setUpConn(c)
	s.Factory = factory.NewFactory(s.State)
}

func (s *JujuConnSuite) TearDownTest(c *gc.C) {
	s.tearDownConn(c)
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuHomeSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

// Reset returns environment state to that which existed at the start of
// the test.
func (s *JujuConnSuite) Reset(c *gc.C) {
	s.tearDownConn(c)
	s.setUpConn(c)
}

func (s *JujuConnSuite) AdminUserTag(c *gc.C) names.UserTag {
	env, err := s.State.StateServerEnvironment()
	c.Assert(err, jc.ErrorIsNil)
	return env.Owner()
}

func (s *JujuConnSuite) MongoInfo(c *gc.C) *mongo.MongoInfo {
	info := s.State.MongoConnectionInfo()
	info.Password = "dummy-secret"
	return info
}

func (s *JujuConnSuite) APIInfo(c *gc.C) *api.Info {
	apiInfo, err := environs.APIInfo(s.Environ)
	c.Assert(err, jc.ErrorIsNil)
	apiInfo.Tag = s.AdminUserTag(c)
	apiInfo.Password = "dummy-secret"
	apiInfo.EnvironTag = s.State.EnvironTag()
	return apiInfo
}

// openAPIAs opens the API and ensures that the api.Connection returned will be
// closed during the test teardown by using a cleanup function.
func (s *JujuConnSuite) openAPIAs(c *gc.C, tag names.Tag, password, nonce string) api.Connection {
	apiInfo := s.APIInfo(c)
	apiInfo.Tag = tag
	apiInfo.Password = password
	apiInfo.Nonce = nonce
	apiState, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiState, gc.NotNil)
	s.apiStates = append(s.apiStates, apiState)
	return apiState
}

// OpenAPIAs opens the API using the given identity tag and password for
// authentication.  The returned api.Connection should not be closed by the caller
// as a cleanup function has been registered to do that.
func (s *JujuConnSuite) OpenAPIAs(c *gc.C, tag names.Tag, password string) api.Connection {
	return s.openAPIAs(c, tag, password, "")
}

// OpenAPIAsMachine opens the API using the given machine tag, password and
// nonce for authentication. The returned api.Connection should not be closed by
// the caller as a cleanup function has been registered to do that.
func (s *JujuConnSuite) OpenAPIAsMachine(c *gc.C, tag names.Tag, password, nonce string) api.Connection {
	return s.openAPIAs(c, tag, password, nonce)
}

// OpenAPIAsNewMachine creates a new machine entry that lives in system state,
// and then uses that to open the API. The returned api.Connection should not be
// closed by the caller as a cleanup function has been registered to do that.
// The machine will run the supplied jobs; if none are given, JobHostUnits is assumed.
func (s *JujuConnSuite) OpenAPIAsNewMachine(c *gc.C, jobs ...state.MachineJob) (api.Connection, *state.Machine) {
	if len(jobs) == 0 {
		jobs = []state.MachineJob{state.JobHostUnits}
	}
	machine, err := s.State.AddMachine("quantal", jobs...)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	return s.openAPIAs(c, machine.Tag(), password, "fake_nonce"), machine
}

func PreferredDefaultVersions(conf *config.Config, template version.Binary) []version.Binary {
	prefVersion := template
	prefVersion.Series = config.PreferredSeries(conf)
	defaultVersion := template
	if prefVersion.Series != testing.FakeDefaultSeries {
		defaultVersion.Series = testing.FakeDefaultSeries
	}
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
	c.Assert(err, jc.ErrorIsNil)
	utils.SetHome(home)
	s.oldJujuHome = osenv.SetJujuHome(filepath.Join(home, ".juju"))
	err = os.Mkdir(osenv.JujuHome(), 0777)
	c.Assert(err, jc.ErrorIsNil)

	err = os.MkdirAll(s.DataDir(), 0777)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchEnvironment(osenv.JujuEnvEnvKey, "")

	// TODO(rog) remove these files and add them only when
	// the tests specifically need them (in cmd/juju for example)
	s.writeSampleConfig(c, osenv.JujuHomePath("environments.yaml"))

	err = ioutil.WriteFile(osenv.JujuHomePath("dummyenv-cert.pem"), []byte(testing.CACert), 0666)
	c.Assert(err, jc.ErrorIsNil)

	err = ioutil.WriteFile(osenv.JujuHomePath("dummyenv-private-key.pem"), []byte(testing.CAKey), 0600)
	c.Assert(err, jc.ErrorIsNil)

	store, err := configstore.Default()
	c.Assert(err, jc.ErrorIsNil)
	s.ConfigStore = store

	ctx := testing.Context(c)
	environ, err := environs.PrepareFromName("dummyenv", envcmd.BootstrapContext(ctx), s.ConfigStore)
	c.Assert(err, jc.ErrorIsNil)
	// sanity check we've got the correct environment.
	c.Assert(environ.Config().Name(), gc.Equals, "dummyenv")
	s.PatchValue(&dummy.DataDir, s.DataDir())
	s.LogDir = c.MkDir()
	s.PatchValue(&dummy.LogDir, s.LogDir)

	versions := PreferredDefaultVersions(environ.Config(), version.Binary{Number: version.Current.Number, Series: "precise", Arch: "amd64"})
	versions = append(versions, version.Current)

	// Upload tools for both preferred and fake default series
	s.DefaultToolsStorageDir = c.MkDir()
	s.PatchValue(&tools.DefaultBaseURL, s.DefaultToolsStorageDir)
	stor, err := filestorage.NewFileStorageWriter(s.DefaultToolsStorageDir)
	c.Assert(err, jc.ErrorIsNil)
	// Upload tools to both release and devel streams since config will dictate that we
	// end up looking in both places.
	envtesting.AssertUploadFakeToolsVersions(c, stor, "released", "released", versions...)
	envtesting.AssertUploadFakeToolsVersions(c, stor, "devel", "devel", versions...)
	s.DefaultToolsStorage = stor

	err = bootstrap.Bootstrap(envcmd.BootstrapContext(ctx), environ, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	s.BackingState = environ.(GetStater).GetStateInAPIServer()

	s.State, err = newState(environ, s.BackingState.MongoConnectionInfo())
	c.Assert(err, jc.ErrorIsNil)

	s.APIState, err = juju.NewAPIState(s.AdminUserTag(c), environ, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SetAPIHostPorts(s.APIState.APIHostPorts())
	c.Assert(err, jc.ErrorIsNil)

	// Make sure the config store has the api endpoint address set
	info, err := s.ConfigStore.ReadInfo("dummyenv")
	c.Assert(err, jc.ErrorIsNil)
	endpoint := info.APIEndpoint()
	endpoint.Addresses = []string{s.APIState.APIHostPorts()[0][0].String()}
	info.SetAPIEndpoint(endpoint)
	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)

	// Make sure the jenv file has the local host ports.
	c.Logf("jenv host ports: %#v", s.APIState.APIHostPorts())

	s.Environ = environ

	// Insert expected values...
	servingInfo := state.StateServingInfo{
		PrivateKey:   testing.ServerKey,
		Cert:         testing.ServerCert,
		CAPrivateKey: testing.CAKey,
		SharedSecret: "really, really secret",
		APIPort:      4321,
		StatePort:    1234,
	}
	s.State.SetStateServingInfo(servingInfo)
}

// AddToolsToState adds tools to tools storage.
func (s *JujuConnSuite) AddToolsToState(c *gc.C, versions ...version.Binary) {
	stor, err := s.State.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer stor.Close()
	for _, v := range versions {
		content := v.String()
		hash := fmt.Sprintf("sha256(%s)", content)
		err := stor.AddTools(strings.NewReader(content), toolstorage.Metadata{
			Version: v,
			Size:    int64(len(content)),
			SHA256:  hash,
		})
		c.Assert(err, jc.ErrorIsNil)
	}
}

// AddDefaultToolsToState adds tools to tools storage for
// {Number: version.Current.Number, Arch: amd64}, for the
// "precise" series and the environment's preferred series.
// The preferred series is default-series if specified,
// otherwise the latest LTS.
func (s *JujuConnSuite) AddDefaultToolsToState(c *gc.C) {
	preferredVersion := version.Current
	preferredVersion.Arch = "amd64"
	versions := PreferredDefaultVersions(s.Environ.Config(), preferredVersion)
	versions = append(versions, version.Current)
	s.AddToolsToState(c, versions...)
}

var redialStrategy = utils.AttemptStrategy{
	Total: 60 * time.Second,
	Delay: 250 * time.Millisecond,
}

// newState returns a new State that uses the given environment.
// The environment must have already been bootstrapped.
func newState(environ environs.Environ, mongoInfo *mongo.MongoInfo) (*state.State, error) {
	config := environ.Config()
	password := config.AdminSecret()
	if password == "" {
		return nil, fmt.Errorf("cannot connect without admin-secret")
	}
	environUUID, ok := config.UUID()
	if !ok {
		return nil, fmt.Errorf("cannot connect without environment UUID")
	}
	environTag := names.NewEnvironTag(environUUID)

	mongoInfo.Password = password
	opts := mongo.DefaultDialOpts()
	st, err := state.Open(environTag, mongoInfo, opts, environs.NewStatePolicy())
	if errors.IsUnauthorized(errors.Cause(err)) {
		// We try for a while because we might succeed in
		// connecting to mongo before the state has been
		// initialized and the initial password set.
		for a := redialStrategy.Start(); a.Next(); {
			st, err = state.Open(environTag, mongoInfo, opts, environs.NewStatePolicy())
			if !errors.IsUnauthorized(errors.Cause(err)) {
				break
			}
		}
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if err := updateSecrets(environ, st); err != nil {
		st.Close()
		return nil, fmt.Errorf("unable to push secrets: %v", err)
	}
	return st, nil
}

func updateSecrets(env environs.Environ, st *state.State) error {
	secrets, err := env.Provider().SecretAttrs(env.Config())
	if err != nil {
		return err
	}
	cfg, err := st.EnvironConfig()
	if err != nil {
		return err
	}
	secretAttrs := make(map[string]interface{})
	attrs := cfg.AllAttrs()
	for k, v := range secrets {
		if _, exists := attrs[k]; exists {
			// Environment already has secrets. Won't send again.
			return nil
		} else {
			secretAttrs[k] = v
		}
	}
	return st.UpdateEnvironConfig(secretAttrs, nil, nil)
}

// PutCharm uploads the given charm to provider storage, and adds a
// state.Charm to the state.  The charm is not uploaded if a charm with
// the same URL already exists in the state.
// If bumpRevision is true, the charm must be a local directory,
// and the revision number will be incremented before pushing.
func PutCharm(st *state.State, curl *charm.URL, repo charmrepo.Interface, bumpRevision bool) (*state.Charm, error) {
	if curl.Revision == -1 {
		rev, err := charmrepo.Latest(repo, curl)
		if err != nil {
			return nil, fmt.Errorf("cannot get latest charm revision: %v", err)
		}
		curl = curl.WithRevision(rev)
	}
	ch, err := repo.Get(curl)
	if err != nil {
		return nil, fmt.Errorf("cannot get charm: %v", err)
	}
	if bumpRevision {
		chd, ok := ch.(*charm.CharmDir)
		if !ok {
			return nil, fmt.Errorf("cannot increment revision of charm %q: not a directory", curl)
		}
		if err = chd.SetDiskRevision(chd.Revision() + 1); err != nil {
			return nil, fmt.Errorf("cannot increment revision of charm %q: %v", curl, err)
		}
		curl = curl.WithRevision(chd.Revision())
	}
	if sch, err := st.Charm(curl); err == nil {
		return sch, nil
	}
	return addCharm(st, curl, ch)
}

func addCharm(st *state.State, curl *charm.URL, ch charm.Charm) (*state.Charm, error) {
	var f *os.File
	name := charm.Quote(curl.String())
	switch ch := ch.(type) {
	case *charm.CharmDir:
		var err error
		if f, err = ioutil.TempFile("", name); err != nil {
			return nil, err
		}
		defer os.Remove(f.Name())
		defer f.Close()
		err = ch.ArchiveTo(f)
		if err != nil {
			return nil, fmt.Errorf("cannot bundle charm: %v", err)
		}
		if _, err := f.Seek(0, 0); err != nil {
			return nil, err
		}
	case *charm.CharmArchive:
		var err error
		if f, err = os.Open(ch.Path); err != nil {
			return nil, fmt.Errorf("cannot read charm bundle: %v", err)
		}
		defer f.Close()
	default:
		return nil, fmt.Errorf("unknown charm type %T", ch)
	}
	digest, size, err := utils.ReadSHA256(f)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}

	stor := statestorage.NewStorage(st.EnvironUUID(), st.MongoSession())
	storagePath := fmt.Sprintf("/charms/%s-%s", curl.String(), digest)
	if err := stor.Put(storagePath, f, size); err != nil {
		return nil, fmt.Errorf("cannot put charm: %v", err)
	}
	sch, err := st.AddCharm(ch, curl, storagePath, digest)
	if err != nil {
		return nil, fmt.Errorf("cannot add charm: %v", err)
	}
	return sch, nil
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
	c.Assert(err, jc.ErrorIsNil)
	s.WriteConfig(string(data))
}

type GetStater interface {
	GetStateInAPIServer() *state.State
}

func (s *JujuConnSuite) tearDownConn(c *gc.C) {
	testServer := gitjujutesting.MgoServer.Addr()
	serverAlive := testServer != ""

	// Close any api connections we know about first.
	for _, st := range s.apiStates {
		err := st.Close()
		if serverAlive {
			c.Check(err, jc.ErrorIsNil)
		}
	}
	s.apiStates = nil
	if s.APIState != nil {
		err := s.APIState.Close()
		s.APIState = nil
		if serverAlive {
			c.Check(err, gc.IsNil,
				gc.Commentf("closing api state failed\n%s\n", errors.ErrorStack(err)),
			)
		}
	}
	// Close state.
	if s.State != nil {
		err := s.State.Close()
		if serverAlive {
			// This happens way too often with failing tests,
			// so add some context in case of an error.
			c.Check(err, gc.IsNil,
				gc.Commentf("closing state failed\n%s\n", errors.ErrorStack(err)),
			)
		}
		s.State = nil
	}

	dummy.Reset()
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

func (s *JujuConnSuite) ConfDir() string {
	if s.RootDir == "" {
		panic("DataDir called out of test context")
	}
	return filepath.Join(s.RootDir, "/etc/juju")
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
	ch := testcharms.Repo.CharmDir(name)
	ident := fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL("local:quantal/" + ident)
	repo, err := charmrepo.InferRepository(
		curl.Reference(),
		charmrepo.NewCharmStoreParams{},
		testcharms.Repo.Path())
	c.Assert(err, jc.ErrorIsNil)
	sch, err := PutCharm(s.State, curl, repo, false)
	c.Assert(err, jc.ErrorIsNil)
	return sch
}

func (s *JujuConnSuite) AddTestingService(c *gc.C, name string, ch *state.Charm) *state.Service {
	return s.AddTestingServiceWithNetworks(c, name, ch, nil)
}

func (s *JujuConnSuite) AddTestingServiceWithStorage(c *gc.C, name string, ch *state.Charm, storage map[string]state.StorageConstraints) *state.Service {
	owner := s.AdminUserTag(c).String()
	service, err := s.State.AddService(name, owner, ch, nil, storage)
	c.Assert(err, jc.ErrorIsNil)
	return service
}

func (s *JujuConnSuite) AddTestingServiceWithNetworks(c *gc.C, name string, ch *state.Charm, networks []string) *state.Service {
	c.Assert(s.State, gc.NotNil)
	owner := s.AdminUserTag(c).String()
	service, err := s.State.AddService(name, owner, ch, networks, nil)
	c.Assert(err, jc.ErrorIsNil)
	return service
}

func (s *JujuConnSuite) AgentConfigForTag(c *gc.C, tag names.Tag) agent.ConfigSetter {
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	config, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			DataDir:           s.DataDir(),
			Tag:               tag,
			UpgradedToVersion: version.Current.Number,
			Password:          password,
			Nonce:             "nonce",
			StateAddresses:    s.MongoInfo(c).Addrs,
			APIAddresses:      s.APIInfo(c).Addrs,
			CACert:            testing.CACert,
			Environment:       s.State.EnvironTag(),
		})
	c.Assert(err, jc.ErrorIsNil)
	return config
}

// AssertConfigParameterUpdated updates environment parameter and
// asserts that no errors were encountered
func (s *JujuConnSuite) AssertConfigParameterUpdated(c *gc.C, key string, value interface{}) {
	err := s.BackingState.UpdateEnvironConfig(map[string]interface{}{key: value}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}
