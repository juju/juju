// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The dummy provider implements an environment provider for testing
// purposes, registered with environs under the name "dummy".
//
// The configuration YAML for the testing environment
// must specify a "state-server" property with a boolean
// value. If this is true, a state server will be started
// the first time StateInfo is called on a newly reset environment.
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
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/schema"
	"github.com/juju/utils"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/network"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/provider"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/apiserver"
	"github.com/juju/juju/testing"
)

var logger = loggo.GetLogger("juju.provider.dummy")

// SampleConfig() returns an environment configuration with all required
// attributes set.
func SampleConfig() testing.Attrs {
	return testing.Attrs{
		"type":                      "dummy",
		"name":                      "only",
		"authorized-keys":           testing.FakeAuthKeys,
		"firewall-mode":             config.FwInstance,
		"admin-secret":              testing.DefaultMongoPassword,
		"ca-cert":                   testing.CACert,
		"ca-private-key":            testing.CAKey,
		"ssl-hostname-verification": true,
		"development":               false,
		"state-port":                1234,
		"api-port":                  4321,
		"syslog-port":               2345,
		"default-series":            "precise",

		"secret":       "pork",
		"state-server": true,
	}
}

// stateInfo returns a *state.Info which allows clients to connect to the
// shared dummy state, if it exists.
func stateInfo() *state.Info {
	if testing.MgoServer.Addr() == "" {
		panic("dummy environ state tests must be run with MgoTestPackage")
	}
	return &state.Info{
		Addrs:  []string{testing.MgoServer.Addr()},
		CACert: testing.CACert,
	}
}

// Operation represents an action on the dummy provider.
type Operation interface{}

type OpBootstrap struct {
	Context environs.BootstrapContext
	Env     string
	Args    environs.BootstrapParams
}

type OpDestroy struct {
	Env   string
	Error error
}

type OpAllocateAddress struct {
	Env        string
	InstanceId instance.Id
	NetworkId  network.Id
	Address    instance.Address
}

type OpStartInstance struct {
	Env          string
	MachineId    string
	MachineNonce string
	Instance     instance.Instance
	Constraints  constraints.Value
	Networks     []string
	NetworkInfo  []network.Info
	Info         *state.Info
	APIInfo      *api.Info
	Secret       string
}

type OpStopInstances struct {
	Env string
	Ids []instance.Id
}

type OpOpenPorts struct {
	Env        string
	MachineId  string
	InstanceId instance.Id
	Ports      []instance.Port
}

type OpClosePorts struct {
	Env        string
	MachineId  string
	InstanceId instance.Id
	Ports      []instance.Port
}

type OpPutFile struct {
	Env      string
	FileName string
}

// environProvider represents the dummy provider.  There is only ever one
// instance of this type (providerInstance)
type environProvider struct {
	mu          sync.Mutex
	ops         chan<- Operation
	statePolicy state.Policy
	// We have one state for each environment name
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
	globalPorts  map[instance.Port]bool
	bootstrapped bool
	storageDelay time.Duration
	storage      *storageServer
	httpListener net.Listener
	apiServer    *apiserver.Server
	apiState     *state.State
}

// environ represents a client's connection to a given environment's
// state.
type environ struct {
	common.SupportsUnitPlacementPolicy

	name         string
	ecfgMutex    sync.Mutex
	ecfgUnlocked *environConfig
}

var _ imagemetadata.SupportsCustomSources = (*environ)(nil)
var _ tools.SupportsCustomSources = (*environ)(nil)
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
	Reset()

	// parse errors are ignored
	providerDelay, _ = time.ParseDuration(os.Getenv("JUJU_DUMMY_DELAY"))
}

// Reset resets the entire dummy environment and forgets any registered
// operation listener.  All opened environments after Reset will share
// the same underlying state.
func Reset() {
	logger.Infof("reset environment")
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	providerInstance.ops = discardOperations
	for _, s := range p.state {
		s.httpListener.Close()
		s.destroy()
	}
	providerInstance.state = make(map[int]*environState)
	if mongoAlive() {
		testing.MgoServer.Reset()
	}
	providerInstance.statePolicy = environs.NewStatePolicy()
}

func (state *environState) destroy() {
	state.storage.files = make(map[string][]byte)
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
		testing.MgoServer.Reset()
	}
	state.bootstrapped = false
}

// mongoAlive reports whether the mongo server is
// still alive (i.e. has not been deliberately destroyed).
// If it has been deliberately destroyed, we will
// expect some errors when closing things down.
func mongoAlive() bool {
	return testing.MgoServer.Addr() != ""
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

// newState creates the state for a new environment with the
// given name and starts an http server listening for
// storage requests.
func newState(name string, ops chan<- Operation, policy state.Policy) *environState {
	s := &environState{
		name:        name,
		ops:         ops,
		statePolicy: policy,
		insts:       make(map[instance.Id]*dummyInstance),
		globalPorts: make(map[instance.Port]bool),
	}
	s.storage = newStorageServer(s, "/"+name+"/private")
	s.listen()
	return s
}

// listen starts a network listener listening for http
// requests to retrieve files in the state's storage.
func (s *environState) listen() {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Errorf("cannot start listener: %v", err))
	}
	s.httpListener = l
	mux := http.NewServeMux()
	mux.Handle(s.storage.path+"/", http.StripPrefix(s.storage.path+"/", s.storage))
	go http.Serve(l, mux)
}

// SetStatePolicy sets the state.Policy to use when a
// state server is initialised by dummy.
func SetStatePolicy(policy state.Policy) {
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	p.statePolicy = policy
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

// SetStorageDelay causes any storage download operation in any current
// environment to be delayed for the given duration.
func SetStorageDelay(d time.Duration) {
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, st := range p.state {
		st.mu.Lock()
		st.storageDelay = d
		st.mu.Unlock()
	}
}

var configFields = schema.Fields{
	"state-server": schema.Bool(),
	"broken":       schema.String(),
	"secret":       schema.String(),
	"state-id":     schema.String(),
}
var configDefaults = schema.Defaults{
	"broken":   "",
	"secret":   "pork",
	"state-id": schema.Omit,
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (c *environConfig) stateServer() bool {
	return c.attrs["state-server"].(bool)
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
		return nil, provider.ErrNotPrepared
	}
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	if state := p.state[stateId]; state != nil {
		return state, nil
	}
	return nil, provider.ErrDestroyed
}

func (p *environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ecfg, err := p.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	if ecfg.stateId() == noStateId {
		return nil, provider.ErrNotPrepared
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

func (p *environProvider) Prepare(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
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
	// The environment has not been prepared,
	// so create it and set its state identifier accordingly.
	if ecfg.stateServer() && len(p.state) != 0 {
		for _, old := range p.state {
			panic(fmt.Errorf("cannot share a state between two dummy environs; old %q; new %q", old.name, name))
		}
	}
	state := newState(name, p.ops, p.statePolicy)
	p.maxStateId++
	state.id = p.maxStateId
	p.state[state.id] = state
	// Add the state id to the configuration we use to
	// in the returned environment.
	return cfg.Apply(map[string]interface{}{
		"state-id": fmt.Sprint(state.id),
	})
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

func (*environProvider) BoilerplateConfig() string {
	return `
# Fake configuration for dummy provider.
dummy:
    type: dummy

`[1:]
}

var errBroken = errors.New("broken environment")

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

func (e *environ) Name() string {
	return e.name
}

// SupportedArchitectures is specified on the EnvironCapability interface.
func (*environ) SupportedArchitectures() ([]string, error) {
	return []string{arch.AMD64, arch.I386, arch.PPC64}, nil
}

// SupportNetworks is specified on the EnvironCapability interface.
func (*environ) SupportNetworks() bool {
	return true
}

// PrecheckInstance is specified in the state.Prechecker interface.
func (*environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if placement != "" && placement != "valid" {
		return fmt.Errorf("%s placement is invalid", placement)
	}
	return nil
}

// GetImageSources returns a list of sources which are used to search for simplestreams image metadata.
func (e *environ) GetImageSources() ([]simplestreams.DataSource, error) {
	return []simplestreams.DataSource{
		storage.NewStorageSimpleStreamsDataSource("cloud storage", e.Storage(), storage.BaseImagesPath)}, nil
}

// GetToolsSources returns a list of sources which are used to search for simplestreams tools metadata.
func (e *environ) GetToolsSources() ([]simplestreams.DataSource, error) {
	return []simplestreams.DataSource{
		storage.NewStorageSimpleStreamsDataSource("cloud storage", e.Storage(), storage.BaseToolsPath)}, nil
}

func (e *environ) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) error {
	selectedTools, err := common.EnsureBootstrapTools(ctx, e, config.PreferredSeries(e.Config()), args.Constraints.Arch)
	if err != nil {
		return err
	}

	defer delay()
	if err := e.checkBroken("Bootstrap"); err != nil {
		return err
	}
	password := e.Config().AdminSecret()
	if password == "" {
		return fmt.Errorf("admin-secret is required for bootstrap")
	}
	if _, ok := e.Config().CACert(); !ok {
		return fmt.Errorf("no CA certificate in environment configuration")
	}

	logger.Infof("would pick tools from %s", selectedTools)
	cfg, err := environs.BootstrapConfig(e.Config())
	if err != nil {
		return fmt.Errorf("cannot make bootstrap config: %v", err)
	}

	estate, err := e.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	if estate.bootstrapped {
		return fmt.Errorf("environment is already bootstrapped")
	}
	// Write the bootstrap file just like a normal provider. However
	// we need to release the mutex for the save state to work, so regain
	// it after the call.
	estate.mu.Unlock()
	if err := bootstrap.SaveState(e.Storage(), &bootstrap.BootstrapState{StateInstances: []instance.Id{"localhost"}}); err != nil {
		logger.Errorf("failed to save state instances: %v", err)
		estate.mu.Lock() // otherwise defered unlock will fail
		return err
	}
	estate.mu.Lock() // back at it

	if e.ecfg().stateServer() {
		// TODO(rog) factor out relevant code from cmd/jujud/bootstrap.go
		// so that we can call it here.

		info := stateInfo()
		st, err := state.Initialize(info, cfg, state.DefaultDialOpts(), estate.statePolicy)
		if err != nil {
			panic(err)
		}
		if err := st.SetEnvironConstraints(args.Constraints); err != nil {
			panic(err)
		}
		if err := st.SetAdminMongoPassword(utils.UserPasswordHash(password, utils.CompatSalt)); err != nil {
			panic(err)
		}
		_, err = st.AddUser("admin", "", password)
		if err != nil {
			panic(err)
		}
		estate.apiServer, err = apiserver.NewServer(st, "localhost:0", []byte(testing.ServerCert), []byte(testing.ServerKey), DataDir, LogDir)
		if err != nil {
			panic(err)
		}
		estate.apiState = st
	}
	estate.bootstrapped = true
	estate.ops <- OpBootstrap{Context: ctx, Env: e.name, Args: args}
	return nil
}

func (e *environ) StateInfo() (*state.Info, *api.Info, error) {
	estate, err := e.state()
	if err != nil {
		return nil, nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	if err := e.checkBroken("StateInfo"); err != nil {
		return nil, nil, err
	}
	if !e.ecfg().stateServer() {
		return nil, nil, errors.New("dummy environment has no state configured")
	}
	if !estate.bootstrapped {
		return nil, nil, environs.ErrNotBootstrapped
	}
	return stateInfo(), &api.Info{
		Addrs:  []string{estate.apiServer.Addr()},
		CACert: testing.CACert,
	}, nil
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
		if err == provider.ErrDestroyed {
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

// StartInstance is specified in the InstanceBroker interface.
func (e *environ) StartInstance(args environs.StartInstanceParams) (instance.Instance, *instance.HardwareCharacteristics, []network.Info, error) {

	defer delay()
	machineId := args.MachineConfig.MachineId
	logger.Infof("dummy startinstance, machine %s", machineId)
	if err := e.checkBroken("StartInstance"); err != nil {
		return nil, nil, nil, err
	}
	estate, err := e.state()
	if err != nil {
		return nil, nil, nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	if args.MachineConfig.MachineNonce == "" {
		return nil, nil, nil, fmt.Errorf("cannot start instance: missing machine nonce")
	}
	if _, ok := e.Config().CACert(); !ok {
		return nil, nil, nil, fmt.Errorf("no CA certificate in environment configuration")
	}
	if args.MachineConfig.StateInfo.Tag != names.MachineTag(machineId) {
		return nil, nil, nil, fmt.Errorf("entity tag must match started machine")
	}
	if args.MachineConfig.APIInfo.Tag != names.MachineTag(machineId) {
		return nil, nil, nil, fmt.Errorf("entity tag must match started machine")
	}
	logger.Infof("would pick tools from %s", args.Tools)
	series := args.Tools.OneSeries()

	idString := fmt.Sprintf("%s-%d", e.name, estate.maxId)
	i := &dummyInstance{
		id:           instance.Id(idString),
		addresses:    instance.NewAddresses(idString + ".dns"),
		ports:        make(map[instance.Port]bool),
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
	// Simulate networks added when requested.
	networks := append(args.Constraints.IncludeNetworks(), args.MachineConfig.Networks...)
	networkInfo := make([]network.Info, len(networks))
	for i, netName := range networks {
		if strings.HasPrefix(netName, "bad-") {
			// Simulate we didn't get correct information for the network.
			networkInfo[i] = network.Info{
				ProviderId:  network.Id(netName),
				NetworkName: netName,
				CIDR:        "invalid",
			}
		} else {
			networkInfo[i] = network.Info{
				ProviderId:    network.Id(netName),
				NetworkName:   netName,
				CIDR:          fmt.Sprintf("0.%d.2.0/24", i+1),
				InterfaceName: fmt.Sprintf("eth%d", i),
				VLANTag:       i,
				MACAddress:    fmt.Sprintf("aa:bb:cc:dd:ee:f%d", i),
				IsVirtual:     i > 0,
			}
		}
	}
	estate.insts[i.id] = i
	estate.maxId++
	estate.ops <- OpStartInstance{
		Env:          e.name,
		MachineId:    machineId,
		MachineNonce: args.MachineConfig.MachineNonce,
		Constraints:  args.Constraints,
		Networks:     args.MachineConfig.Networks,
		NetworkInfo:  networkInfo,
		Instance:     i,
		Info:         args.MachineConfig.StateInfo,
		APIInfo:      args.MachineConfig.APIInfo,
		Secret:       e.ecfg().secret(),
	}
	return i, hc, networkInfo, nil
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

// AllocateAddress requests a new address to be allocated for the
// given instance on the given network.
func (env *environ) AllocateAddress(instId instance.Id, netId network.Id) (instance.Address, error) {
	if err := env.checkBroken("AllocateAddress"); err != nil {
		return instance.Address{}, err
	}

	estate, err := env.state()
	if err != nil {
		return instance.Address{}, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	estate.maxAddr++
	// TODO(dimitern) Once we have integrated networks
	// and addresses, make sure we return a valid address
	// for the given network, and we also have the network
	// already registered.
	newAddress := instance.NewAddress(
		fmt.Sprintf("0.1.2.%d", estate.maxAddr),
		instance.NetworkCloudLocal,
	)
	estate.ops <- OpAllocateAddress{
		Env:        env.name,
		InstanceId: instId,
		NetworkId:  netId,
		Address:    newAddress,
	}
	return newAddress, nil
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

func (e *environ) OpenPorts(ports []instance.Port) error {
	if mode := e.ecfg().FirewallMode(); mode != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for opening ports on environment", mode)
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

func (e *environ) ClosePorts(ports []instance.Port) error {
	if mode := e.ecfg().FirewallMode(); mode != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for closing ports on environment", mode)
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

func (e *environ) Ports() (ports []instance.Port, err error) {
	if mode := e.ecfg().FirewallMode(); mode != config.FwGlobal {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from environment", mode)
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
	instance.SortPorts(ports)
	return
}

func (*environ) Provider() environs.EnvironProvider {
	return &providerInstance
}

type dummyInstance struct {
	state        *environState
	ports        map[instance.Port]bool
	id           instance.Id
	status       string
	machineId    string
	series       string
	firewallMode string

	mu        sync.Mutex
	addresses []instance.Address
}

func (inst *dummyInstance) Id() instance.Id {
	return inst.id
}

func (inst *dummyInstance) Status() string {
	return inst.status
}

// SetInstanceAddresses sets the addresses associated with the given
// dummy instance.
func SetInstanceAddresses(inst instance.Instance, addrs []instance.Address) {
	inst0 := inst.(*dummyInstance)
	inst0.mu.Lock()
	inst0.addresses = append(inst0.addresses[:0], addrs...)
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

func (inst *dummyInstance) DNSName() (string, error) {
	defer delay()
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if len(inst.addresses) == 0 {
		return "", instance.ErrNoDNSName
	}
	return inst.addresses[0].String(), nil
}

func (*dummyInstance) Refresh() error {
	return nil
}

func (inst *dummyInstance) Addresses() ([]instance.Address, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return append([]instance.Address{}, inst.addresses...), nil
}

func (inst *dummyInstance) WaitDNSName() (string, error) {
	return common.WaitDNSName(inst)
}

func (inst *dummyInstance) OpenPorts(machineId string, ports []instance.Port) error {
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

func (inst *dummyInstance) ClosePorts(machineId string, ports []instance.Port) error {
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

func (inst *dummyInstance) Ports(machineId string) (ports []instance.Port, err error) {
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
	for p := range inst.ports {
		ports = append(ports, p)
	}
	instance.SortPorts(ports)
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
