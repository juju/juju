// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UpgradeSeries", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newAPIv3(ctx) // Adds SetStatus.
	}, reflect.TypeOf((*APIv3)(nil)))
	registry.MustRegister("UpgradeSeries", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx) // Removes CurrentSeries and TargetSeries
	}, reflect.TypeOf((*API)(nil)))
}

func newAPIv3(ctx facade.Context) (*APIv3, error) {
	api, err := newAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv3{api}, nil
}

// newAPI creates a new instance of the API with the given context
func newAPI(ctx facade.Context) (*API, error) {
	leadership, err := common.NewLeadershipPinningFromContext(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewUpgradeSeriesAPI(common.UpgradeSeriesState{St: ctx.State()}, ctx.Resources(), ctx.Auth(), leadership)
}
