// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/life"
	corenetwork "github.com/juju/juju/core/network"
	providercommon "github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// NOTE: All of the following code is only tested with a feature test.

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

func NewStateShim(st *state.State) (*stateShim, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &stateShim{stateenvirons.EnvironConfigGetter{State: st, Model: m}, st, m}, nil
}

// stateShim forwards and adapts state.State methods to Backing
// method.
// TODO - CAAS(ericclaudejones): This should contain state and stateenvirons alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	stateenvirons.EnvironConfigGetter
	st *state.State
	m  *state.Model
}

func (s *stateShim) AddSpace(name string, providerId corenetwork.Id, subnetIds []string, public bool) error {
	_, err := s.st.AddSpace(name, providerId, subnetIds, public)
	return err
}

func (s *stateShim) AllSpaces() ([]BackingSpace, error) {
	results, err := s.st.AllSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}
	spaces := make([]BackingSpace, len(results))
	for i, result := range results {
		spaces[i] = &spaceShim{Space: result}
	}
	return spaces, nil
}

func (s *stateShim) AddSubnet(info BackingSubnetInfo) (BackingSubnet, error) {
	_, err := s.st.AddSubnet(corenetwork.SubnetInfo{
		CIDR:              info.CIDR,
		VLANTag:           info.VLANTag,
		ProviderId:        info.ProviderId,
		ProviderNetworkId: info.ProviderNetworkId,
		AvailabilityZones: info.AvailabilityZones,
		SpaceName:         info.SpaceName,
	})
	return nil, err // Drop the first result, as it's unused.
}

func (s *stateShim) AllSubnets() ([]BackingSubnet, error) {
	results, err := s.st.AllSubnets()
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnets := make([]BackingSubnet, len(results))
	for i, result := range results {
		subnets[i] = &subnetShim{Subnet: result}
	}
	return subnets, nil
}

func (s *stateShim) AvailabilityZones() ([]providercommon.AvailabilityZone, error) {
	// TODO(dimitern): Fix this to get them from state when available!
	return nil, nil
}

func (s *stateShim) SetAvailabilityZones(zones []providercommon.AvailabilityZone) error {
	return nil
}

func (s *stateShim) ModelTag() names.ModelTag {
	return s.m.ModelTag()
}
