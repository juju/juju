// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	ecservice "github.com/juju/juju/domain/externalcontroller/service"
	ecstate "github.com/juju/juju/domain/externalcontroller/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("RemoteRelations", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx) // Adds UpdateControllersForModels and WatchLocalRelationChanges.
	}, reflect.TypeOf((*API)(nil)))
}

// newAPI creates a new server-side API facade backed by global state.
func newAPI(ctx facade.Context) (*API, error) {
	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ecService := ecservice.NewService(
		ecstate.NewState(changestream.NewTxnRunnerFactory(ctx.ControllerDB)),
		domain.NewWatcherFactory(
			ctx.ControllerDB,
			ctx.Logger().Child("remoterelations"),
		),
	)
	return NewRemoteRelationsAPI(
		stateShim{st: ctx.State(), Backend: commoncrossmodel.GetBackend(ctx.State())},
		ecService,
		common.NewControllerConfigAPI(systemState, ecService),
		ctx.Resources(), ctx.Auth(),
	)
}
