// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	stdcontext "context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/credential/service"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Cloud", 7, func(stdCtx stdcontext.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV7(ctx) // Do not set error if forcing credential update.
	}, reflect.TypeOf((*CloudAPI)(nil)))
}

// newFacadeV7 is used for API registration.
func newFacadeV7(context facade.ModelContext) (*CloudAPI, error) {
	serviceFactory := context.ServiceFactory()
	systemState, err := context.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	credentialService := serviceFactory.Credential().
		WithLegacyUpdater(systemState.CloudCredentialUpdated).
		WithLegacyRemover(systemState.RemoveModelsCredential)

	credentialCallContextGetter := func(ctx stdcontext.Context, modelUUID coremodel.UUID) (service.CredentialValidationContext, error) {
		modelState, err := context.StatePool().Get(modelUUID.String())
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

		cld, err := context.ServiceFactory().Cloud().Cloud(ctx, m.CloudName())
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

	credentialService = credentialService.WithValidationContextGetter(credentialCallContextGetter)
	modelCredentialService := context.State()

	controllerInfo, err := systemState.ControllerInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewCloudAPI(
		systemState.ControllerTag(),
		controllerInfo.CloudName,
		modelCredentialService,
		serviceFactory.Cloud(),
		serviceFactory.Access(),
		credentialService,
		context.Auth(), context.Logger().Child("cloud"),
	)
}
