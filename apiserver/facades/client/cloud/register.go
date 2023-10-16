// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	stdcontext "context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/cloud"
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
	credentialCallContextGetter := func(modelUUID string) (*cloud.Cloud, credentialcommon.Model, credentialcommon.MachineService, envcontext.ProviderCallContext, error) {
		modelState, err := context.StatePool().Get(modelUUID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		defer modelState.Release()

		model, err := modelState.Model()
		if err != nil {
			return nil, nil, nil, nil, err
		}
		cld, err := context.ServiceFactory().Cloud().Get(stdcontext.Background(), model.CloudName())
		if err != nil {
			return nil, nil, nil, nil, err
		}
		return cld, model, credentialcommon.NewMachineService(modelState.State), envcontext.CallContext(modelState.State), err
	}
	systemState, err := context.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudPermissionService := systemState

	userService := stateShim{context.State()}
	modelCredentialService := context.State()

	controllerInfo, err := systemState.ControllerInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}

	serviceFactory := context.ServiceFactory()

	return NewCloudAPI(
		systemState.ControllerTag(),
		controllerInfo.CloudName,
		credentialCallContextGetter,
		credentialcommon.ValidateNewModelCredential,
		userService,
		modelCredentialService,
		serviceFactory.Cloud(),
		cloudPermissionService,
		serviceFactory.Credential(),
		context.Auth(), context.Logger().Child("cloud"),
	)
}
