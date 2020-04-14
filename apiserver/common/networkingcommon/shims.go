// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/state"
)

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
