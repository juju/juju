// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The dummy provider implements an environment provider for testing
// purposes, registered with environs under the name "dummy".
//
// The configuration YAML for the testing environment
// must specify a "controller" property with a boolean
// value. If this is true, a controller will be started
// when the environment is bootstrapped.
//
// The configuration data also accepts a "broken" property
// of type boolean. If this is non-empty, any operation
// after the environment has been opened will return
// the error "broken environment", and will also log that.
//
// The DNS name of instances is the same as the Id,
// with ".dns" appended.
package dummy

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/retry"
	"github.com/juju/schema"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/lxdprofile"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	jujuversion "github.com/juju/juju/juju/version"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/pubsub/centralhub"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/worker/lease"
	"github.com/juju/juju/worker/modelcache"
)

var logger = loggo.GetLogger("juju.provider.dummy")

var transientErrorInjection chan error

const BootstrapInstanceId = "localhost"

var errNotPrepared = errors.New("model is not prepared")

// SampleCloudSpec returns an environs.CloudSpec that can be used to
// open a dummy Environ.
func SampleCloudSpec() environs.CloudSpec {
	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"username": "dummy", "password": "secret"})
	return environs.CloudSpec{
		Type:             "dummy",
		Name:             "dummy",
		Endpoint:         "dummy-endpoint",
		IdentityEndpoint: "dummy-identity-endpoint",
		Region:           "dummy-region",
		StorageEndpoint:  "dummy-storage-endpoint",
		Credential:       &cred,
	}
}

// SampleConfig() returns an environment configuration with all required
// attributes set.
func SampleConfig() testing.Attrs {
	return testing.Attrs{
		"type":                      "dummy",
		"name":                      "only",
		"uuid":                      testing.ModelTag.Id(),
		"authorized-keys":           testing.FakeAuthKeys,
		"firewall-mode":             config.FwInstance,
		"ssl-hostname-verification": true,
		"development":               false,
		"default-series":            jujuversion.SupportedLTS(),

		"secret":     "pork",
		"controller": true,
	}
}

// PatchTransientErrorInjectionChannel sets the transientInjectionError
// channel which can be used to inject errors into StartInstance for
// testing purposes
// The injected errors will use the string received on the channel
// and the instance's state will eventually go to error, while the
// received string will appear in the info field of the machine's status
func PatchTransientErrorInjectionChannel(c chan error) func() {
	return gitjujutesting.PatchValue(&transientErrorInjection, c)
}

// mongoInfo returns a mongo.MongoInfo which allows clients to connect to the
// shared dummy state, if it exists.
func mongoInfo() mongo.MongoInfo {
	if gitjujutesting.MgoServer.Addr() == "" {
		panic("dummy environ state tests must be run with MgoTestPackage")
	}
	mongoPort := strconv.Itoa(gitjujutesting.MgoServer.Port())
	addrs := []string{net.JoinHostPort("localhost", mongoPort)}
	return mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:      addrs,
			CACert:     testing.CACert,
			DisableTLS: !gitjujutesting.MgoServer.SSLEnabled(),
		},
	}
}

// Operation represents an action on the dummy provider.
type Operation interface{}

type OpBootstrap struct {
	Context environs.BootstrapContext
	Env     string
	Args    environs.BootstrapParams
}

type OpFinalizeBootstrap struct {
	Context        environs.BootstrapContext
	Env            string
	InstanceConfig *instancecfg.InstanceConfig
}

type OpDestroy struct {
	Env         string
	Cloud       string
	CloudRegion string
	Error       error
}

type OpNetworkInterfaces struct {
	Env        string
	InstanceId instance.Id
	Info       []network.InterfaceInfo
}

type OpSubnets struct {
	Env        string
	InstanceId instance.Id
	SubnetIds  []corenetwork.Id
	Info       []corenetwork.SubnetInfo
}

type OpStartInstance struct {
	Env               string
	MachineId         string
	MachineNonce      string
	PossibleTools     coretools.List
	Instance          instances.Instance
	Constraints       constraints.Value
	SubnetsToZones    map[corenetwork.Id][]string
	NetworkInfo       []network.InterfaceInfo
	Volumes           []storage.Volume
	VolumeAttachments []storage.VolumeAttachment
	Info              *mongo.MongoInfo
	Jobs              []multiwatcher.MachineJob
	APIInfo           *api.Info
	Secret            string
	AgentEnvironment  map[string]string
}

type OpStopInstances struct {
	Env string
	Ids []instance.Id
}

type OpOpenPorts struct {
	Env        string
	MachineId  string
	InstanceId instance.Id
	Rules      []network.IngressRule
}

type OpClosePorts struct {
	Env        string
	MachineId  string
	InstanceId instance.Id
	Rules      []network.IngressRule
}

type OpPutFile struct {
	Env      string
	FileName string
}

// environProvider represents the dummy provider.  There is only ever one
// instance of this type (dummy)
type environProvider struct {
	mu                     sync.Mutex
	ops                    chan<- Operation
	newStatePolicy         state.NewPolicyFunc
	supportsSpaces         bool
	supportsSpaceDiscovery bool
	apiPort                int
	controllerState        *environState
	state                  map[string]*environState
}

// APIPort returns the random api port used by the given provider instance.
func APIPort(p environs.EnvironProvider) int {
	return p.(*environProvider).apiPort
}

// environState represents the state of an environment.
// It can be shared between several environ values,
// so that a given environment can be opened several times.
type environState struct {
	name           string
	ops            chan<- Operation
	newStatePolicy state.NewPolicyFunc
	mu             sync.Mutex
	maxId          int // maximum instance id allocated so far.
	maxAddr        int // maximum allocated address last byte
	insts          map[instance.Id]*dummyInstance
	globalRules    network.IngressRuleSlice
	bootstrapped   bool
	mux            *apiserverhttp.Mux
	httpServer     *httptest.Server
	apiServer      *apiserver.Server
	apiState       *state.State
	apiStatePool   *state.StatePool
	hub            *pubsub.StructuredHub
	presence       *fakePresence
	leaseManager   *lease.Manager
	creator        string

	modelCacheWorker worker.Worker
	controller       *cache.Controller
}

// environ represents a client's connection to a given environment's
// state.
type environ struct {
	storage.ProviderRegistry
	name         string
	modelUUID    string
	cloud        environs.CloudSpec
	ecfgMutex    sync.Mutex
	ecfgUnlocked *environConfig
	spacesMutex  sync.RWMutex
}

var _ environs.Environ = (*environ)(nil)
var _ environs.Networking = (*environ)(nil)

// discardOperations discards all Operations written to it.
var discardOperations = make(chan Operation)

func init() {
	environs.RegisterProvider("dummy", &dummy)

	// Prime the first ops channel, so that naive clients can use
	// the testing environment by simply importing it.
	go func() {
		for range discardOperations {
		}
	}()
}

// dummy is the dummy environmentProvider singleton.
var dummy = environProvider{
	ops:                    discardOperations,
	state:                  make(map[string]*environState),
	newStatePolicy:         stateenvirons.GetNewPolicyFunc(),
	supportsSpaces:         true,
	supportsSpaceDiscovery: false,
}

// Reset resets the entire dummy environment and forgets any registered
// operation listener. All opened environments after Reset will share
// the same underlying state.
func Reset(c *gc.C) {
	logger.Infof("reset model")
	dummy.mu.Lock()
	dummy.ops = discardOperations
	oldState := dummy.state
	dummy.controllerState = nil
	dummy.state = make(map[string]*environState)
	dummy.newStatePolicy = stateenvirons.GetNewPolicyFunc()
	dummy.supportsSpaces = true
	dummy.supportsSpaceDiscovery = false
	dummy.mu.Unlock()

	// NOTE(axw) we must destroy the old states without holding
	// the provider lock, or we risk deadlocking. Destroying
	// state involves closing the embedded API server, which
	// may require waiting on RPC calls that interact with the
	// EnvironProvider (e.g. EnvironProvider.Open).
	for _, s := range oldState {
		if s.httpServer != nil {
			logger.Debugf("closing httpServer")
			s.httpServer.Close()
		}
		s.destroy()
	}
	if mongoAlive() {
		err := retry.Call(retry.CallArgs{
			Func: gitjujutesting.MgoServer.Reset,
			// Only interested in retrying the intermittent
			// 'unexpected message'.
			IsFatalError: func(err error) bool {
				return !strings.HasSuffix(err.Error(), "unexpected message")
			},
			Delay:    time.Millisecond,
			Clock:    clock.WallClock,
			Attempts: 5,
		})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (state *environState) destroy() {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.destroyLocked()
}

func (state *environState) destroyLocked() {
	if !state.bootstrapped {
		return
	}
	apiServer := state.apiServer
	apiStatePool := state.apiStatePool
	leaseManager := state.leaseManager
	modelCacheWorker := state.modelCacheWorker
	state.apiServer = nil
	state.apiStatePool = nil
	state.apiState = nil
	state.controller = nil
	state.leaseManager = nil
	state.bootstrapped = false
	state.hub = nil
	state.modelCacheWorker = nil

	// Release the lock while we close resources. In particular,
	// we must not hold the lock while the API server is being
	// closed, as it may need to interact with the Environ while
	// shutting down.
	state.mu.Unlock()
	defer state.mu.Lock()

	// The apiServer depends on the modelCache, so stop the apiserver first.
	if apiServer != nil {
		logger.Debugf("stopping apiServer")
		if err := apiServer.Stop(); err != nil && mongoAlive() {
			panic(err)
		}
	}

	if modelCacheWorker != nil {
		logger.Debugf("stopping modelCache worker")
		if err := worker.Stop(modelCacheWorker); err != nil {
			panic(err)
		}
	}

	if leaseManager != nil {
		if err := worker.Stop(leaseManager); err != nil && mongoAlive() {
			panic(err)
		}
	}

	if apiStatePool != nil {
		logger.Debugf("closing apiStatePool")
		if err := apiStatePool.Close(); err != nil && mongoAlive() {
			panic(err)
		}
	}

	if mongoAlive() {
		logger.Debugf("resetting MgoServer")
		gitjujutesting.MgoServer.Reset()
	}
}

// mongoAlive reports whether the mongo server is
// still alive (i.e. has not been deliberately destroyed).
// If it has been deliberately destroyed, we will
// expect some errors when closing things down.
func mongoAlive() bool {
	return gitjujutesting.MgoServer.Addr() != ""
}

// GetStateInAPIServer returns the state connection used by the API server
// This is so code in the test suite can trigger Syncs, etc that the API server
// will see, which will then trigger API watchers, etc.
func (e *environ) GetStateInAPIServer() *state.State {
	st, err := e.state()
	if err != nil {
		panic(err)
	}
	return st.apiState
}

// GetStatePoolInAPIServer returns the StatePool used by the API
// server.  As for GetStatePoolInAPIServer, this is so code in the
// test suite can trigger Syncs etc.
func (e *environ) GetStatePoolInAPIServer() *state.StatePool {
	st, err := e.state()
	if err != nil {
		panic(err)
	}
	return st.apiStatePool
}

// GetHubInAPIServer returns the central hub used by the API server.
func (e *environ) GetHubInAPIServer() *pubsub.StructuredHub {
	st, err := e.state()
	if err != nil {
		panic(err)
	}
	return st.hub
}

// GetLeaseManagerInAPIServer returns the channel used to update the
// cache.Controller used by the API server
func (e *environ) GetLeaseManagerInAPIServer() corelease.Manager {
	st, err := e.state()
	if err != nil {
		panic(err)
	}
	return st.leaseManager
}

// GetController returns the cache.Controller used by the API server.
func (e *environ) GetController() *cache.Controller {
	st, err := e.state()
	if err != nil {
		panic(err)
	}
	return st.controller
}

// newState creates the state for a new environment with the given name.
func newState(name string, ops chan<- Operation, newStatePolicy state.NewPolicyFunc) *environState {
	buf := make([]byte, 8192)
	buf = buf[:runtime.Stack(buf, false)]
	s := &environState{
		name:           name,
		ops:            ops,
		newStatePolicy: newStatePolicy,
		insts:          make(map[instance.Id]*dummyInstance),
		creator:        string(buf),
	}
	return s
}

// listenAPI starts an HTTP server listening for API connections.
func (s *environState) listenAPI() int {
	certPool, err := api.CreateCertPool(testing.CACert)
	if err != nil {
		panic(err)
	}
	tlsConfig := api.NewTLSConfig(certPool)
	tlsConfig.ServerName = "juju-apiserver"
	tlsConfig.Certificates = []tls.Certificate{*testing.ServerTLSCert}
	s.mux = apiserverhttp.NewMux()
	s.httpServer = httptest.NewUnstartedServer(s.mux)
	s.httpServer.TLS = tlsConfig
	return s.httpServer.Listener.Addr().(*net.TCPAddr).Port
}

// SetSupportsSpaces allows to enable and disable SupportsSpaces for tests.
func SetSupportsSpaces(supports bool) bool {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	current := dummy.supportsSpaces
	dummy.supportsSpaces = supports
	return current
}

// SetSupportsSpaceDiscovery allows to enable and disable
// SupportsSpaceDiscovery for tests.
func SetSupportsSpaceDiscovery(supports bool) bool {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	current := dummy.supportsSpaceDiscovery
	dummy.supportsSpaceDiscovery = supports
	return current
}

// Listen directs subsequent operations on any dummy environment
// to channel c (if not nil).
func Listen(c chan<- Operation) {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	if c == nil {
		c = discardOperations
	}
	dummy.ops = c
	for _, st := range dummy.state {
		st.mu.Lock()
		st.ops = c
		st.mu.Unlock()
	}
}

var configSchema = environschema.Fields{
	"controller": {
		Description: "Whether the model should start a controller",
		Type:        environschema.Tbool,
	},
	"broken": {
		Description: "Whitespace-separated Environ methods that should return an error when called",
		Type:        environschema.Tstring,
	},
	"secret": {
		Description: "A secret",
		Type:        environschema.Tstring,
	},
}

var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

var configDefaults = schema.Defaults{
	"broken":     "",
	"secret":     "pork",
	"controller": false,
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (c *environConfig) controller() bool {
	return c.attrs["controller"].(bool)
}

func (c *environConfig) broken() string {
	return c.attrs["broken"].(string)
}

func (c *environConfig) secret() string {
	return c.attrs["secret"].(string)
}

func (p *environProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

func (p *environProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

var _ config.ConfigSchemaSource = (*environProvider)(nil)

// ConfigSchema returns extra config attributes specific
// to this provider only.
func (p *environProvider) ConfigSchema() schema.Fields {
	return configFields
}

// ConfigDefaults returns the default values for the
// provider specific config attributes.
func (p *environProvider) ConfigDefaults() schema.Defaults {
	return configDefaults
}

func (*environProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.EmptyAuthType: {},
		cloud.UserPassAuthType: {
			{
				"username", cloud.CredentialAttr{Description: "The username to authenticate with."},
			}, {
				"password", cloud.CredentialAttr{
					Description: "The password for the specified username.",
					Hidden:      true,
				},
			},
		},
	}
}

func (*environProvider) DetectCredentials() (*cloud.CloudCredential, error) {
	return cloud.NewEmptyCloudCredential(), nil
}

func (*environProvider) FinalizeCredential(ctx environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}

func (*environProvider) DetectRegions() ([]cloud.Region, error) {
	return []cloud.Region{{Name: "dummy"}}, nil
}

func (p *environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	// Apply the coerced unknown values back into the config.
	return cfg.Apply(validated)
}

func (e *environ) state() (*environState, error) {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	state, ok := dummy.state[e.modelUUID]
	if !ok {
		return nil, errNotPrepared
	}
	return state, nil
}

// Version is part of the EnvironProvider interface.
func (*environProvider) Version() int {
	return 0
}

func (p *environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ecfg, err := p.newConfig(args.Config)
	if err != nil {
		return nil, err
	}
	env := &environ{
		ProviderRegistry: StorageProviders(),
		name:             ecfg.Name(),
		modelUUID:        args.Config.UUID(),
		cloud:            args.Cloud,
		ecfgUnlocked:     ecfg,
	}
	if err := env.checkBroken("Open"); err != nil {
		return nil, err
	}
	return env, nil
}

// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does not support custom clouds, this always returns nil.
func (p *environProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p *environProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig is specified in the EnvironProvider interface.
func (p *environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if _, err := dummy.newConfig(args.Config); err != nil {
		return nil, err
	}
	return args.Config, nil
}

// Override for testing - the data directory with which the state api server is initialised.
var DataDir = ""
var LogDir = ""

func (e *environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	ecfg := e.ecfgUnlocked
	e.ecfgMutex.Unlock()
	return ecfg
}

func (e *environ) checkBroken(method string) error {
	for _, m := range strings.Fields(e.ecfg().broken()) {
		if m == method {
			return fmt.Errorf("dummy.%s is broken", method)
		}
	}
	return nil
}

// PrecheckInstance is specified in the environs.InstancePrechecker interface.
func (*environ) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if args.Placement != "" && args.Placement != "valid" {
		return fmt.Errorf("%s placement is invalid", args.Placement)
	}
	return nil
}

// Create is part of the Environ interface.
func (e *environ) Create(ctx context.ProviderCallContext, args environs.CreateParams) error {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	dummy.state[e.modelUUID] = newState(e.name, dummy.ops, dummy.newStatePolicy)
	return nil
}

// PrepareForBootstrap is part of the Environ interface.
func (e *environ) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	ecfg := e.ecfgUnlocked

	if ecfg.controller() && dummy.controllerState != nil {
		// Because of global variables, we can only have one dummy
		// controller per process. Panic if there is an attempt to
		// bootstrap while there is another active controller.
		old := dummy.controllerState
		panic(fmt.Errorf("cannot share a state between two dummy environs; old %q; new %q: %s", old.name, e.name, old.creator))
	}

	// The environment has not been prepared, so create it and record it.
	// We don't start listening for State or API connections until
	// Bootstrap has been called.
	envState := newState(e.name, dummy.ops, dummy.newStatePolicy)
	if ecfg.controller() {
		dummy.apiPort = envState.listenAPI()
		dummy.controllerState = envState
	}
	dummy.state[e.modelUUID] = envState
	return nil
}

func (e *environ) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	series := config.PreferredSeries(e.Config())
	availableTools, err := args.AvailableTools.Match(coretools.Filter{Series: series})
	if err != nil {
		return nil, err
	}
	arch := availableTools.Arches()[0]

	defer delay()
	if err := e.checkBroken("Bootstrap"); err != nil {
		return nil, err
	}
	if _, ok := args.ControllerConfig.CACert(); !ok {
		return nil, errors.New("no CA certificate in controller configuration")
	}

	logger.Infof("would pick agent binaries from %s", availableTools)

	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	if estate.bootstrapped {
		return nil, errors.New("model is already bootstrapped")
	}

	// Create an instance for the bootstrap node.
	logger.Infof("creating bootstrap instance")
	i := &dummyInstance{
		id:           BootstrapInstanceId,
		addresses:    network.NewAddresses("localhost"),
		machineId:    agent.BootstrapControllerId,
		series:       series,
		firewallMode: e.Config().FirewallMode(),
		state:        estate,
		controller:   true,
	}
	estate.insts[i.id] = i
	estate.bootstrapped = true
	estate.ops <- OpBootstrap{Context: ctx, Env: e.name, Args: args}

	finalize := func(ctx environs.BootstrapContext, icfg *instancecfg.InstanceConfig, _ environs.BootstrapDialOpts) (err error) {
		if e.ecfg().controller() {
			icfg.Bootstrap.BootstrapMachineInstanceId = BootstrapInstanceId
			if err := instancecfg.FinishInstanceConfig(icfg, e.Config()); err != nil {
				return err
			}

			adminUser := names.NewUserTag("admin@local")
			var cloudCredentialTag names.CloudCredentialTag
			if icfg.Bootstrap.ControllerCloudCredentialName != "" {
				id := fmt.Sprintf(
					"%s/%s/%s",
					icfg.Bootstrap.ControllerCloud.Name,
					adminUser.Id(),
					icfg.Bootstrap.ControllerCloudCredentialName,
				)
				if !names.IsValidCloudCredential(id) {
					return errors.NotValidf("cloud credential ID %q", id)
				}
				cloudCredentialTag = names.NewCloudCredentialTag(id)
			}

			cloudCredentials := make(map[names.CloudCredentialTag]cloud.Credential)
			if icfg.Bootstrap.ControllerCloudCredential != nil && icfg.Bootstrap.ControllerCloudCredentialName != "" {
				cloudCredentials[cloudCredentialTag] = *icfg.Bootstrap.ControllerCloudCredential
			}

			session, err := mongo.DialWithInfo(mongoInfo(), mongotest.DialOpts())
			if err != nil {
				return err
			}
			defer session.Close()

			// Since the admin user isn't setup until after here,
			// the password in the info structure is empty, so the admin
			// user is constructed with an empty password here.
			// It is set just below.
			controller, err := state.Initialize(state.InitializeParams{
				Clock:            clock.WallClock,
				ControllerConfig: icfg.Controller.Config,
				ControllerModelArgs: state.ModelArgs{
					Type:                    state.ModelTypeIAAS,
					Owner:                   adminUser,
					Config:                  icfg.Bootstrap.ControllerModelConfig,
					Constraints:             icfg.Bootstrap.BootstrapMachineConstraints,
					CloudName:               icfg.Bootstrap.ControllerCloud.Name,
					CloudRegion:             icfg.Bootstrap.ControllerCloudRegion,
					CloudCredential:         cloudCredentialTag,
					StorageProviderRegistry: e,
				},
				Cloud:            icfg.Bootstrap.ControllerCloud,
				CloudCredentials: cloudCredentials,
				MongoSession:     session,
				NewPolicy:        estate.newStatePolicy,
				AdminPassword:    icfg.Controller.MongoInfo.Password,
			})
			if err != nil {
				return err
			}
			st := controller.SystemState()
			defer func() {
				if err != nil {
					controller.Close()
				}
			}()
			if err := st.SetModelConstraints(args.ModelConstraints); err != nil {
				return errors.Trace(err)
			}
			if err := st.SetAdminMongoPassword(icfg.Controller.MongoInfo.Password); err != nil {
				return errors.Trace(err)
			}
			if err := st.MongoSession().DB("admin").Login("admin", icfg.Controller.MongoInfo.Password); err != nil {
				return err
			}
			env, err := st.Model()
			if err != nil {
				return errors.Trace(err)
			}
			owner, err := st.User(env.Owner())
			if err != nil {
				return errors.Trace(err)
			}
			// We log this out for test purposes only. No one in real life can use
			// a dummy provider for anything other than testing, so logging the password
			// here is fine.
			logger.Debugf("setting password for %q to %q", owner.Name(), icfg.Controller.MongoInfo.Password)
			owner.SetPassword(icfg.Controller.MongoInfo.Password)
			statePool := controller.StatePool()
			stateAuthenticator, err := stateauthenticator.NewAuthenticator(statePool, clock.WallClock)
			if err != nil {
				return errors.Trace(err)
			}
			stateAuthenticator.AddHandlers(estate.mux)

			machineTag := names.NewMachineTag("0")
			estate.httpServer.StartTLS()
			estate.presence = &fakePresence{make(map[string]presence.Status)}
			estate.hub = centralhub.New(machineTag)

			estate.leaseManager, err = leaseManager(
				icfg.Controller.Config.ControllerUUID(),
				st,
			)
			if err != nil {
				return errors.Trace(err)
			}

			modelCache, err := modelcache.NewWorker(modelcache.Config{
				Logger: loggo.GetLogger("dummy"),
				WatcherFactory: func() modelcache.BackingWatcher {
					return statePool.SystemState().WatchAllModels(statePool)
				},
				PrometheusRegisterer: noopRegisterer{},
				Cleanup:              func() {},
			})
			if err != nil {
				return errors.Trace(err)
			}
			estate.modelCacheWorker = modelCache
			err = modelcache.ExtractCacheController(modelCache, &estate.controller)
			if err != nil {
				worker.Stop(modelCache)
				return errors.Trace(err)
			}

			estate.apiServer, err = apiserver.NewServer(apiserver.ServerConfig{
				StatePool:      statePool,
				Controller:     estate.controller,
				Authenticator:  stateAuthenticator,
				Clock:          clock.WallClock,
				GetAuditConfig: func() auditlog.Config { return auditlog.Config{} },
				Tag:            machineTag,
				DataDir:        DataDir,
				LogDir:         LogDir,
				Mux:            estate.mux,
				Hub:            estate.hub,
				Presence:       estate.presence,
				LeaseManager:   estate.leaseManager,
				NewObserver: func() observer.Observer {
					logger := loggo.GetLogger("juju.apiserver")
					ctx := observer.RequestObserverContext{
						Clock:  clock.WallClock,
						Logger: logger,
						Hub:    estate.hub,
					}
					return observer.NewRequestObserver(ctx)
				},
				RateLimitConfig: apiserver.DefaultRateLimitConfig(),
				PublicDNSName:   icfg.Controller.Config.AutocertDNSName(),
				UpgradeComplete: func() bool {
					return true
				},
				RestoreStatus: func() state.RestoreStatus {
					return state.RestoreNotActive
				},
				MetricsCollector: apiserver.NewMetricsCollector(),
			})
			if err != nil {
				panic(err)
			}
			estate.apiState = st
			estate.apiStatePool = statePool

			// Maintain the state authenticator (time out local user interactions).
			abort := make(chan struct{})
			go stateAuthenticator.Maintain(abort)
			go func(apiServer *apiserver.Server) {
				defer close(abort)
				apiServer.Wait()
			}(estate.apiServer)
		}
		estate.ops <- OpFinalizeBootstrap{Context: ctx, Env: e.name, InstanceConfig: icfg}
		return nil
	}

	bsResult := &environs.BootstrapResult{
		Arch:                    arch,
		Series:                  series,
		CloudBootstrapFinalizer: finalize,
	}
	return bsResult, nil
}

func leaseManager(controllerUUID string, st *state.State) (*lease.Manager, error) {
	target := st.LeaseNotifyTarget(
		ioutil.Discard,
		loggo.GetLogger("juju.state.raftlease"),
	)
	dummyStore := newLeaseStore(clock.WallClock, target, st.LeaseTrapdoorFunc())
	return lease.NewManager(lease.ManagerConfig{
		Secretary:            lease.SecretaryFinder(controllerUUID),
		Store:                dummyStore,
		Logger:               loggo.GetLogger("juju.worker.lease.dummy"),
		Clock:                clock.WallClock,
		MaxSleep:             time.Minute,
		EntityUUID:           controllerUUID,
		PrometheusRegisterer: noopRegisterer{},
	})
}

func (e *environ) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	if err := e.checkBroken("ControllerInstances"); err != nil {
		return nil, err
	}
	if !estate.bootstrapped {
		return nil, environs.ErrNotBootstrapped
	}
	var controllerInstances []instance.Id
	for _, v := range estate.insts {
		if v.controller {
			controllerInstances = append(controllerInstances, v.Id())
		}
	}
	return controllerInstances, nil
}

func (e *environ) Config() *config.Config {
	return e.ecfg().Config
}

func (e *environ) SetConfig(cfg *config.Config) error {
	if err := e.checkBroken("SetConfig"); err != nil {
		return err
	}
	ecfg, err := dummy.newConfig(cfg)
	if err != nil {
		return err
	}
	e.ecfgMutex.Lock()
	e.ecfgUnlocked = ecfg
	e.ecfgMutex.Unlock()
	return nil
}

// AdoptResources is part of the Environ interface.
func (e *environ) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	// This provider doesn't track instance -> controller.
	return nil
}

func (e *environ) Destroy(ctx context.ProviderCallContext) (res error) {
	defer delay()
	estate, err := e.state()
	if err != nil {
		if err == errNotPrepared {
			return nil
		}
		return err
	}
	defer func() {
		// The estate is a pointer to a structure that is stored in the dummy global.
		// The Listen method can change the ops channel of any state, and will do so
		// under the covers. What we need to do is use the state mutex to add a memory
		// barrier such that the ops channel we see here is the latest.
		estate.mu.Lock()
		ops := estate.ops
		name := estate.name
		delete(dummy.state, e.modelUUID)
		estate.mu.Unlock()
		ops <- OpDestroy{
			Env:         name,
			Cloud:       e.cloud.Name,
			CloudRegion: e.cloud.Region,
			Error:       res,
		}
	}()
	if err := e.checkBroken("Destroy"); err != nil {
		return err
	}
	if !e.ecfg().controller() {
		return nil
	}
	estate.destroy()
	return nil
}

func (e *environ) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	if err := e.Destroy(ctx); err != nil {
		return err
	}
	dummy.mu.Lock()
	dummy.controllerState = nil
	dummy.mu.Unlock()
	return nil
}

// ConstraintsValidator is defined on the Environs interface.
func (e *environ) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported([]string{constraints.CpuPower, constraints.VirtType})
	validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
	validator.RegisterVocabulary(constraints.Arch, []string{arch.AMD64, arch.ARM64, arch.I386, arch.PPC64EL})
	return validator, nil
}

// MaintainInstance is specified in the InstanceBroker interface.
func (*environ) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	return nil
}

// StartInstance is specified in the InstanceBroker interface.
func (e *environ) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {

	defer delay()
	machineId := args.InstanceConfig.MachineId
	logger.Infof("dummy startinstance, machine %s", machineId)
	if err := e.checkBroken("StartInstance"); err != nil {
		return nil, err
	}
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()

	// check if an error has been injected on the transientErrorInjection channel (testing purposes)
	select {
	case injectedError := <-transientErrorInjection:
		return nil, injectedError
	default:
	}

	if args.InstanceConfig.MachineNonce == "" {
		return nil, errors.New("cannot start instance: missing machine nonce")
	}
	if args.InstanceConfig.Controller != nil {
		if args.InstanceConfig.Controller.MongoInfo.Tag != names.NewMachineTag(machineId) {
			return nil, errors.New("entity tag must match started machine")
		}
	}
	if args.InstanceConfig.APIInfo.Tag != names.NewMachineTag(machineId) {
		return nil, errors.New("entity tag must match started machine")
	}
	logger.Infof("would pick agent binaries from %s", args.Tools)
	series := args.Tools.OneSeries()

	idString := fmt.Sprintf("%s-%d", e.name, estate.maxId)
	// Add the addresses we want to see in the machine doc. This means both
	// IPv4 and IPv6 loopback, as well as the DNS name.
	addrs := network.NewAddresses(idString+".dns", "127.0.0.1", "::1")
	logger.Debugf("StartInstance addresses: %v", addrs)
	i := &dummyInstance{
		id:           instance.Id(idString),
		addresses:    addrs,
		machineId:    machineId,
		series:       series,
		firewallMode: e.Config().FirewallMode(),
		state:        estate,
	}

	var hc *instance.HardwareCharacteristics
	// To match current system capability, only provide hardware characteristics for
	// environ machines, not containers.
	if state.ParentId(machineId) == "" {
		// Assume that the provided Availability Zone won't fail,
		// though one is required.
		var zone string
		if args.Placement != "" {
			split := strings.Split(args.Placement, "=")
			if len(split) == 2 && split[0] == "zone" {
				zone = split[1]
			}
		}
		if zone == "" && args.AvailabilityZone != "" {
			zone = args.AvailabilityZone
		}

		// We will just assume the instance hardware characteristics exactly matches
		// the supplied constraints (if specified).
		hc = &instance.HardwareCharacteristics{
			Arch:             args.Constraints.Arch,
			Mem:              args.Constraints.Mem,
			RootDisk:         args.Constraints.RootDisk,
			CpuCores:         args.Constraints.CpuCores,
			CpuPower:         args.Constraints.CpuPower,
			Tags:             args.Constraints.Tags,
			AvailabilityZone: &zone,
		}
		// Fill in some expected instance hardware characteristics if constraints not specified.
		if hc.Arch == nil {
			arch := "amd64"
			hc.Arch = &arch
		}
		if hc.Mem == nil {
			mem := uint64(1024)
			hc.Mem = &mem
		}
		if hc.RootDisk == nil {
			disk := uint64(8192)
			hc.RootDisk = &disk
		}
		if hc.CpuCores == nil {
			cores := uint64(1)
			hc.CpuCores = &cores
		}
	}
	// Simulate subnetsToZones gets populated when spaces given in constraints.
	spaces := args.Constraints.IncludeSpaces()
	var subnetsToZones map[corenetwork.Id][]string
	for isp := range spaces {
		// Simulate 2 subnets per space.
		if subnetsToZones == nil {
			subnetsToZones = make(map[corenetwork.Id][]string)
		}
		for isn := 0; isn < 2; isn++ {
			providerId := fmt.Sprintf("subnet-%d", isp+isn)
			zone := fmt.Sprintf("zone%d", isp+isn)
			subnetsToZones[corenetwork.Id(providerId)] = []string{zone}
		}
	}
	// Simulate creating volumes when requested.
	volumes := make([]storage.Volume, len(args.Volumes))
	for iv, v := range args.Volumes {
		persistent, _ := v.Attributes["persistent"].(bool)
		volumes[iv] = storage.Volume{
			Tag: v.Tag,
			VolumeInfo: storage.VolumeInfo{
				Size:       v.Size,
				Persistent: persistent,
			},
		}
	}
	// Simulate attaching volumes when requested.
	volumeAttachments := make([]storage.VolumeAttachment, len(args.VolumeAttachments))
	for iv, v := range args.VolumeAttachments {
		volumeAttachments[iv] = storage.VolumeAttachment{
			Volume:  v.Volume,
			Machine: v.Machine,
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
				DeviceName: fmt.Sprintf("sd%c", 'b'+rune(iv)),
				ReadOnly:   v.ReadOnly,
			},
		}
	}
	var mongoInfo *mongo.MongoInfo
	if args.InstanceConfig.Controller != nil {
		mongoInfo = args.InstanceConfig.Controller.MongoInfo
	}
	estate.insts[i.id] = i
	estate.maxId++
	estate.ops <- OpStartInstance{
		Env:               e.name,
		MachineId:         machineId,
		MachineNonce:      args.InstanceConfig.MachineNonce,
		PossibleTools:     args.Tools,
		Constraints:       args.Constraints,
		SubnetsToZones:    subnetsToZones,
		Volumes:           volumes,
		VolumeAttachments: volumeAttachments,
		Instance:          i,
		Jobs:              args.InstanceConfig.Jobs,
		Info:              mongoInfo,
		APIInfo:           args.InstanceConfig.APIInfo,
		AgentEnvironment:  args.InstanceConfig.AgentEnvironment,
		Secret:            e.ecfg().secret(),
	}
	return &environs.StartInstanceResult{
		Instance: i,
		Hardware: hc,
	}, nil
}

func (e *environ) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	defer delay()
	if err := e.checkBroken("StopInstance"); err != nil {
		return err
	}
	estate, err := e.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, id := range ids {
		delete(estate.insts, id)
	}
	estate.ops <- OpStopInstances{
		Env: e.name,
		Ids: ids,
	}
	return nil
}

func (e *environ) Instances(ctx context.ProviderCallContext, ids []instance.Id) (insts []instances.Instance, err error) {
	defer delay()
	if err := e.checkBroken("Instances"); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	notFound := 0
	for _, id := range ids {
		inst := estate.insts[id]
		if inst == nil {
			err = environs.ErrPartialInstances
			notFound++
			insts = append(insts, nil)
		} else {
			insts = append(insts, inst)
		}
	}
	if notFound == len(ids) {
		return nil, environs.ErrNoInstances
	}
	return
}

// SupportsSpaces is specified on environs.Networking.
func (env *environ) SupportsSpaces(ctx context.ProviderCallContext) (bool, error) {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	if !dummy.supportsSpaces {
		return false, errors.NotSupportedf("spaces")
	}
	return true, nil
}

// SupportsSpaceDiscovery is specified on environs.Networking.
func (env *environ) SupportsSpaceDiscovery(ctx context.ProviderCallContext) (bool, error) {
	if err := env.checkBroken("SupportsSpaceDiscovery"); err != nil {
		return false, err
	}
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	if !dummy.supportsSpaceDiscovery {
		return false, nil
	}
	return true, nil
}

// SupportsContainerAddresses is specified on environs.Networking.
func (env *environ) SupportsContainerAddresses(ctx context.ProviderCallContext) (bool, error) {
	return false, errors.NotSupportedf("container addresses")
}

// Spaces is specified on environs.Networking.
func (env *environ) Spaces(ctx context.ProviderCallContext) ([]corenetwork.SpaceInfo, error) {
	if err := env.checkBroken("Spaces"); err != nil {
		return []corenetwork.SpaceInfo{}, err
	}
	return []corenetwork.SpaceInfo{{
		Name:       "foo",
		ProviderId: corenetwork.Id("0"),
		Subnets: []corenetwork.SubnetInfo{{
			ProviderId:        corenetwork.Id("1"),
			AvailabilityZones: []string{"zone1"},
		}, {
			ProviderId:        corenetwork.Id("2"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "Another Foo 99!",
		ProviderId: "1",
		Subnets: []corenetwork.SubnetInfo{{
			ProviderId:        corenetwork.Id("3"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "foo-",
		ProviderId: "2",
		Subnets: []corenetwork.SubnetInfo{{
			ProviderId:        corenetwork.Id("4"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "---",
		ProviderId: "3",
		Subnets: []corenetwork.SubnetInfo{{
			ProviderId:        corenetwork.Id("5"),
			AvailabilityZones: []string{"zone1"},
		}}}}, nil
}

// NetworkInterfaces implements Environ.NetworkInterfaces().
func (env *environ) NetworkInterfaces(ctx context.ProviderCallContext, instId instance.Id) ([]network.InterfaceInfo, error) {
	if err := env.checkBroken("NetworkInterfaces"); err != nil {
		return nil, err
	}

	estate, err := env.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()

	// Simulate 3 NICs - primary and secondary enabled plus a disabled NIC.
	// all configured using DHCP and having fake DNS servers and gateway.
	info := make([]network.InterfaceInfo, 3)
	for i, netName := range []string{"private", "public", "disabled"} {
		info[i] = network.InterfaceInfo{
			DeviceIndex:      i,
			ProviderId:       corenetwork.Id(fmt.Sprintf("dummy-eth%d", i)),
			ProviderSubnetId: corenetwork.Id("dummy-" + netName),
			InterfaceType:    network.EthernetInterface,
			CIDR:             fmt.Sprintf("0.%d.0.0/24", (i+1)*10),
			InterfaceName:    fmt.Sprintf("eth%d", i),
			VLANTag:          i,
			MACAddress:       fmt.Sprintf("aa:bb:cc:dd:ee:f%d", i),
			Disabled:         i == 2,
			NoAutoStart:      i%2 != 0,
			ConfigType:       network.ConfigDHCP,
			Address: network.NewAddress(
				fmt.Sprintf("0.%d.0.%d", (i+1)*10, estate.maxAddr+2),
			),
			DNSServers: network.NewAddresses("ns1.dummy", "ns2.dummy"),
			GatewayAddress: network.NewAddress(
				fmt.Sprintf("0.%d.0.1", (i+1)*10),
			),
		}
	}

	estate.ops <- OpNetworkInterfaces{
		Env:        env.name,
		InstanceId: instId,
		Info:       info,
	}

	return info, nil
}

type azShim struct {
	name      string
	available bool
}

func (az azShim) Name() string {
	return az.name
}

func (az azShim) Available() bool {
	return az.available
}

// AvailabilityZones implements environs.ZonedEnviron.
func (env *environ) AvailabilityZones(ctx context.ProviderCallContext) ([]common.AvailabilityZone, error) {
	// TODO(dimitern): Fix this properly.
	return []common.AvailabilityZone{
		azShim{"zone1", true},
		azShim{"zone2", false},
		azShim{"zone3", true},
		azShim{"zone4", true},
	}, nil
}

// InstanceAvailabilityZoneNames implements environs.ZonedEnviron.
func (env *environ) InstanceAvailabilityZoneNames(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error) {
	if err := env.checkBroken("InstanceAvailabilityZoneNames"); err != nil {
		return nil, errors.NotSupportedf("instance availability zones")
	}
	availabilityZones, err := env.AvailabilityZones(ctx)
	if err != nil {
		return nil, err
	}
	azMaxIndex := len(availabilityZones) - 1
	azIndex := 0
	returnValue := make([]string, len(ids))
	for i := range ids {
		if availabilityZones[azIndex].Available() {
			returnValue[i] = availabilityZones[azIndex].Name()
		} else {
			// Based on knowledge of how the AZs are setup above
			// in AvailabilityZones()
			azIndex += 1
			returnValue[i] = availabilityZones[azIndex].Name()
		}
		azIndex += 1
		if azIndex == azMaxIndex {
			azIndex = 0
		}
	}
	return returnValue, nil
}

// DeriveAvailabilityZones is part of the common.ZonedEnviron interface.
func (env *environ) DeriveAvailabilityZones(ctx context.ProviderCallContext, args environs.StartInstanceParams) ([]string, error) {
	return nil, nil
}

// Subnets implements environs.Environ.Subnets.
func (env *environ) Subnets(
	ctx context.ProviderCallContext, instId instance.Id, subnetIds []corenetwork.Id,
) ([]corenetwork.SubnetInfo, error) {
	if err := env.checkBroken("Subnets"); err != nil {
		return nil, err
	}

	estate, err := env.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()

	if ok, _ := env.SupportsSpaceDiscovery(ctx); ok {
		// Space discovery needs more subnets to work with.
		return env.subnetsForSpaceDiscovery(estate)
	}

	allSubnets := []corenetwork.SubnetInfo{{
		CIDR:              "0.10.0.0/24",
		ProviderId:        "dummy-private",
		AvailabilityZones: []string{"zone1", "zone2"},
	}, {
		CIDR:       "0.20.0.0/24",
		ProviderId: "dummy-public",
	}}

	// Filter result by ids, if given.
	var result []corenetwork.SubnetInfo
	for _, subId := range subnetIds {
		switch subId {
		case "dummy-private":
			result = append(result, allSubnets[0])
		case "dummy-public":
			result = append(result, allSubnets[1])
		}
	}
	if len(subnetIds) == 0 {
		result = append([]corenetwork.SubnetInfo{}, allSubnets...)
	}
	if len(result) == 0 {
		// No results, so just return them now.
		estate.ops <- OpSubnets{
			Env:        env.name,
			InstanceId: instId,
			SubnetIds:  subnetIds,
			Info:       result,
		}
		return result, nil
	}

	estate.ops <- OpSubnets{
		Env:        env.name,
		InstanceId: instId,
		SubnetIds:  subnetIds,
		Info:       result,
	}
	return result, nil
}

func (env *environ) subnetsForSpaceDiscovery(estate *environState) ([]corenetwork.SubnetInfo, error) {
	result := []corenetwork.SubnetInfo{{
		ProviderId:        corenetwork.Id("1"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.1.0/24",
	}, {
		ProviderId:        corenetwork.Id("2"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.2.0/24",
		VLANTag:           1,
	}, {
		ProviderId:        corenetwork.Id("3"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.3.0/24",
	}, {
		ProviderId:        corenetwork.Id("4"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.4.0/24",
	}, {
		ProviderId:        corenetwork.Id("5"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.5.0/24",
	}}
	estate.ops <- OpSubnets{
		Env:        env.name,
		InstanceId: instance.UnknownId,
		SubnetIds:  []corenetwork.Id{},
		Info:       result,
	}
	return result, nil
}

func (e *environ) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return e.instancesForMethod(ctx, "AllInstances")
}

func (e *environ) AllRunningInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return e.instancesForMethod(ctx, "AllRunningInstances")
}

func (e *environ) instancesForMethod(ctx context.ProviderCallContext, method string) ([]instances.Instance, error) {
	defer delay()
	if err := e.checkBroken(method); err != nil {
		return nil, err
	}
	var insts []instances.Instance
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, v := range estate.insts {
		insts = append(insts, v)
	}
	return insts, nil
}

func (e *environ) OpenPorts(ctx context.ProviderCallContext, rules []network.IngressRule) error {
	if mode := e.ecfg().FirewallMode(); mode != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for opening ports on model", mode)
	}
	estate, err := e.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, r := range rules {
		if len(r.SourceCIDRs) == 0 {
			r.SourceCIDRs = []string{"0.0.0.0/0"}
		}
		found := false
		for _, rule := range estate.globalRules {
			if r.String() == rule.String() {
				found = true
			}
		}
		if !found {
			estate.globalRules = append(estate.globalRules, r)
		}
	}

	return nil
}

func (e *environ) ClosePorts(ctx context.ProviderCallContext, rules []network.IngressRule) error {
	if mode := e.ecfg().FirewallMode(); mode != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for closing ports on model", mode)
	}
	estate, err := e.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, r := range rules {
		for i, rule := range estate.globalRules {
			if r.String() == rule.String() {
				estate.globalRules = estate.globalRules[:i+copy(estate.globalRules[i:], estate.globalRules[i+1:])]
			}
		}
	}
	return nil
}

func (e *environ) IngressRules(ctx context.ProviderCallContext) (rules []network.IngressRule, err error) {
	if mode := e.ecfg().FirewallMode(); mode != config.FwGlobal {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ingress rules from model", mode)
	}
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, r := range estate.globalRules {
		rules = append(rules, r)
	}
	network.SortIngressRules(rules)
	return
}

func (*environ) Provider() environs.EnvironProvider {
	return &dummy
}

type dummyInstance struct {
	state        *environState
	rules        network.IngressRuleSlice
	id           instance.Id
	status       string
	machineId    string
	series       string
	firewallMode string
	controller   bool

	mu        sync.Mutex
	addresses []network.Address
	broken    []string
}

func (inst *dummyInstance) Id() instance.Id {
	return inst.id
}

func (inst *dummyInstance) Status(ctx context.ProviderCallContext) instance.Status {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	// TODO(perrito666) add a provider status -> juju status mapping.
	jujuStatus := status.Pending
	if inst.status != "" {
		dummyStatus := status.Status(inst.status)
		if dummyStatus.KnownInstanceStatus() {
			jujuStatus = dummyStatus
		}
	}

	return instance.Status{
		Status:  jujuStatus,
		Message: inst.status,
	}

}

// SetInstanceAddresses sets the addresses associated with the given
// dummy instance.
func SetInstanceAddresses(inst instances.Instance, addrs []network.Address) {
	inst0 := inst.(*dummyInstance)
	inst0.mu.Lock()
	inst0.addresses = append(inst0.addresses[:0], addrs...)
	logger.Debugf("setting instance %q addresses to %v", inst0.Id(), addrs)
	inst0.mu.Unlock()
}

// SetInstanceStatus sets the status associated with the given
// dummy instance.
func SetInstanceStatus(inst instances.Instance, status string) {
	inst0 := inst.(*dummyInstance)
	inst0.mu.Lock()
	inst0.status = status
	inst0.mu.Unlock()
}

// SetInstanceBroken marks the named methods of the instance as broken.
// Any previously broken methods not in the set will no longer be broken.
func SetInstanceBroken(inst instances.Instance, methods ...string) {
	inst0 := inst.(*dummyInstance)
	inst0.mu.Lock()
	inst0.broken = methods
	inst0.mu.Unlock()
}

func (inst *dummyInstance) checkBroken(method string) error {
	for _, m := range inst.broken {
		if m == method {
			return fmt.Errorf("dummyInstance.%s is broken", method)
		}
	}
	return nil
}

func (inst *dummyInstance) Addresses(ctx context.ProviderCallContext) ([]network.Address, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if err := inst.checkBroken("Addresses"); err != nil {
		return nil, err
	}
	return append([]network.Address{}, inst.addresses...), nil
}

func (inst *dummyInstance) OpenPorts(ctx context.ProviderCallContext, machineId string, rules []network.IngressRule) error {
	defer delay()
	logger.Infof("openPorts %s, %#v", machineId, rules)
	if inst.firewallMode != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for opening ports on instance",
			inst.firewallMode)
	}
	if inst.machineId != machineId {
		panic(fmt.Errorf("OpenPorts with mismatched machine id, expected %q got %q", inst.machineId, machineId))
	}
	inst.state.mu.Lock()
	defer inst.state.mu.Unlock()
	if err := inst.checkBroken("OpenPorts"); err != nil {
		return err
	}
	inst.state.ops <- OpOpenPorts{
		Env:        inst.state.name,
		MachineId:  machineId,
		InstanceId: inst.Id(),
		Rules:      rules,
	}
	for _, r := range rules {
		if len(r.SourceCIDRs) == 0 {
			r.SourceCIDRs = []string{"0.0.0.0/0"}
		}
		found := false
		for i, rule := range inst.rules {
			if r.PortRange == rule.PortRange {
				ruleCopy := r
				inst.rules[i] = ruleCopy
				found = true
				break
			}
			if r.String() == rule.String() {
				found = true
				break
			}
		}
		if !found {
			inst.rules = append(inst.rules, r)
		}
	}
	return nil
}

func (inst *dummyInstance) ClosePorts(ctx context.ProviderCallContext, machineId string, rules []network.IngressRule) error {
	defer delay()
	if inst.firewallMode != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for closing ports on instance",
			inst.firewallMode)
	}
	if inst.machineId != machineId {
		panic(fmt.Errorf("ClosePorts with mismatched machine id, expected %s got %s", inst.machineId, machineId))
	}
	inst.state.mu.Lock()
	defer inst.state.mu.Unlock()
	if err := inst.checkBroken("ClosePorts"); err != nil {
		return err
	}
	inst.state.ops <- OpClosePorts{
		Env:        inst.state.name,
		MachineId:  machineId,
		InstanceId: inst.Id(),
		Rules:      rules,
	}
	for _, r := range rules {
		for i, rule := range inst.rules {
			if r.String() == rule.String() {
				inst.rules = inst.rules[:i+copy(inst.rules[i:], inst.rules[i+1:])]
			}
		}
	}
	return nil
}

func (inst *dummyInstance) IngressRules(ctx context.ProviderCallContext, machineId string) (rules []network.IngressRule, err error) {
	defer delay()
	if inst.firewallMode != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ingress rules from instance",
			inst.firewallMode)
	}
	if inst.machineId != machineId {
		panic(fmt.Errorf("Rules with mismatched machine id, expected %q got %q", inst.machineId, machineId))
	}
	inst.state.mu.Lock()
	defer inst.state.mu.Unlock()
	if err := inst.checkBroken("IngressRules"); err != nil {
		return nil, err
	}
	for _, r := range inst.rules {
		rules = append(rules, r)
	}
	network.SortIngressRules(rules)
	return
}

// providerDelay controls the delay before dummy responds.
// non empty values in JUJU_DUMMY_DELAY will be parsed as
// time.Durations into this value.
var providerDelay, _ = time.ParseDuration(os.Getenv("JUJU_DUMMY_DELAY")) // parse errors are ignored

// pause execution to simulate the latency of a real provider
func delay() {
	if providerDelay > 0 {
		logger.Infof("pausing for %v", providerDelay)
		<-time.After(providerDelay)
	}
}

func (e *environ) AllocateContainerAddresses(ctx context.ProviderCallContext, hostInstanceID instance.Id, containerTag names.MachineTag, preparedInfo []network.InterfaceInfo) ([]network.InterfaceInfo, error) {
	return nil, errors.NotSupportedf("container address allocation")
}

func (e *environ) ReleaseContainerAddresses(ctx context.ProviderCallContext, interfaces []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container address allocation")
}

// ProviderSpaceInfo implements NetworkingEnviron.
func (*environ) ProviderSpaceInfo(
	ctx context.ProviderCallContext, space *corenetwork.SpaceInfo,
) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("provider space info")
}

// AreSpacesRoutable implements NetworkingEnviron.
func (*environ) AreSpacesRoutable(ctx context.ProviderCallContext, space1, space2 *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

// MaybeWriteLXDProfile implements environs.LXDProfiler.
func (env *environ) MaybeWriteLXDProfile(pName string, put *charm.LXDProfile) error {
	return nil
}

// LXDProfileNames implements environs.LXDProfiler.
func (env *environ) LXDProfileNames(containerName string) ([]string, error) {
	return nil, nil
}

// AssignLXDProfiles implements environs.LXDProfiler.
func (env *environ) AssignLXDProfiles(instId string, profilesNames []string, profilePosts []lxdprofile.ProfilePost) (current []string, err error) {
	return profilesNames, nil
}

// SSHAddresses implements environs.SSHAddresses.
// For testing we cut "100.100.100.100" out of this list.
func (*environ) SSHAddresses(ctx context.ProviderCallContext, addresses []network.Address) ([]network.Address, error) {
	var rv []network.Address
	for _, addr := range addresses {
		if addr.Value != "100.100.100.100" {
			rv = append(rv, addr)
		}
	}
	return rv, nil
}

// SuperSubnets implements environs.SuperSubnets
func (*environ) SuperSubnets(ctx context.ProviderCallContext) ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
}

// SetAgentStatus sets the presence for a particular agent in the fake presence implementation.
func (e *environ) SetAgentStatus(agent string, status presence.Status) {
	estate, err := e.state()
	if err != nil {
		panic(err)
	}
	estate.presence.agent[agent] = status
}

// fakePresence returns alive for all agent alive requests.
type fakePresence struct {
	agent map[string]presence.Status
}

func (*fakePresence) Disable()        {}
func (*fakePresence) Enable()         {}
func (*fakePresence) IsEnabled() bool { return true }
func (*fakePresence) Connect(server, model, agent string, id uint64, controllerAgent bool, userData string) {
}
func (*fakePresence) Disconnect(server string, id uint64)                            {}
func (*fakePresence) Activity(server string, id uint64)                              {}
func (*fakePresence) ServerDown(server string)                                       {}
func (*fakePresence) UpdateServer(server string, connections []presence.Value) error { return nil }
func (f *fakePresence) Connections() presence.Connections                            { return f }

func (f *fakePresence) ForModel(model string) presence.Connections   { return f }
func (f *fakePresence) ForServer(server string) presence.Connections { return f }
func (f *fakePresence) ForAgent(agent string) presence.Connections   { return f }
func (*fakePresence) Count() int                                     { return 0 }
func (*fakePresence) Models() []string                               { return nil }
func (*fakePresence) Servers() []string                              { return nil }
func (*fakePresence) Agents() []string                               { return nil }
func (*fakePresence) Values() []presence.Value                       { return nil }

func (f *fakePresence) AgentStatus(agent string) (presence.Status, error) {
	if status, found := f.agent[agent]; found {
		return status, nil
	}
	return presence.Alive, nil
}

type noopRegisterer struct {
	prometheus.Registerer
}

func (noopRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (noopRegisterer) Unregister(prometheus.Collector) bool {
	return true
}
