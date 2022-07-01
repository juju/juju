// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/v2/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CharmHub", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*CharmHubAPI)(nil)))
}

// newFacade creates a new CharmHubAPI facade.
func newFacade(ctx facade.Context) (*CharmHubAPI, error) {
	m, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return newCharmHubAPI(m, ctx.Auth(), charmHubClientFactory{
		requestRecorder: ctx.RequestRecorder(),
	})
}
