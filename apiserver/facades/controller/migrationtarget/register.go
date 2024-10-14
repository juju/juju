// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/application/service"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// Register is called to expose a package of facades onto a given registry.
func Register(requiredMigrationFacadeVersions facades.FacadeVersions) func(registry facade.FacadeRegistry) {
	return func(registry facade.FacadeRegistry) {
		registry.MustRegisterForMultiModel("MigrationTarget", 3, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
			api, err := makeFacade(stdCtx, ctx, requiredMigrationFacadeVersions)
			if err != nil {
				return nil, errors.Errorf("making migration target version 3: %w", err)
			}
			return api, nil
		}, reflect.TypeOf((*API)(nil)))
	}
}

// makeFacade is responsible for constructing a new migration target facade and
// its dependencies.
func makeFacade(
	stdCtx context.Context,
	ctx facade.MultiModelContext,
	facadeVersions facades.FacadeVersions,
) (*API, error) {
	auth := ctx.Auth()
	st := ctx.State()
	if err := checkAuth(stdCtx, auth, st); err != nil {
		return nil, err
	}

	domainServices := ctx.DomainServices()

	modelMigrationServiceGetter := func(modelId model.UUID) ModelMigrationService {
		return ctx.DomainServicesForModel(modelId).ModelMigration()
	}

	return NewAPI(
		ctx,
		auth,
		domainServices.ControllerConfig(),
		domainServices.ExternalController(),
		domainServices.Application(service.ApplicationServiceParams{
			StorageRegistry:               storage.NotImplementedProviderRegistry{},
			BackendAdminConfigGetter:      secretservice.NotImplementedBackendConfigGetter,
			SecretBackendReferenceDeleter: service.NotImplementedSecretDeleter{},
		}),
		domainServices.Upgrade(),
		modelMigrationServiceGetter,
		facadeVersions,
		ctx.LogDir(),
	)
}
