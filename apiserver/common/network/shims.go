// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

// linkLayerMachine wraps a state.Machine reference in order to
// implement the LinkLayerMachine indirection.
type linkLayerMachine struct {
	*state.Machine
}

// AllLinkLayerDevices returns all layer-2 devices for the machine
// as a slice of the LinkLayerDevice indirection.
func (m *linkLayerMachine) AllLinkLayerDevices() ([]LinkLayerDevice, error) {
	devList, err := m.Machine.AllLinkLayerDevices()
	if err != nil {
		return nil, err
	}

	out := make([]LinkLayerDevice, len(devList))
	for i, dev := range devList {
		out[i] = dev
	}

	return out, nil
}

// AllDeviceAddresses returns all layer-3 addresses for the machine
// as a slice of the LinkLayerAddress indirection.
func (m *linkLayerMachine) AllDeviceAddresses() ([]LinkLayerAddress, error) {
	addrList, err := m.Machine.AllDeviceAddresses()
	if err != nil {
		return nil, err
	}

	out := make([]LinkLayerAddress, len(addrList))
	for i, addr := range addrList {
		out[i] = addr
	}

	return out, nil
}

type linkLayerState struct {
	*state.State
}

func (s *linkLayerState) Machine(id string) (LinkLayerMachine, error) {
	m, err := s.State.Machine(id)
	return &linkLayerMachine{m}, errors.Trace(err)
}
