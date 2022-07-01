// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/v3/apiserver/common/networkingcommon"
	"github.com/juju/juju/v3/core/network"
	"github.com/juju/juju/v3/state"
	"github.com/juju/juju/v3/state/stateenvirons"
)

func NewStateShim(st *state.State) (*stateShim, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &stateShim{EnvironConfigGetter: stateenvirons.EnvironConfigGetter{Model: m},
		State: st, modelTag: m.ModelTag()}, nil
}

// stateShim forwards and adapts state.State methods to Backing
// method.
type stateShim struct {
	stateenvirons.EnvironConfigGetter
	*state.State
	modelTag names.ModelTag
}

func (s *stateShim) ModelTag() names.ModelTag {
	return s.modelTag
}

func (s *stateShim) AddSubnet(info networkingcommon.BackingSubnetInfo) (networkingcommon.BackingSubnet, error) {
	_, err := s.State.AddSubnet(network.SubnetInfo{
		CIDR:              info.CIDR,
		VLANTag:           info.VLANTag,
		ProviderId:        info.ProviderId,
		ProviderNetworkId: info.ProviderNetworkId,
		AvailabilityZones: info.AvailabilityZones,
		SpaceID:           info.SpaceID,
	})
	return nil, err // Drop the first result, as it's unused.
}

func (s *stateShim) AllSubnets() ([]networkingcommon.BackingSubnet, error) {
	results, err := s.State.AllSubnets()
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnets := make([]networkingcommon.BackingSubnet, len(results))
	for i, result := range results {
		subnets[i] = result
	}
	return subnets, nil
}

func (s *stateShim) SubnetByCIDR(cidr string) (networkingcommon.BackingSubnet, error) {
	result, err := s.State.SubnetByCIDR(cidr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

// SubnetsByCIDR wraps each result of a call to state.SubnetsByCIDR
// in a subnet shim and returns the result.
func (s *stateShim) SubnetsByCIDR(cidr string) ([]networkingcommon.BackingSubnet, error) {
	subnets, err := s.State.SubnetsByCIDR(cidr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(subnets) == 0 {
		return nil, nil
	}

	result := make([]networkingcommon.BackingSubnet, len(subnets))
	for i, subnet := range subnets {
		result[i] = subnet
	}
	return result, nil
}

func (s *stateShim) AvailabilityZones() (network.AvailabilityZones, error) {
	// TODO (hml) 2019-09-13
	// now available... include.
	// AvailabilityZones() is defined in the common.ZonedEnviron interface
	return nil, nil
}

func (s *stateShim) SetAvailabilityZones(_ network.AvailabilityZones) error {
	return nil
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
