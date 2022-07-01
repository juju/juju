// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/v2/apiserver/facade"
	"github.com/juju/juju/v2/apiserver/facades/client/charms/services"
	"github.com/juju/juju/v2/charmhub"
	"github.com/juju/juju/v2/state/storage"
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
	stateBackend := stateShim{rawState}
	modelBackend, err := rawState.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	resourcesBackend := resourcesShim{ctx.Resources()}

	httpTransport := charmhub.RequestHTTPTransport(ctx.RequestRecorder(), charmhub.DefaultRetryPolicy())

	return newAPI(
		authorizer,
		resourcesBackend,
		stateBackend,
		modelBackend,
		clock.WallClock,
		httpTransport(logger),
		func(modelUUID string) services.Storage {
			return storage.NewStorage(modelUUID, rawState.MongoSession())
		},
		func(cfg services.CharmDownloaderConfig) (Downloader, error) {
			return services.NewCharmDownloader(cfg)
		},
	), nil
}
