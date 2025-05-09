// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/uuid"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterForMultiModel("ModelManager", 10, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return newFacadeV10(stdCtx, ctx)
	}, reflect.TypeOf((*ModelManagerAPIV10)(nil)))
	registry.MustRegisterForMultiModel("ModelManager", 11, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return newFacadeV11(stdCtx, ctx)
	}, reflect.TypeOf((*ModelManagerAPI)(nil)))
}

// newFacadeV10 is used for API registration.
func newFacadeV10(stdCtx context.Context, ctx facade.MultiModelContext) (*ModelManagerAPIV10, error) {
	api, err := newFacadeV11(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ModelManagerAPIV10{ModelManagerAPI: api}, nil
}

// newFacadeV11 is used for API registration.
func newFacadeV11(stdCtx context.Context, ctx facade.MultiModelContext) (*ModelManagerAPI, error) {
	auth := ctx.Auth()
	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	// Pretty much all of the user manager methods have special casing for admin
	// users, so look once when we start and remember if the user is an admin.
	err := auth.HasPermission(stdCtx, permission.SuperuserAccess, names.NewControllerTag(ctx.ControllerUUID()))
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return nil, errors.Trace(err)
	}
	isAdmin := err == nil
	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := auth.GetAuthTag().(names.UserTag)

	st := ctx.State()
	pool := ctx.StatePool()

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	backend := commonmodel.NewUserAwareModelManagerBackend(model, pool, apiUser)

	controllerUUID, err := uuid.UUIDFromString(ctx.ControllerUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	domainServicesGetter := domainServicesGetter{ctx: ctx}

	machineServiceGetter := func(ctx context.Context, modelUUID coremodel.UUID) (commonmodel.MachineService, error) {
		svc, err := domainServicesGetter.DomainServicesForModel(ctx, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Machine(), nil
	}
	statusServiceGetter := func(ctx context.Context, modelUUID coremodel.UUID) (commonmodel.StatusService, error) {
		svc, err := domainServicesGetter.DomainServicesForModel(ctx, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Status(), nil
	}

	domainServices := ctx.DomainServices()
	modelStatusAPI := commonmodel.NewModelStatusAPI(
		backend,
		ctx.ControllerUUID(),
		machineServiceGetter,
		statusServiceGetter,
		auth,
		apiUser,
	)

	return NewModelManagerAPI(
		stdCtx,
		backend,
		isAdmin,
		apiUser,
		modelStatusAPI,
		func(c context.Context, modelUUID coremodel.UUID, legacyState facade.LegacyStateExporter) (ModelExporter, error) {
			return ctx.ModelExporter(c, modelUUID, legacyState)
		},
		controllerUUID,
		Services{
			DomainServicesGetter: domainServicesGetter,
			CredentialService:    domainServices.Credential(),
			ModelService:         domainServices.Model(),
			ModelDefaultsService: domainServices.ModelDefaults(),
			AccessService:        domainServices.Access(),
			ObjectStore:          ctx.ObjectStore(),
			SecretBackendService: domainServices.SecretBackend(),
			NetworkService:       domainServices.Network(),
			MachineService:       domainServices.Machine(),
			ApplicationService:   domainServices.Application(),
		},
		common.NewBlockChecker(domainServices.BlockCommand()),
		auth,
	), nil
}
