// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicescaler

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

// This file contains untested shims to let us wrap state in a sensible
// interface and avoid writing tests that depend on mongodb. If you were
// to change any part of it so that it were no longer *obviously* and
// *trivially* correct, you would be Doing It Wrong.

// init really ought to live somewhere else: we should be declaring our API
// in *one* place, not scattered all over the place.
func init() {
	common.RegisterStandardFacade("ServiceScaler", 1, newFacade)
}

// newFacade wraps the supplied *state.State for the use of the Facade.
func newFacade(st *state.State, res *common.Resources, auth common.Authorizer) (*Facade, error) {
	return NewFacade(backend{st}, res, auth)
}

// backend wraps a *State to implement Backend without pulling in dependencies
// on the state package. It would be awesome if we were to put this in state
// and test it properly there, where we have no choice but to test against
// mongodb anyway, but that's relatively low priority...
//
// ...so long as it stays simple.
type backend struct {
	st *state.State
}

// WatchScaledServices is part of the Backend interface.
func (backend backend) WatchScaledServices() state.StringsWatcher {
	return backend.st.WatchMinUnits()
}

// RescaleService is part of the Backend interface.
func (backend backend) RescaleService(name string) error {
	service, err := backend.st.Service(name)
	if err != nil {
		return errors.Trace(err)
	}
	return service.EnsureMinUnits()
}
