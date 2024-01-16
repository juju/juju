// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("KeyUpdater", 1, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newKeyUpdaterAPI(ctx.Auth(), ctx.Resources(), ctx.State(), ctx.ServiceFactory().ControllerConfig())
	}, reflect.TypeOf((*KeyUpdaterAPI)(nil)))
}

func newKeyUpdaterAPI(
	authorizer facade.Authorizer,
	resources facade.Resources,
	st *state.State,
	controllerConfigService ControllerConfigService,
) (*KeyUpdaterAPI, error) {
	// Only machine agents have access to the keyupdater service.
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}
	// No-one else except the machine itself can only read a machine's own credentials.
	getCanRead := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &KeyUpdaterAPI{
		controllerConfigService: controllerConfigService,
		state:                   st,
		model:                   m,
		resources:               resources,
		authorizer:              authorizer,
		getCanRead:              getCanRead,
	}, nil
}
