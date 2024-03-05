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
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretBackendsManager", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewSecretBackendsManagerAPI(ctx)
	}, reflect.TypeOf((*SecretBackendsManagerAPI)(nil)))
}

// NewSecretBackendsManagerAPI creates a SecretBackendsManagerAPI.
func NewSecretBackendsManagerAPI(context facade.ModelContext) (*SecretBackendsManagerAPI, error) {
	if !context.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	model, err := context.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &SecretBackendsManagerAPI{
		watcherRegistry: context.WatcherRegistry(),
		controllerUUID:  model.ControllerUUID(),
		modelUUID:       model.UUID(),
		modelName:       model.Name(),
		backendRotate:   context.State(),
		backendState:    state.NewSecretBackends(context.State()),
		clock:           clock.WallClock,
		logger:          context.Logger().Child("secretbackendmanager"),
	}, nil
}
