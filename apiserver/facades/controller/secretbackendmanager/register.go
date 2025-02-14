// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendmanager

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
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
	domainServices := ctx.DomainServices()
	backendService := domainServices.SecretBackend()
	return &SecretBackendsManagerAPI{
		watcherRegistry: ctx.WatcherRegistry(),
		backendService:  backendService,
		clock:           ctx.Clock(),
	}, nil
}
