// Copyright 2014, 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"github.com/juju/errors"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
)

// environStatePolicy implements state.Policy in
// terms of environs.Environ and related types.
type environStatePolicy struct {
	st                   *state.State
	storageServiceGetter storageServiceGetter
}

type storageServiceGetter func(modelUUID coremodel.UUID) (state.StoragePoolGetter, error)

// GetNewPolicyFunc returns a state.NewPolicyFunc that will return
// a state.Policy implemented in terms of either environs.Environ
// or caas.Broker and related types.
func GetNewPolicyFunc(
	storageServiceGetter storageServiceGetter,
) state.NewPolicyFunc {
	return func(st *state.State) state.Policy {
		return &environStatePolicy{
			st:                   st,
			storageServiceGetter: storageServiceGetter,
		}
	}
}

// StorageServices implements state.Policy.
func (p *environStatePolicy) StorageServices() (state.StoragePoolGetter, error) {
	model, err := p.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return p.storageServiceGetter(coremodel.UUID(model.UUID()))
}
