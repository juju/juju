// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/state"
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
		httpClient := ctx.HTTPClient(facade.CharmhubHTTPClient)
		return common.CharmhubClient(charmhubClientStateShim{state: st}, httpClient, logger)
	}
	return NewCharmRevisionUpdaterAPIState(
		StateShim{State: ctx.State()},
		clock.WallClock,
		newCharmstoreClient,
		newCharmhubClient,
	)
}
