// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/clock"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/charm/services"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CharmDownloader", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*CharmDownloaderAPI)(nil)))
}

// newFacadeV1 provides the signature required for facade V1 registration.
func newFacadeV1(ctx facade.ModelContext) (*CharmDownloaderAPI, error) {
	authorizer := ctx.Auth()
	rawState := ctx.State()
	stateBackend := stateShim{rawState}
	resourcesBackend := resourcesShim{ctx.Resources()}

	charmhubHTTPClient, err := ctx.HTTPClient(facade.CharmhubHTTPClient)
	if err != nil {
		return nil, fmt.Errorf(
			"getting charm hub http client: %w",
			err,
		)
	}

	return newAPI(
		authorizer,
		resourcesBackend,
		stateBackend,
		ctx.ServiceFactory().Config(),
		clock.WallClock,
		charmhubHTTPClient,
		ctx.ObjectStore(),
		func(cfg services.CharmDownloaderConfig) (Downloader, error) {
			return services.NewCharmDownloader(cfg)
		},
		ctx.Logger().Child("charmdownloader"),
	), nil
}
