// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juju/charm/v11"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub/v2"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/network"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	envcontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/filestorage"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	statestorage "github.com/juju/juju/state/storage"
	statetesting "github.com/juju/juju/state/testing"
	statewatcher "github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
)

const (
	ControllerName = "kontroll"
)

var (
	// KubernetesSeriesName is the kubernetes series name that is validated at
	// runtime, otherwise it panics.
	KubernetesSeriesName = strings.ToLower(coreos.Kubernetes.String())
)

// JujuConnSuite provides a freshly bootstrapped juju.Conn
// for each test. It also includes testing.BaseSuite.
//
// It also sets up RootDir to point to a directory hierarchy
// mirroring the intended juju directory structure, including
// the following:
//
//	RootDir/var/lib/juju
//	    An empty directory returned as DataDir - the
//	    root of the juju data storage space.
//
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
	mgotesting.MgoSuite
	testing.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture

	InitialLoggingConfig string

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
	BackingState        *state.State          // The State being used by the API server.
	Hub                 *pubsub.StructuredHub // The central hub being used by the API server.
	Controller          *cache.Controller     // The cache.Controller used by the API server.
	LeaseManager        lease.Manager         // The lease manager being used by the API server.
	RootDir             string                // The faked-up root directory.
	LogDir              string
	oldHome             string
	oldJujuXDGDataHome  string
	DummyConfig         testing.Attrs
	Factory             *factory.Factory
	ProviderCallContext envcontext.ProviderCallContext

	idleFuncMutex       *sync.Mutex
	txnSyncNotify       chan struct{}
	modelWatcherIdle    chan string
	controllerIdle      chan struct{}
	controllerIdleCount int
}

const AdminSecret = "dummy-secret"

func (s *JujuConnSuite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	s.PatchValue(&paths.Chown, func(name string, uid, gid int) error { return nil })
}

func (s *JujuConnSuite) TearDownSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *JujuConnSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	if s.InitialLoggingConfig != "" {
		_ = loggo.ConfigureLoggers(s.InitialLoggingConfig)
	}

	// This needs to be a pointer as there are other Mixin structures
	// that copy the lock otherwise. Yet another reason to move away from
	// the glorious JujuConnSuite.
	s.idleFuncMutex = &sync.Mutex{}
	s.txnSyncNotify = make(chan struct{})
	s.modelWatcherIdle = nil
	s.controllerIdle = nil
	s.PatchValue(&statewatcher.TxnPollNotifyFunc, s.txnNotifyFunc)
	s.PatchValue(&statewatcher.HubWatcherIdleFunc, s.hubWatcherIdleFunc)
	s.PatchValue(&cache.IdleFunc, s.controllerIdleFunc)
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
	s.idleFuncMutex.Lock()
	idleChan := s.modelWatcherIdle
	s.idleFuncMutex.Unlock()
	if idleChan == nil {
		return
	}
	// There is a very small race condition between when the
	// idle channel is cleared and when the function exits.
	// Under normal circumstances, there is a goroutine in a tight loop
	// reading off the idle channel. If the channel isn't read
	// within a short wait, we don't send the message.
	select {
	case idleChan <- modelUUID:
	case <-time.After(testing.ShortWait):
	}
}

func (s *JujuConnSuite) controllerIdleFunc() {
	s.idleFuncMutex.Lock()
	idleChan := s.controllerIdle
	s.idleFuncMutex.Unlock()
	if idleChan == nil {
		return
	}
	// Here we have a similar condition to the txn watcher idle.
	// Between test start and when we listen, there may be an idle event.
	// So when we have an idleChan set, we wait for the second idle before
	// we signal that we're idle.
	if s.controllerIdleCount == 0 {
		s.controllerIdleCount++
		return
	}
	// There is a very small race condition between when the
	// idle channel is cleared and when the function exits.
	// Under normal circumstances, there is a goroutine in a tight loop
	// reading off the idle channel. If the channel isn't read
	// within a short wait, we don't send the message.
	select {
	case idleChan <- struct{}{}:
	case <-time.After(testing.ShortWait):
	}
}

func (s *JujuConnSuite) WaitForNextSync(c *gc.C) {
	select {
	case <-s.txnSyncNotify:
	case <-time.After(jujutesting.LongWait):
		c.Fatal("no sync event sent, is the watcher dead?")
	}
	// It is possible that the previous sync was in progress
	// while we were waiting, so wait for a second sync to make sure
	// that the changes in the test goroutine have been processed by
	// the txnwatcher.
	select {
	case <-s.txnSyncNotify:
	case <-time.After(jujutesting.LongWait):
		c.Fatal("no sync event sent, is the watcher dead?")
	}
}

func (s *JujuConnSuite) WaitForModelWatchersIdle(c *gc.C, modelUUID string) {
	// Use a logger rather than c.Log so we get timestamps.
	logger := loggo.GetLogger("test")
	logger.Infof("waiting for model %s to be idle", modelUUID)
	s.WaitForNextSync(c)
	watcherIdleChan := make(chan string)
	controllerIdleChan := make(chan struct{})
	// Now, in theory we shouldn't start waiting for the controller to be idle until
	// we have noticed that the model watcher is idle. In practice, if the model watcher
	// isn't idle, the controller won't yet be idle. Once the watcher is idle, it is also
	// very likely that the controller will become idle very soon after. We do it this way
	// so we don't add 50ms to every call of this function. In practice that time without
	// events should be shared across both the things we are waiting on, so that idle time
	// happens in parallel.
	s.idleFuncMutex.Lock()
	s.modelWatcherIdle = watcherIdleChan
	s.controllerIdleCount = 0
	s.controllerIdle = controllerIdleChan
	s.idleFuncMutex.Unlock()

	defer func() {
		s.idleFuncMutex.Lock()
		s.modelWatcherIdle = nil
		s.controllerIdle = nil
		s.idleFuncMutex.Unlock()
		// Clear out any pending events.
		for {
			select {
			case <-watcherIdleChan:
			case <-controllerIdleChan:
			default:
				logger.Infof("WaitForModelWatchersIdle(%q) done", modelUUID)
				return
			}
		}
	}()

	timeout := time.After(jujutesting.LongWait)
watcher:
	for {
		select {
		case uuid := <-watcherIdleChan:
			logger.Infof("model %s is idle", uuid)
			if uuid == modelUUID {
				break watcher
			}
		case <-timeout:
			c.Fatal("no idle event sent, is the watcher dead?")
		}
	}
	select {
	case <-controllerIdleChan:
		// done
	case <-time.After(jujutesting.LongWait):
		c.Fatal("no controller idle event sent, is the controller dead?")
	}
}

// EnsureCachedModel is used to ensure that the model specified is
// in the model cache. This is used when tests create models and then
// want to do things with those models where the actions may touch
// the model cache.
func (s *JujuConnSuite) EnsureCachedModel(c *gc.C, uuid string) {
	timeout := time.After(testing.LongWait)
	retry := time.After(0)
	for {
		select {
		case <-retry:
			_, err := s.Controller.Model(uuid)
			if err == nil {
				return
			}
			if !errors.Is(err, errors.NotFound) {
				c.Fatalf("problem getting model from cache: %v", err)
			}
			retry = time.After(testing.ShortWait)
		case <-timeout:
			c.Fatalf("model %v not seen in cache after %v", uuid, testing.LongWait)
		}
	}
}

func (s *JujuConnSuite) AdminUserTag(c *gc.C) names.UserTag {
	owner, err := s.State.ControllerOwner()
	c.Assert(err, jc.ErrorIsNil)
	return owner
}

func (s *JujuConnSuite) MongoInfo() *mongo.MongoInfo {
	info := statetesting.NewMongoInfo()
	info.Password = AdminSecret
	return info
}

func (s *JujuConnSuite) APIInfo(c *gc.C) *api.Info {
	apiInfo, err := environs.APIInfo(s.ProviderCallContext, s.ControllerConfig.ControllerUUID(), testing.ModelTag.Id(), testing.CACert, s.ControllerConfig.APIPort(), s.Environ)
	c.Assert(err, jc.ErrorIsNil)
	apiInfo.Tag = s.AdminUserTag(c)
	apiInfo.Password = "dummy-secret"
	apiInfo.ControllerUUID = s.ControllerConfig.ControllerUUID()
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

	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), jobs...)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	return s.openAPIAs(c, machine.Tag(), password, "fake_nonce", false), machine
}

// DefaultVersions returns a slice of unique 'versions' for the current
// environment's host architecture. Additionally, it ensures that 'versions'
// for amd64 are returned if that is not the current host's architecture.
func DefaultVersions(conf *config.Config) []version.Binary {
	agentVersion, isSet := conf.AgentVersion()
	if !isSet {
		agentVersion = jujuversion.Current
	}
	osTypes := set.NewStrings("ubuntu")
	osTypes.Add(coreos.HostOSTypeName())
	var versions []version.Binary
	for _, osType := range osTypes.Values() {
		versions = append(versions, version.Binary{
			Number:  agentVersion,
			Arch:    arch.HostArch(),
			Release: osType,
		})
		if arch.HostArch() != "amd64" {
			versions = append(versions, version.Binary{
				Number:  agentVersion,
				Arch:    "amd64",
				Release: osType,
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

// CreateUserHome creates a home directory and Juju data home for user username.
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

	cfg, err := config.New(config.UseDefaults, s.sampleConfig())
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	s.ControllerConfig = testing.FakeControllerConfig()
	for key, value := range s.ControllerConfigAttrs {
		s.ControllerConfig[key] = value
	}
	// Explicitly disable rate limiting.
	s.ControllerConfig[controller.AgentRateLimitMax] = 0
	cloudSpec := dummy.SampleCloudSpec()
	bootstrapEnviron, err := bootstrap.PrepareController(
		false,
		modelcmd.BootstrapContext(context.Background(), ctx),
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
	s.ProviderCallContext = envcontext.NewCloudCallContext(context.Background())
	err = bootstrap.Bootstrap(modelcmd.BootstrapContext(context.Background(), ctx), environ, s.ProviderCallContext, bootstrap.BootstrapParams{
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
		CloudCredential:         cloudSpec.Credential,
		CloudCredentialName:     "cred",
		AdminSecret:             AdminSecret,
		CAPrivateKey:            testing.CAKey,
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	})
	c.Assert(err, jc.ErrorIsNil)

	getStater := environ.(GetStater)
	s.BackingState = getStater.GetStateInAPIServer()
	s.StatePool = getStater.GetStatePoolInAPIServer()
	s.Hub = getStater.GetHubInAPIServer()
	s.LeaseManager = getStater.GetLeaseManagerInAPIServer()
	s.Controller = getStater.GetController()

	s.State, err = s.StatePool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	s.Model, err = s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	apiInfo, err := environs.APIInfo(s.ProviderCallContext, s.ControllerConfig.ControllerUUID(), testing.ModelTag.Id(), testing.CACert, s.ControllerConfig.APIPort(), environ)
	c.Assert(err, jc.ErrorIsNil)
	apiInfo.Tag = s.AdminUserTag(c)
	apiInfo.Password = AdminSecret
	s.APIState, err = api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)

	// The machine host-ports recorded against the API need to be wrapped in
	// space host-ports as accepted by state.
	mHsPs := s.APIState.APIHostPorts()
	sHsPs := make([]network.SpaceHostPorts, len(mHsPs))
	for i, mHPs := range mHsPs {
		sHPs := make(network.SpaceHostPorts, len(mHPs))
		for j, mHP := range mHPs {
			sHPs[j] = network.SpaceHostPort{
				SpaceAddress: network.SpaceAddress{MachineAddress: mHP.MachineAddress},
				NetPort:      mHP.NetPort,
			}
		}
		sHsPs[i] = sHPs
	}

	err = s.State.SetAPIHostPorts(sHsPs)
	c.Assert(err, jc.ErrorIsNil)

	// Make sure the controller store has the controller api endpoint address set
	ctrl, err := s.ControllerStore.ControllerByName(ControllerName)
	c.Assert(err, jc.ErrorIsNil)
	ctrl.APIEndpoints = []string{s.APIState.APIHostPorts()[0][0].String()}
	err = s.ControllerStore.UpdateController(ControllerName, *ctrl)
	c.Assert(err, jc.ErrorIsNil)
	err = s.ControllerStore.SetCurrentController(ControllerName)
	c.Assert(err, jc.ErrorIsNil)

	s.Environ = environ

	// Insert expected values...
	servingInfo := controller.StateServingInfo{
		PrivateKey:   testing.ServerKey,
		Cert:         testing.ServerCert,
		CAPrivateKey: testing.CAKey,
		SharedSecret: "really, really secret",
		APIPort:      s.ControllerConfig.APIPort(),
		StatePort:    s.ControllerConfig.StatePort(),
	}
	_ = s.State.SetStateServingInfo(servingInfo)
}

// AddToolsToState adds tools to tools storage.
func (s *JujuConnSuite) AddToolsToState(c *gc.C, versions ...version.Binary) {
	stor, err := s.State.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		_ = stor.Close()
	}()
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

// AddDefaultToolsToState adds tools to tools storage for default juju
// series and architectures.
func (s *JujuConnSuite) AddDefaultToolsToState(c *gc.C) {
	versions := DefaultVersions(s.Environ.Config())
	s.AddToolsToState(c, versions...)
}

// PutCharm uploads the given charm to provider storage, and adds a
// state.Charm to the state.  The charm is not uploaded if a charm with
// the same URL already exists in the state.
func PutCharm(st *state.State, curl *charm.URL, ch *charm.CharmDir) (*state.Charm, error) {
	if curl.Revision == -1 {
		curl.Revision = ch.Revision()
	}
	if sch, err := st.Charm(curl.String()); err == nil {
		return sch, nil
	}
	return AddCharm(st, curl, ch, false)
}

// AddCharm adds the charm to state and storage.
func AddCharm(st *state.State, curl *charm.URL, ch charm.Charm, force bool) (*state.Charm, error) {
	var f *os.File
	name := charm.Quote(curl.String())
	switch ch := ch.(type) {
	case *charm.CharmDir:
		var err error
		if f, err = os.CreateTemp("", name); err != nil {
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

	// ValidateLXDProfile is used here to replicate the same flow as the
	// not testing version.
	if err := lxdprofile.ValidateLXDProfile(lxdCharmProfiler{
		Charm: ch,
	}); err != nil && !force {
		return nil, err
	}

	stor := statestorage.NewStorage(st.ModelUUID(), st.MongoSession())
	storagePath := fmt.Sprintf("/charms/%s-%s", curl.String(), digest)
	if err := stor.Put(storagePath, f, size); err != nil {
		return nil, fmt.Errorf("cannot put charm: %v", err)
	}
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl.String(),
		StoragePath: storagePath,
		SHA256:      digest,
	}
	sch, err := st.AddCharm(info)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot add charm")
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
	GetController() *cache.Controller
}

func (s *JujuConnSuite) tearDownConn(c *gc.C) {
	testServer := mgotesting.MgoServer.Addr()
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

func (s *JujuConnSuite) TransientDataDir() string {
	if s.RootDir == "" {
		panic("TransientDataDir called out of test context")
	}
	return filepath.Join(s.RootDir, "/var/run/juju")
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
	sch, err := PutCharm(s.State, curl, ch)
	c.Assert(err, jc.ErrorIsNil)
	return sch
}

func (s *JujuConnSuite) AddTestingApplication(c *gc.C, name string, ch *state.Charm) *state.Application {
	curl := charm.MustParseURL(ch.URL())
	appSeries := curl.Series
	if appSeries == "kubernetes" {
		appSeries = corebase.LegacyKubernetesSeries()
	}
	rev := curl.Revision
	if rev == -1 {
		rev = 0
	}
	base, err := corebase.GetBaseFromSeries(appSeries)
	c.Assert(err, jc.ErrorIsNil)
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: name, Charm: ch,
		CharmOrigin: &state.CharmOrigin{
			Source: "charm-hub",
			Platform: &state.Platform{
				OS:      base.OS,
				Channel: base.Channel.String(),
			},
			Revision: &rev,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	return app
}

func (s *JujuConnSuite) AddTestingApplicationWithOrigin(c *gc.C, name string, ch *state.Charm, origin *state.CharmOrigin) *state.Application {
	c.Assert(origin.Source, gc.Not(gc.Equals), "", gc.Commentf("supplied origin must have a source"))
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:        name,
		Charm:       ch,
		CharmOrigin: origin,
	})
	c.Assert(err, jc.ErrorIsNil)
	return app
}

func (s *JujuConnSuite) AddTestingApplicationWithArch(c *gc.C, name string, ch *state.Charm, arch string) *state.Application {
	curl := charm.MustParseURL(ch.URL())
	base, err := corebase.GetBaseFromSeries(curl.Series)
	c.Assert(err, jc.ErrorIsNil)
	rev := curl.Revision
	if rev == -1 {
		rev = 0
	}
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  name,
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{
			Source: "charm-hub",
			Platform: &state.Platform{
				Architecture: arch,
				OS:           base.OS,
				Channel:      base.Channel.String(),
			},
			Revision: &rev,
		},
		Constraints: constraints.MustParse("arch=" + arch),
	})
	c.Assert(err, jc.ErrorIsNil)
	return app
}

func (s *JujuConnSuite) AddTestingApplicationWithStorage(c *gc.C, name string, ch *state.Charm, storage map[string]state.StorageConstraints) *state.Application {
	curl := charm.MustParseURL(ch.URL())
	base, err := corebase.GetBaseFromSeries(curl.Series)
	c.Assert(err, jc.ErrorIsNil)
	rev := curl.Revision
	if rev == -1 {
		rev = 0
	}
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  name,
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{
			Source: "charm-hub",
			Platform: &state.Platform{
				OS:      base.OS,
				Channel: base.Channel.String(),
			},
			Revision: &rev,
		},
		Storage: storage,
	})
	c.Assert(err, jc.ErrorIsNil)
	return app
}

func (s *JujuConnSuite) AddTestingApplicationWithBindings(c *gc.C, name string, ch *state.Charm, bindings map[string]string) *state.Application {
	curl := charm.MustParseURL(ch.URL())
	base, err := corebase.GetBaseFromSeries(curl.Series)
	c.Assert(err, jc.ErrorIsNil)
	rev := curl.Revision
	if rev == -1 {
		rev = 0
	}
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  name,
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{
			Source: "charm-hub",
			Platform: &state.Platform{
				OS:      base.OS,
				Channel: base.Channel.String(),
			},
			Revision: &rev,
		},
		EndpointBindings: bindings,
	})
	c.Assert(err, jc.ErrorIsNil)
	return app
}

func (s *JujuConnSuite) AgentConfigForTag(c *gc.C, tag names.Tag) agent.ConfigSetterWriter {
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	defaultPaths := agent.DefaultPaths
	defaultPaths.DataDir = s.DataDir()
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	agentConfig, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             defaultPaths,
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
	return agentConfig
}

// AssertConfigParameterUpdated updates environment parameter and
// asserts that no errors were encountered
func (s *JujuConnSuite) AssertConfigParameterUpdated(c *gc.C, key string, value interface{}) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{key: value}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

// lxdCharmProfiler massages a charm.Charm into a LXDProfiler inside of the
// core package.
type lxdCharmProfiler struct {
	Charm charm.Charm
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p lxdCharmProfiler) LXDProfile() lxdprofile.LXDProfile {
	if p.Charm == nil {
		return nil
	}
	if profiler, ok := p.Charm.(charm.LXDProfiler); ok {
		profile := profiler.LXDProfile()
		if profile == nil {
			return nil
		}
		return profile
	}
	return nil
}
