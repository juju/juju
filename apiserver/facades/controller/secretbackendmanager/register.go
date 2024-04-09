// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendmanager

import (
	"context"
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/secrets/provider"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretBackendsManager", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewSecretBackendsManagerAPI(ctx)
	}, reflect.TypeOf((*SecretBackendsManagerAPI)(nil)))
}

// NewSecretBackendsManagerAPI creates a SecretBackendsManagerAPI.
func NewSecretBackendsManagerAPI(ctx facade.ModelContext) (*SecretBackendsManagerAPI, error) {
	if !ctx.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	serviceFactory := ctx.ServiceFactory()
	backendService := serviceFactory.SecretBackend(model.ControllerUUID(), provider.Provider)
	return &SecretBackendsManagerAPI{
		watcherRegistry: ctx.WatcherRegistry(),
		backendService:  &serviceShim{backendService},
		clock:           clock.WallClock,
		logger:          ctx.Logger().Child("secretbackendmanager"),
	}, nil
}
