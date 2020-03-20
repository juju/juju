// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"

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
	addresses, err := m.Machine.AllAddresses()
	if err != nil {
		return nil, err
	}
	shimAddr := make([]Address, len(addresses))
	for i, address := range addresses {
		shimAddr[i] = address
	}
	return shimAddr, nil
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
func NewStateShim(st *state.State) (*stateShim, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &stateShim{
		EnvironConfigGetter: stateenvirons.EnvironConfigGetter{Model: m},
		State:               st,
		model:               m,
	}, nil
}

func (s *stateShim) AddSpace(name string, providerId network.Id, subnetIds []string, public bool) error {
	_, err := s.State.AddSpace(name, providerId, subnetIds, public)
	return err
}

func (s *stateShim) SpaceByName(name string) (networkingcommon.BackingSpace, error) {
	result, err := s.State.SpaceByName(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	space := networkingcommon.NewSpaceShim(result)
	return space, nil
}

// AllEndpointBindings returns all endpoint bindings and maps it to a corresponding common type
func (s *stateShim) AllEndpointBindings() ([]ApplicationEndpointBindingsShim, error) {
	endpointBindings, err := s.model.AllEndpointBindings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	all := make([]ApplicationEndpointBindingsShim, len(endpointBindings))
	for i, value := range endpointBindings {
		all[i].AppName = value.AppName
		all[i].Bindings = value.Bindings.Map()
	}
	return all, nil
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
		spaces[i] = networkingcommon.NewSpaceShim(result)
	}
	return spaces, nil
}

func (s *stateShim) SubnetByCIDR(cidr string) (networkingcommon.BackingSubnet, error) {
	result, err := s.State.SubnetByCIDR(cidr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return networkingcommon.NewSubnetShim(result), nil
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
