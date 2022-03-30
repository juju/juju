// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("RemoteRelations", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newAPIv1(ctx)
	}, reflect.TypeOf((*APIv1)(nil)))
	registry.MustRegister("RemoteRelations", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx) // Adds UpdateControllersForModels and WatchLocalRelationChanges.
	}, reflect.TypeOf((*API)(nil)))
}

// newAPIv1 creates a new server-side API facade backed by global state.
func newAPIv1(ctx facade.Context) (*APIv1, error) {
	api, err := newAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv1{api}, nil
}

// newAPI creates a new server-side API facade backed by global state.
func newAPI(ctx facade.Context) (*API, error) {
	return NewRemoteRelationsAPI(
		stateShim{st: ctx.State(), Backend: commoncrossmodel.GetBackend(ctx.State())},
		common.NewStateControllerConfig(ctx.StatePool().SystemState()),
		ctx.Resources(), ctx.Auth(),
	)
}
