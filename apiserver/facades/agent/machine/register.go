// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Machiner", 5, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newMachinerAPI(stdCtx, ctx) // Adds RecordAgentHostAndStartTime.
	}, reflect.TypeOf((*MachinerAPI)(nil)))
}

// newMachinerAPI creates a new instance of the Machiner API.
func newMachinerAPI(stdCtx context.Context, ctx facade.ModelContext) (*MachinerAPI, error) {
	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	serviceFactory := ctx.ServiceFactory()

	m, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelLogger, err := ctx.ModelLogger(m.UUID(), m.Name(), m.Owner().Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewMachinerAPIForState(
		stdCtx,
		systemState,
		ctx.State(),
		serviceFactory.ControllerConfig(),
		serviceFactory.Cloud(),
		ctx.Resources(),
		ctx.Auth(),
		common.NewStatusHistoryRecorder(ctx.MachineTag().String(), modelLogger),
	)
}
