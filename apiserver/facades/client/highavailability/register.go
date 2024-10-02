// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/storage"
	oldstate "github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("HighAvailability", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newHighAvailabilityAPIV2(ctx)
	}, reflect.TypeOf((*HighAvailabilityAPIV2)(nil)))
	registry.MustRegister("HighAvailability", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newHighAvailabilityAPI(ctx)
	}, reflect.TypeOf((*HighAvailabilityAPI)(nil)))
}

// newHighAvailabilityAPI creates a new server-side highavailability API end point.
func newHighAvailabilityAPIV2(ctx facade.ModelContext) (*HighAvailabilityAPIV2, error) {
	v3, err := newHighAvailabilityAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &HighAvailabilityAPIV2{*v3}, nil
}

// newHighAvailabilityAPI creates a new server-side highavailability API end point.
func newHighAvailabilityAPI(ctx facade.ModelContext) (*HighAvailabilityAPI, error) {
	// Only clients can access the high availability facade.
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if model.Type() == oldstate.ModelTypeCAAS {
		return nil, errors.NotSupportedf("high availability on kubernetes controllers")
	}

	domainServices := ctx.DomainServices()

	// For adding additional controller units, we don't need a storage registry.
	applicationService := domainServices.Application(applicationservice.ApplicationServiceParams{
		StorageRegistry: storage.NotImplementedProviderRegistry{},
		Secrets:         applicationservice.NotImplementedSecretService{},
	})

	return &HighAvailabilityAPI{
		st:                      st,
		nodeService:             domainServices.ControllerNode(),
		machineService:          domainServices.Machine(),
		applicationService:      applicationService,
		controllerConfigService: domainServices.ControllerConfig(),
		networkService:          domainServices.Network(),
		modelConfigService:      domainServices.Config(),
		authorizer:              authorizer,
		logger:                  ctx.Logger().Child("highavailability"),
	}, nil
}
