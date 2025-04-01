// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

// machineShim implements Machine.
type machineShim struct {
	*state.Machine
}

// AllAddresses implements Machine by wrapping each state.Address
// reference in the Address indirection.
func (m *machineShim) AllAddresses() ([]Address, error) {
	addresses, err := m.Machine.AllDeviceAddresses()
	if err != nil {
		return nil, err
	}
	shimAddr := make([]Address, len(addresses))
	for i, address := range addresses {
		shimAddr[i] = address
	}
	return shimAddr, nil
}

// Units implements Machine by wrapping each state.Unit
// reference in the Unit indirection.
func (m *machineShim) Units() ([]Unit, error) {
	units, err := m.Machine.Units()
	if err != nil {
		return nil, err
	}
	indirected := make([]Unit, len(units))
	for i, unit := range units {
		indirected[i] = unit
	}
	return indirected, nil
}

// stateShim forwards and adapts state.State
// methods to Backing methods.
type stateShim struct {
	*state.State
}

// NewStateShim returns a new state shim.
func NewStateShim(st *state.State) (*stateShim, error) {
	return &stateShim{
		State: st,
	}, nil
}

// AllEndpointBindings returns all endpoint bindings,
// with the map values indirected.
func (s *stateShim) AllEndpointBindings() (map[string]Bindings, error) {
	allBindings, err := s.AllEndpointBindings()
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string]Bindings, len(allBindings))
	for app, bindings := range allBindings {
		result[app] = bindings
	}
	return result, nil
}

// AllMachines returns all machines and maps it to a corresponding common type.
func (s *stateShim) AllMachines() ([]Machine, error) {
	allStateMachines, err := s.State.AllMachines()
	if err != nil {
		return nil, err
	}
	all := make([]Machine, len(allStateMachines))
	for i, m := range allStateMachines {
		all[i] = &machineShim{m}
	}
	return all, nil
}

// AllConstraints returns all constraints in the model,
// wrapped in the Constraints indirection.
func (s *stateShim) AllConstraints() ([]Constraints, error) {
	found, err := s.State.AllConstraints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cons := make([]Constraints, len(found))
	for i, v := range found {
		cons[i] = v
	}
	return cons, nil
}

func (s *stateShim) ConstraintsBySpaceName(spaceName string) ([]Constraints, error) {
	found, err := s.State.ConstraintsBySpaceName(spaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cons := make([]Constraints, len(found))
	for i, v := range found {
		cons[i] = v
	}
	return cons, nil
}
