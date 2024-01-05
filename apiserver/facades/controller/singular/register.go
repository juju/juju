// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Singular", 2, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newExternalFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newExternalFacade is for API registration.
func newExternalFacade(context facade.Context) (*Facade, error) {
	st := context.State()
	auth := context.Auth()

	m, err := st.Model()
	if err != nil {
		return nil, err
	}

	claimer, err := context.SingularClaimer()
	if err != nil {
		return nil, errors.Trace(err)
	}

	backend := getBackend(st, m.ModelTag())
	return NewFacade(backend, claimer, auth)
}
