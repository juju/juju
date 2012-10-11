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
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/schema"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/version"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// stateInfo returns a *state.Info which allows clients to connect to the
// shared dummy state, if it exists.
func stateInfo() *state.Info {
	if testing.MgoAddr == "" {
		panic("dummy environ state tests must be run with MgoTestPackage")
	}
	return &state.Info{Addrs: []string{testing.MgoAddr}}
}

// Operation represents an action on the dummy provider.
type Operation interface{}

type GenericOperation struct {
	Env string
}

type OpBootstrap GenericOperation

type OpDestroy GenericOperation

type OpStartInstance struct {
	Env       string
	MachineId int
	Instance  environs.Instance
	Info      *state.Info
	Secret    string
}

type OpStopInstances struct {
	Env       string
	Instances []environs.Instance
}

type OpOpenPorts struct {
	Env        string
	MachineId  int
	InstanceId string
	Ports      []state.Port
}

type OpClosePorts struct {
	Env        string
	MachineId  int
	InstanceId string
	Ports      []state.Port
}

type OpPutFile GenericOperation

// environProvider represents the dummy provider.  There is only ever one
// instance of this type (providerInstance)
type environProvider struct {
	mu  sync.Mutex
	ops chan<- Operation
	// We have one state for each environment name
	state map[string]*environState
}

var providerInstance environProvider

// environState represents the state of an environment.
// It can be shared between several environ values,
// so that a given environment can be opened several times.
type environState struct {
	name          string
	ops           chan<- Operation
	mu            sync.Mutex
	maxId         int // maximum instance id allocated so far.
	insts         map[string]*instance
	globalPorts   map[state.Port]bool
	firewallMode  config.FirewallMode
	bootstrapped  bool
	storageDelay  time.Duration
	storage       *storage
	publicStorage *storage
	httpListener  net.Listener
}

// environ represents a client's connection to a given environment's
// state.
type environ struct {
	state        *environState
	ecfgMutex    sync.Mutex
	ecfgUnlocked *environConfig
}

// storage holds the storage for an environState.
// There are two instances for each environState
// instance, one for public files and one for private.
type storage struct {
	path     string // path prefix in http space.
	state    *environState
	files    map[string][]byte
	poisoned map[string]error
}

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
	log.Printf("environs/dummy: reset environment")
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.state {
		s.httpListener.Close()
	}
	providerInstance.ops = discardOperations
	providerInstance.state = make(map[string]*environState)
	if testing.MgoAddr != "" {
		testing.MgoReset()
	}
}

// newState creates the state for a new environment with the
// given name and starts an http server listening for
// storage requests.
func newState(name string, ops chan<- Operation, fwmode config.FirewallMode) *environState {
	s := &environState{
		name:         name,
		ops:          ops,
		insts:        make(map[string]*instance),
		globalPorts:  make(map[state.Port]bool),
		firewallMode: fwmode,
	}
	s.storage = newStorage(s, "/"+name+"/private")
	s.publicStorage = newStorage(s, "/"+name+"/public")
	putFakeTools(s.publicStorage)
	s.listen()
	return s
}

// putFakeTools writes something
// that looks like a tools archive so Bootstrap can
// find some tools and initialise the state correctly.
func putFakeTools(s environs.StorageWriter) {
	log.Printf("environs/dummy: putting fake tools")
	path := environs.ToolsStoragePath(version.Current)
	toolsContents := "tools archive, honest guv"
	err := s.Put(path, strings.NewReader(toolsContents), int64(len(toolsContents)))
	if err != nil {
		panic(err)
	}
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
	mux.Handle(s.publicStorage.path+"/", http.StripPrefix(s.publicStorage.path+"/", s.publicStorage))
	go http.Serve(l, mux)
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

var checker = schema.StrictFieldMap(
	schema.Fields{
		"state-server": schema.Bool(),
		"broken":       schema.String(),
		"secret":       schema.String(),
	},
	schema.Defaults{
		"broken": "",
		"secret": "pork",
	},
)

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

func (p *environProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

func (p *environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	v, err := checker.Coerce(cfg.UnknownAttrs(), nil)
	if err != nil {
		return nil, err
	}
	return cfg.Apply(v.(map[string]interface{}))
}

func (p *environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	name := cfg.Name()
	ecfg, err := p.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	state := p.state[name]
	if state == nil {
		if ecfg.stateServer() && len(p.state) != 0 {
			var old string
			for oldName := range p.state {
				old = oldName
				break
			}
			panic(fmt.Errorf("cannot share a state between two dummy environs; old %q; new %q", old, name))
		}
		state = newState(name, p.ops, ecfg.FirewallMode())
		p.state[name] = state
	}
	env := &environ{
		state:        state,
		ecfgUnlocked: ecfg,
	}
	if err := env.checkBroken("Open"); err != nil {
		return nil, err
	}
	return env, nil
}

func (*environProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	m["secret"] = ecfg.secret()
	return m, nil

}

func (*environProvider) PublicAddress() (string, error) {
	return "public.dummy.address.example.com", nil
}

func (*environProvider) PrivateAddress() (string, error) {
	return "private.dummy.address.example.com", nil
}

var errBroken = errors.New("broken environment")

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
	return e.state.name
}

func (e *environ) Bootstrap(uploadTools bool) error {
	defer delay()
	if err := e.checkBroken("Bootstrap"); err != nil {
		return err
	}
	var tools *state.Tools
	var err error
	if uploadTools {
		tools, err = environs.PutTools(e.Storage(), nil)
		if err != nil {
			return err
		}
	} else {
		flags := environs.HighestVersion | environs.CompatVersion
		tools, err = environs.FindTools(e, version.Current, flags)
		if err != nil {
			return err
		}
	}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	e.state.ops <- OpBootstrap{Env: e.state.name}
	if e.state.bootstrapped {
		return fmt.Errorf("environment is already bootstrapped")
	}
	if e.ecfg().stateServer() {
		info := stateInfo()
		cfg, err := environs.BootstrapConfig(&providerInstance, e.ecfg().Config, tools)
		if err != nil {
			return fmt.Errorf("cannot make bootstrap config: %v", err)
		}
		st, err := state.Initialize(info, cfg)
		if err != nil {
			panic(err)
		}
		if password := e.Config().AdminSecret(); password != "" {
			if err := st.SetAdminPassword(trivial.PasswordHash(password)); err != nil {
				return err
			}
		}
		if err := st.Close(); err != nil {
			panic(err)
		}
	}
	e.state.bootstrapped = true
	return nil
}

func (e *environ) StateInfo() (*state.Info, error) {
	if err := e.checkBroken("StateInfo"); err != nil {
		return nil, err
	}
	if !e.ecfg().stateServer() {
		return nil, errors.New("dummy environment has no state configured")
	}
	if !e.state.bootstrapped {
		return nil, errors.New("dummy environment not bootstrapped")
	}
	return stateInfo(), nil
}

func (e *environ) AssignmentPolicy() state.AssignmentPolicy {
	return state.AssignUnused
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

func (e *environ) Destroy([]environs.Instance) error {
	defer delay()
	if err := e.checkBroken("Destroy"); err != nil {
		return err
	}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	e.state.ops <- OpDestroy{Env: e.state.name}
	if testing.MgoAddr != "" {
		testing.MgoReset()
	}
	e.state.bootstrapped = false
	e.state.storage.files = make(map[string][]byte)

	return nil
}

func (e *environ) StartInstance(machineId int, info *state.Info, tools *state.Tools) (environs.Instance, error) {
	defer delay()
	log.Printf("environs/dummy: dummy startinstance, machine %d", machineId)
	if err := e.checkBroken("StartInstance"); err != nil {
		return nil, err
	}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	if info.EntityName != state.MachineEntityName(machineId) {
		return nil, fmt.Errorf("entity name must match started machine")
	}
	if tools != nil && (strings.HasPrefix(tools.Series, "unknown") || strings.HasPrefix(tools.Arch, "unknown")) {
		return nil, fmt.Errorf("cannot find image for %s-%s", tools.Series, tools.Arch)
	}
	i := &instance{
		state:     e.state,
		id:        fmt.Sprintf("%s-%d", e.state.name, e.state.maxId),
		ports:     make(map[state.Port]bool),
		machineId: machineId,
	}
	e.state.insts[i.id] = i
	e.state.maxId++
	e.state.ops <- OpStartInstance{
		Env:       e.state.name,
		MachineId: machineId,
		Instance:  i,
		Info:      info,
		Secret:    e.ecfg().secret(),
	}
	return i, nil
}

func (e *environ) StopInstances(is []environs.Instance) error {
	defer delay()
	if err := e.checkBroken("StopInstance"); err != nil {
		return err
	}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	for _, i := range is {
		delete(e.state.insts, i.(*instance).id)
	}
	e.state.ops <- OpStopInstances{
		Env:       e.state.name,
		Instances: is,
	}
	return nil
}

func (e *environ) Instances(ids []string) (insts []environs.Instance, err error) {
	defer delay()
	if err := e.checkBroken("Instances"); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	notFound := 0
	for _, id := range ids {
		inst := e.state.insts[id]
		if inst == nil {
			err = environs.ErrPartialInstances
			notFound++
		}
		insts = append(insts, inst)
	}
	if notFound == len(ids) {
		return nil, environs.ErrNoInstances
	}
	return
}

func (e *environ) AllInstances() ([]environs.Instance, error) {
	defer delay()
	if err := e.checkBroken("AllInstances"); err != nil {
		return nil, err
	}
	var insts []environs.Instance
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	for _, v := range e.state.insts {
		insts = append(insts, v)
	}
	return insts, nil
}

func (*environ) Provider() environs.EnvironProvider {
	return &providerInstance
}

type instance struct {
	state     *environState
	ports     map[state.Port]bool
	id        string
	machineId int
}

func (inst *instance) Id() string {
	return inst.id
}

func (inst *instance) DNSName() (string, error) {
	defer delay()
	return inst.id + ".dns", nil
}

func (inst *instance) WaitDNSName() (string, error) {
	return inst.DNSName()
}

func (inst *instance) OpenPorts(machineId int, ports []state.Port) error {
	defer delay()
	log.Printf("environs/dummy: openPorts %d, %#v", machineId, ports)
	if inst.machineId != machineId {
		panic(fmt.Errorf("OpenPorts with mismatched machine id, expected %d got %d", inst.machineId, machineId))
	}
	inst.state.mu.Lock()
	defer inst.state.mu.Unlock()
	inst.state.ops <- OpOpenPorts{
		Env:        inst.state.name,
		MachineId:  machineId,
		InstanceId: inst.Id(),
		Ports:      ports,
	}
	m := inst.portsMap()
	for _, p := range ports {
		m[p] = true
	}
	return nil
}

func (inst *instance) ClosePorts(machineId int, ports []state.Port) error {
	defer delay()
	if inst.machineId != machineId {
		panic(fmt.Errorf("ClosePorts with mismatched machine id, expected %d got %d", inst.machineId, machineId))
	}
	inst.state.mu.Lock()
	defer inst.state.mu.Unlock()
	inst.state.ops <- OpClosePorts{
		Env:        inst.state.name,
		MachineId:  machineId,
		InstanceId: inst.Id(),
		Ports:      ports,
	}
	m := inst.portsMap()
	for _, p := range ports {
		delete(m, p)
	}
	return nil
}

func (inst *instance) Ports(machineId int) (ports []state.Port, err error) {
	defer delay()
	if inst.machineId != machineId {
		panic(fmt.Errorf("Ports with mismatched machine id, expected %d got %d", inst.machineId, machineId))
	}
	inst.state.mu.Lock()
	defer inst.state.mu.Unlock()
	m := inst.portsMap()
	for p := range m {
		ports = append(ports, p)
	}
	state.SortPorts(ports)
	return
}

// portsMap returns the map of open ports for the instance, taking
// into account the firewall mode in the environment configuration.
func (inst *instance) portsMap() map[state.Port]bool {
	switch inst.state.firewallMode {
	case config.FwDefault:
		// dummy simulates ec2 by default: per instance security groups.
		return inst.ports
	case config.FwGlobal:
		return inst.state.globalPorts
	}
	panic("unknown firewall mode: " + string(inst.state.firewallMode))
}

// providerDelay controls the delay before dummy responds.
// non empty values in JUJU_DUMMY_DELAY will be parsed as 
// time.Durations into this value.
var providerDelay time.Duration

// pause execution to simulate the latency of a real provider
func delay() {
	if providerDelay > 0 {
		log.Printf("environs/dummy: pausing for %v", providerDelay)
		<-time.After(providerDelay)
	}
}
