// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretBackends", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newSecretBackendsAPI(ctx)
	}, reflect.TypeOf((*SecretBackendsAPI)(nil)))
}

// newSecretBackendsAPI creates a SecretBackendsAPI.
func newSecretBackendsAPI(context facade.ModelContext) (*SecretBackendsAPI, error) {
	if !context.Auth().AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	domainServices := context.DomainServices()
	secretBackendService := domainServices.SecretBackend()
	return &SecretBackendsAPI{
		authorizer:     context.Auth(),
		controllerUUID: context.ControllerUUID(),
		backendService: secretBackendService,
	}, nil
}
