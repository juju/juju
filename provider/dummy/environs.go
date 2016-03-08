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
//
// To avoid enumerating all possible series and architectures,
// any series or architecture with the prefix "unknown" is
// treated as bad when starting a new instance.
package dummy

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/schema"
	gitjujutesting "github.com/juju/testing"
	"github.com/juju/utils/arch"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

var logger = loggo.GetLogger("juju.provider.dummy")

var transientErrorInjection chan error

const (
	BootstrapInstanceId = instance.Id("localhost")
)

var (
	ErrNotPrepared = errors.New("model is not prepared")
	ErrDestroyed   = errors.New("model has been destroyed")
)

// SampleConfig() returns an environment configuration with all required
// attributes set.
func SampleConfig() testing.Attrs {
	return testing.Attrs{
		"type":                      "dummy",
		"name":                      "only",
		"uuid":                      testing.ModelTag.Id(),
		"authorized-keys":           testing.FakeAuthKeys,
		"firewall-mode":             config.FwInstance,
		"admin-secret":              testing.DefaultMongoPassword,
		"ca-cert":                   testing.CACert,
		"ca-private-key":            testing.CAKey,
		"ssl-hostname-verification": true,
		"development":               false,
		"state-port":                1234,
		"api-port":                  4321,
		"default-series":            config.LatestLtsSeries(),

		"secret":      "pork",
		"controller":  true,
		"prefer-ipv6": true,
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
// shared dummy state, if it exists. If preferIPv6 is true, an IPv6 endpoint
// will be added as primary.
func stateInfo(preferIPv6 bool) *mongo.MongoInfo {
	if gitjujutesting.MgoServer.Addr() == "" {
		panic("dummy environ state tests must be run with MgoTestPackage")
	}
	mongoPort := strconv.Itoa(gitjujutesting.MgoServer.Port())
	var addrs []string
	if preferIPv6 {
		addrs = []string{
			net.JoinHostPort("::1", mongoPort),
			net.JoinHostPort("localhost", mongoPort),
		}
	} else {
		addrs = []string{net.JoinHostPort("localhost", mongoPort)}
	}
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
	Env   string
	Error error
}

type OpAllocateAddress struct {
	Env        string
	InstanceId instance.Id
	SubnetId   network.Id
	Address    network.Address
	MACAddress string
	HostName   string
}

type OpReleaseAddress struct {
	Env        string
	InstanceId instance.Id
	SubnetId   network.Id
	Address    network.Address
	MACAddress string
	HostName   string
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
	Networks         []string
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
// instance of this type (providerInstance)
type environProvider struct {
	mu                     sync.Mutex
	ops                    chan<- Operation
	statePolicy            state.Policy
	supportsSpaces         bool
	supportsSpaceDiscovery bool
	// We have one state for each environment name.
	state      map[int]*environState
	maxStateId int
}

var providerInstance environProvider

const noStateId = 0

// environState represents the state of an environment.
// It can be shared between several environ values,
// so that a given environment can be opened several times.
type environState struct {
	id           int
	name         string
	ops          chan<- Operation
	statePolicy  state.Policy
	mu           sync.Mutex
	maxId        int // maximum instance id allocated so far.
	maxAddr      int // maximum allocated address last byte
	insts        map[instance.Id]*dummyInstance
	globalPorts  map[network.PortRange]bool
	bootstrapped bool
	apiListener  net.Listener
	apiServer    *apiserver.Server
	apiState     *state.State
	preferIPv6   bool
}

// environ represents a client's connection to a given environment's
// state.
type environ struct {
	common.SupportsUnitPlacementPolicy

	name         string
	ecfgMutex    sync.Mutex
	ecfgUnlocked *environConfig
	spacesMutex  sync.RWMutex
}

var _ environs.Environ = (*environ)(nil)

// discardOperations discards all Operations written to it.
var discardOperations chan<- Operation

func init() {
	environs.RegisterProvider("dummy", &providerInstance)

	// Prime the first ops channel, so that naive clients can use
	// the testing environment by simply importing it.
	c := make(chan Operation)
	go func() {
		for _ = range c {
		}
	}()
	discardOperations = c
	if err := Reset(); err != nil {
		panic(err)
	}

	// parse errors are ignored
	providerDelay, _ = time.ParseDuration(os.Getenv("JUJU_DUMMY_DELAY"))
}

// Reset resets the entire dummy environment and forgets any registered
// operation listener.  All opened environments after Reset will share
// the same underlying state.
func Reset() error {
	logger.Infof("reset model")
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	providerInstance.ops = discardOperations
	for _, s := range p.state {
		if s.apiListener != nil {
			s.apiListener.Close()
		}
		s.destroy()
	}
	providerInstance.state = make(map[int]*environState)
	if mongoAlive() {
		if err := gitjujutesting.MgoServer.Reset(); err != nil {
			return errors.Trace(err)
		}
	}
	providerInstance.statePolicy = environs.NewStatePolicy()
	providerInstance.supportsSpaces = true
	providerInstance.supportsSpaceDiscovery = false
	return nil
}

func (state *environState) destroy() {
	if !state.bootstrapped {
		return
	}
	if state.apiServer != nil {
		if err := state.apiServer.Stop(); err != nil && mongoAlive() {
			panic(err)
		}
		state.apiServer = nil
		if err := state.apiState.Close(); err != nil && mongoAlive() {
			panic(err)
		}
		state.apiState = nil
	}
	if mongoAlive() {
		gitjujutesting.MgoServer.Reset()
	}
	state.bootstrapped = false
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

// newState creates the state for a new environment with the given name.
func newState(name string, ops chan<- Operation, policy state.Policy) *environState {
	s := &environState{
		name:        name,
		ops:         ops,
		statePolicy: policy,
		insts:       make(map[instance.Id]*dummyInstance),
		globalPorts: make(map[network.PortRange]bool),
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

// SetStatePolicy sets the state.Policy to use when a
// controller is initialised by dummy.
func SetStatePolicy(policy state.Policy) {
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	p.statePolicy = policy
}

// SetSupportsSpaces allows to enable and disable SupportsSpaces for tests.
func SetSupportsSpaces(supports bool) bool {
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	current := p.supportsSpaces
	p.supportsSpaces = supports
	return current
}

// SetSupportsSpaceDiscovery allows to enable and disable
// SupportsSpaceDiscovery for tests.
func SetSupportsSpaceDiscovery(supports bool) bool {
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	current := p.supportsSpaceDiscovery
	p.supportsSpaceDiscovery = supports
	return current
}

// Listen closes the previously registered listener (if any).
// Subsequent operations on any dummy environment can be received on c
// (if not nil).
func Listen(c chan<- Operation) {
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	if c == nil {
		c = discardOperations
	}
	if p.ops != discardOperations {
		close(p.ops)
	}
	p.ops = c
	for _, st := range p.state {
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
	"state-id": {
		Description: "Id of controller",
		Type:        environschema.Tstring,
		Group:       environschema.JujuGroup,
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
	"state-id":   schema.Omit,
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

func (c *environConfig) stateId() int {
	idStr, ok := c.attrs["state-id"].(string)
	if !ok {
		return noStateId
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		panic(fmt.Errorf("unexpected state-id %q (should have pre-checked)", idStr))
	}
	return id
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

func (p *environProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{cloud.EmptyAuthType: {}}
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
	if idStr, ok := validated["state-id"].(string); ok {
		if _, err := strconv.Atoi(idStr); err != nil {
			return nil, fmt.Errorf("invalid state-id %q", idStr)
		}
	}
	// Apply the coerced unknown values back into the config.
	return cfg.Apply(validated)
}

func (e *environ) state() (*environState, error) {
	stateId := e.ecfg().stateId()
	if stateId == noStateId {
		return nil, ErrNotPrepared
	}
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	if state := p.state[stateId]; state != nil {
		return state, nil
	}
	return nil, ErrDestroyed
}

func (p *environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ecfg, err := p.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	if ecfg.stateId() == noStateId {
		return nil, ErrNotPrepared
	}
	env := &environ{
		name:         ecfg.Name(),
		ecfgUnlocked: ecfg,
	}
	if err := env.checkBroken("Open"); err != nil {
		return nil, err
	}
	return env, nil
}

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (p *environProvider) RestrictedConfigAttributes() []string {
	return nil
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (p *environProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	return cfg, nil
}

func (p *environProvider) PrepareForBootstrap(ctx environs.BootstrapContext, args environs.PrepareForBootstrapParams) (environs.Environ, error) {
	cfg := args.Config
	cfg, err := p.prepare(cfg)
	if err != nil {
		return nil, err
	}
	return p.Open(cfg)
}

// prepare is the internal version of Prepare - it prepares the
// environment but does not open it.
func (p *environProvider) prepare(cfg *config.Config) (*config.Config, error) {
	ecfg, err := p.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	name := cfg.Name()
	if ecfg.stateId() != noStateId {
		return cfg, nil
	}
	if ecfg.controller() && len(p.state) != 0 {
		for _, old := range p.state {
			panic(fmt.Errorf("cannot share a state between two dummy environs; old %q; new %q", old.name, name))
		}
	}
	// The environment has not been prepared,
	// so create it and set its state identifier accordingly.
	state := newState(name, p.ops, p.statePolicy)
	p.maxStateId++
	state.id = p.maxStateId
	p.state[state.id] = state

	attrs := map[string]interface{}{"state-id": fmt.Sprint(state.id)}
	if ecfg.controller() {
		attrs["api-port"] = state.listenAPI()
	}
	return cfg.Apply(attrs)
}

func (*environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"secret": ecfg.secret(),
	}, nil
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

// SupportedArchitectures is specified on the EnvironCapability interface.
func (*environ) SupportedArchitectures() ([]string, error) {
	return []string{arch.AMD64, arch.I386, arch.PPC64EL, arch.ARM64}, nil
}

// PrecheckInstance is specified in the state.Prechecker interface.
func (*environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if placement != "" && placement != "valid" {
		return fmt.Errorf("%s placement is invalid", placement)
	}
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
	network.SetPreferIPv6(e.Config().PreferIPv6())
	password := e.Config().AdminSecret()
	if password == "" {
		return nil, fmt.Errorf("admin-secret is required for bootstrap")
	}
	if _, ok := e.Config().CACert(); !ok {
		return nil, fmt.Errorf("no CA certificate in model configuration")
	}

	logger.Infof("would pick tools from %s", availableTools)
	cfg, err := environs.BootstrapConfig(e.Config())
	if err != nil {
		return nil, fmt.Errorf("cannot make bootstrap config: %v", err)
	}

	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	if estate.bootstrapped {
		return nil, fmt.Errorf("model is already bootstrapped")
	}
	estate.preferIPv6 = e.Config().PreferIPv6()

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

	if e.ecfg().controller() {
		// TODO(rog) factor out relevant code from cmd/jujud/bootstrap.go
		// so that we can call it here.

		info := stateInfo(estate.preferIPv6)
		// Since the admin user isn't setup until after here,
		// the password in the info structure is empty, so the admin
		// user is constructed with an empty password here.
		// It is set just below.
		st, err := state.Initialize(
			names.NewUserTag("admin@local"), info, cfg,
			mongo.DefaultDialOpts(), estate.statePolicy)
		if err != nil {
			panic(err)
		}
		if err := st.SetModelConstraints(args.EnvironConstraints); err != nil {
			panic(err)
		}
		if err := st.SetAdminMongoPassword(password); err != nil {
			panic(err)
		}
		if err := st.MongoSession().DB("admin").Login("admin", password); err != nil {
			panic(err)
		}
		env, err := st.Model()
		if err != nil {
			panic(err)
		}
		owner, err := st.User(env.Owner())
		if err != nil {
			panic(err)
		}
		// We log this out for test purposes only. No one in real life can use
		// a dummy provider for anything other than testing, so logging the password
		// here is fine.
		logger.Debugf("setting password for %q to %q", owner.Name(), password)
		owner.SetPassword(password)

		estate.apiServer, err = apiserver.NewServer(st, estate.apiListener, apiserver.ServerConfig{
			Cert:    []byte(testing.ServerCert),
			Key:     []byte(testing.ServerKey),
			Tag:     names.NewMachineTag("0"),
			DataDir: DataDir,
			LogDir:  LogDir,
		})
		if err != nil {
			panic(err)
		}
		estate.apiState = st
	}
	estate.bootstrapped = true
	estate.ops <- OpBootstrap{Context: ctx, Env: e.name, Args: args}
	finalize := func(ctx environs.BootstrapContext, icfg *instancecfg.InstanceConfig) error {
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

func (e *environ) ControllerInstances() ([]instance.Id, error) {
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
	ecfg, err := providerInstance.newConfig(cfg)
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
		if err == ErrDestroyed {
			return nil
		}
		return err
	}
	defer func() { estate.ops <- OpDestroy{Env: estate.name, Error: res} }()
	if err := e.checkBroken("Destroy"); err != nil {
		return err
	}
	p := &providerInstance
	p.mu.Lock()
	delete(p.state, estate.id)
	p.mu.Unlock()

	estate.mu.Lock()
	defer estate.mu.Unlock()
	estate.destroy()
	return nil
}

// ConstraintsValidator is defined on the Environs interface.
func (e *environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported([]string{constraints.CpuPower})
	validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
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
	if _, ok := e.Config().CACert(); !ok {
		return nil, errors.New("no CA certificate in model configuration")
	}
	if args.InstanceConfig.MongoInfo.Tag != names.NewMachineTag(machineId) {
		return nil, errors.New("entity tag must match started machine")
	}
	if args.InstanceConfig.APIInfo.Tag != names.NewMachineTag(machineId) {
		return nil, errors.New("entity tag must match started machine")
	}
	logger.Infof("would pick tools from %s", args.Tools)
	series := args.Tools.OneSeries()

	idString := fmt.Sprintf("%s-%d", e.name, estate.maxId)
	addrs := network.NewAddresses(idString+".dns", "127.0.0.1")
	if estate.preferIPv6 {
		addrs = append(addrs, network.NewAddress(fmt.Sprintf("fc00::%x", estate.maxId+1)))
	}
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
		Info:             args.InstanceConfig.MongoInfo,
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
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.supportsSpaces {
		return false, errors.NotSupportedf("spaces")
	}
	return true, nil
}

// SupportsSpaceDiscovery is specified on environs.Networking.
func (env *environ) SupportsSpaceDiscovery() (bool, error) {
	if err := env.checkBroken("SupportsSpaceDiscovery"); err != nil {
		return false, err
	}
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.supportsSpaceDiscovery {
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
		ProviderId: network.Id("foo"),
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("1"),
			AvailabilityZones: []string{"zone1"},
		}, {
			ProviderId:        network.Id("2"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		ProviderId: network.Id("Another Foo 99!"),
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("3"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		ProviderId: network.Id("foo-"),
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("4"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		ProviderId: network.Id("---"),
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("5"),
			AvailabilityZones: []string{"zone1"},
		}}}}, nil
}

// SupportsAddressAllocation is specified on environs.Networking.
func (env *environ) SupportsAddressAllocation(subnetId network.Id) (bool, error) {
	if !environs.AddressAllocationEnabled() {
		return false, errors.NotSupportedf("address allocation")
	}

	if err := env.checkBroken("SupportsAddressAllocation"); err != nil {
		return false, err
	}
	// Any subnetId starting with "noalloc-" will cause this to return
	// false, so it can be used in tests.
	if strings.HasPrefix(string(subnetId), "noalloc-") {
		return false, nil
	}
	return true, nil
}

// AllocateAddress requests an address to be allocated for the
// given instance on the given subnet.
func (env *environ) AllocateAddress(instId instance.Id, subnetId network.Id, addr *network.Address, macAddress, hostname string) error {
	if !environs.AddressAllocationEnabled() {
		// Any instId starting with "i-alloc-" when the feature flag is off will
		// still work, in order to be able to test MAAS 1.8+ environment where
		// we can use devices for containers.
		if !strings.HasPrefix(string(instId), "i-alloc-") {
			return errors.NotSupportedf("address allocation")
		}
		// Also, in this case we expect addr to be non-nil, but empty, so it can
		// be used as an output argument (same as in provider/maas).
		if addr == nil || addr.Value != "" {
			return errors.NewNotValid(nil, "invalid address: nil or non-empty")
		}
	}

	if err := env.checkBroken("AllocateAddress"); err != nil {
		return err
	}

	estate, err := env.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	estate.maxAddr++

	if addr.Value == "" {
		*addr = network.NewAddress(fmt.Sprintf("0.10.0.%v", estate.maxAddr))
	}

	estate.ops <- OpAllocateAddress{
		Env:        env.name,
		InstanceId: instId,
		SubnetId:   subnetId,
		Address:    *addr,
		MACAddress: macAddress,
		HostName:   hostname,
	}
	return nil
}

// ReleaseAddress releases a specific address previously allocated with
// AllocateAddress.
func (env *environ) ReleaseAddress(instId instance.Id, subnetId network.Id, addr network.Address, macAddress, hostname string) error {
	if !environs.AddressAllocationEnabled() {
		return errors.NotSupportedf("address allocation")
	}

	if err := env.checkBroken("ReleaseAddress"); err != nil {
		return err
	}
	estate, err := env.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	estate.maxAddr--
	estate.ops <- OpReleaseAddress{
		Env:        env.name,
		InstanceId: instId,
		SubnetId:   subnetId,
		Address:    addr,
		MACAddress: macAddress,
		HostName:   hostname,
	}
	return nil
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
			NetworkName:      "juju-" + netName,
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

	if strings.HasPrefix(string(instId), "i-no-nics-") {
		// Simulate no NICs on instances with id prefix "i-no-nics-".
		info = info[:0]
	} else if strings.HasPrefix(string(instId), "i-nic-no-subnet-") {
		// Simulate a nic with no subnet on instances with id prefix
		// "i-nic-no-subnet-"
		info = []network.InterfaceInfo{{
			DeviceIndex:   0,
			ProviderId:    network.Id("dummy-eth0"),
			NetworkName:   "juju-public",
			InterfaceName: "eth0",
			MACAddress:    "aa:bb:cc:dd:ee:f0",
			Disabled:      false,
			NoAutoStart:   false,
			ConfigType:    network.ConfigDHCP,
		}}
	} else if strings.HasPrefix(string(instId), "i-disabled-nic-") {
		info = info[2:]
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

	p := &providerInstance
	if p.supportsSpaceDiscovery {
		// Space discovery needs more subnets to work with.
		return env.subnetsForSpaceDiscovery(estate)
	}

	allSubnets := []network.SubnetInfo{{
		CIDR:              "0.10.0.0/24",
		ProviderId:        "dummy-private",
		AllocatableIPLow:  net.ParseIP("0.10.0.0"),
		AllocatableIPHigh: net.ParseIP("0.10.0.255"),
		AvailabilityZones: []string{"zone1", "zone2"},
	}, {
		CIDR:              "0.20.0.0/24",
		ProviderId:        "dummy-public",
		AllocatableIPLow:  net.ParseIP("0.20.0.0"),
		AllocatableIPHigh: net.ParseIP("0.20.0.255"),
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

	noSubnets := strings.HasPrefix(string(instId), "i-no-subnets")
	noNICSubnets := strings.HasPrefix(string(instId), "i-nic-no-subnet-")
	if noSubnets || noNICSubnets {
		// Simulate no subnets available if the instance id has prefix
		// "i-no-subnets-".
		result = result[:0]
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

	makeNoAlloc := func(info network.SubnetInfo) network.SubnetInfo {
		// Remove the allocatable range and change the provider id
		// prefix.
		pid := string(info.ProviderId)
		pid = strings.TrimPrefix(pid, "dummy-")
		info.ProviderId = network.Id("noalloc-" + pid)
		info.AllocatableIPLow = nil
		info.AllocatableIPHigh = nil
		return info
	}
	if strings.HasPrefix(string(instId), "i-no-alloc-") {
		iid := strings.TrimPrefix(string(instId), "i-no-alloc-")
		lastIdx := len(result) - 1
		if strings.HasPrefix(iid, "all") {
			// Simulate all subnets have no allocatable ranges set.
			for i := range result {
				result[i] = makeNoAlloc(result[i])
			}
		} else if idx, err := strconv.Atoi(iid); err == nil {
			// Simulate only the subnet with index idx in the result
			// have no allocatable range set.
			if idx < 0 || idx > lastIdx {
				err = errors.Errorf("index %d out of range; expected 0..%d", idx, lastIdx)
				return nil, err
			}
			result[idx] = makeNoAlloc(result[idx])
		} else {
			err = errors.Errorf("invalid index %q; expected int", iid)
			return nil, err
		}
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
	return &providerInstance
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

func (inst *dummyInstance) Status() string {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.status
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
var providerDelay time.Duration

// pause execution to simulate the latency of a real provider
func delay() {
	if providerDelay > 0 {
		logger.Infof("pausing for %v", providerDelay)
		<-time.After(providerDelay)
	}
}
