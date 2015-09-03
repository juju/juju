// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"github.com/juju/errors"
	"github.com/juju/juju/environs/config"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	providercommon "github.com/juju/juju/provider/common"
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
	common.NetworkBacking
	st *state.State
}

func (s *stateShim) EnvironConfig() (*config.Config, error) {
	return s.st.EnvironConfig()
}

func (s *stateShim) AllSpaces() ([]common.BackingSpace, error) {
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

func (s *stateShim) AddSubnet(info common.BackingSubnetInfo) (common.BackingSubnet, error) {
	// TODO(dimitern): Add multiple AZs per subnet in state.
	var firstZone string
	if len(info.AvailabilityZones) > 0 {
		firstZone = info.AvailabilityZones[0]
	}
	_, err := s.st.AddSubnet(state.SubnetInfo{
		CIDR:             info.CIDR,
		VLANTag:          info.VLANTag,
		ProviderId:       info.ProviderId,
		AvailabilityZone: firstZone,
		SpaceName:        info.SpaceName,
	})
	return nil, err // Drop the first result, as it's unused.
}

func (s *stateShim) AllSubnets() ([]common.BackingSubnet, error) {
	results, err := s.st.AllSubnets()
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnets := make([]common.BackingSubnet, len(results))
	for i, result := range results {
		subnets[i] = &subnetShim{subnet: result}
	}
	return subnets, nil
}

type availZoneShim struct{}

func (availZoneShim) Name() string    { return "not-set" }
func (availZoneShim) Available() bool { return true }

func (s *stateShim) AvailabilityZones() ([]providercommon.AvailabilityZone, error) {
	// TODO(dimitern): Fix this to get them from state when available!
	logger.Debugf("not getting availability zones from state yet")
	return nil, nil
}

func (s *stateShim) SetAvailabilityZones(zones []providercommon.AvailabilityZone) error {
	logger.Debugf("not setting availability zones in state yet: %+v", zones)
	return nil
}
