// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/charms/services"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	ccservice "github.com/juju/juju/domain/controllerconfig/service"
	ccstate "github.com/juju/juju/domain/controllerconfig/state"
	"github.com/juju/juju/state/storage"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CharmDownloader", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*CharmDownloaderAPI)(nil)))
}

// newFacadeV1 provides the signature required for facade V1 registration.
func newFacadeV1(ctx facade.Context) (*CharmDownloaderAPI, error) {
	authorizer := ctx.Auth()
	rawState := ctx.State()
	ctrlConfigService := ccservice.NewService(
		ccstate.NewState(changestream.NewTxnRunnerFactory(ctx.ControllerDB)),
		domain.NewWatcherFactory(
			ctx.ControllerDB,
			ctx.Logger().Child("controllerconfig"),
		),
	)
	stateBackend := stateShim{rawState, ctrlConfigService}
	modelBackend, err := rawState.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	resourcesBackend := resourcesShim{ctx.Resources()}

	return newAPI(
		authorizer,
		resourcesBackend,
		stateBackend,
		modelBackend,
		clock.WallClock,
		ctx.HTTPClient(facade.CharmhubHTTPClient),
		func(modelUUID string) services.Storage {
			return storage.NewStorage(modelUUID, rawState.MongoSession())
		},
		func(cfg services.CharmDownloaderConfig) (Downloader, error) {
			return services.NewCharmDownloader(cfg)
		},
		ctx.Logger().Child("charmdownloader"),
	), nil
}
