// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Annotations", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newAPI returns a new charm annotator API facade.
func newAPI(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &API{
		access:     getState(st, m),
		authorizer: authorizer,
	}, nil
}
