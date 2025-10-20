// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterForMultiModel("CrossModelSecrets", 1, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return makeStateCrossModelSecretsAPIV1(stdCtx, ctx)
	}, reflect.TypeOf((*CrossModelSecretsAPIV1)(nil)))
	registry.MustRegisterForMultiModel("CrossModelSecrets", 2, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		api, err := makeStateCrossModelSecretsAPI(stdCtx, ctx)
		return api, fmt.Errorf("creating CrossModelSecrets facade: %w", err)
	}, reflect.TypeOf((*CrossModelSecretsAPI)(nil)))
}

// makeStateCrossModelSecretsAPIV1 creates a new server-side CrossModelSecrets V1 API facade.
func makeStateCrossModelSecretsAPIV1(stdCtx context.Context, ctx facade.MultiModelContext) (*CrossModelSecretsAPIV1, error) {
	api, err := makeStateCrossModelSecretsAPI(stdCtx, ctx)
	if err != nil {
		return nil, fmt.Errorf("creating CrossModelSecrets V1 facade: %w", err)
	}
	return &CrossModelSecretsAPIV1{CrossModelSecretsAPI: api}, nil
}

// makeStateCrossModelSecretsAPI creates a new server-side CrossModelSecrets API facade
// backed by global state.
func makeStateCrossModelSecretsAPI(stdCtx context.Context, ctx facade.MultiModelContext) (*CrossModelSecretsAPI, error) {
	secretsServiceGetter := func(c context.Context, modelUUID model.UUID) (SecretService, error) {
		domainServices, err := ctx.DomainServicesForModel(stdCtx, modelUUID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		return domainServices.Secret(), nil
	}
	applicationServiceGetter := func(c context.Context, modelUUID model.UUID) (ApplicationService, error) {
		domainServices, err := ctx.DomainServicesForModel(stdCtx, modelUUID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		return domainServices.Application(), nil
	}
	crossModelRelationServiceGetter := func(c context.Context, modelUUID model.UUID) (CrossModelRelationService, error) {
		domainServices, err := ctx.DomainServicesForModel(stdCtx, modelUUID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		return domainServices.CrossModelRelation(), nil
	}

	return NewCrossModelSecretsAPI(
		ctx.ControllerUUID(),
		ctx.ModelUUID(),
		ctx.CrossModelAuthContext(),
		ctx.DomainServices().SecretBackend(),
		secretsServiceGetter,
		applicationServiceGetter,
		crossModelRelationServiceGetter,
		ctx.Logger().Child("crossmodelsecrets"),
	)
}
