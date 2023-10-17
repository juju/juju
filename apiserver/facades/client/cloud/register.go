// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/domain/model"
	envcontext "github.com/juju/juju/environs/context"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Cloud", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV7(ctx) // Do not set error if forcing credential update.
	}, reflect.TypeOf((*CloudAPI)(nil)))
}

// newFacadeV7 is used for API registration.
func newFacadeV7(context facade.Context) (*CloudAPI, error) {
	serviceFactory := context.ServiceFactory()
	systemState, err := context.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	credentialService := serviceFactory.Credential().
		WithLegacyUpdater(systemState.CloudCredentialUpdated).
		WithLegacyRemover(systemState.RemoveModelsCredential)

	credentialCallContextGetter := func(modelUUID model.UUID) (credentialcommon.CredentialValidationContext, error) {
		modelState, err := context.StatePool().Get(string(modelUUID))
		if err != nil {
			return credentialcommon.CredentialValidationContext{}, err
		}
		defer modelState.Release()

		m, err := modelState.Model()
		if err != nil {
			return credentialcommon.CredentialValidationContext{}, err
		}
		cfg, err := m.Config()
		if err != nil {
			return credentialcommon.CredentialValidationContext{}, err
		}

		// TODO(wallyworld) - we don't want to get the tag here but fixing it needs a big refactor in another PR.
		// For now we need to update both dqlite and mongo.
		tag, _ := m.CloudCredentialTag()
		credentialInvalidator := envcontext.NewCredentialInvalidator(
			tag, credentialService.InvalidateCredential, modelState.State.InvalidateModelCredential)

		callCtx := envcontext.CallContext(credentialInvalidator)
		cld, err := context.ServiceFactory().Cloud().Get(callCtx, m.CloudName())
		if err != nil {
			return credentialcommon.CredentialValidationContext{}, err
		}

		ctx := credentialcommon.CredentialValidationContext{
			ControllerUUID: m.ControllerUUID(),
			Context:        callCtx,
			Config:         cfg,
			MachineService: credentialcommon.NewMachineService(modelState.State),
			ModelType:      model.Type(m.Type()),
			Cloud:          *cld,
			Region:         m.CloudRegion(),
		}
		return ctx, err
	}

	credentialService = credentialService.WithValidationContextGetter(credentialCallContextGetter)
	cloudPermissionService := systemState
	userService := stateShim{context.State()}
	modelCredentialService := context.State()

	controllerInfo, err := systemState.ControllerInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewCloudAPI(
		systemState.ControllerTag(),
		controllerInfo.CloudName,
		userService,
		modelCredentialService,
		serviceFactory.Cloud(),
		cloudPermissionService,
		credentialService,
		context.Auth(), context.Logger().Child("cloud"),
	)
}
