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
	"launchpad.net/juju/go/version"
	"sync"
	"time"
)

type Operation struct {
	Kind        OperationKind
	Env string

	// Valid for OpUploadTools only. The receiver must close the reader
	// when done.
	Upload io.ReadCloser
	Version version.Version	
}

// Operation represents an action on the dummy provider.
type OperationKind int

const (
	OpNone OperationKind = iota
	OpBootstrap
	OpDestroy
	OpStartInstance
	OpStopInstances
	OpUploadTools
)

var kindNames = []string{
	OpNone:          "OpNone",
	OpBootstrap:     "OpBootstrap",
	OpDestroy:       "OpDestroy",
	OpStartInstance: "OpStartInstance",
	OpStopInstances: "OpStopInstances",
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
	Reset(discardOperations)
}

// Reset closes any previously registered operation channel,
// cleans the environment state, and registers c to receive
// notifications of operations performed on newly opened
// dummy environments. All opened environments after a Reset
// will share the same underlying state (instances, etc).
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
func Reset(c chan<- Operation) {
	providerInstance.reset(c)
}

func (e *environProvider) reset(c chan<- Operation) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if c == nil {
		c = discardOperations
	}
	if ops := e.ops; ops != discardOperations && ops != nil {
		close(ops)
	}
	e.ops = c
	e.state = &environState{
		insts: make(map[string]*instance),
		files: make(map[string][]byte),
	}
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

func (e *environ) Bootstrap() error {
	if e.broken {
		return errBroken
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

func (e *environ) PutFile(name string, r io.Reader, length int64) error {
	if e.broken {
		return errBroken
	}
	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	if err != nil {
		return err
	}
	e.state.mu.Lock()
	e.state.files[name] = buf.Bytes()
	e.state.mu.Unlock()
	return nil
}

func (e *environ) UploadTools(r io.Reader, length int64, version version.Version) error {
	if e.broken {
		return errBroken
	}
	notify := make(chan bool)
	e.ops <- Operation{
		Kind: OpUploadTools,
		Env: e.name,
		Upload: &notifyCloser{r, notify},
		Version: version,
	}
	// Make sure that if we get a test wrong that we don't hang up
	// indefinitely.
	select {
	case <-notify:
	case <-time.After(2 * time.Second):
		panic("dummy environment upload tools reader has taken too long to be closed")
	}
	return nil
}

func (e *environ) GetFile(name string) (io.ReadCloser, error) {
	if e.broken {
		return nil, errBroken
	}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	data, ok := e.state.files[name]
	if !ok {
		return nil, fmt.Errorf("file %q not found", name)
	}
	return ioutil.NopCloser(bytes.NewBuffer(data)), nil
}

func (e *environ) RemoveFile(name string) error {
	if e.broken {
		return errBroken
	}
	e.state.mu.Lock()
	delete(e.state.files, name)
	e.state.mu.Unlock()
	return nil
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
