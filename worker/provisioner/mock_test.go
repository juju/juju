// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/worker/provisioner"
)

// This file contains stub implementations of interfaces required by NewProvisionerTask.

var (
	_ provisioner.MachineGetter          = (mockMachineGetter)(nil)
	_ provisioner.Watcher                = (*mockWatcher)(nil)
	_ provisioner.Machine                = (*mockMachine)(nil)
	_ environs.InstanceBroker            = (*mockInstanceBroker)(nil)
	_ provisioner.AuthenticationProvider = (*mockAuthenticationProvider)(nil)
)

type mockMachineGetter func(tag string) (provisioner.Machine, error)

func (g mockMachineGetter) Machine(tag string) (provisioner.Machine, error) {
	return g(tag)
}

type mockWatcher struct {
	changes chan []string
}

func (w *mockWatcher) Err() error {
	return fmt.Errorf("an error")
}

func (w *mockWatcher) Stop() error {
	return fmt.Errorf("an error")
}

func (w *mockWatcher) Changes() <-chan []string {
//	return w.changes
	c := make(chan []string)
	close(c)
	return c
}

type mockMachine struct {
	id         string
	instanceId instance.Id
	life       params.Life
	status     params.Status
	remove     func(id string) error
}

func (m *mockMachine) Id() string {
	return m.id
}

func (m *mockMachine) InstanceId() (instance.Id, error) {
	return m.instanceId, nil
}

func (m *mockMachine) Constraints() (constraints.Value, error) {
	return constraints.Value{}, nil
}

func (m *mockMachine) Series() (string, error) {
	return "series", nil
}

func (m *mockMachine) String() string {
	return m.id
}

func (m *mockMachine) Remove() error {
	if m.remove == nil {
		return nil
	}
	return m.remove(m.id)
}

func (m *mockMachine) Life() params.Life {
	return m.life
}

func (m *mockMachine) EnsureDead() error {
	m.life = params.Dead
	return nil
}

func (m *mockMachine) Status() (params.Status, string, error) {
	return params.StatusPending, "", nil
}

func (m *mockMachine) SetStatus(status params.Status, info string) error {
	return nil
}

func (m *mockMachine) SetProvisioned(id instance.Id, nonce string, characteristics *instance.HardwareCharacteristics) error {
	return nil
}

func (m *mockMachine) SetPassword(password string) error {
	return nil
}

func (m *mockMachine) Tag() string {
	return "machine-" + m.id
}

type mockInstanceBroker struct {
	startInstance func(cons constraints.Value, possibleTools tools.List,
	machineConfig *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error)

	stopInstances func([]instance.Instance) error
	allInstances  func() ([]instance.Instance, error)
}

func (b *mockInstanceBroker) StartInstance(
cons constraints.Value, possibleTools tools.List,
	machineConfig *cloudinit.MachineConfig,
) (instance.Instance, *instance.HardwareCharacteristics, error) {
	if b.startInstance == nil {
		return nil, nil, fmt.Errorf("cannot start instance")
	}
	return b.startInstance(cons, possibleTools, machineConfig)
}

func (b *mockInstanceBroker) StopInstances(insts []instance.Instance) error {
	if b.stopInstances == nil {
		return nil
	}
	return b.stopInstances(insts)
}

func (b *mockInstanceBroker) AllInstances() ([]instance.Instance, error) {
	if b.allInstances == nil {
		return nil, nil
	}
	return b.allInstances()
}

type mockAuthenticationProvider struct {
}

func (p *mockAuthenticationProvider) SetupAuthentication(machine provisioner.TaggedPasswordChanger) (*state.Info, *api.Info, error) {
	password, err := utils.RandomPassword()
	if err != nil {
		panic(fmt.Errorf("random password failed: %v", err))
	}
	return &state.Info{
		Addrs:    []string{"0.1.2.3:123"},
		CACert:   []byte("cert"),
		Tag:      machine.Tag(),
		Password: password,
	}, &api.Info{
		Addrs:    []string{"0.1.2.3:124"},
		CACert:   []byte("cert"),
		Tag:      machine.Tag(),
		Password: password,
	}, nil
}
