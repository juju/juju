// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/core/model"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/state/stateenvirons"
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
// it's dependencies.
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

	credentialCallContextGetter := func(stdctx context.Context, modelUUID coremodel.UUID) (service.CredentialValidationContext, error) {
		modelState, err := ctx.StatePool().Get(string(modelUUID))
		if err != nil {
			return service.CredentialValidationContext{}, err
		}
		defer modelState.Release()

		m, err := modelState.Model()
		if err != nil {
			return service.CredentialValidationContext{}, err
		}
		cfg, err := m.Config()
		if err != nil {
			return service.CredentialValidationContext{}, err
		}

		cld, err := ctx.ServiceFactory().Cloud().Cloud(stdctx, m.CloudName())
		if err != nil {
			return service.CredentialValidationContext{}, err
		}

		return service.CredentialValidationContext{
			ControllerUUID: ctx.ControllerUUID(),
			Config:         cfg,
			MachineService: credentialcommon.NewMachineService(modelState.State),
			ModelType:      coremodel.ModelType(m.Type()),
			Cloud:          *cld,
			Region:         m.CloudRegion(),
		}, nil
	}

	serviceFactory := ctx.ServiceFactory()
	credentialService := serviceFactory.Credential()
	// TODO(wallyworld) - service factory in tests returns a nil service.
	if credentialService != nil {
		credentialService = credentialService.WithValidationContextGetter(credentialCallContextGetter)
	}

	modelMigrationServiceGetter := func(modelId model.UUID) ModelMigrationService {
		return ctx.ServiceFactoryForModel(modelId).ModelMigration()
	}

	return NewAPI(
		ctx,
		auth,
		serviceFactory.ControllerConfig(),
		serviceFactory.ExternalController(),
		serviceFactory.Upgrade(),
		serviceFactory.Cloud(),
		credentialService,
		credentialcommon.CredentialInvalidatorGetter(ctx),
		modelMigrationServiceGetter,
		stateenvirons.GetNewEnvironFunc(environs.New),
		stateenvirons.GetNewCAASBrokerFunc(caas.New),
		facadeVersions,
		ctx.LogDir(),
	)
}
