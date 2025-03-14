// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("RemoteRelations", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		api, err := makeAPI(stdCtx, ctx) // Adds UpdateControllersForModels and WatchLocalRelationChanges.
		if err != nil {
			return nil, fmt.Errorf("creating RemoteRelations facade: %w", err)
		}
		return api, nil
	}, reflect.TypeOf((*API)(nil)))
}

// makeAPI creates a new server-side API facade backed by global state.
func makeAPI(stdCtx context.Context, ctx facade.ModelContext) (*API, error) {
	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	domainServices := ctx.DomainServices()
	controllerConfigService := domainServices.ControllerConfig()
	externalControllerService := domainServices.ExternalController()
	modelInfo, err := domainServices.ModelInfo().GetModelInfo(stdCtx)
	if err != nil {
		return nil, fmt.Errorf("retrieving model info: %w", err)
	}
	return NewRemoteRelationsAPI(
		modelInfo.UUID,
		commoncrossmodel.GetBackend(ctx.State()),
		externalControllerService,
		domainServices.Secret(),
		common.NewControllerConfigAPI(systemState, controllerConfigService, externalControllerService),
		ctx.Resources(), ctx.Auth(),
		ctx.Logger().Child("remoterelations", corelogger.CMR),
	)
}
