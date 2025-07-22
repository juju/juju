// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"context"
	"reflect"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/errors"
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
	credentialService := domainServices.Credential()
	modelService := domainServices.Model()

	// Get the controller model UUID
	controllerUUID, err := modelService.GetControllerModelUUID(stdCtx)
	if err != nil {
		return nil, errors.Errorf("failed to get controller model UUID: %v", err)
	}

	// Get the controller cloud name
	controllerCloud, _, err := modelService.DefaultModelCloudInfo(stdCtx)
	if err != nil {
		return nil, errors.Errorf("failed to get controller cloud name: %v", err)
	}

	return NewCloudAPI(
		stdCtx,
		names.NewControllerTag(controllerUUID.String()),
		controllerCloud,
		domainServices.Cloud(),
		domainServices.Access(),
		credentialService,
		context.Auth(), context.Logger().Child("cloud"),
	)
}
