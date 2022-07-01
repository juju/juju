// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/v2/apiserver/common"
	"github.com/juju/juju/v2/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UpgradeSeries", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newAPIv1(ctx)
	}, reflect.TypeOf((*APIv1)(nil)))
	registry.MustRegister("UpgradeSeries", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newAPIv2(ctx) // Adds CurrentSeries.
	}, reflect.TypeOf((*APIv2)(nil)))
	registry.MustRegister("UpgradeSeries", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx) // Adds SetStatus.
	}, reflect.TypeOf((*API)(nil)))
}

// newAPIv1 is a wrapper that creates a V1 upgrade-series API.
func newAPIv1(ctx facade.Context) (*APIv1, error) {
	api, err := newAPIv2(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv1{api}, nil
}

// newAPIv2 is a wrapper that creates a V2 upgrade-series API.
func newAPIv2(ctx facade.Context) (*APIv2, error) {
	api, err := newAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}

// newAPI creates a new instance of the API with the given context
func newAPI(ctx facade.Context) (*API, error) {
	leadership, err := common.NewLeadershipPinningFromContext(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewUpgradeSeriesAPI(common.UpgradeSeriesState{St: ctx.State()}, ctx.Resources(), ctx.Auth(), leadership)
}
