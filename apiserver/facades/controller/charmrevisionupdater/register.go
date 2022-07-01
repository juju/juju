// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/v2/apiserver/common"
	apiservererrors "github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/apiserver/facade"
	"github.com/juju/juju/v2/charmhub"
	"github.com/juju/juju/v2/charmstore"
	"github.com/juju/juju/v2/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CharmRevisionUpdater", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newCharmRevisionUpdaterAPI(ctx)
	}, reflect.TypeOf((*CharmRevisionUpdaterAPI)(nil)))
}

// newCharmRevisionUpdaterAPI creates a new server-side charmrevisionupdater API end point.
func newCharmRevisionUpdaterAPI(ctx facade.Context) (*CharmRevisionUpdaterAPI, error) {
	if !ctx.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	newCharmstoreClient := func(st State) (charmstore.Client, error) {
		controllerCfg, err := st.ControllerConfig()
		if err != nil {
			return charmstore.Client{}, errors.Trace(err)
		}
		return charmstore.NewCachingClient(state.MacaroonCache{MacaroonCacheState: st}, controllerCfg.CharmStoreURL())
	}
	newCharmhubClient := func(st State) (CharmhubRefreshClient, error) {
		// TODO (stickupkid): Get the http transport from the facade context
		transport := charmhub.DefaultHTTPTransport(logger)
		return common.CharmhubClient(charmhubClientStateShim{state: st}, transport, logger)
	}
	return NewCharmRevisionUpdaterAPIState(
		StateShim{State: ctx.State()},
		clock.WallClock,
		newCharmstoreClient,
		newCharmhubClient,
	)
}
