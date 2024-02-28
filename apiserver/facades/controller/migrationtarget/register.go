// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/facades"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(requiredMigrationFacadeVersions facades.FacadeVersions) func(registry facade.FacadeRegistry) {
	return func(registry facade.FacadeRegistry) {
		registry.MustRegister("MigrationTarget", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
			return newFacadeV1(ctx)
		}, reflect.TypeOf((*APIV1)(nil)))
		registry.MustRegister("MigrationTarget", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
			return newFacadeV2(ctx)
		}, reflect.TypeOf((*APIV2)(nil)))
		registry.MustRegister("MigrationTarget", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
			return newFacade(ctx, requiredMigrationFacadeVersions)
		}, reflect.TypeOf((*API)(nil)))
	}
}

// newFacadeV1 is used for APIV1 registration.
func newFacadeV1(ctx facade.ModelContext) (*APIV1, error) {
	api, err := newFacade(ctx, facades.FacadeVersions{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIV1{API: api}, nil
}

// newFacadeV2 is used for APIV2 registration.
func newFacadeV2(ctx facade.ModelContext) (*APIV2, error) {
	api, err := newFacade(ctx, facades.FacadeVersions{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIV2{APIV1: &APIV1{API: api}}, nil
}

// newFacade is used for API registration.
func newFacade(ctx facade.ModelContext, facadeVersions facades.FacadeVersions) (*API, error) {
	auth := ctx.Auth()
	st := ctx.State()
	if err := checkAuth(auth, st); err != nil {
		return nil, errors.Trace(err)
	}

	credentialCallContextGetter := func(stdctx context.Context, modelUUID model.UUID) (service.CredentialValidationContext, error) {
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

		cld, err := ctx.ServiceFactory().Cloud().Get(stdctx, m.CloudName())
		if err != nil {
			return service.CredentialValidationContext{}, err
		}

		return service.CredentialValidationContext{
			ControllerUUID: m.ControllerUUID(),
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
	return NewAPI(
		ctx,
		auth,
		serviceFactory.ControllerConfig(),
		serviceFactory.ExternalController(),
		serviceFactory.Upgrade(),
		serviceFactory.Cloud(),
		credentialService,
		service.NewCredentialValidator(),
		credentialCallContextGetter,
		credentialcommon.CredentialInvalidatorGetter(ctx),
		stateenvirons.GetNewEnvironFunc(environs.New),
		stateenvirons.GetNewCAASBrokerFunc(caas.New),
		facadeVersions,
		ctx.LogDir(),
	)
}
