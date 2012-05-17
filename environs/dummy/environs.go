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
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/schema"
	"launchpad.net/juju/go/state"
	"sort"
	"strings"
	"sync"
)

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
	mu    sync.Mutex
	state *environState
	ops   chan<- Operation
}

// environState represents the state of an environment.
// It can be shared between several environ values,
// so that a given environment can be opened several times.
type environState struct {
	mu           sync.Mutex
	maxId        int // maximum instance id allocated so far.
	insts        map[string]*instance
	files        map[string][]byte
	bootstrapped bool
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
	providerInstance.ops = c
	Listen(nil)
}

// Listen cleans the environment state and registers c to receive
// notifications of operations performed on subsequently opened dummy
// environments.  All opened environments after a Listen will share the
// same underlying state (instances, etc).  If c is non-nil, Close must
// be called before calling Listen again; otherwise the environment is
// cleaned without registering a channel.
func Listen(c chan<- Operation) {
	if c == nil {
		c = discardOperations
	}
	e := &providerInstance
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ops != discardOperations {
		panic("Listen called without Close")
	}
	e.ops = c
	e.state = &environState{
		insts: make(map[string]*instance),
		files: make(map[string][]byte),
	}
}

// Close closes the channel currently registered with Listen.
func Close() {
	e := &providerInstance
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ops == discardOperations {
		panic("Close called without Listen")
	}
	close(e.ops)
	e.ops = discardOperations
}

func (e *environProvider) ConfigChecker() schema.Checker {
	return schema.FieldMap(
		schema.Fields{
			"type":      schema.Const("dummy"),
			"zookeeper": schema.Const(false), // TODO
			"broken":    schema.Bool(),
		},
		[]string{
			"broken",
		},
	)
}

func (e *environProvider) Open(name string, attributes interface{}) (environs.Environ, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg := attributes.(schema.MapType)
	env := &environ{
		name:      name,
		zookeeper: cfg["zookeeper"].(bool),
		ops:       e.ops,
		state:     e.state,
	}
	env.broken, _ = cfg["broken"].(bool)
	return env, nil
}

type environ struct {
	ops       chan<- Operation
	name      string
	state     *environState
	broken    bool
	zookeeper bool
}

var errBroken = errors.New("broken environment")

// EnvironName returns the name of the environment,
// which must be opened from a dummy environment.
func EnvironName(e environs.Environ) string {
	return e.(*environ).name
}

func (e *environ) Bootstrap(uploadTools bool) error {
	if e.broken {
		return errBroken
	}
	if uploadTools {
		err := environs.PutTools(e.Storage())
		if err != nil {
			return err
		}
	}
	e.ops <- Operation{Kind: OpBootstrap, Env: e.name}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	if e.state.bootstrapped {
		return fmt.Errorf("environment is already bootstrapped")
	}
	e.state.bootstrapped = true
	return nil
}

func (e *environ) StateInfo() (*state.Info, error) {
	if e.broken {
		return nil, errBroken
	}
	// TODO start a zookeeper server
	return &state.Info{Addrs: []string{"3.2.1.0:0"}}, nil
}

func (e *environ) Destroy([]environs.Instance) error {
	if e.broken {
		return errBroken
	}
	e.ops <- Operation{Kind: OpDestroy, Env: e.name}
	e.state.mu.Lock()
	e.state.bootstrapped = false
	e.state.mu.Unlock()
	return nil
}

func (e *environ) StartInstance(machineId int, _ *state.Info) (environs.Instance, error) {
	if e.broken {
		return nil, errBroken
	}
	e.ops <- Operation{Kind: OpStartInstance, Env: e.name}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	i := &instance{
		id: fmt.Sprintf("%s-%d", e.name, e.state.maxId),
	}
	e.state.insts[i.id] = i
	e.state.maxId++
	return i, nil
}

func (e *environ) StopInstances(is []environs.Instance) error {
	if e.broken {
		return errBroken
	}
	e.ops <- Operation{Kind: OpStopInstances, Env: e.name}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	for _, i := range is {
		delete(e.state.insts, i.(*instance).id)
	}
	return nil
}

func (e *environ) Instances(ids []string) (insts []environs.Instance, err error) {
	if e.broken {
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

// storage uses the same object as environ,
// but we use a new type to keep the name spaces
// separate.
type storage environ

func (e *environ) Storage() environs.Storage {
	return (*storage)(e)
}

func (e *environ) PublicStorage() environs.StorageReader {
	return environs.EmptyStorage
}

func (s *storage) Put(name string, r io.Reader, length int64) error {
	if s.broken {
		return errBroken
	}
	s.ops <- Operation{Kind: OpPutFile, Env: s.name}
	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	if err != nil {
		return err
	}
	s.state.mu.Lock()
	s.state.files[name] = buf.Bytes()
	s.state.mu.Unlock()
	return nil
}

func (s *storage) Get(name string) (io.ReadCloser, error) {
	if s.broken {
		return nil, errBroken
	}
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	data, ok := s.state.files[name]
	if !ok {
		return nil, &environs.NotFoundError{fmt.Errorf("file %q not found", name)}
	}
	return ioutil.NopCloser(bytes.NewBuffer(data)), nil
}

func (s *storage) Remove(name string) error {
	if s.broken {
		return errBroken
	}
	s.state.mu.Lock()
	delete(s.state.files, name)
	s.state.mu.Unlock()
	return nil
}

func (s *storage) List(prefix string) ([]string, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	var names []string
	for name := range s.state.files {
		if strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
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

// notifyCloser sends on the notify channel when
// it's closed.
type notifyCloser struct {
	io.Reader
	notify chan bool
}

func (r *notifyCloser) Close() error {
	r.notify <- true
	return nil
}
