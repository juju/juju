// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
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
func makeAPI(_ context.Context, ctx facade.ModelContext) (*API, error) {
	domainServices := ctx.DomainServices()
	controllerConfigService := domainServices.ControllerConfig()
	controllerNodeService := domainServices.ControllerNode()
	externalControllerService := domainServices.ExternalController()
	modelService := domainServices.Model()
	return NewRemoteRelationsAPI(
		externalControllerService,
		domainServices.Secret(),
		common.NewControllerConfigAPI(
			controllerConfigService,
			controllerNodeService,
			externalControllerService,
			modelService,
		),
		ctx.Auth(),
	)
}
