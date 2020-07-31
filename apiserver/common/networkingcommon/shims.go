// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/life"
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

// AllLinkLayerDevices returns all layer-3 addresses for the machine
// as a slice of the LinkLayerAddress indirection.
func (m *linkLayerMachine) AllAddresses() ([]LinkLayerAddress, error) {
	addrList, err := m.Machine.AllAddresses()
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

// NOTE: All of the following code is only tested with a feature test.

// NewSubnetShim creates new subnet shim to be used by subnets and spaces Facades.
func NewSubnetShim(sub *state.Subnet) *subnetShim {
	return &subnetShim{Subnet: sub}
}

// subnetShim forwards and adapts state.Subnets methods to BackingSubnet.
type subnetShim struct {
	*state.Subnet
}

func (s *subnetShim) Life() life.Value {
	return life.Value(s.Subnet.Life().String())
}

func (s *subnetShim) Status() string {
	if life.IsNotAlive(s.Life()) {
		return "terminating"
	}
	return "in-use"
}

// NewSpaceShim creates new subnet shim to be used by subnets and spaces Facades.
func NewSpaceShim(sp *state.Space) *spaceShim {
	return &spaceShim{Space: sp}
}

// spaceShim forwards and adapts state.Space methods to BackingSpace.
type spaceShim struct {
	*state.Space
}

func (s *spaceShim) Subnets() ([]BackingSubnet, error) {
	results, err := s.Space.Subnets()
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnets := make([]BackingSubnet, len(results))
	for i, result := range results {
		subnets[i] = &subnetShim{Subnet: result}
	}
	return subnets, nil
}
