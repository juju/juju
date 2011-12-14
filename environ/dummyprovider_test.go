// Dummy is a bare minimum provider that doesn't actually do anything.
// The configuration requires a single value, "basename", which
// is used as the base name of any machines that are "created".
// It has no persistent state.
//
// Note that this file contains no tests as such - it is
// just used by the testing code.
package environ_test

import (
	"fmt"
	"launchpad.net/juju/go/environ"
	"launchpad.net/juju/go/schema"
	"sync"
)

func init() {
	environ.RegisterProvider("dummy", dummyProvider{})
}

type dummyInstance struct {
	name string
}

func (m *dummyInstance) Id() string {
	return fmt.Sprintf("dummy-%s", m.name)
}

func (m *dummyInstance) DNSName() string {
	return m.name
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

func (dummyProvider) Open(name string, attributes interface{}) (e environ.Environ, err error) {
	cfg := attributes.(schema.MapType)
	return &dummyEnviron{
		baseName:  cfg["basename"].(string),
		instances: make(map[string]*dummyInstance),
	}, nil
}

func (*dummyEnviron) Destroy() error {
	return nil
}

func (e *dummyEnviron) StartInstance(id int) (environ.Instance, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	i := &dummyInstance{
		name: fmt.Sprintf("%s-%d", e.baseName, e.n),
	}
	e.instances[i.name] = i
	e.n++
	return i, nil
}

func (e *dummyEnviron) StopInstances(is []environ.Instance) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, i := range is {
		delete(e.instances, i.(*dummyInstance).name)
	}
	return nil
}

func (e *dummyEnviron) Instances() ([]environ.Instance, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var is []environ.Instance
	for _, i := range e.instances {
		is = append(is, i)
	}
	return is, nil
}
