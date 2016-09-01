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
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/schema"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

var logger = loggo.GetLogger("juju.provider.dummy")

var transientErrorInjection chan error

const BootstrapInstanceId = "localhost"

var errNotPrepared = errors.New("model is not prepared")

// SampleCloudSpec returns an environs.CloudSpec that can be used to
// open a dummy Environ.
func SampleCloudSpec() environs.CloudSpec {
	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"username": "dummy", "passeord": "secret"})
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
		"default-series":            series.LatestLts(),

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

// stateInfo returns a *state.Info which allows clients to connect to the
// shared dummy state, if it exists.
func stateInfo() *mongo.MongoInfo {
	if gitjujutesting.MgoServer.Addr() == "" {
		panic("dummy environ state tests must be run with MgoTestPackage")
	}
	mongoPort := strconv.Itoa(gitjujutesting.MgoServer.Port())
	addrs := []string{net.JoinHostPort("localhost", mongoPort)}
	return &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  addrs,
			CACert: testing.CACert,
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
	SubnetIds  []network.Id
	Info       []network.SubnetInfo
}

type OpStartInstance struct {
	Env              string
	MachineId        string
	MachineNonce     string
	PossibleTools    coretools.List
	Instance         instance.Instance
	Constraints      constraints.Value
	SubnetsToZones   map[network.Id][]string
	NetworkInfo      []network.InterfaceInfo
	Volumes          []storage.Volume
	Info             *mongo.MongoInfo
	Jobs             []multiwatcher.MachineJob
	APIInfo          *api.Info
	Secret           string
	AgentEnvironment map[string]string
}

type OpStopInstances struct {
	Env string
	Ids []instance.Id
}

type OpOpenPorts struct {
	Env        string
	MachineId  string
	InstanceId instance.Id
	Ports      []network.PortRange
}

type OpClosePorts struct {
	Env        string
	MachineId  string
	InstanceId instance.Id
	Ports      []network.PortRange
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

// ApiPort returns the randon api port used by the given provider instance.
func ApiPort(p environs.EnvironProvider) int {
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
	globalPorts    map[network.PortRange]bool
	bootstrapped   bool
	apiListener    net.Listener
	apiServer      *apiserver.Server
	apiState       *state.State
	apiStatePool   *state.StatePool
	creator        string
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

// discardOperations discards all Operations written to it.
var discardOperations = make(chan Operation)

func init() {
	environs.RegisterProvider("dummy", &dummy)

	// Prime the first ops channel, so that naive clients can use
	// the testing environment by simply importing it.
	go func() {
		for _ = range discardOperations {
		}
	}()
}

// dummy is the dummy environmentProvider singleton.
var dummy = environProvider{
	ops:   discardOperations,
	state: make(map[string]*environState),
	newStatePolicy: stateenvirons.GetNewPolicyFunc(
		stateenvirons.GetNewEnvironFunc(environs.New),
	),
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
	dummy.newStatePolicy = stateenvirons.GetNewPolicyFunc(
		stateenvirons.GetNewEnvironFunc(environs.New),
	)
	dummy.supportsSpaces = true
	dummy.supportsSpaceDiscovery = false
	dummy.mu.Unlock()

	// NOTE(axw) we must destroy the old states without holding
	// the provider lock, or we risk deadlocking. Destroying
	// state involves closing the embedded API server, which
	// may require waiting on RPC calls that interact with the
	// EnvironProvider (e.g. EnvironProvider.Open).
	for _, s := range oldState {
		if s.apiListener != nil {
			s.apiListener.Close()
		}
		s.destroy()
	}
	if mongoAlive() {
		err := gitjujutesting.MgoServer.Reset()
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
	apiState := state.apiState
	state.apiServer = nil
	state.apiStatePool = nil
	state.apiState = nil
	state.bootstrapped = false

	// Release the lock while we close resources. In particular,
	// we must not hold the lock while the API server is being
	// closed, as it may need to interact with the Environ while
	// shutting down.
	state.mu.Unlock()
	defer state.mu.Lock()

	if apiServer != nil {
		if err := apiServer.Stop(); err != nil && mongoAlive() {
			panic(err)
		}
	}

	if apiStatePool != nil {
		if err := apiStatePool.Close(); err != nil && mongoAlive() {
			panic(err)
		}
	}

	if apiState != nil {
		if err := apiState.Close(); err != nil && mongoAlive() {
			panic(err)
		}
	}

	if mongoAlive() {
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

// newState creates the state for a new environment with the given name.
func newState(name string, ops chan<- Operation, newStatePolicy state.NewPolicyFunc) *environState {
	buf := make([]byte, 8192)
	buf = buf[:runtime.Stack(buf, false)]
	s := &environState{
		name:           name,
		ops:            ops,
		newStatePolicy: newStatePolicy,
		insts:          make(map[instance.Id]*dummyInstance),
		globalPorts:    make(map[network.PortRange]bool),
		creator:        string(buf),
	}
	return s
}

// listenAPI starts a network listener listening for API
// connections and proxies them to the API server port.
func (s *environState) listenAPI() int {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(fmt.Errorf("cannot start listener: %v", err))
	}
	s.apiListener = l
	return l.Addr().(*net.TCPAddr).Port
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
func (p environProvider) ConfigSchema() schema.Fields {
	return configFields
}

// ConfigDefaults returns the default values for the
// provider specific config attributes.
func (p environProvider) ConfigDefaults() schema.Defaults {
	return configDefaults
}

func (environProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
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

// PrecheckInstance is specified in the state.Prechecker interface.
func (*environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if placement != "" && placement != "valid" {
		return fmt.Errorf("%s placement is invalid", placement)
	}
	return nil
}

// Create is part of the Environ interface.
func (e *environ) Create(args environs.CreateParams) error {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	dummy.state[e.modelUUID] = newState(e.name, dummy.ops, dummy.newStatePolicy)
	return nil
}

// PrepareForBootstrap is part of the Environ interface.
func (e *environ) PrepareForBootstrap(ctx environs.BootstrapContext) error {
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

func (e *environ) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
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

	logger.Infof("would pick tools from %s", availableTools)

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
		ports:        make(map[network.PortRange]bool),
		machineId:    agent.BootstrapMachineId,
		series:       series,
		firewallMode: e.Config().FirewallMode(),
		state:        estate,
		controller:   true,
	}
	estate.insts[i.id] = i
	estate.bootstrapped = true
	estate.ops <- OpBootstrap{Context: ctx, Env: e.name, Args: args}

	finalize := func(ctx environs.BootstrapContext, icfg *instancecfg.InstanceConfig, _ environs.BootstrapDialOpts) error {
		if e.ecfg().controller() {
			icfg.Bootstrap.BootstrapMachineInstanceId = BootstrapInstanceId
			if err := instancecfg.FinishInstanceConfig(icfg, e.Config()); err != nil {
				return err
			}

			adminUser := names.NewUserTag("admin@local")
			var cloudCredentialTag names.CloudCredentialTag
			if icfg.Bootstrap.ControllerCloudCredentialName != "" {
				cloudCredentialTag = names.NewCloudCredentialTag(fmt.Sprintf(
					"%s/%s/%s",
					icfg.Bootstrap.ControllerCloudName,
					adminUser.Canonical(),
					icfg.Bootstrap.ControllerCloudCredentialName,
				))
			}

			cloudCredentials := make(map[names.CloudCredentialTag]cloud.Credential)
			if icfg.Bootstrap.ControllerCloudCredential != nil && icfg.Bootstrap.ControllerCloudCredentialName != "" {
				cloudCredentials[cloudCredentialTag] = *icfg.Bootstrap.ControllerCloudCredential
			}

			info := stateInfo()
			// Since the admin user isn't setup until after here,
			// the password in the info structure is empty, so the admin
			// user is constructed with an empty password here.
			// It is set just below.
			st, err := state.Initialize(state.InitializeParams{
				ControllerConfig: icfg.Controller.Config,
				ControllerModelArgs: state.ModelArgs{
					Owner:                   adminUser,
					Config:                  icfg.Bootstrap.ControllerModelConfig,
					Constraints:             icfg.Bootstrap.BootstrapMachineConstraints,
					CloudName:               icfg.Bootstrap.ControllerCloudName,
					CloudRegion:             icfg.Bootstrap.ControllerCloudRegion,
					CloudCredential:         cloudCredentialTag,
					StorageProviderRegistry: e,
				},
				Cloud:            icfg.Bootstrap.ControllerCloud,
				CloudName:        icfg.Bootstrap.ControllerCloudName,
				CloudCredentials: cloudCredentials,
				MongoInfo:        info,
				MongoDialOpts:    mongotest.DialOpts(),
				NewPolicy:        estate.newStatePolicy,
			})
			if err != nil {
				return err
			}
			if err := st.SetModelConstraints(args.ModelConstraints); err != nil {
				return err
			}
			if err := st.SetAdminMongoPassword(icfg.Controller.MongoInfo.Password); err != nil {
				return err
			}
			if err := st.MongoSession().DB("admin").Login("admin", icfg.Controller.MongoInfo.Password); err != nil {
				return err
			}
			env, err := st.Model()
			if err != nil {
				return err
			}
			owner, err := st.User(env.Owner())
			if err != nil {
				return err
			}
			// We log this out for test purposes only. No one in real life can use
			// a dummy provider for anything other than testing, so logging the password
			// here is fine.
			logger.Debugf("setting password for %q to %q", owner.Name(), icfg.Controller.MongoInfo.Password)
			owner.SetPassword(icfg.Controller.MongoInfo.Password)

			estate.apiStatePool = state.NewStatePool(st)

			estate.apiServer, err = apiserver.NewServer(st, estate.apiListener, apiserver.ServerConfig{
				Cert:        []byte(testing.ServerCert),
				Key:         []byte(testing.ServerKey),
				Tag:         names.NewMachineTag("0"),
				DataDir:     DataDir,
				LogDir:      LogDir,
				StatePool:   estate.apiStatePool,
				NewObserver: func() observer.Observer { return &fakeobserver.Instance{} },
			})
			if err != nil {
				panic(err)
			}
			estate.apiState = st
		}
		estate.ops <- OpFinalizeBootstrap{Context: ctx, Env: e.name, InstanceConfig: icfg}
		return nil
	}

	bsResult := &environs.BootstrapResult{
		Arch:     arch,
		Series:   series,
		Finalize: finalize,
	}
	return bsResult, nil
}

func (e *environ) ControllerInstances(controllerUUID string) ([]instance.Id, error) {
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

func (e *environ) Destroy() (res error) {
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

func (e *environ) DestroyController(controllerUUID string) error {
	if err := e.Destroy(); err != nil {
		return err
	}
	dummy.mu.Lock()
	dummy.controllerState = nil
	dummy.mu.Unlock()
	return nil
}

// ConstraintsValidator is defined on the Environs interface.
func (e *environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported([]string{constraints.CpuPower, constraints.VirtType})
	validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
	validator.RegisterVocabulary(constraints.Arch, []string{arch.AMD64, arch.ARM64, arch.I386, arch.PPC64EL})
	return validator, nil
}

// MaintainInstance is specified in the InstanceBroker interface.
func (*environ) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
}

// StartInstance is specified in the InstanceBroker interface.
func (e *environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {

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
	logger.Infof("would pick tools from %s", args.Tools)
	series := args.Tools.OneSeries()

	idString := fmt.Sprintf("%s-%d", e.name, estate.maxId)
	addrs := network.NewAddresses(idString+".dns", "127.0.0.1")
	logger.Debugf("StartInstance addresses: %v", addrs)
	i := &dummyInstance{
		id:           instance.Id(idString),
		addresses:    addrs,
		ports:        make(map[network.PortRange]bool),
		machineId:    machineId,
		series:       series,
		firewallMode: e.Config().FirewallMode(),
		state:        estate,
	}

	var hc *instance.HardwareCharacteristics
	// To match current system capability, only provide hardware characteristics for
	// environ machines, not containers.
	if state.ParentId(machineId) == "" {
		// We will just assume the instance hardware characteristics exactly matches
		// the supplied constraints (if specified).
		hc = &instance.HardwareCharacteristics{
			Arch:     args.Constraints.Arch,
			Mem:      args.Constraints.Mem,
			RootDisk: args.Constraints.RootDisk,
			CpuCores: args.Constraints.CpuCores,
			CpuPower: args.Constraints.CpuPower,
			Tags:     args.Constraints.Tags,
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
	var subnetsToZones map[network.Id][]string
	for isp := range spaces {
		// Simulate 2 subnets per space.
		if subnetsToZones == nil {
			subnetsToZones = make(map[network.Id][]string)
		}
		for isn := 0; isn < 2; isn++ {
			providerId := fmt.Sprintf("subnet-%d", isp+isn)
			zone := fmt.Sprintf("zone%d", isp+isn)
			subnetsToZones[network.Id(providerId)] = []string{zone}
		}
	}
	// Simulate creating volumes when requested.
	volumes := make([]storage.Volume, len(args.Volumes))
	for iv, v := range args.Volumes {
		persistent, _ := v.Attributes["persistent"].(bool)
		volumes[iv] = storage.Volume{
			Tag: names.NewVolumeTag(strconv.Itoa(iv + 1)),
			VolumeInfo: storage.VolumeInfo{
				Size:       v.Size,
				Persistent: persistent,
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
		Env:              e.name,
		MachineId:        machineId,
		MachineNonce:     args.InstanceConfig.MachineNonce,
		PossibleTools:    args.Tools,
		Constraints:      args.Constraints,
		SubnetsToZones:   subnetsToZones,
		Volumes:          volumes,
		Instance:         i,
		Jobs:             args.InstanceConfig.Jobs,
		Info:             mongoInfo,
		APIInfo:          args.InstanceConfig.APIInfo,
		AgentEnvironment: args.InstanceConfig.AgentEnvironment,
		Secret:           e.ecfg().secret(),
	}
	return &environs.StartInstanceResult{
		Instance: i,
		Hardware: hc,
	}, nil
}

func (e *environ) StopInstances(ids ...instance.Id) error {
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

func (e *environ) Instances(ids []instance.Id) (insts []instance.Instance, err error) {
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
func (env *environ) SupportsSpaces() (bool, error) {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	if !dummy.supportsSpaces {
		return false, errors.NotSupportedf("spaces")
	}
	return true, nil
}

// SupportsSpaceDiscovery is specified on environs.Networking.
func (env *environ) SupportsSpaceDiscovery() (bool, error) {
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

// Spaces is specified on environs.Networking.
func (env *environ) Spaces() ([]network.SpaceInfo, error) {
	if err := env.checkBroken("Spaces"); err != nil {
		return []network.SpaceInfo{}, err
	}
	return []network.SpaceInfo{{
		Name:       "foo",
		ProviderId: network.Id("0"),
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("1"),
			AvailabilityZones: []string{"zone1"},
		}, {
			ProviderId:        network.Id("2"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "Another Foo 99!",
		ProviderId: "1",
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("3"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "foo-",
		ProviderId: "2",
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("4"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "---",
		ProviderId: "3",
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("5"),
			AvailabilityZones: []string{"zone1"},
		}}}}, nil
}

// NetworkInterfaces implements Environ.NetworkInterfaces().
func (env *environ) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
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
			ProviderId:       network.Id(fmt.Sprintf("dummy-eth%d", i)),
			ProviderSubnetId: network.Id("dummy-" + netName),
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
func (env *environ) AvailabilityZones() ([]common.AvailabilityZone, error) {
	// TODO(dimitern): Fix this properly.
	return []common.AvailabilityZone{
		azShim{"zone1", true},
		azShim{"zone2", false},
	}, nil
}

// InstanceAvailabilityZoneNames implements environs.ZonedEnviron.
func (env *environ) InstanceAvailabilityZoneNames(ids []instance.Id) ([]string, error) {
	// TODO(dimitern): Fix this properly.
	if err := env.checkBroken("InstanceAvailabilityZoneNames"); err != nil {
		return nil, errors.NotSupportedf("instance availability zones")
	}
	return []string{"zone1"}, nil
}

// Subnets implements environs.Environ.Subnets.
func (env *environ) Subnets(instId instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	if err := env.checkBroken("Subnets"); err != nil {
		return nil, err
	}

	estate, err := env.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()

	if ok, _ := env.SupportsSpaceDiscovery(); ok {
		// Space discovery needs more subnets to work with.
		return env.subnetsForSpaceDiscovery(estate)
	}

	allSubnets := []network.SubnetInfo{{
		CIDR:              "0.10.0.0/24",
		ProviderId:        "dummy-private",
		AvailabilityZones: []string{"zone1", "zone2"},
	}, {
		CIDR:       "0.20.0.0/24",
		ProviderId: "dummy-public",
	}}

	// Filter result by ids, if given.
	var result []network.SubnetInfo
	for _, subId := range subnetIds {
		switch subId {
		case "dummy-private":
			result = append(result, allSubnets[0])
		case "dummy-public":
			result = append(result, allSubnets[1])
		}
	}
	if len(subnetIds) == 0 {
		result = append([]network.SubnetInfo{}, allSubnets...)
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

func (env *environ) subnetsForSpaceDiscovery(estate *environState) ([]network.SubnetInfo, error) {
	result := []network.SubnetInfo{{
		ProviderId:        network.Id("1"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.1.0/24",
	}, {
		ProviderId:        network.Id("2"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.2.0/24",
		VLANTag:           1,
	}, {
		ProviderId:        network.Id("3"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.3.0/24",
	}, {
		ProviderId:        network.Id("4"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.4.0/24",
	}, {
		ProviderId:        network.Id("5"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.5.0/24",
	}}
	estate.ops <- OpSubnets{
		Env:        env.name,
		InstanceId: instance.UnknownId,
		SubnetIds:  []network.Id{},
		Info:       result,
	}
	return result, nil
}

func (e *environ) AllInstances() ([]instance.Instance, error) {
	defer delay()
	if err := e.checkBroken("AllInstances"); err != nil {
		return nil, err
	}
	var insts []instance.Instance
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

func (e *environ) OpenPorts(ports []network.PortRange) error {
	if mode := e.ecfg().FirewallMode(); mode != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for opening ports on model", mode)
	}
	estate, err := e.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, p := range ports {
		estate.globalPorts[p] = true
	}
	return nil
}

func (e *environ) ClosePorts(ports []network.PortRange) error {
	if mode := e.ecfg().FirewallMode(); mode != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for closing ports on model", mode)
	}
	estate, err := e.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, p := range ports {
		delete(estate.globalPorts, p)
	}
	return nil
}

func (e *environ) Ports() (ports []network.PortRange, err error) {
	if mode := e.ecfg().FirewallMode(); mode != config.FwGlobal {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from model", mode)
	}
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for p := range estate.globalPorts {
		ports = append(ports, p)
	}
	network.SortPortRanges(ports)
	return
}

func (*environ) Provider() environs.EnvironProvider {
	return &dummy
}

type dummyInstance struct {
	state        *environState
	ports        map[network.PortRange]bool
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

func (inst *dummyInstance) Status() instance.InstanceStatus {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	// TODO(perrito666) add a provider status -> juju status mapping.
	jujuStatus := status.StatusPending
	if inst.status != "" {
		dummyStatus := status.Status(inst.status)
		if dummyStatus.KnownInstanceStatus() {
			jujuStatus = dummyStatus
		}
	}

	return instance.InstanceStatus{
		Status:  jujuStatus,
		Message: inst.status,
	}

}

// SetInstanceAddresses sets the addresses associated with the given
// dummy instance.
func SetInstanceAddresses(inst instance.Instance, addrs []network.Address) {
	inst0 := inst.(*dummyInstance)
	inst0.mu.Lock()
	inst0.addresses = append(inst0.addresses[:0], addrs...)
	logger.Debugf("setting instance %q addresses to %v", inst0.Id(), addrs)
	inst0.mu.Unlock()
}

// SetInstanceStatus sets the status associated with the given
// dummy instance.
func SetInstanceStatus(inst instance.Instance, status string) {
	inst0 := inst.(*dummyInstance)
	inst0.mu.Lock()
	inst0.status = status
	inst0.mu.Unlock()
}

// SetInstanceBroken marks the named methods of the instance as broken.
// Any previously broken methods not in the set will no longer be broken.
func SetInstanceBroken(inst instance.Instance, methods ...string) {
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

func (inst *dummyInstance) Addresses() ([]network.Address, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if err := inst.checkBroken("Addresses"); err != nil {
		return nil, err
	}
	return append([]network.Address{}, inst.addresses...), nil
}

func (inst *dummyInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	defer delay()
	logger.Infof("openPorts %s, %#v", machineId, ports)
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
		Ports:      ports,
	}
	for _, p := range ports {
		inst.ports[p] = true
	}
	return nil
}

func (inst *dummyInstance) ClosePorts(machineId string, ports []network.PortRange) error {
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
		Ports:      ports,
	}
	for _, p := range ports {
		delete(inst.ports, p)
	}
	return nil
}

func (inst *dummyInstance) Ports(machineId string) (ports []network.PortRange, err error) {
	defer delay()
	if inst.firewallMode != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from instance",
			inst.firewallMode)
	}
	if inst.machineId != machineId {
		panic(fmt.Errorf("Ports with mismatched machine id, expected %q got %q", inst.machineId, machineId))
	}
	inst.state.mu.Lock()
	defer inst.state.mu.Unlock()
	if err := inst.checkBroken("Ports"); err != nil {
		return nil, err
	}
	for p := range inst.ports {
		ports = append(ports, p)
	}
	network.SortPortRanges(ports)
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

func (e *environ) AllocateContainerAddresses(hostInstanceID instance.Id, containerTag names.MachineTag, preparedInfo []network.InterfaceInfo) ([]network.InterfaceInfo, error) {
	return nil, errors.NotSupportedf("container address allocation")
}

func (e *environ) ReleaseContainerAddresses(interfaces []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container address allocation")
}
