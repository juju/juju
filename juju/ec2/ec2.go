package ec2

import (
	"fmt"
	"launchpad.net/goamz/ec2"
	"launchpad.net/juju/go/juju"
	"sync"
)

func init() {
	juju.RegisterProvider("ec2", environProvider{})
}

type environProvider struct{}

var _ juju.EnvironProvider = environProvider{}

type environ struct {
	mu       sync.Mutex
	baseName string
	n        int // instance count

	instances map[string]*instance
	config    *providerConfig
	ec2       *ec2.EC2
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
	cfg := config.(*providerConfig)
	return &environ{
		baseName:  name,
		instances: make(map[string]*instance),
		config:    cfg,
		ec2:       ec2.New(cfg.auth, cfg.region),
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
