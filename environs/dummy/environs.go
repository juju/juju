// The dummy provider implements an environment provider for testing
// purposes, registered with environs under the name "dummy".
// 
// The configuration YAML for the testing environment
// must specify a "zookeeper" property with a boolean
// value. If this is true, a zookeeper instance will be started
// the first time StateInfo is called on a newly reset environment.
// NOTE: ZooKeeper isn't actually being started yet.
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
	"errors"
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/schema"
	"launchpad.net/juju/go/state"
	"launchpad.net/juju/go/testing"
	"net"
	"net/http"
	"sync"
)

// zkServer holds the shared zookeeper server which is used by all dummy
// environs to store state.
var zkServer *zookeeper.Server

// SetZookeeper sets the zookeeper server that will be used to hold the
// environ's state.State. If the environ's "zookeeper" config setting is
// true and no zookeeper server has been set, the StateInfo method will
// panic (unless the "isBroken" config setting is also true).
func SetZookeeper(srv *zookeeper.Server) {
	zkServer = srv
}

// stateInfo returns a *state.Info which allows clients to connect to the
// shared dummy zookeeper, if it exists.
func stateInfo() *state.Info {
	if zkServer == nil {
		panic("SetZookeeper not called")
	}
	addr, err := zkServer.Addr()
	if err != nil {
		panic(err)
	}
	return &state.Info{Addrs: []string{addr}}
}

type Operation struct {
	Kind OperationKind
	Env  string
}

// Operation represents an action on the dummy provider.
type OperationKind int

const (
	OpNone OperationKind = iota
	OpBootstrap
	OpDestroy
	OpStartInstance
	OpStopInstances
	OpPutFile
)

var kindNames = []string{
	OpNone:          "OpNone",
	OpBootstrap:     "OpBootstrap",
	OpDestroy:       "OpDestroy",
	OpStartInstance: "OpStartInstance",
	OpStopInstances: "OpStopInstances",
	OpPutFile:       "OpPutFile",
}

func (k OperationKind) String() string {
	return kindNames[k]
}

// environProvider represents the dummy provider.  There is only ever one
// instance of this type (providerInstance)
type environProvider struct {
	mu  sync.Mutex
	ops chan<- Operation
	// We have one state for each environment name
	state map[string]*environState
}

// environState represents the state of an environment.
// It can be shared between several environ values,
// so that a given environment can be opened several times.
type environState struct {
	name          string
	ops           chan<- Operation
	mu            sync.Mutex
	maxId         int // maximum instance id allocated so far.
	insts         map[string]*instance
	bootstrapped  bool
	storage       *storage
	publicStorage *storage
	httpListener  net.Listener
}

// environ represents a client's connection to a given environment's
// state.
type environ struct {
	state *environState

	// configMutex protects visibility of config.
	configMutex sync.Mutex
	config      *environConfig
}

// storage holds the storage for an environState.
// There are two instances for each environState
// instance, one for public files and one for private.
type storage struct {
	path  string // path prefix in http space.
	state *environState
	files map[string][]byte
}

var providerInstance environProvider

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
}

// Reset resets the entire dummy environment and forgets any registered
// operation listener.  All opened environments after Reset will share
// the same underlying state.
func Reset() {
	p := &providerInstance
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.state {
		s.httpListener.Close()
	}
	providerInstance.ops = discardOperations
	providerInstance.state = make(map[string]*environState)
	if zkServer != nil {
		testing.ResetZkServer(zkServer)
	}
}

// newState creates the state for a new environment with the
// given name and starts an http server listening for
// storage requests.
func newState(name string, ops chan<- Operation) *environState {
	s := &environState{
		name:  name,
		ops:   ops,
		insts: make(map[string]*instance),
	}
	s.storage = newStorage(s, "/"+name+"/private")
	s.publicStorage = newStorage(s, "/"+name+"/public")
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
	mux.Handle(s.publicStorage.path+"/", http.StripPrefix(s.publicStorage.path+"/", s.publicStorage))
	go http.Serve(l, mux)
}

// Listen closes the previously registered listener (if any),
// and if c is not nil registers it to receive notifications 
// of follow up operations in the environment.
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
}

type environConfig struct {
	provider  *environProvider
	name      string
	zookeeper bool
	broken    bool
}

var checker = schema.FieldMap(
	schema.Fields{
		"type":      schema.Const("dummy"),
		"zookeeper": schema.Bool(),
		"broken":    schema.Bool(),
		"name":      schema.String(),
	},
	[]string{
		"broken",
	},
)

func (p *environProvider) NewConfig(attrs map[string]interface{}) (environs.EnvironConfig, error) {
	m0, err := checker.Coerce(attrs, nil)
	if err != nil {
		return nil, err
	}
	m1 := m0.(schema.MapType)
	cfg := &environConfig{
		provider:  p,
		name:      m1["name"].(string),
		zookeeper: m1["zookeeper"].(bool),
	}
	cfg.broken, _ = m1["broken"].(bool)
	return cfg, nil
}

func (cfg *environConfig) Open() (environs.Environ, error) {
	p := cfg.provider
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.state[cfg.name]
	if state == nil {
		if cfg.zookeeper && len(p.state) != 0 {
			panic("cannot share a zookeeper between two dummy environs")
		}
		state = newState(cfg.name, p.ops)
		p.state[cfg.name] = state
	}
	env := &environ{
		state: state,
	}
	env.SetConfig(cfg)
	return env, nil
}

var errBroken = errors.New("broken environment")

// EnvironName returns the name of the environment,
// which must be opened from a dummy environment.
func EnvironName(e environs.Environ) string {
	return e.(*environ).state.name
}

func (e *environ) Bootstrap(uploadTools bool) error {
	if e.isBroken() {
		return errBroken
	}
	if uploadTools {
		err := environs.PutTools(e.Storage())
		if err != nil {
			return err
		}
	}
	e.state.ops <- Operation{Kind: OpBootstrap, Env: e.state.name}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	if e.state.bootstrapped {
		return fmt.Errorf("environment is already bootstrapped")
	}
	if e.config.zookeeper {
		info := stateInfo()
		st, err := state.Initialize(info)
		if err != nil {
			panic(err)
		}
		err = st.Close()
		if err != nil {
			panic(err)
		}
	}
	e.state.bootstrapped = true
	return nil
}

func (e *environ) StateInfo() (*state.Info, error) {
	if e.isBroken() {
		return nil, errBroken
	}
	if e.config.zookeeper && e.state.bootstrapped {
		return stateInfo(), nil
	}
	return nil, errors.New("no state info available for this environ")
}

func (e *environ) AssignmentPolicy() state.AssignmentPolicy {
	return state.AssignUnused
}

func (e *environ) SetConfig(cfg environs.EnvironConfig) {
	config := cfg.(*environConfig)
	e.configMutex.Lock()
	defer e.configMutex.Unlock()
	e.config = config
}

func (e *environ) Destroy([]environs.Instance) error {
	if e.isBroken() {
		return errBroken
	}
	e.state.ops <- Operation{Kind: OpDestroy, Env: e.state.name}
	e.state.mu.Lock()
	if zkServer != nil {
		testing.ResetZkServer(zkServer)
	}
	e.state.bootstrapped = false
	e.state.storage.files = make(map[string][]byte)
	e.state.mu.Unlock()
	return nil
}

func (e *environ) StartInstance(machineId int, _ *state.Info) (environs.Instance, error) {
	if e.isBroken() {
		return nil, errBroken
	}
	e.state.ops <- Operation{Kind: OpStartInstance, Env: e.state.name}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	i := &instance{
		id: fmt.Sprintf("%s-%d", e.state.name, e.state.maxId),
	}
	e.state.insts[i.id] = i
	e.state.maxId++
	return i, nil
}

func (e *environ) StopInstances(is []environs.Instance) error {
	if e.isBroken() {
		return errBroken
	}
	e.state.ops <- Operation{Kind: OpStopInstances, Env: e.state.name}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	for _, i := range is {
		delete(e.state.insts, i.(*instance).id)
	}
	return nil
}

func (e *environ) Instances(ids []string) (insts []environs.Instance, err error) {
	if e.isBroken() {
		return nil, errBroken
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

func (e *environ) isBroken() bool {
	e.configMutex.Lock()
	defer e.configMutex.Unlock()
	return e.config.broken
}

type instance struct {
	id string
}

func (m *instance) Id() string {
	return m.id
}

func (m *instance) DNSName() (string, error) {
	return m.id + ".dns", nil
}

func (m *instance) WaitDNSName() (string, error) {
	return m.DNSName()
}
