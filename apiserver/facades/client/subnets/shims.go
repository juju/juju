// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/core/network"
	providercommon "github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
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
		subnets[i] = networkingcommon.NewSubnetShim(result)
	}
	return subnets, nil
}

func (s *stateShim) SubnetByCIDR(cidr string) (networkingcommon.BackingSubnet, error) {
	result, err := s.State.SubnetByCIDR(cidr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return networkingcommon.NewSubnetShim(result), nil
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
		result[i] = networkingcommon.NewSubnetShim(subnet)
	}
	return result, nil
}

func (s *stateShim) AvailabilityZones() ([]providercommon.AvailabilityZone, error) {
	// TODO (hml) 2019-09-13
	// now available... include.
	// AvailabilityZones() is defined in the common.ZonedEnviron interface
	return nil, nil
}

func (s *stateShim) SetAvailabilityZones(zones []providercommon.AvailabilityZone) error {
	return nil
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
