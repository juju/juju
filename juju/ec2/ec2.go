package ec2

import (
	"fmt"
	"launchpad.net/juju/go/juju"
	"launchpad.net/juju/go/schema"
	"sync"
)

func init() {
	juju.RegisterProvider("ec2", environProvider{})
}

type checker struct{}

func (checker) Coerce(v interface{}, path []string) (interface{}, error) {
	return &providerConfig{}, nil
}

type environProvider struct{}

func (environProvider) ConfigChecker() schema.Checker {
	return checker{}
}

var _ juju.EnvironProvider = environProvider{}

// providerConfig is a placeholder for any config information
// that we will have in a configuration file.
type providerConfig struct{}

type environ struct {
	mu       sync.Mutex
	baseName string
	n        int // instance count

	instances map[string]*instance
}

var _ juju.Environ = (*environ)(nil)

type instance struct {
	name string
}

func (m *instance) DNSName() string {
	return m.name
}

func (m *instance) Id() string {
	return fmt.Sprintf("dummy-%s", m.name)
}

func (environProvider) Open(name string, config interface{}) (e juju.Environ, err error) {
	return &environ{
		baseName:  name,
		instances: make(map[string]*instance),
	}, nil
}

func (e *environ) Destroy() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.instances = make(map[string]*instance)
	return nil
}

func (e *environ) StartInstance(machineId int) (juju.Instance, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	i := &instance{
		name: fmt.Sprintf("%s-%d", e.baseName, e.n),
	}
	e.instances[i.name] = i
	e.n++
	return i, nil
}

func (e *environ) StopInstances(is []juju.Instance) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, i := range is {
		delete(e.instances, i.(*instance).name)
	}
	return nil
}

func (e *environ) Instances() ([]juju.Instance, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var is []juju.Instance
	for _, i := range e.instances {
		is = append(is, i)
	}
	return is, nil
}
