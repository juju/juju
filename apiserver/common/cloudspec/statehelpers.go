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
	Get(string) (*state.State, state.StatePoolReleaser, error)
}

// MakeCloudSpecGetter returns a function which returns a CloudSpec
// for a given model, using the given Pool.
func MakeCloudSpecGetter(pool Pool) func(names.ModelTag) (environs.CloudSpec, error) {
	return func(tag names.ModelTag) (environs.CloudSpec, error) {
		st, release, err := pool.Get(tag.Id())
		if err != nil {
			return environs.CloudSpec{}, errors.Trace(err)
		}
		defer release()
		return stateenvirons.EnvironConfigGetter{st}.CloudSpec()
	}
}

// MakeCloudSpecGetterForModel returns a function which returns a
// CloudSpec for a single model. Attempts to request a CloudSpec for
// any other model other than the one associated with the given
// state.State results in an error.
func MakeCloudSpecGetterForModel(st *state.State) func(names.ModelTag) (environs.CloudSpec, error) {
	configGetter := stateenvirons.EnvironConfigGetter{st}
	return func(tag names.ModelTag) (environs.CloudSpec, error) {
		if tag.Id() != st.ModelUUID() {
			return environs.CloudSpec{}, errors.New("cannot get cloud spec for this model")
		}
		return configGetter.CloudSpec()
	}
}
