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

// This file contains stub implementations of interfaces required
// by NewProvisionerTask.

var (
	_ provisioner.MachineGetter          = (machineGetter)(nil)
	_ provisioner.Watcher                = (*watcher)(nil)
	_ provisioner.Machine                = (*machine)(nil)
	_ environs.InstanceBroker            = (*instanceBroker)(nil)
	_ provisioner.AuthenticationProvider = (*authenticationProvider)(nil)
)

type machineGetter func(tag string) (provisioner.Machine, error)

func (g machineGetter) Machine(tag string) (provisioner.Machine, error) {
	return g(tag)
}

type watcher struct {
	changes chan []string
}

func (w *watcher) Err() error {
	return nil
}

func (w *watcher) Stop() error {
	return nil
}

func (w *watcher) Changes() <-chan []string {
	return w.changes
}

type machine struct {
	id         string
	instanceId instance.Id
	life       params.Life
	status     params.Status
	remove     func(id string) error
}

func (m *machine) Id() string {
	return m.id
}

func (m *machine) InstanceId() (instance.Id, error) {
	return m.instanceId, nil
}

func (m *machine) Constraints() (constraints.Value, error) {
	return constraints.Value{}, nil
}

func (m *machine) Series() (string, error) {
	return "series", nil
}

func (m *machine) String() string {
	return m.id
}

func (m *machine) Remove() error {
	if m.remove == nil {
		return nil
	}
	return m.remove(m.id)
}

func (m *machine) Life() params.Life {
	return m.life
}

func (m *machine) EnsureDead() error {
	m.life = params.Dead
	return nil
}

func (m *machine) Status() (params.Status, string, error) {
	return params.StatusPending, "", nil
}

func (m *machine) SetStatus(status params.Status, info string) error {
	return nil
}

func (m *machine) SetProvisioned(id instance.Id, nonce string, characteristics *instance.HardwareCharacteristics) error {
	return nil
}

func (m *machine) SetPassword(password string) error {
	return nil
}

func (m *machine) Tag() string {
	return "machine-" + m.id
}

type instanceBroker struct {
	startInstance func(
		cons constraints.Value, possibleTools tools.List,
		machineConfig *cloudinit.MachineConfig,
	) (instance.Instance, *instance.HardwareCharacteristics, error)
	stopInstances func([]instance.Instance) error
	allInstances  func() ([]instance.Instance, error)
}

func (b *instanceBroker) StartInstance(
	cons constraints.Value, possibleTools tools.List,
	machineConfig *cloudinit.MachineConfig,
) (instance.Instance, *instance.HardwareCharacteristics, error) {
	if b.startInstance == nil {
		return nil, nil, fmt.Errorf("cannot start instance")
	}
	return b.startInstance(cons, possibleTools, machineConfig)
}

func (b *instanceBroker) StopInstances(insts []instance.Instance) error {
	if b.stopInstances == nil {
		return nil
	}
	return b.stopInstances(insts)
}

func (b *instanceBroker) AllInstances() ([]instance.Instance, error) {
	if b.allInstances == nil {
		return nil, nil
	}
	return b.allInstances()
}

type AuthenticationProvider interface {
	SetupAuthentication(machine provisioner.TaggedPasswordChanger) (*state.Info, *api.Info, error)
}

type authenticationProvider struct {
}

func (p *authenticationProvider) SetupAuthentication(machine provisioner.TaggedPasswordChanger) (*state.Info, *api.Info, error) {
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
