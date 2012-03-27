package dummy

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/schema"
	"launchpad.net/juju/go/state"
	"sync"
)

type Operation struct {
	Kind OperationKind
	Name string
}

// Operation represents an action on the testing provider.
type OperationKind int

const (
	_ OperationKind = iota
	OpBootstrap
	OpDestroy
	OpStartInstance
	OpStopInstances
)

// testEenvirons represents the dummy provider.  There is only ever one
// instance of this type (testingEnvirons)
type testEenvirons struct {
	mu    sync.Mutex
	state *environState
	ops   chan<- Operation
}

// environState represents the state of an environment.
// It can be shared between several environ insts,
// so that a given environment can be opened several times.
type environState struct {
	mu    sync.Mutex
	n     int // instance count
	insts map[string]*instance
	files map[string][]byte
	bootstrapped bool
}

var testingEnvirons testEenvirons

// DiscardOperations can be used to pass to ListenEnvirons.
// It discards all Operations written to it.
var DiscardOperations chan<- Operation

func init() {
	environs.RegisterProvider("dummy", &testingEnvirons)

	// Prime the first ops channel, so that naive clients can use
	// the testing environment by simply importing it.
	c := make(chan Operation)
	go func() {
		for _ = range c {
		}
	}()
	DiscardOperations = c
	ListenEnvirons(DiscardOperations)
}

// ListenEnvirons registers the given channel to receive operations
// executed when the "testing" provider type is subsequently opened.
// The opened environment will be freshly created for the
// next Open.
// The previously registered channel will be closed,
// unless it was nil, and so will panic if any values are sent to it.
// 
// The configuration YAML for the testing environment
// must specify a "name" property. Instance ids will
// start with this value, and the values sent on the channel
// will contain it.
// 
// The DNS name of insts is the same as the Id,
// with ".dns" appended.
func ListenEnvirons(c chan<- Operation) {
	testingEnvirons.mu.Lock()
	defer testingEnvirons.mu.Unlock()
	if testingEnvirons.ops != nil {
		close(testingEnvirons.ops)
	}
	testingEnvirons.ops = c
	testingEnvirons.state = &environState{
		insts: make(map[string]*instance),
		files: make(map[string][]byte),
	}

}

func (e *testEenvirons) ConfigChecker() schema.Checker {
	return schema.FieldMap(
		schema.Fields{
			"type": schema.Const("dummy"),
			"name": schema.String(),
		},
		nil,
	)
}

func (e *testEenvirons) Open(name string, attributes interface{}) (environs.Environ, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg := attributes.(schema.MapType)
	return &environ{
		name:  cfg["name"].(string),
		ops:   e.ops,
		state: e.state,
	}, nil
}

type environ struct {
	ops   chan<- Operation
	name  string
	state *environState
}

func (e *environ) Bootstrap() error {
	e.ops <- Operation{OpBootstrap, e.name}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	if e.state.bootstrapped {
		return fmt.Errorf("environment is already bootstrapped")
	}
	e.state.bootstrapped = true
	return nil
}

func (*environ) StateInfo() (*state.Info, error) {
	// TODO start a zookeeper server
	return &state.Info{Addrs: []string{"3.2.1.0:0"}}, nil
}

func (e *environ) Destroy([]environs.Instance) error {
	e.ops <- Operation{OpDestroy, e.name}
	e.state.mu.Lock()
	e.state.bootstrapped = false
	e.state.mu.Unlock()
	return nil
}

func (e *environ) StartInstance(machineId int, _ *state.Info) (environs.Instance, error) {
	e.ops <- Operation{OpStartInstance, e.name}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	i := &instance{
		id: fmt.Sprintf("%s-%d", e.name, e.state.n),
	}
	e.state.insts[i.id] = i
	e.state.n++
	return i, nil
}

func (e *environ) StopInstances(is []environs.Instance) error {
	e.ops <- Operation{OpStopInstances, e.name}
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	for _, i := range is {
		delete(e.state.insts, i.(*instance).id)
	}
	return nil
}

func (e *environ) Instances(ids []string) (insts []environs.Instance, err error) {
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

func (e *environ) GetFile(name string) (io.ReadCloser, error) {
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	data, ok := e.state.files[name]
	if !ok {
		return nil, fmt.Errorf("file %q not found", name)
	}
	return ioutil.NopCloser(bytes.NewBuffer(data)), nil
}

func (e *environ) RemoveFile(name string) error {
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
