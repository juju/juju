// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Cleaner", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newCleanerAPI(ctx)
	}, reflect.TypeOf((*CleanerAPI)(nil)))
}

// newCleanerAPI creates a new instance of the Cleaner API.
func newCleanerAPI(ctx facade.ModelContext) (*CleanerAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	m, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Annotate(err, "getting model")
	}
	modelLogger, err := ctx.ModelLogger(m.UUID(), m.Name(), m.Owner().Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	serviceFactory := ctx.ServiceFactory()
	return &CleanerAPI{
		st:             getState(ctx.State()),
		resources:      ctx.Resources(),
		objectStore:    ctx.ObjectStore(),
		machineRemover: serviceFactory.Machine(),
		// For removing applications, we don't need a storage registry.
		appRemover:      serviceFactory.Application(nil),
		unitRemover:     serviceFactory.Unit(),
		historyRecorder: common.NewStatusHistoryRecorder(ctx.MachineTag().String(), modelLogger),
	}, nil
}
