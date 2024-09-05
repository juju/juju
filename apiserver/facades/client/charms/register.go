// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	charmscommon "github.com/juju/juju/apiserver/common/charms"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corecharm "github.com/juju/juju/core/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/charm/services"
	"github.com/juju/juju/internal/storage"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Charms", 7, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV7(stdCtx, ctx)
	}, reflect.TypeOf((*APIv7)(nil)))
}

func newFacadeV7(stdCtx context.Context, ctx facade.ModelContext) (*APIv7, error) {
	api, err := makeFacadeBase(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv7{api}, nil
}

// makeFacadeBase provides the signature required for facade registration.
func makeFacadeBase(stdCtx context.Context, ctx facade.ModelContext) (*API, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	modelTag := names.NewModelTag(ctx.ModelUUID().String())

	serviceFactory := ctx.ServiceFactory()
	applicationService := serviceFactory.Application(applicationservice.ApplicationServiceParams{
		StorageRegistry: storage.NotImplementedProviderRegistry{},
		Secrets:         applicationservice.NotImplementedSecretService{},
	})

	charmInfoAPI, err := charmscommon.NewCharmInfoAPI(modelTag, applicationService, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelInfo, err := serviceFactory.ModelInfo().GetModelInfo(stdCtx)
	if err != nil {
		return nil, fmt.Errorf("getting model info: %w", err)
	}
	charmhubHTTPClient, err := ctx.HTTPClient(facade.CharmhubHTTPClient)
	if err != nil {
		return nil, fmt.Errorf(
			"getting charm hub http client: %w",
			err,
		)
	}

	return &API{
		charmInfoAPI:       charmInfoAPI,
		authorizer:         authorizer,
		backendState:       newStateShim(ctx.State()),
		modelConfigService: serviceFactory.Config(),
		applicationService: applicationService,
		charmhubHTTPClient: charmhubHTTPClient,
		newRepoFactory: func(cfg services.CharmRepoFactoryConfig) corecharm.RepositoryFactory {
			return services.NewCharmRepoFactory(cfg)
		},
		tag:             names.NewModelTag(modelInfo.UUID.String()),
		requestRecorder: ctx.RequestRecorder(),
		logger:          ctx.Logger().Child("charms"),
	}, nil
}
