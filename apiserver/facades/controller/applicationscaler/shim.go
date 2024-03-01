// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationscaler

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// This file contains untested shims to let us wrap state in a sensible
// interface and avoid writing tests that depend on mongodb. If you were
// to change any part of it so that it were no longer *obviously* and
// *trivially* correct, you would be Doing It Wrong.

// newAPI provides the required signature for facade registration.
func newAPI(ctx facade.ModelContext) (*Facade, error) {
	st := ctx.State()
	serviceFactory := ctx.ServiceFactory()
	prechecker, err := stateenvirons.NewInstancePrechecker(st, serviceFactory.Cloud(), serviceFactory.Credential())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewFacade(backendShim{st: st, prechecker: prechecker}, ctx.Resources(), ctx.Auth())
}

// backendShim wraps a *State to implement Backend without pulling in direct
// mongodb dependencies. It would be awesome if we were to put this in state
// and test it properly there, where we have no choice but to test against
// mongodb anyway, but that's relatively low priority...
//
// ...so long as it stays simple, and the full functionality remains tested
// elsewhere.
type backendShim struct {
	st         *state.State
	prechecker environs.InstancePrechecker
}

// WatchScaledServices is part of the Backend interface.
func (shim backendShim) WatchScaledServices() state.StringsWatcher {
	return shim.st.WatchMinUnits()
}

// RescaleService is part of the Backend interface.
func (shim backendShim) RescaleService(name string) error {
	service, err := shim.st.Application(name)
	if err != nil {
		return errors.Trace(err)
	}
	return service.EnsureMinUnits(shim.prechecker)
}
