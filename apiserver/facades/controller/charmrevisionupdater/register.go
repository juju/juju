// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/clock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/charmhub"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CharmRevisionUpdater", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newCharmRevisionUpdaterAPI(ctx)
	}, reflect.TypeOf((*CharmRevisionUpdaterAPI)(nil)))
}

// newCharmRevisionUpdaterAPI creates a new server-side charmrevisionupdater API end point.
func newCharmRevisionUpdaterAPI(ctx facade.ModelContext) (*CharmRevisionUpdaterAPI, error) {
	if !ctx.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	charmhubHTTPClient, err := ctx.HTTPClient(facade.CharmhubHTTPClient)
	if err != nil {
		return nil, fmt.Errorf(
			"getting charm hub http client: %w",
			err,
		)
	}
	modelConfigService := ctx.DomainServices().Config()

	newCharmhubClient := func(stdCtx context.Context) (CharmhubRefreshClient, error) {
		httpClient := charmhubHTTPClient
		config, err := modelConfigService.ModelConfig(stdCtx)
		if err != nil {
			return nil, fmt.Errorf("getting model config %w", err)
		}
		chURL, _ := config.CharmHubURL()
		return charmhub.NewClient(charmhub.Config{
			URL:        chURL,
			HTTPClient: httpClient,
			Logger:     ctx.Logger().Child("charmrevisionupdater"),
		})

	}
	return NewCharmRevisionUpdaterAPIState(
		StateShim{State: ctx.State()},
		ctx.ObjectStore(),
		clock.WallClock,
		modelConfigService,
		newCharmhubClient,
		ctx.Logger().Child("charmrevisionupdater"),
	)
}
