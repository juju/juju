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
	"os"
	"sync"
)

func init() {
	juju.RegisterProvider("dummy", dummyProvider{})
}

type dummyMachine struct {
	name string
	id   int
}

func (m *dummyMachine) Id() string {
	return fmt.Sprintf("dummy-%d", m.id)
}

func (m *dummyMachine) DNSName() string {
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
	machines map[int]*dummyMachine
}

func (dummyProvider) Open(name string, attributes interface{}) (e juju.Environ, err os.Error) {
	cfg := attributes.(schema.MapType)
	return &dummyEnviron{
		baseName: cfg["basename"].(string),
		machines: make(map[int]*dummyMachine),
	}, nil
}

func (*dummyEnviron) Bootstrap() os.Error {
	return nil
}

func (*dummyEnviron) Destroy() os.Error {
	return nil
}

func (c *dummyEnviron) StartMachine() (juju.Machine, os.Error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := &dummyMachine{
		name: fmt.Sprintf("%s-%d", c.baseName, c.n),
		id:   c.n,
	}
	c.machines[m.id] = m
	c.n++
	return m, nil
}

func (c *dummyEnviron) StopMachines(ms []juju.Machine) os.Error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range ms {
		c.machines[m.(*dummyMachine).id] = nil, false
	}
	return nil
}

func (c *dummyEnviron) Machines() ([]juju.Machine, os.Error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var ms []juju.Machine
	for _, m := range c.machines {
		ms = append(ms, m)
	}
	return ms, nil
}
