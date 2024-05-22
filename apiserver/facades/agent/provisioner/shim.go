// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"

	jujucharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/network/containerizer"
	"github.com/juju/juju/state"
)

// MachineShim implements Machine.
// It is required for use of the containerizer and to mock container
// creation tests here.
type MachineShim struct {
	*state.Machine
}

// NewMachine wraps the given state.machine in a MachineShim.
func NewMachine(m *state.Machine) *MachineShim {
	return &MachineShim{m}
}

// AllLinkLayerDevices implements Machine by wrapping each
// state.LinkLayerDevice reference in returned collection
// with the containerizer LinkLayerDevice implementation.
func (m *MachineShim) AllLinkLayerDevices() ([]containerizer.LinkLayerDevice, error) {
	devs, err := m.Machine.AllLinkLayerDevices()
	if err != nil {
		return nil, errors.Trace(err)
	}

	wrapped := make([]containerizer.LinkLayerDevice, len(devs))
	for i, d := range devs {
		wrapped[i] = containerizer.NewLinkLayerDevice(d)
	}
	return wrapped, nil
}

// AllDeviceAddresses implements Machine by wrapping each
// state.Address reference in returned collection with
// the containerizer Address implementation.
func (m *MachineShim) AllDeviceAddresses() ([]containerizer.Address, error) {
	addrs, err := m.Machine.AllDeviceAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}

	wrapped := make([]containerizer.Address, len(addrs))
	for i, a := range addrs {
		wrapped[i] = containerizer.NewAddress(a)
	}
	return wrapped, nil
}

// Raw returns the inner state.Machine reference.
func (m *MachineShim) Raw() *state.Machine {
	return m.Machine
}

// Units implements Machine by wrapping each
// state.Unit reference in returned collection with
// the Unit implementation.
func (m *MachineShim) Units() ([]Unit, error) {
	units, err := m.Machine.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}
	wrapped := make([]Unit, len(units))
	for i, u := range units {
		wrapped[i] = &unitShim{u}
	}
	return wrapped, nil
}

// unitShim implements Unit.
// It is required to mock the return of Units from MachineShim.
type unitShim struct {
	*state.Unit
}

var _ Unit = (*unitShim)(nil)

// Application implements Unit by wrapping each
// state.Application reference in returned collection with
// the Application implementation.
func (u *unitShim) Application() (Application, error) {
	app, err := u.Unit.Application()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &applicationShim{app}, nil
}

// applicationShim implements Application.
// It is required to mock the return a Charm from applicationShim.
type applicationShim struct {
	*state.Application
}

var _ Application = (*applicationShim)(nil)

// Charm implements Application by wrapping each
// state.Charm reference in returned collection with
// the Charm implementation.
func (a *applicationShim) Charm() (Charm, bool, error) {
	charm, ok, err := a.Application.Charm()
	if err != nil {
		return nil, ok, errors.Trace(err)
	}
	newCharm := &charmShim{
		Charm: charm,
	}
	return newCharm, ok, nil
}

type charmShim struct {
	*state.Charm
}

func (s *charmShim) LXDProfile() *jujucharm.LXDProfile {
	profile := s.Charm.LXDProfile()
	if profile == nil {
		return nil
	}
	return &jujucharm.LXDProfile{
		Description: profile.Description,
		Config:      profile.Config,
		Devices:     profile.Devices,
	}
}
