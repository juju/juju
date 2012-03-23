// Dummy is a bare minimum environs that doesn't actually do anything.
// The configuration requires a single value, "basename", which
// is used as the base name of any machines that are "created".
// It has no persistent state.
//
// Note that this file contains no tests as such - it is
// just used by the testing code.
package testing

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

// EnvOp represents an action on the testing provider.
type EnvOp int

const (
	_ EnvOp = iota
	EnvBootstrap
	EnvDestroy
	EnvStartInstance
	EnvStopInstances
)

// testEenvirons represents the testing provider.  There is only ever one
// instance of this type (testingEnvirons)
type testEenvirons struct {
	mu    sync.Mutex
	state *environState
	ops   chan<- EnvOp
}

// environState represents the state of an environment.
// It can be shared between several environ insts,
// so that a given environment can be opened several times.
type environState struct {
	mu    sync.Mutex
	n     int // instance count
	insts map[string]*instance
	files map[string][]byte
}

func newState() *environState {
	return &environState{
		insts: make(map[string]*instance),
		files: make(map[string][]byte),
	}
}

var testingEnvirons testEenvirons

func init() {
	environs.RegisterProvider("testing", &testingEnvirons)

	// Prime the first ops channel, so that naive clients can use
	// the testing environment by simply importing it.
	ops := make(chan EnvOp)
	go func() {
		for _ = range ops {
		}
	}()
	testingEnvirons.ops = ops
	testingEnvirons.state = newState()
}

// ListenEnvirons registers the given channel to receive operations
// executed when the "testing" provider type is subsequently opened.
// The opened environment will be freshly created for the
// next Open.
// 
// The configuration YAML for the testing environment
// must specify a "basename" property. Instance ids will
// start with this value.
// 
// The DNS name of insts is the same as the Id,
// with ".dns" appended.
func ListenEnvirons(c chan<- EnvOp) {
	testingEnvirons.mu.Lock()
	testingEnvirons.ops = c
	testingEnvirons.state = newState()
	testingEnvirons.mu.Unlock()
}

func (e *testEenvirons) ConfigChecker() schema.Checker {
	return schema.FieldMap(
		schema.Fields{
			"type":     schema.Const("testing"),
			"basename": schema.String(),
		},
		nil,
	)
}

func (e *testEenvirons) Open(name string, attributes interface{}) (environs.Environ, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg := attributes.(schema.MapType)
	return &environ{
		baseName: cfg["basename"].(string),
		ops:      e.ops,
		state:    e.state,
	}, nil
}

type environ struct {
	ops      chan<- EnvOp
	baseName string
	state    *environState
}

func (e *environ) Bootstrap() error {
	e.ops <- EnvBootstrap
	return nil
}

func (*environ) StateInfo() (*state.Info, error) {
	// TODO start a zookeeper server
	return nil, fmt.Errorf("not yet implemented")
}

func (e *environ) Destroy([]environs.Instance) error {
	e.ops <- EnvDestroy
	return nil
}

func (e *environ) StartInstance(machineId int, _ *state.Info) (environs.Instance, error) {
	e.ops <- EnvStartInstance
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	i := &instance{
		id: fmt.Sprintf("%s-%d", e.baseName, e.state.n),
	}
	e.state.insts[i.id] = i
	e.state.n++
	return i, nil
}

func (e *environ) StopInstances(is []environs.Instance) error {
	e.ops <- EnvStopInstances
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	for _, i := range is {
		delete(e.state.insts, i.(*instance).id)
	}
	return nil
}

func (e *environ) Instances(ids []string) (insts []environs.Instance, err error) {
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	n := 0
	for _, id := range ids {
		inst := e.state.insts[id]
		if inst == nil {
			err = environs.ErrPartialInstances
			n++
		}
		insts = append(insts, inst)
	}
	if n == len(ids) {
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
