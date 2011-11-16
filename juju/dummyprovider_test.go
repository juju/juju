// Dummy is a bare minimum provider that doesn't actually do anything.
// The configuration requires a single value, "basename", which
// is used as the base name of any machines that are "created".
// It has no persistent state.
//
// Note that this file contains no tests as such - it is
// just used by the testing code.
package juju_test

import (
	"fmt"
	"launchpad.net/juju/go/juju"
	"launchpad.net/juju/go/schema"
	"sync"
)

func init() {
	juju.RegisterProvider("dummy", dummyProvider{})
}

type dummyInstance struct {
	name string
	id   string
}

func (m *dummyInstance) Id() string {
	return fmt.Sprintf("dummy-%d", m.id)
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
	n        int // machine count
	machines map[string]*dummyInstance
}

func (dummyProvider) Open(name string, attributes interface{}) (e juju.Environ, err error) {
	cfg := attributes.(schema.MapType)
	return &dummyEnviron{
		baseName: cfg["basename"].(string),
		machines: make(map[string]*dummyInstance),
	}, nil
}

func (*dummyEnviron) Bootstrap() error {
	return nil
}

func (*dummyEnviron) Destroy() error {
	return nil
}

func (c *dummyEnviron) StartInstance(id string) (juju.Instance, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := &dummyInstance{
		name: fmt.Sprintf("%s-%d", c.baseName, c.n),
		id:   id,
	}
	c.machines[m.id] = m
	c.n++
	return m, nil
}

func (c *dummyEnviron) StopInstances(ms []juju.Instance) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range ms {
		delete(c.machines, m.(*dummyInstance).id)
	}
	return nil
}

func (c *dummyEnviron) Instances() ([]juju.Instance, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var ms []juju.Instance
	for _, m := range c.machines {
		ms = append(ms, m)
	}
	return ms, nil
}
