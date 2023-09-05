// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
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

// movingSubnet wraps a state.Subnet
// reference so that it implements MovingSubnet.
type movingSubnet struct {
	*state.Subnet
}

// stateShim forwards and adapts state.State
// methods to Backing methods.
type stateShim struct {
	stateenvirons.EnvironConfigGetter
	*state.State
	model *state.Model
}

// NewStateShim returns a new state shim.
func NewStateShim(st *state.State, credentialService common.CredentialService) (*stateShim, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &stateShim{
		EnvironConfigGetter: stateenvirons.EnvironConfigGetter{Model: m, CredentialService: credentialService},
		State:               st,
		model:               m,
	}, nil
}

func (s *stateShim) AddSpace(
	name string, providerId network.Id, subnetIds []string, public bool,
) (networkingcommon.BackingSpace, error) {
	result, err := s.State.AddSpace(name, providerId, subnetIds, public)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

func (s *stateShim) SpaceByName(name string) (networkingcommon.BackingSpace, error) {
	result, err := s.State.SpaceByName(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

// AllEndpointBindings returns all endpoint bindings,
// with the map values indirected.
func (s *stateShim) AllEndpointBindings() (map[string]Bindings, error) {
	allBindings, err := s.model.AllEndpointBindings()
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

func (s *stateShim) AllSpaces() ([]networkingcommon.BackingSpace, error) {
	results, err := s.State.AllSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}
	spaces := make([]networkingcommon.BackingSpace, len(results))
	for i, result := range results {
		spaces[i] = result
	}
	return spaces, nil
}

func (s *stateShim) SubnetByCIDR(cidr string) (networkingcommon.BackingSubnet, error) {
	result, err := s.State.SubnetByCIDR(cidr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

// MovingSubnet wraps state.Subnet so that it
// returns the MovingSubnet indirection.
func (s *stateShim) MovingSubnet(id string) (MovingSubnet, error) {
	result, err := s.State.Subnet(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &movingSubnet{result}, nil
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
