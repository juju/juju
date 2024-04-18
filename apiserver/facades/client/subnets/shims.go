// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

func NewStateShim(st *state.State, cloudService common.CloudService, credentialService common.CredentialService) (*stateShim, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &stateShim{EnvironConfigGetter: stateenvirons.EnvironConfigGetter{Model: m, CloudService: cloudService, CredentialService: credentialService},
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

func (s *stateShim) AvailabilityZones() (network.AvailabilityZones, error) {
	// TODO (hml) 2019-09-13
	// now available... include.
	// AvailabilityZones() is defined in the common.ZonedEnviron interface
	return nil, nil
}

func (s *stateShim) SetAvailabilityZones(_ network.AvailabilityZones) error {
	return nil
}
