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
	"sync"
)

func init() {
	environs.RegisterProvider("dummy", dummyProvider{})
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

func (dummyProvider) Open(name string, attributes interface{}) (e environs.Environ, err error) {
	cfg := attributes.(schema.MapType)
	return &dummyEnviron{
		baseName:  cfg["basename"].(string),
		instances: make(map[string]*dummyInstance),
	}, nil
}

func Zookeepers() ([]string, error) {
	return nil, nil
}

func (*dummyEnviron) Bootstrap() error {
	return nil
}

func (*dummyEnviron) Destroy() error {
	return nil
}

func (e *dummyEnviron) StartInstance(id int) (environs.Instance, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	i := &dummyInstance{
		name: fmt.Sprintf("%s-%d", e.baseName, e.n),
	}
	e.instances[i.name] = i
	e.n++
	return i, nil
}

func (e *dummyEnviron) StopInstances(is []environs.Instance) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, i := range is {
		delete(e.instances, i.(*dummyInstance).name)
	}
	return nil
}

func (e *dummyEnviron) Instances() ([]environs.Instance, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var is []environs.Instance
	for _, i := range e.instances {
		is = append(is, i)
	}
	return is, nil
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
