// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
)

// Register is called to expose a package of facades onto a given registry.
func Register(requiredMigrationFacadeVersions facades.FacadeVersions) func(registry facade.FacadeRegistry) {
	return func(registry facade.FacadeRegistry) {
		registry.MustRegisterForMultiModel("MigrationTarget", 3, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
			api, err := makeFacade(stdCtx, ctx, requiredMigrationFacadeVersions)
			if err != nil {
				return nil, errors.Errorf("making migration target version 3: %w", err)
			}
			return api, nil
		}, reflect.TypeOf((*API)(nil)))
	}
}

// makeFacade is responsible for constructing a new migration target facade and
// it's dependencies.
func makeFacade(
	stdCtx context.Context,
	ctx facade.MultiModelContext,
	facadeVersions facades.FacadeVersions,
) (*API, error) {
	auth := ctx.Auth()
	st := ctx.State()
	if err := checkAuth(stdCtx, auth, st); err != nil {
		return nil, err
	}

	serviceFactory := ctx.ServiceFactory()

	modelMigrationServiceGetter := func(modelId model.UUID) ModelMigrationService {
		return ctx.ServiceFactoryForModel(modelId).ModelMigration()
	}

	return NewAPI(
		ctx,
		auth,
		serviceFactory.ControllerConfig(),
		serviceFactory.ExternalController(),
		serviceFactory.Upgrade(),
		modelMigrationServiceGetter,
		facadeVersions,
		ctx.LogDir(),
	)
}
