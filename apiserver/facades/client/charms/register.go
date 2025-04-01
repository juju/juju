// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	charmscommon "github.com/juju/juju/apiserver/internal/charms"
	corecharm "github.com/juju/juju/core/charm"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/internal/charm/repository"
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
func makeFacadeBase(_ context.Context, ctx facade.ModelContext) (*API, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	modelTag := names.NewModelTag(ctx.ModelUUID().String())

	domainServices := ctx.DomainServices()
	applicationService := domainServices.Application()

	charmInfoAPI, err := charmscommon.NewCharmInfoAPI(modelTag, applicationService, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}

	charmhubHTTPClient, err := ctx.HTTPClient(corehttp.CharmhubPurpose)
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
		modelConfigService: domainServices.Config(),
		applicationService: applicationService,
		charmhubHTTPClient: charmhubHTTPClient,
		newCharmHubRepository: func(cfg repository.CharmHubRepositoryConfig) (corecharm.Repository, error) {
			return repository.NewCharmHubRepository(cfg)
		},
		modelTag:        names.NewModelTag(ctx.ModelUUID().String()),
		controllerTag:   names.NewControllerTag(ctx.ControllerUUID()),
		requestRecorder: ctx.RequestRecorder(),
		logger:          ctx.Logger().Child("charms"),
	}, nil
}
