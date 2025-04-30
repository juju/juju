// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Cloud", 7, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV7(stdCtx, ctx) // Do not set error if forcing credential update.
	}, reflect.TypeOf((*CloudAPI)(nil)))
}

// newFacadeV7 is used for API registration.
func newFacadeV7(stdCtx context.Context, context facade.ModelContext) (*CloudAPI, error) {
	domainServices := context.DomainServices()
	systemState, err := context.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	credentialService := domainServices.Credential()
	controllerInfo, err := systemState.ControllerInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewCloudAPI(
		stdCtx,
		systemState.ControllerTag(),
		controllerInfo.CloudName,
		domainServices.Cloud(),
		domainServices.Access(),
		credentialService,
		context.Auth(), context.Logger().Child("cloud"),
	)
}
