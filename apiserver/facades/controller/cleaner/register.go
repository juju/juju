// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	applicationservice "github.com/juju/juju/domain/application/service"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/storage"
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

	domainServices := ctx.DomainServices()
	backendService := domainServices.SecretBackend()
	secretService := domainServices.Secret(
		secretservice.SecretServiceParams{
			BackendAdminConfigGetter:      secretservice.NotImplementedBackendConfigGetter,
			BackendUserSecretConfigGetter: secretservice.NotImplementedBackendUserSecretConfigGetter,
		},
	)
	applicationService := domainServices.Application(applicationservice.ApplicationServiceParams{
		// For removing applications, we don't need a storage registry.
		StorageRegistry: storage.NotImplementedProviderRegistry{},
		BackendAdminConfigGetter: secretbackendservice.AdminBackendConfigGetterFunc(
			backendService, ctx.ModelUUID(),
		),
		SecretBackendReferenceDeleter: secretService,
	})
	return &CleanerAPI{
		st:             getState(ctx.State()),
		resources:      ctx.Resources(),
		objectStore:    ctx.ObjectStore(),
		machineRemover: domainServices.Machine(),
		appService:     applicationService,
	}, nil
}
