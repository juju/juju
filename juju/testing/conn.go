// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/os/series"
	"github.com/juju/pubsub"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/filestorage"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/state/stateenvirons"
	statestorage "github.com/juju/juju/state/storage"
	statetesting "github.com/juju/juju/state/testing"
	statewatcher "github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
)

const ControllerName = "kontroll"

// JujuConnSuite provides a freshly bootstrapped juju.Conn
// for each test. It also includes testing.BaseSuite.
//
// It also sets up RootDir to point to a directory hierarchy
// mirroring the intended juju directory structure, including
// the following:
//     RootDir/var/lib/juju
//         An empty directory returned as DataDir - the
//         root of the juju data storage space.
// $HOME is set to point to RootDir/home/ubuntu.
type JujuConnSuite struct {
	// ConfigAttrs can be set up before SetUpTest
	// is invoked. Any attributes set here will be
	// added to the suite's environment configuration.
	ConfigAttrs map[string]interface{}

	// ControllerConfigAttrs can be set up before SetUpTest
	// is invoked. Any attributes set here will be added to
	// the suite's controller configuration.
	ControllerConfigAttrs map[string]interface{}

	// TODO: JujuConnSuite should not be concerned both with JUJU_DATA and with
	// /var/lib/juju: the use cases are completely non-overlapping, and any tests that
	// really do need both to exist ought to be embedding distinct fixtures for the
	// distinct environments.
	gitjujutesting.MgoSuite
	testing.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture

	DefaultToolsStorageDir string
	DefaultToolsStorage    storage.Storage

	ControllerConfig    controller.Config
	State               *state.State
	StatePool           *state.StatePool
	Model               *state.Model
	Environ             environs.Environ
	APIState            api.Connection
	apiStates           []api.Connection // additional api.Connections to close on teardown
	ControllerStore     jujuclient.ClientStore
	BackingState        *state.State          // The State being used by the API server
	BackingStatePool    *state.StatePool      // The StatePool being used by the API server
	Hub                 *pubsub.StructuredHub // The central hub being used by the API server.
	LeaseManager        lease.Manager         // The lease manager being used by the API server.
	RootDir             string                // The faked-up root directory.
	LogDir              string
	oldHome             string
	oldJujuXDGDataHome  string
	DummyConfig         testing.Attrs
	Factory             *factory.Factory
	ProviderCallContext context.ProviderCallContext

	txnSyncNotify     chan struct{}
	modelWatcherIdle  chan string
	modelWatcherMutex *sync.Mutex
}

const AdminSecret = "dummy-secret"

func (s *JujuConnSuite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	s.PatchValue(&utils.OutgoingAccessAllowed, false)
	s.PatchValue(&cert.NewCA, testing.NewCA)
	s.PatchValue(&cert.NewLeafKeyBits, 512)
}

func (s *JujuConnSuite) TearDownSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *JujuConnSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	s.txnSyncNotify = make(chan struct{})
	s.modelWatcherIdle = nil
	s.modelWatcherMutex = &sync.Mutex{}
	s.PatchValue(&statewatcher.TxnPollNotifyFunc, s.txnNotifyFunc)
	s.PatchValue(&statewatcher.HubWatcherIdleFunc, s.hubWatcherIdleFunc)
	s.setUpConn(c)
	s.Factory = factory.NewFactory(s.State, s.StatePool)
}

func (s *JujuConnSuite) TearDownTest(c *gc.C) {
	s.tearDownConn(c)
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

// Reset returns environment state to that which existed at the start of
// the test.
func (s *JujuConnSuite) Reset(c *gc.C) {
	s.tearDownConn(c)
	s.setUpConn(c)
}

func (s *JujuConnSuite) txnNotifyFunc() {
	select {
	case s.txnSyncNotify <- struct{}{}:
		// Try to send something down the channel.
	default:
		// However don't get stressed if noone is listening.
	}
}

func (s *JujuConnSuite) hubWatcherIdleFunc(modelUUID string) {
	s.modelWatcherMutex.Lock()
	idleChan := s.modelWatcherIdle
	s.modelWatcherMutex.Unlock()
	if idleChan == nil {
		return
	}
	idleChan <- modelUUID
}

func (s *JujuConnSuite) WaitForNextSync(c *gc.C) {
	select {
	case <-s.txnSyncNotify:
	case <-time.After(gitjujutesting.LongWait):
		c.Fatal("no sync event sent, is the watcher dead?")
	}
	// It is possible that the previous sync was in progress
	// while we were waiting, so wait for a second sync to make sure
	// that the changes in the test goroutine have been processed by
	// the txnwatcher.
	select {
	case <-s.txnSyncNotify:
	case <-time.After(gitjujutesting.LongWait):
		c.Fatal("no sync event sent, is the watcher dead?")
	}
}

func (s *JujuConnSuite) WaitForModelWatchersIdle(c *gc.C, modelUUID string) {
	c.Logf("waiting for model %s to be idle", modelUUID)
	s.WaitForNextSync(c)
	s.modelWatcherMutex.Lock()
	idleChan := make(chan string)
	s.modelWatcherIdle = idleChan
	s.modelWatcherMutex.Unlock()

	defer func() {
		s.modelWatcherMutex.Lock()
		s.modelWatcherIdle = nil
		s.modelWatcherMutex.Unlock()
		// Clear out any pending events.
		for {
			select {
			case <-idleChan:
			default:
				return
			}
		}
	}()

	timeout := time.After(gitjujutesting.LongWait)
	for {
		select {
		case uuid := <-idleChan:
			if uuid == modelUUID {
				return
			} else {
				c.Logf("model %s is idle", uuid)
			}
		case <-timeout:
			c.Fatal("no sync event sent, is the watcher dead?")
		}
	}
}

func (s *JujuConnSuite) AdminUserTag(c *gc.C) names.UserTag {
	owner, err := s.State.ControllerOwner()
	c.Assert(err, jc.ErrorIsNil)
	return owner
}

func (s *JujuConnSuite) MongoInfo(c *gc.C) *mongo.MongoInfo {
	info := statetesting.NewMongoInfo()
	info.Password = AdminSecret
	return info
}

func (s *JujuConnSuite) APIInfo(c *gc.C) *api.Info {
	apiInfo, err := environs.APIInfo(s.ProviderCallContext, s.ControllerConfig.ControllerUUID(), testing.ModelTag.Id(), testing.CACert, s.ControllerConfig.APIPort(), s.Environ)
	c.Assert(err, jc.ErrorIsNil)
	apiInfo.Tag = s.AdminUserTag(c)
	apiInfo.Password = "dummy-secret"
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	apiInfo.ModelTag = model.ModelTag()
	return apiInfo
}

// openAPIAs opens the API and ensures that the api.Connection returned will be
// closed during the test teardown by using a cleanup function.
func (s *JujuConnSuite) openAPIAs(c *gc.C, tag names.Tag, password, nonce string, controllerOnly bool) api.Connection {
	apiInfo := s.APIInfo(c)
	apiInfo.Tag = tag
	apiInfo.Password = password
	apiInfo.Nonce = nonce
	if controllerOnly {
		apiInfo.ModelTag = names.ModelTag{}
	}
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
	return s.openAPIAs(c, tag, password, "", false)
}

func (s *JujuConnSuite) OpenControllerAPIAs(c *gc.C, tag names.Tag, password string) api.Connection {
	return s.openAPIAs(c, tag, password, "", true)
}

// OpenAPIAsMachine opens the API using the given machine tag, password and
// nonce for authentication. The returned api.Connection should not be closed by
// the caller as a cleanup function has been registered to do that.
func (s *JujuConnSuite) OpenAPIAsMachine(c *gc.C, tag names.Tag, password, nonce string) api.Connection {
	return s.openAPIAs(c, tag, password, nonce, false)
}

func (s *JujuConnSuite) OpenControllerAPI(c *gc.C) api.Connection {
	return s.OpenControllerAPIAs(c, s.AdminUserTag(c), AdminSecret)
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
	return s.openAPIAs(c, machine.Tag(), password, "fake_nonce", false), machine
}

// DefaultVersions returns a slice of unique 'versions' for the current
// environment's preferred series and host architecture, as well supported LTS
// series for the host architecture. Additionally, it ensures that 'versions'
// for amd64 are returned if that is not the current host's architecture.
func DefaultVersions(conf *config.Config) []version.Binary {
	var versions []version.Binary
	supported := series.SupportedLts()
	defaultSeries := set.NewStrings(supported...)
	defaultSeries.Add(config.PreferredSeries(conf))
	defaultSeries.Add(series.MustHostSeries())
	agentVersion, set := conf.AgentVersion()
	if !set {
		agentVersion = jujuversion.Current
	}
	for _, s := range defaultSeries.Values() {
		versions = append(versions, version.Binary{
			Number: agentVersion,
			Arch:   arch.HostArch(),
			Series: s,
		})
		if arch.HostArch() != "amd64" {
			versions = append(versions, version.Binary{
				Number: agentVersion,
				Arch:   "amd64",
				Series: s,
			})

		}
	}
	return versions
}

// UserHomeParams stores parameters with which to create an os user home dir.
type UserHomeParams struct {
	// The username of the operating system user whose fake home
	// directory is to be created.
	Username string

	// Override the default osenv.JujuModelEnvKey.
	ModelEnvKey string

	// Should the oldJujuXDGDataHome field be set?
	// This is likely only true during setUpConn, as we want teardown to
	// reset to the most original value.
	SetOldHome bool
}

// Create a home directory and Juju data home for user username.
// This is used by setUpConn to create the 'ubuntu' user home, after RootDir,
// and may be used again later for other users.
func (s *JujuConnSuite) CreateUserHome(c *gc.C, params *UserHomeParams) {
	if s.RootDir == "" {
		c.Fatal("JujuConnSuite.setUpConn required first for RootDir")
	}
	c.Assert(params.Username, gc.Not(gc.Equals), "")
	home := filepath.Join(s.RootDir, "home", params.Username)
	err := os.MkdirAll(home, 0777)
	c.Assert(err, jc.ErrorIsNil)
	err = utils.SetHome(home)
	c.Assert(err, jc.ErrorIsNil)

	jujuHome := filepath.Join(home, ".local", "share")
	err = os.MkdirAll(filepath.Join(home, ".local", "share"), 0777)
	c.Assert(err, jc.ErrorIsNil)

	previousJujuXDGDataHome := osenv.SetJujuXDGDataHome(jujuHome)
	if params.SetOldHome {
		s.oldJujuXDGDataHome = previousJujuXDGDataHome
	}

	err = os.MkdirAll(s.DataDir(), 0777)
	c.Assert(err, jc.ErrorIsNil)

	jujuModelEnvKey := "JUJU_MODEL"
	if params.ModelEnvKey != "" {
		jujuModelEnvKey = params.ModelEnvKey
	}
	s.PatchEnvironment(osenv.JujuModelEnvKey, jujuModelEnvKey)

	s.ControllerStore = jujuclient.NewFileClientStore()
}

func (s *JujuConnSuite) setUpConn(c *gc.C) {
	if s.RootDir != "" {
		c.Fatal("JujuConnSuite.setUpConn without teardown")
	}
	s.RootDir = c.MkDir()
	s.oldHome = utils.Home()
	userHomeParams := UserHomeParams{
		Username:    "ubuntu",
		ModelEnvKey: "controller",
		SetOldHome:  true,
	}
	s.CreateUserHome(c, &userHomeParams)

	cfg, err := config.New(config.UseDefaults, (map[string]interface{})(s.sampleConfig()))
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	s.ControllerConfig = testing.FakeControllerConfig()
	for key, value := range s.ControllerConfigAttrs {
		s.ControllerConfig[key] = value
	}
	cloudSpec := dummy.SampleCloudSpec()
	bootstrapEnviron, err := bootstrap.PrepareController(
		false,
		modelcmd.BootstrapContext(ctx),
		s.ControllerStore,
		bootstrap.PrepareParams{
			ControllerConfig: s.ControllerConfig,
			ModelConfig:      cfg.AllAttrs(),
			Cloud:            cloudSpec,
			ControllerName:   ControllerName,
			AdminSecret:      AdminSecret,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	environ := bootstrapEnviron.(environs.Environ)
	// sanity check we've got the correct environment.
	c.Assert(environ.Config().Name(), gc.Equals, "controller")
	s.PatchValue(&dummy.DataDir, s.DataDir())
	s.LogDir = c.MkDir()
	s.PatchValue(&dummy.LogDir, s.LogDir)

	versions := DefaultVersions(environ.Config())

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

	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	// Dummy provider uses a random port, which is added to cfg used to create environment.
	apiPort := dummy.APIPort(environ.Provider())
	s.ControllerConfig["api-port"] = apiPort
	s.ProviderCallContext = context.NewCloudCallContext()
	err = bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), environ, s.ProviderCallContext, bootstrap.BootstrapParams{
		ControllerConfig: s.ControllerConfig,
		CloudRegion:      "dummy-region",
		Cloud: cloud.Cloud{
			Name:             cloudSpec.Name,
			Type:             cloudSpec.Type,
			AuthTypes:        []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
			Endpoint:         cloudSpec.Endpoint,
			IdentityEndpoint: cloudSpec.IdentityEndpoint,
			StorageEndpoint:  cloudSpec.StorageEndpoint,
			Regions: []cloud.Region{
				{
					Name:             "dummy-region",
					Endpoint:         "dummy-endpoint",
					IdentityEndpoint: "dummy-identity-endpoint",
					StorageEndpoint:  "dummy-storage-endpoint",
				},
			},
		},
		CloudCredential:     cloudSpec.Credential,
		CloudCredentialName: "cred",
		AdminSecret:         AdminSecret,
		CAPrivateKey:        testing.CAKey,
	})
	c.Assert(err, jc.ErrorIsNil)

	getStater := environ.(GetStater)
	s.BackingState = getStater.GetStateInAPIServer()
	s.BackingStatePool = getStater.GetStatePoolInAPIServer()
	s.Hub = getStater.GetHubInAPIServer()
	s.LeaseManager = getStater.GetLeaseManagerInAPIServer()

	s.StatePool, err = newState(s.ControllerConfig.ControllerUUID(), environ, s.MongoInfo(c))
	c.Assert(err, jc.ErrorIsNil)
	s.State = s.StatePool.SystemState()

	s.Model, err = s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	apiInfo, err := environs.APIInfo(s.ProviderCallContext, s.ControllerConfig.ControllerUUID(), testing.ModelTag.Id(), testing.CACert, s.ControllerConfig.APIPort(), environ)
	c.Assert(err, jc.ErrorIsNil)
	apiInfo.Tag = s.AdminUserTag(c)
	apiInfo.Password = AdminSecret
	s.APIState, err = api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SetAPIHostPorts(s.APIState.APIHostPorts())
	c.Assert(err, jc.ErrorIsNil)

	// Make sure the controller store has the controller api endpoint address set
	controller, err := s.ControllerStore.ControllerByName(ControllerName)
	c.Assert(err, jc.ErrorIsNil)
	controller.APIEndpoints = []string{s.APIState.APIHostPorts()[0][0].String()}
	err = s.ControllerStore.UpdateController(ControllerName, *controller)
	c.Assert(err, jc.ErrorIsNil)
	err = s.ControllerStore.SetCurrentController(ControllerName)
	c.Assert(err, jc.ErrorIsNil)

	s.Environ = environ

	// Insert expected values...
	servingInfo := state.StateServingInfo{
		PrivateKey:   testing.ServerKey,
		Cert:         testing.ServerCert,
		CAPrivateKey: testing.CAKey,
		SharedSecret: "really, really secret",
		APIPort:      s.ControllerConfig.APIPort(),
		StatePort:    s.ControllerConfig.StatePort(),
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
		err := stor.Add(strings.NewReader(content), binarystorage.Metadata{
			Version: v.String(),
			Size:    int64(len(content)),
			SHA256:  hash,
		})
		c.Assert(err, jc.ErrorIsNil)
	}
}

// AddDefaultTools adds tools to tools storage for default juju
// series and architectures.
func (s *JujuConnSuite) AddDefaultToolsToState(c *gc.C) {
	versions := DefaultVersions(s.Environ.Config())
	s.AddToolsToState(c, versions...)
}

// TODO(katco): 2016-08-09: lp:1611427
var redialStrategy = utils.AttemptStrategy{
	Total: 60 * time.Second,
	Delay: 250 * time.Millisecond,
}

// newState returns a new State that uses the given environment.
// The environment must have already been bootstrapped.
func newState(controllerUUID string, environ environs.Environ, mongoInfo *mongo.MongoInfo) (*state.StatePool, error) {
	if controllerUUID == "" {
		return nil, errors.New("missing controller UUID")
	}
	config := environ.Config()
	modelTag := names.NewModelTag(config.UUID())

	mongoInfo.Password = AdminSecret
	opts := mongotest.DialOpts()
	session, err := mongo.DialWithInfo(*mongoInfo, opts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer session.Close()

	newPolicyFunc := stateenvirons.GetNewPolicyFunc()
	controllerTag := names.NewControllerTag(controllerUUID)
	args := state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      controllerTag,
		ControllerModelTag: modelTag,
		MongoSession:       session,
		NewPolicy:          newPolicyFunc,
	}
	pool, err := state.OpenStatePool(args)
	if errors.IsUnauthorized(errors.Cause(err)) {
		// We try for a while because we might succeed in
		// connecting to mongo before the state has been
		// initialized and the initial password set.
		for a := redialStrategy.Start(); a.Next(); {
			pool, err = state.OpenStatePool(args)
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
	return pool, nil
}

// PutCharm uploads the given charm to provider storage, and adds a
// state.Charm to the state.  The charm is not uploaded if a charm with
// the same URL already exists in the state.
// If bumpRevision is true, the charm must be a local directory,
// and the revision number will be incremented before pushing.
func PutCharm(st *state.State, curl *charm.URL, repo charmrepo.Interface, bumpRevision, force bool) (*state.Charm, error) {
	if curl.Revision == -1 {
		var err error
		curl, _, err = repo.Resolve(curl)
		if err != nil {
			return nil, fmt.Errorf("cannot get latest charm revision: %v", err)
		}
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
	return AddCharm(st, curl, ch, force)
}

// AddCharm adds the charm to state and storage.
func AddCharm(st *state.State, curl *charm.URL, ch charm.Charm, force bool) (*state.Charm, error) {
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

	// ValidateCharmLXDProfile is used here to replicate the same flow as the
	// not testing version.
	if err := lxdprofile.ValidateCharmLXDProfile(ch); err != nil && !force {
		return nil, err
	}

	stor := statestorage.NewStorage(st.ModelUUID(), st.MongoSession())
	storagePath := fmt.Sprintf("/charms/%s-%s", curl.String(), digest)
	if err := stor.Put(storagePath, f, size); err != nil {
		return nil, fmt.Errorf("cannot put charm: %v", err)
	}
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: storagePath,
		SHA256:      digest,
	}
	sch, err := st.AddCharm(info)
	if err != nil {
		return nil, fmt.Errorf("cannot add charm: %v", err)
	}
	return sch, nil
}

func (s *JujuConnSuite) sampleConfig() testing.Attrs {
	if s.DummyConfig == nil {
		s.DummyConfig = dummy.SampleConfig()
	}
	attrs := s.DummyConfig.Merge(testing.Attrs{
		"name":          "controller",
		"agent-version": jujuversion.Current.String(),
	})
	// Add any custom attributes required.
	for attr, val := range s.ConfigAttrs {
		attrs[attr] = val
	}
	return attrs
}

type GetStater interface {
	GetStateInAPIServer() *state.State
	GetStatePoolInAPIServer() *state.StatePool
	GetHubInAPIServer() *pubsub.StructuredHub
	GetLeaseManagerInAPIServer() lease.Manager
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
	// Close the state pool before we close the underlying state.
	if s.StatePool != nil {
		err := s.StatePool.Close()
		c.Check(err, jc.ErrorIsNil)
		s.StatePool = nil
		s.State = nil
	}

	dummy.Reset(c)
	err := utils.SetHome(s.oldHome)
	c.Assert(err, jc.ErrorIsNil)
	osenv.SetJujuXDGDataHome(s.oldJujuXDGDataHome)
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

func (s *JujuConnSuite) AddTestingCharm(c *gc.C, name string) *state.Charm {
	return s.AddTestingCharmForSeries(c, name, "quantal")
}

func (s *JujuConnSuite) AddTestingCharmForSeries(c *gc.C, name, series string) *state.Charm {
	repo := testcharms.RepoForSeries(series)
	ch := repo.CharmDir(name)
	ident := fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL(fmt.Sprintf("local:%s/%s", series, ident))
	storerepo, err := charmrepo.InferRepository(
		curl,
		charmrepo.NewCharmStoreParams{},
		repo.Path())
	c.Assert(err, jc.ErrorIsNil)
	sch, err := PutCharm(s.State, curl, storerepo, false, false)
	c.Assert(err, jc.ErrorIsNil)
	return sch
}

func (s *JujuConnSuite) AddTestingApplication(c *gc.C, name string, ch *state.Charm) *state.Application {
	app, err := s.State.AddApplication(state.AddApplicationArgs{Name: name, Charm: ch, Series: ch.URL().Series})
	c.Assert(err, jc.ErrorIsNil)
	return app

}

func (s *JujuConnSuite) AddTestingApplicationWithStorage(c *gc.C, name string, ch *state.Charm, storage map[string]state.StorageConstraints) *state.Application {
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: name, Charm: ch, Series: ch.URL().Series, Storage: storage})
	c.Assert(err, jc.ErrorIsNil)
	return app
}

func (s *JujuConnSuite) AddTestingApplicationWithBindings(c *gc.C, name string, ch *state.Charm, bindings map[string]string) *state.Application {
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: name, Charm: ch, Series: ch.URL().Series, EndpointBindings: bindings})
	c.Assert(err, jc.ErrorIsNil)
	return app
}

func (s *JujuConnSuite) AgentConfigForTag(c *gc.C, tag names.Tag) agent.ConfigSetterWriter {
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	paths := agent.DefaultPaths
	paths.DataDir = s.DataDir()
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	config, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             paths,
			Tag:               tag,
			UpgradedToVersion: jujuversion.Current,
			Password:          password,
			Nonce:             "nonce",
			APIAddresses:      s.APIInfo(c).Addrs,
			CACert:            testing.CACert,
			Controller:        s.State.ControllerTag(),
			Model:             model.ModelTag(),
		})
	c.Assert(err, jc.ErrorIsNil)
	return config
}

// AssertConfigParameterUpdated updates environment parameter and
// asserts that no errors were encountered
func (s *JujuConnSuite) AssertConfigParameterUpdated(c *gc.C, key string, value interface{}) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{key: value}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

type agentStatusSetter interface {
	SetAgentStatus(agent string, status presence.Status)
}

func (s *JujuConnSuite) SetAgentPresence(agent string, status presence.Status) {
	s.Environ.(agentStatusSetter).SetAgentStatus(agent, status)
}
