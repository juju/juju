// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudspec

import (
	"github.com/juju/errors"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// Pool describes an interface for retrieving State instances from a
// collection.
type Pool interface {
	Get(string) (*state.PooledState, error)
}

// MakeCloudSpecGetter returns a function which returns a CloudSpec
// for a given model, using the given Pool.
func MakeCloudSpecGetter(pool Pool) func(names.ModelTag) (environs.CloudSpec, error) {
	return func(tag names.ModelTag) (environs.CloudSpec, error) {
		st, err := pool.Get(tag.Id())
		if err != nil {
			return environs.CloudSpec{}, errors.Trace(err)
		}
		defer st.Release()

		m, err := st.Model()
		if err != nil {
			return environs.CloudSpec{}, errors.Trace(err)
		}
		// TODO - CAAS(externalreality): Once cloud methods are migrated
		// to model EnvironConfigGetter will no longer need to contain
		// both state and model but only model.
		// TODO (manadart 2018-02-15): This potentially frees the state from
		// the pool. Release is called, but the state reference survives.
		return stateenvirons.EnvironConfigGetter{st.State, m}.CloudSpec()
	}
}

// MakeCloudSpecGetterForModel returns a function which returns a
// CloudSpec for a single model. Attempts to request a CloudSpec for
// any other model other than the one associated with the given
// state.State results in an error.
func MakeCloudSpecGetterForModel(st *state.State) func(names.ModelTag) (environs.CloudSpec, error) {
	return func(tag names.ModelTag) (environs.CloudSpec, error) {
		m, err := st.Model()
		if err != nil {
			return environs.CloudSpec{}, errors.Trace(err)
		}
		configGetter := stateenvirons.EnvironConfigGetter{st, m}

		if tag.Id() != st.ModelUUID() {
			return environs.CloudSpec{}, errors.New("cannot get cloud spec for this model")
		}
		return configGetter.CloudSpec()
	}
}
