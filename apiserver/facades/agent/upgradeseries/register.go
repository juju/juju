// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UpgradeSeries", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newAPI(ctx) // Adds SetStatus.
	}, reflect.TypeOf((*API)(nil)))
}

// newAPI creates a new instance of the API with the given context
func newAPI(ctx facade.ModelContext) (*API, error) {
	leadership, err := common.NewLeadershipPinningFromContext(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	m, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelLogger, err := ctx.ModelLogger(m.UUID(), m.Name(), m.Owner().Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewUpgradeSeriesAPI(common.UpgradeSeriesState{St: ctx.State()}, ctx.Resources(), ctx.Auth(), leadership, ctx.Logger().Child("upgradeseries"), common.NewStatusHistoryRecorder(ctx.MachineTag().String(), modelLogger))
}
