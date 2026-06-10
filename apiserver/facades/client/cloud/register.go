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
	}, reflect.TypeFor[*CloudAPI]())
	registry.MustRegister("Cloud", 8, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV8(stdCtx, ctx) // Adds the ModelConfigSchema method.
	}, reflect.TypeFor[*CloudAPIV8]())
}

// newFacadeV8 is used for API registration.
func newFacadeV8(stdCtx context.Context, context facade.ModelContext) (*CloudAPIV8, error) {
	api, err := newFacadeV7(stdCtx, context)
	if err != nil {
		return nil, err
	}
	return &CloudAPIV8{CloudAPI: api}, nil
}

// newFacadeV7 is used for API registration.
func newFacadeV7(stdCtx context.Context, context facade.ModelContext) (*CloudAPI, error) {
	domainServices := context.DomainServices()
	credentialService := domainServices.Credential()
	modelService := domainServices.Model()

	// Get the controller UUID
	controllerUUID := context.ControllerUUID()

	// Get the controller cloud name
	controllerCloud, _, err := modelService.DefaultModelCloudInfo(stdCtx)
	if err != nil {
		return nil, errors.Errorf("failed to get controller cloud name: %v", err)
	}

	return NewCloudAPI(
		stdCtx,
		names.NewControllerTag(controllerUUID),
		controllerCloud,
		domainServices.Cloud(),
		domainServices.Access(),
		credentialService,
		domainServices.Config(),
		context.Auth(), context.Logger().Child("cloud"),
	)
}
