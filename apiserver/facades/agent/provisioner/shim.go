// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"

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
