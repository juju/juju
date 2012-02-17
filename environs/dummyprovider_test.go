// Dummy is a bare minimum provider that doesn't actually do anything.
// The configuration requires a single value, "basename", which
// is used as the base name of any machines that are "created".
// It has no persistent state.
//
// Note that this file contains no tests as such - it is
// just used by the testing code.
package environs_test

import (
	"fmt"
	"io"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/schema"
	"launchpad.net/juju/go/state"
	"sync"
)

func init() {
	environs.RegisterProvider("dummy", dummyProvider{})
}

type dummyInstance struct {
	id string
}

func (m *dummyInstance) Id() string {
	return m.id
}

func (m *dummyInstance) DNSName() string {
	return m.id + ".foo"
}

type dummyProvider struct{}

func (dummyProvider) ConfigChecker() schema.Checker {
	return schema.FieldMap(
		schema.Fields{
			"type":     schema.Const("dummy"),
			"basename": schema.String(),
		},
		nil,
	)
}

type dummyEnviron struct {
	mu       sync.Mutex
	baseName string
	n        int // instance count

	instances map[string]*dummyInstance
}

func (dummyProvider) Open(name string, attributes interface{}) (e environs.Environ, err error) {
	cfg := attributes.(schema.MapType)
	return &dummyEnviron{
		baseName:  cfg["basename"].(string),
		instances: make(map[string]*dummyInstance),
	}, nil
}


func (*dummyEnviron) Bootstrap() (error) {
	return fmt.Errorf("not implemented")
}

func (*dummyEnviron) StateInfo() (*state.Info, error) {
	return nil, fmt.Errorf("I'm a dummy, dummy!")
}

func (*dummyEnviron) Destroy([]environs.Instance) error {
	return nil
}

func (e *dummyEnviron) StartInstance(machineId int, _ *state.Info) (environs.Instance, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	i := &dummyInstance{
		id: fmt.Sprintf("%s-%d", e.baseName, e.n),
	}
	e.instances[i.id] = i
	e.n++
	return i, nil
}

func (e *dummyEnviron) StopInstances(is []environs.Instance) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, i := range is {
		delete(e.instances, i.(*dummyInstance).id)
	}
	return nil
}

func (e *dummyEnviron) Instances(ids []string) (insts []environs.Instance, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, id := range ids {
		inst := e.instances[id]
		if inst == nil {
			err = environs.ErrMissingInstance
		}
		insts = append(insts, inst)
	}
	return
}

func (e *dummyEnviron) PutFile(file string, r io.Reader, length int64) error {
	return fmt.Errorf("dummyEnviron doesn't implement files")
}

func (e *dummyEnviron) GetFile(file string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("dummyEnviron doesn't implement files")
}

func (e *dummyEnviron) RemoveFile(file string) error {
	return fmt.Errorf("dummyEnviron doesn't implement files")
}
