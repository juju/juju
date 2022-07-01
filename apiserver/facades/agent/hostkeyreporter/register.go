// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/v3/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("HostKeyReporter", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newFacade wraps New to express the supplied *state.State as a Backend.
func newFacade(ctx facade.Context) (*Facade, error) {
	facade, err := New(ctx.State(), ctx.Resources(), ctx.Auth())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}
