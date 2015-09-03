// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// NOTE: All of the following code is only tested with a feature test.

// subnetShim forwards and adapts state.Subnets methods to
// common.BackingSubnet.
type subnetShim struct {
	common.BackingSubnet
	subnet *state.Subnet
}

func (s *subnetShim) CIDR() string {
	return s.subnet.CIDR()
}

func (s *subnetShim) VLANTag() int {
	return s.subnet.VLANTag()
}

func (s *subnetShim) ProviderId() string {
	return s.subnet.ProviderId()
}

func (s *subnetShim) AvailabilityZones() []string {
	// TODO(dimitern): Add multiple zones to state.Subnet.
	return []string{s.subnet.AvailabilityZone()}
}

func (s *subnetShim) Life() params.Life {
	return params.Life(s.subnet.Life().String())
}

func (s *subnetShim) Status() string {
	// TODO(dimitern): This should happen in a cleaner way.
	if s.Life() != params.Alive {
		return "terminating"
	}
	return "in-use"
}

func (s *subnetShim) SpaceName() string {
	return s.subnet.SpaceName()
}

// spaceShim forwards and adapts state.Space methods to BackingSpace.
type spaceShim struct {
	common.BackingSpace
	space *state.Space
}

func (s *spaceShim) Name() string {
	return s.space.Name()
}

func (s *spaceShim) Subnets() ([]common.BackingSubnet, error) {
	results, err := s.space.Subnets()
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnets := make([]common.BackingSubnet, len(results))
	for i, result := range results {
		subnets[i] = &subnetShim{subnet: result}
	}
	return subnets, nil
}

// stateShim forwards and adapts state.State methods to Backing
// method.
type stateShim struct {
	Backing
	st *state.State
}

func (s *stateShim) AddSpace(name string, subnetIds []string, public bool) error {
	_, err := s.st.AddSpace(name, subnetIds, public)
	return err
}

func (s *stateShim) AllSpaces() ([]common.BackingSpace, error) {
	// TODO(dimitern): Make this ListSpaces() instead.
	results, err := s.st.AllSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}
	spaces := make([]common.BackingSpace, len(results))
	for i, result := range results {
		spaces[i] = &spaceShim{space: result}
	}
	return spaces, nil
}
